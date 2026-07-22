package drift

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func localSnapshot(resources ...inventory.Resource) inventory.Inventory {
	source := inventory.Source{Type: "local", ProjectRoot: "/project"}
	for index := range resources {
		resources[index].Source = source
	}
	return inventory.Inventory{Source: source, Resources: resources}
}

func remoteSnapshot(resources ...inventory.Resource) inventory.Inventory {
	source := inventory.Source{Type: "remote", Environment: "prod", Endpoint: "https://cluster.example/v2"}
	for index := range resources {
		resources[index].Source = source
	}
	return inventory.Inventory{Source: source, Resources: resources}
}

func TestCompareDrift(t *testing.T) {
	local := localSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "order", Digest: "aaa"})
	remote := remoteSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "order", Digest: "bbb", Version: 12})
	r := Compare("prod", local, remote)
	if !HasDrift(r) {
		t.Fatal("expected drift")
	}
	if r.Entries[0].Status != "DRIFT" {
		t.Fatalf("%+v", r.Entries)
	}
	text := FormatText(r)
	if !strings.Contains(text, "DRIFT") {
		t.Fatal(text)
	}
}

func TestInSync(t *testing.T) {
	local := localSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "a", Digest: "x"})
	remote := remoteSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "a", Digest: "x"})
	r := Compare("prod", local, remote)
	if HasDrift(r) {
		t.Fatal(r)
	}
}

func TestCompareRefusesIncompleteAndMalformedInventories(t *testing.T) {
	tests := []struct {
		name   string
		local  inventory.Inventory
		remote inventory.Inventory
	}{
		{name: "partial remote", local: localSnapshot(), remote: func() inventory.Inventory {
			value := remoteSnapshot()
			value.Partial = true
			return value
		}()},
		{name: "unsupported remote", local: localSnapshot(), remote: func() inventory.Inventory {
			value := remoteSnapshot()
			value.Unsupported = []inventory.Unsupported{{Kind: inventory.KindProcess, Required: true, Reason: "unavailable"}}
			return value
		}()},
		{name: "duplicate local", local: localSnapshot(
			inventory.Resource{Kind: inventory.KindProcess, ID: "p", Digest: "a"},
			inventory.Resource{Kind: inventory.KindProcess, ID: "p", Digest: "b"},
		), remote: remoteSnapshot()},
		{name: "wrong source role", local: inventory.Inventory{Source: inventory.Source{
			Type: "remote", Environment: "prod", Endpoint: "https://cluster.example/v2",
		}}, remote: remoteSnapshot()},
		{name: "mismatched environment", local: localSnapshot(), remote: inventory.Inventory{Source: inventory.Source{
			Type: "remote", Environment: "other", Endpoint: "https://cluster.example/v2",
		}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			report := Compare("prod", test.local, test.remote)
			if report.Comparable || report.Status != StatusRefused || len(report.Entries) != 0 || !HasDrift(report) {
				t.Fatalf("unsafe report=%+v", report)
			}
		})
	}
}

func TestCompareMarksOptionalUnsupportedKindUnknown(t *testing.T) {
	local := localSnapshot(inventory.Resource{Kind: inventory.KindDecision, ID: "d", Digest: "same"})
	remote := remoteSnapshot(inventory.Resource{Kind: inventory.KindDecision, ID: "d", Digest: "same"})
	remote.Unsupported = []inventory.Unsupported{{
		Kind: inventory.KindDecision, Reason: "content unavailable",
	}}
	report := Compare("prod", local, remote)
	if report.Status != StatusUnknown || report.Unknown != 1 || len(report.Entries) != 0 || !HasDrift(report) {
		t.Fatalf("unknown state hidden: %+v", report)
	}
}

func TestCompareResolvedRefusesEndpointMismatch(t *testing.T) {
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote",
		Endpoints: map[string]string{"orchestration": "https://expected.example"},
	}}
	report := CompareResolved(resolved, localSnapshot(), remoteSnapshot())
	if report.Comparable || report.Status != StatusRefused || !HasDrift(report) {
		t.Fatalf("endpoint mismatch accepted: %+v", report)
	}
}

const driftBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="%s" name="%s"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`

func TestServiceBuildsPinnedThreeWayGitComparison(t *testing.T) {
	root := initDriftRepo(t)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(strings.ReplaceAll(driftBPMN, "%s", "order"), `name="order"`, `name="baseline"`))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	commit := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))

	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(strings.ReplaceAll(driftBPMN, "%s", "order"), `name="order"`, `name="working"`))
	writeDriftFile(t, filepath.Join(root, "bpmn", "new.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "new"))

	baselineDigest, err := inventory.DigestCanonicalProcess([]byte(strings.ReplaceAll(strings.ReplaceAll(driftBPMN, "%s", "order"), `name="order"`, `name="baseline"`)), "order")
	if err != nil {
		t.Fatal(err)
	}
	remote := remoteSnapshot(
		inventory.Resource{Kind: inventory.KindProcess, ID: "order", Digest: baselineDigest, Version: 1, Key: "1"},
		inventory.Resource{Kind: inventory.KindProcess, ID: "remote", Digest: baselineDigest, Version: 1, Key: "2"},
	)
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote", Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}

	report, err := service.Run(context.Background(), Request{
		ProjectRoot: root, GitRef: "HEAD", Environment: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Baseline.Ref != "HEAD" || report.Baseline.Commit != commit ||
		report.Environment.Profile.Name != resolved.Profile.Name ||
		report.Environment.Profile.Endpoints["orchestration"] != resolved.Profile.Endpoints["orchestration"] {
		t.Fatalf("baseline/environment not pinned: %+v", report)
	}
	if !report.Dirty || !report.Comparable || report.Policy.ExitCode != 1 {
		t.Fatalf("unsafe policy metadata: %+v", report)
	}
	byKey := make(map[string]Entry, len(report.Entries))
	for _, entry := range report.Entries {
		byKey[entry.Key] = entry
	}
	if got := byKey["process/order"]; got.SourceDrift != "DRIFT" || got.DeploymentDrift != "IN_SYNC" || got.PendingDeployment != "DRIFT" {
		t.Fatalf("order classification = %+v", got)
	}
	if got := byKey["process/new"]; got.SourceDrift != "LOCAL_ONLY" || got.PendingDeployment != "LOCAL_ONLY" {
		t.Fatalf("untracked classification = %+v", got)
	}
	if got := byKey["process/remote"]; got.SourceDrift != "IN_SYNC" ||
		got.DeploymentDrift != "CLUSTER_ONLY" || got.PendingDeployment != "CLUSTER_ONLY" {
		t.Fatalf("remote-only classification = %+v", got)
	}
	if len(report.Changes) < 2 {
		t.Fatalf("dirty paths not explicit: %+v", report.Changes)
	}
	if report.Source.Counts.Total != 2 || report.Deployment.Counts.Total != 2 ||
		report.PendingDeployment.Counts.Total != 3 {
		t.Fatalf("pairwise counts include unrelated resources: source=%+v deployment=%+v pending=%+v",
			report.Source.Counts, report.Deployment.Counts, report.PendingDeployment.Counts)
	}
}

func TestServiceReadsBaselineCommitNotStagedIndex(t *testing.T) {
	root := initDriftRepo(t)
	original := strings.ReplaceAll(strings.ReplaceAll(driftBPMN, "%s", "order"), `name="order"`, `name="original"`)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), original)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	changed := strings.ReplaceAll(strings.ReplaceAll(driftBPMN, "%s", "order"), `name="order"`, `name="staged"`)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), changed)
	runGit(t, root, "add", "bpmn/order.bpmn")

	digest, err := inventory.DigestCanonicalProcess([]byte(original), "order")
	if err != nil {
		t.Fatal(err)
	}
	remote := remoteSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "order", Digest: digest, Version: 1, Key: "1"})
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote", Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}
	report, err := service.Run(context.Background(), Request{ProjectRoot: root, GitRef: "HEAD", Environment: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Entries[0].DeploymentDrift != "IN_SYNC" || report.Entries[0].SourceDrift != "DRIFT" {
		t.Fatalf("index content used as baseline: %+v", report.Entries)
	}
}

func TestServiceTreatsSemanticRenameAsSourceInSyncAndDirty(t *testing.T) {
	root := initDriftRepo(t)
	content := strings.ReplaceAll(driftBPMN, "%s", "order")
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), content)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	runGit(t, root, "mv", "bpmn/order.bpmn", "bpmn/renamed.bpmn")
	formatted := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process name="order" id="order"><startEvent id="start"/><endEvent id="end"/>
    <sequenceFlow targetRef="end" sourceRef="start" id="flow"/>
  </process>
</definitions>`
	writeDriftFile(t, filepath.Join(root, "bpmn", "renamed.bpmn"), formatted)
	digest, err := inventory.DigestCanonicalProcess([]byte(content), "order")
	if err != nil {
		t.Fatal(err)
	}
	remote := remoteSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "order", Digest: digest, Version: 1, Key: "1"})
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote", Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}
	report, err := service.Run(context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Dirty || len(report.Entries) != 1 || report.Entries[0].SourceDrift != ComparisonInSync {
		t.Fatalf("semantic rename misclassified: %+v", report)
	}
	if len(report.Changes) != 1 || report.Changes[0].RenamedFrom == "" {
		t.Fatalf("rename metadata missing: %+v", report.Changes)
	}
}

func TestServiceMakesDeletedBaselineResourceExplicit(t *testing.T) {
	root := initDriftRepo(t)
	content := strings.ReplaceAll(driftBPMN, "%s", "order")
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), content)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	if err := os.Remove(filepath.Join(root, "bpmn", "order.bpmn")); err != nil {
		t.Fatal(err)
	}
	digest, err := inventory.DigestCanonicalProcess([]byte(content), "order")
	if err != nil {
		t.Fatal(err)
	}
	remote := remoteSnapshot(inventory.Resource{Kind: inventory.KindProcess, ID: "order", Digest: digest, Version: 1, Key: "1"})
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote", Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}
	report, err := service.Run(context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	entry := report.Entries[0]
	if entry.SourceDrift != ComparisonDrift || !strings.Contains(entry.Detail, "deleted") ||
		len(report.Changes) != 1 || !report.Changes[0].Deleted {
		t.Fatalf("deleted baseline resource hidden: report=%+v", report)
	}
}

func TestServiceFailsClosedOnPartialDeployedInventory(t *testing.T) {
	root := initDriftRepo(t)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	remote := remoteSnapshot()
	remote.Partial = true
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote", Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}
	report, err := service.Run(context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparable || report.Status != StatusUnknown || report.Policy.ExitCode != 2 ||
		len(report.Entries) != 0 || strings.Contains(FormatText(report), "IN_SYNC") {
		t.Fatalf("partial inventory claimed comparison: %+v", report)
	}
	encoded, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"changes":[]`) ||
		!strings.Contains(string(encoded), `"entries":[]`) ||
		!strings.Contains(string(encoded), `"warnings":[`) {
		t.Fatalf("unstable JSON arrays: %s", encoded)
	}
}

func TestServiceRefusesResolvedEnvironmentDifferentFromExplicitRequest(t *testing.T) {
	root := initDriftRepo(t)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	remote := inventory.Inventory{Source: inventory.Source{
		Type: "remote", Environment: "other", Endpoint: "https://cluster.example/v2",
	}}
	resolved := env.Resolved{Profile: env.Profile{
		Name: "other", Kind: "remote", Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}
	report, err := service.Run(context.Background(), Request{
		ProjectRoot: root, GitRef: "HEAD", Environment: "prod",
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.Comparable || report.Status != StatusRefused || report.Policy.ExitCode != 2 ||
		len(report.Entries) != 0 {
		t.Fatalf("explicit environment mismatch accepted: %+v", report)
	}
}

func TestServiceRejectsUnsafeRefBeforeGitInvocation(t *testing.T) {
	runner := &countingGitRunner{}
	service := NewService(nil)
	service.Git = runner
	_, err := service.Run(context.Background(), Request{ProjectRoot: t.TempDir(), GitRef: "--upload-pack=oops"})
	if err == nil || runner.calls != 0 {
		t.Fatalf("unsafe ref reached git: calls=%d err=%v", runner.calls, err)
	}
}

func TestServiceRemoteFailureIsUnknownAndPreservesCause(t *testing.T) {
	root := initDriftRepo(t)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	sentinel := errors.New("cluster unavailable")
	service := NewService(nil)
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return inventory.Inventory{}, env.Resolved{}, sentinel
	}
	report, err := service.Run(context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"})
	if !errors.Is(err, sentinel) || report.Status != StatusUnknown || report.Comparable || report.Policy.ExitCode != 2 || !HasUnknown(report) {
		t.Fatalf("remote failure was not fail-closed: report=%+v err=%v", report, err)
	}
	if strings.Contains(FormatText(report), "IN_SYNC") {
		t.Fatalf("unknown report claimed sync:\n%s", FormatText(report))
	}
}

func TestServiceGitCommandsDisableOptionalLocksAndPreserveIndex(t *testing.T) {
	root := initDriftRepo(t)
	writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "baseline")
	indexPath := filepath.Join(root, ".git", "index")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	runner := &recordingGitRunner{delegate: ExecGitRunner{}}
	service := serviceWithRemote(runner, remoteSnapshot())
	if _, err := service.Run(context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"}); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) || beforeInfo.Size() != afterInfo.Size() ||
		beforeInfo.Mode() != afterInfo.Mode() || !beforeInfo.ModTime().Equal(afterInfo.ModTime()) {
		t.Fatalf("drift mutated Git index: before=%+v after=%+v", beforeInfo, afterInfo)
	}
	if len(runner.commands) == 0 {
		t.Fatal("no Git commands recorded")
	}
	for _, args := range runner.commands {
		if len(args) < 2 || args[0] != "--no-optional-locks" || args[1] != "--literal-pathspecs" {
			t.Fatalf("Git command lacks safe global options: %q", args)
		}
	}
}

func TestServiceRefusesGitlinkIntersectingConfiguredAssets(t *testing.T) {
	root := initDriftRepo(t)
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")
	commit := strings.TrimSpace(runGit(t, root, "rev-parse", "HEAD"))
	runGit(t, root, "update-index", "--add", "--cacheinfo", "160000,"+commit+",bpmn/vendor")
	runGit(t, root, "commit", "-m", "gitlink")
	report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
		context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
	)
	if err == nil || report.Status != StatusRefused || report.Comparable ||
		!warningContains(report, "gitlink") {
		t.Fatalf("configured gitlink was not refused: report=%+v err=%v", report, err)
	}
}

func TestServiceRefusesConfiguredFilterAndLFSPointer(t *testing.T) {
	t.Run("custom filter", func(t *testing.T) {
		root := initDriftRepo(t)
		writeDriftFile(t, filepath.Join(root, ".gitattributes"), "*.bpmn filter=xml-clean\n")
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "filtered")
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err == nil || report.Status != StatusRefused || !warningContains(report, "filter") {
			t.Fatalf("configured filter accepted: report=%+v err=%v", report, err)
		}
	})
	t.Run("LFS pointer baseline", func(t *testing.T) {
		root := initDriftRepo(t)
		pointer := "version https://git-lfs.github.com/spec/v1\noid sha256:" + strings.Repeat("a", 64) + "\nsize 123\n"
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), pointer)
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "pointer")
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err == nil || report.Status != StatusRefused || !warningContains(report, "LFS") {
			t.Fatalf("LFS pointer accepted: report=%+v err=%v", report, err)
		}
	})
	t.Run("unaffected attribute", func(t *testing.T) {
		root := initDriftRepo(t)
		writeDriftFile(t, filepath.Join(root, ".gitattributes"), "*.txt filter=text-clean\n")
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "unaffected")
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err != nil || !report.Comparable {
			t.Fatalf("unaffected attribute refused: report=%+v err=%v", report, err)
		}
	})
	t.Run("working-only filter", func(t *testing.T) {
		root := initDriftRepo(t)
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "baseline")
		writeDriftFile(t, filepath.Join(root, ".gitattributes"), "bpmn/*.bpmn filter=working-clean\n")
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err == nil || report.Status != StatusRefused || !warningContains(report, "filter") {
			t.Fatalf("working filter accepted: report=%+v err=%v", report, err)
		}
	})
	t.Run("repository parent attributes", func(t *testing.T) {
		repository := t.TempDir()
		runGit(t, repository, "init", "-q")
		runGit(t, repository, "config", "user.name", "Test")
		runGit(t, repository, "config", "user.email", "test@example.com")
		projectRoot := filepath.Join(repository, "projects", "orders")
		writeDriftFile(t, filepath.Join(repository, ".gitattributes"), "*.bpmn filter=parent-clean\n")
		writeDriftFile(t, filepath.Join(projectRoot, ".camunda.yaml"), "name: parent\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n")
		writeDriftFile(t, filepath.Join(projectRoot, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, repository, "add", ".")
		runGit(t, repository, "commit", "-m", "parent-filter")
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: projectRoot, GitRef: "HEAD"},
		)
		if err == nil || report.Status != StatusRefused || !warningContains(report, "filter") {
			t.Fatalf("parent filter accepted: report=%+v err=%v", report, err)
		}
	})
}

func TestServiceUsesLiteralScopedProjectPathspec(t *testing.T) {
	repository := t.TempDir()
	runGit(t, repository, "init", "-q")
	runGit(t, repository, "config", "user.name", "Test")
	runGit(t, repository, "config", "user.email", "test@example.com")
	projectRoot := filepath.Join(repository, "project[]*?:")
	writeDriftFile(t, filepath.Join(projectRoot, ".camunda.yaml"), "name: literal\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n")
	writeDriftFile(t, filepath.Join(projectRoot, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
	for index := 0; index < 300; index++ {
		writeDriftFile(t, filepath.Join(repository, "unrelated", fmt.Sprintf("long-unrelated-name-%03d.txt", index)), strings.Repeat("x", 1024))
	}
	runGit(t, repository, "add", ".")
	runGit(t, repository, "commit", "-m", "monorepo")
	report, err := serviceWithRemote(ExecGitRunner{MaxOutputBytes: 2048}, remoteSnapshot()).Run(
		context.Background(), Request{ProjectRoot: projectRoot, GitRef: "HEAD"},
	)
	if err != nil || !report.Comparable || len(report.Entries) != 1 {
		t.Fatalf("literal/scoped project failed: report=%+v err=%v", report, err)
	}
}

func TestServiceUsesEffectiveWorkingFilterAttributes(t *testing.T) {
	t.Run("info attributes", func(t *testing.T) {
		root := initDriftRepo(t)
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "baseline")
		writeDriftFile(t, filepath.Join(root, ".git", "info", "attributes"), "*.bpmn filter=info-clean\n")
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err == nil || report.Status != StatusRefused || !warningContains(report, "filter") {
			t.Fatalf("info attributes filter accepted: report=%+v err=%v", report, err)
		}
	})
	t.Run("external core attributes file", func(t *testing.T) {
		root := initDriftRepo(t)
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "baseline")
		external := filepath.Join(t.TempDir(), "global-attributes")
		writeDriftFile(t, external, "*.bpmn filter=external-clean\n")
		runGit(t, root, "config", "core.attributesFile", external)
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err == nil || report.Status != StatusRefused || !warningContains(report, "filter") {
			t.Fatalf("external attributes filter accepted: report=%+v err=%v", report, err)
		}
	})
	t.Run("unspecified and unset controls", func(t *testing.T) {
		root := initDriftRepo(t)
		writeDriftFile(t, filepath.Join(root, "bpmn", "order.bpmn"), strings.ReplaceAll(driftBPMN, "%s", "order"))
		runGit(t, root, "add", ".")
		runGit(t, root, "commit", "-m", "baseline")
		external := filepath.Join(t.TempDir(), "global-attributes")
		writeDriftFile(t, external, "*.txt filter=text-clean\n*.bpmn -filter\n")
		runGit(t, root, "config", "core.attributesFile", external)
		report, err := serviceWithRemote(nil, remoteSnapshot()).Run(
			context.Background(), Request{ProjectRoot: root, GitRef: "HEAD"},
		)
		if err != nil || !report.Comparable {
			t.Fatalf("unspecified/unset filter refused: report=%+v err=%v", report, err)
		}
	})
}

func TestBaselineAttributeBudgetBoundaries(t *testing.T) {
	t.Run("exact boundary", func(t *testing.T) {
		budget := attributeBudget{maxFiles: 2, maxBytes: 10}
		if err := budget.reserveFiles(2); err != nil {
			t.Fatal(err)
		}
		if err := budget.addBytes(4); err != nil {
			t.Fatal(err)
		}
		if err := budget.addBytes(6); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("over count", func(t *testing.T) {
		budget := attributeBudget{maxFiles: 2, maxBytes: 10}
		if err := budget.reserveFiles(3); err == nil {
			t.Fatal("attribute over-count accepted")
		}
	})
	t.Run("over aggregate bytes", func(t *testing.T) {
		budget := attributeBudget{maxFiles: 2, maxBytes: 10}
		if err := budget.reserveFiles(2); err != nil {
			t.Fatal(err)
		}
		if err := budget.addBytes(6); err != nil {
			t.Fatal(err)
		}
		if err := budget.addBytes(5); err == nil {
			t.Fatal("aggregate attribute bytes accepted")
		}
	})
}

func TestBaselineAttributeFilesEnforceBudgetsBeforeAndDuringReads(t *testing.T) {
	object := strings.Repeat("a", 40)
	t.Run("over count before blob reads", func(t *testing.T) {
		entries := make([]treeEntry, 0, maxBaselineAttributeFiles+1)
		entries = append(entries, treeEntry{Mode: "100644", Type: "blob", Object: object, Path: ".gitattributes"})
		for index := 0; index < maxBaselineAttributeFiles; index++ {
			entries = append(entries, treeEntry{
				Mode: "100644", Type: "blob", Object: object,
				Path: fmt.Sprintf("nested-%04d/.gitattributes", index),
			})
		}
		runner := &staticBlobGitRunner{blob: []byte("*.txt text\n")}
		_, err := baselineAttributeFiles(context.Background(), runner, "/repo", strings.Repeat("b", 40), "", entries)
		if err == nil || runner.blobCalls != 0 {
			t.Fatalf("over-count read blobs: calls=%d err=%v", runner.blobCalls, err)
		}
	})
	t.Run("exact aggregate and one-byte over", func(t *testing.T) {
		filesAtLimit := maxBaselineAttributeBytes / maxAttributeFileBytes
		entries := make([]treeEntry, 0, filesAtLimit+1)
		for index := 0; index < filesAtLimit+1; index++ {
			path := fmt.Sprintf("nested-%04d/.gitattributes", index)
			if index == 0 {
				path = ".gitattributes"
			}
			entries = append(entries, treeEntry{
				Mode: "100644", Type: "blob", Object: object, Path: path,
			})
		}
		runner := &staticBlobGitRunner{blob: bytes.Repeat([]byte{'x'}, maxAttributeFileBytes)}
		files, err := baselineAttributeFiles(
			context.Background(), runner, "/repo", strings.Repeat("b", 40), "",
			entries[:filesAtLimit],
		)
		if err != nil || len(files) != filesAtLimit {
			t.Fatalf("exact aggregate rejected: files=%d err=%v", len(files), err)
		}
		runner.calls, runner.blobCalls = 0, 0
		_, err = baselineAttributeFiles(
			context.Background(), runner, "/repo", strings.Repeat("b", 40), "", entries,
		)
		if err == nil || runner.blobCalls != 0 {
			t.Fatalf("aggregate overage read blobs before sizing completed: calls=%d err=%v", runner.blobCalls, err)
		}
	})
	t.Run("canceled sizing reads no blobs", func(t *testing.T) {
		entries := []treeEntry{
			{Mode: "100644", Type: "blob", Object: object, Path: ".gitattributes"},
			{Mode: "100644", Type: "blob", Object: object, Path: "nested/.gitattributes"},
		}
		ctx, cancel := context.WithCancel(context.Background())
		runner := &cancelSizingGitRunner{blobSize: 10, cancel: cancel}
		_, err := baselineAttributeFiles(ctx, runner, "/repo", strings.Repeat("b", 40), "", entries)
		if !errors.Is(err, context.Canceled) || runner.blobCalls != 0 {
			t.Fatalf("canceled sizing read blobs: calls=%d err=%v", runner.blobCalls, err)
		}
	})
}

type countingGitRunner struct{ calls int }

func (r *countingGitRunner) Run(context.Context, string, ...string) ([]byte, error) {
	r.calls++
	return nil, nil
}

type staticBlobGitRunner struct {
	blob      []byte
	calls     int
	blobCalls int
}

type cancelSizingGitRunner struct {
	blobSize  int
	sizeCalls int
	blobCalls int
	cancel    context.CancelFunc
}

func (r *cancelSizingGitRunner) Run(ctx context.Context, _ string, args ...string) ([]byte, error) {
	if len(args) != 5 || args[2] != "cat-file" {
		return nil, fmt.Errorf("unexpected Git command: %q", args)
	}
	switch args[3] {
	case "-s":
		r.sizeCalls++
		if r.sizeCalls == 2 {
			r.cancel()
			return nil, ctx.Err()
		}
		return []byte(strconv.Itoa(r.blobSize) + "\n"), nil
	case "blob":
		r.blobCalls++
		return bytes.Repeat([]byte{'x'}, r.blobSize), nil
	default:
		return nil, fmt.Errorf("unexpected cat-file mode: %q", args)
	}
}

func (r *staticBlobGitRunner) Run(_ context.Context, _ string, args ...string) ([]byte, error) {
	r.calls++
	if len(args) != 5 || args[0] != "--no-optional-locks" || args[1] != "--literal-pathspecs" ||
		args[2] != "cat-file" {
		return nil, fmt.Errorf("unexpected Git command: %q", args)
	}
	switch args[3] {
	case "-s":
		return []byte(strconv.Itoa(len(r.blob)) + "\n"), nil
	case "blob":
		r.blobCalls++
		return append([]byte(nil), r.blob...), nil
	default:
		return nil, fmt.Errorf("unexpected cat-file mode: %q", args)
	}
}

type recordingGitRunner struct {
	delegate GitRunner
	commands [][]string
}

func (r *recordingGitRunner) Run(ctx context.Context, root string, args ...string) ([]byte, error) {
	r.commands = append(r.commands, append([]string(nil), args...))
	return r.delegate.Run(ctx, root, args...)
}

func (r *recordingGitRunner) RunInput(ctx context.Context, root string, input []byte, args ...string) ([]byte, error) {
	r.commands = append(r.commands, append([]string(nil), args...))
	return r.delegate.(GitInputRunner).RunInput(ctx, root, input, args...)
}

func serviceWithRemote(runner GitRunner, remote inventory.Inventory) *Service {
	resolved := env.Resolved{Profile: env.Profile{
		Name:      remote.Source.Environment,
		Kind:      remote.Source.Type,
		Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}}
	service := NewService(nil)
	if runner != nil {
		service.Git = runner
	}
	service.BuildDeployed = func(context.Context, Request) (inventory.Inventory, env.Resolved, error) {
		return remote, resolved, nil
	}
	return service
}

func warningContains(report Report, text string) bool {
	for _, warning := range report.Warnings {
		if strings.Contains(warning.Message, text) {
			return true
		}
	}
	return false
}

func initDriftRepo(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	runGit(t, root, "config", "user.name", "Test")
	runGit(t, root, "config", "user.email", "test@example.com")
	writeDriftFile(t, filepath.Join(root, ".camunda.yaml"), "name: drift\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n")
	return root
}

func writeDriftFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func runGit(t *testing.T, root string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", root}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
	return string(output)
}
