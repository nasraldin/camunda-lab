package toolkit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
)

func TestLintUsesExplicitInputAndPreservesEveryProcess(t *testing.T) {
	result, err := (Service{}).Lint(context.Background(), LintRequest{
		Inputs: []BPMNInput{{Name: "multi.bpmn", Content: []byte(multiProcessBPMN)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Status != StatusFailed {
		t.Fatalf("status = %q complete=%v", result.Status, result.Complete)
	}
	if len(result.Documents) != 1 || len(result.Documents[0].Processes) != 2 {
		t.Fatalf("documents = %+v", result.Documents)
	}
	if !hasLintElement(result, "secondTask") {
		t.Fatalf("second process was not linted: %+v", result.Findings)
	}
}

func TestLintAttributesProcessFindingsAndRunsDocumentRulesOnce(t *testing.T) {
	source := strings.Replace(multiProcessBPMN, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><message id="m1" name="duplicate"/><message id="m2" name="duplicate"/>`, 1)
	result, err := (Service{}).Lint(context.Background(), LintRequest{
		Inputs: []BPMNInput{{Name: "multi.bpmn", Content: []byte(source)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	global := 0
	for _, finding := range result.Findings {
		if finding.Finding.Rule == "bpmn/duplicate-message-name" {
			global++
			if finding.ProcessID != "" {
				t.Fatalf("document finding attributed to process: %+v", finding)
			}
		}
		if finding.Finding.Element == "secondTask" && finding.ProcessID != "two" {
			t.Fatalf("process finding = %+v", finding)
		}
	}
	if global != 1 {
		t.Fatalf("document findings = %d; all = %+v", global, result.Findings)
	}
}

func TestLintDiscoversConfiguredProjectInputs(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".camunda.yaml"), "name: toolkit\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: models\n  dmn: dmn\n  forms: forms\n  tests: tests\n")
	write(t, filepath.Join(root, "models", "nested", "process.bpmn"), singleProcessBPMN)

	result, err := (Service{}).Lint(context.Background(), LintRequest{ProjectDir: root})
	if err != nil {
		t.Fatal(err)
	}
	expected, err := filepath.EvalSymlinks(filepath.Join(root, "models", "nested", "process.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Inputs) != 1 || result.Inputs[0] != expected {
		t.Fatalf("inputs = %v", result.Inputs)
	}
}

func TestLintAppliesProjectConfigurationToExplicitContent(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".camunda.yaml"), "name: toolkit\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: models\n  dmn: dmn\n  forms: forms\n  tests: tests\nlint:\n  ignore:\n    - bpmn/service-task-retry\n")

	result, err := (Service{}).Lint(context.Background(), LintRequest{
		ProjectDir: root,
		Inputs:     []BPMNInput{{Name: "process.bpmn", Content: []byte(singleProcessBPMN)}},
		FailOn:     "warning",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || result.Status != StatusCompleted {
		t.Fatalf("project lint config was not applied: %+v", result)
	}
}

func TestMalformedInputReturnsTypedInputError(t *testing.T) {
	_, err := (Service{}).Explain(context.Background(), ExplainRequest{
		Input: BPMNInput{Name: "broken.bpmn", Content: []byte("<broken>")},
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorInput || serviceErr.Operation != OperationExplain {
		t.Fatalf("error = %#v", err)
	}
}

func TestDiffReturnsTypedGitFailure(t *testing.T) {
	gitErr := errors.New("revision missing")
	_, err := (Service{Git: stubGit{err: gitErr}}).Diff(context.Background(), DiffRequest{
		BeforeGit: &GitInput{Ref: "HEAD", Path: "process.bpmn"},
		After:     BPMNInput{Name: "process.bpmn", Content: []byte(singleProcessBPMN)},
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorGit || !errors.Is(err, gitErr) {
		t.Fatalf("error = %#v", err)
	}
}

func TestDiffRechecksContextAfterGitBeforeParsing(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	git := &cancelingGit{cancel: cancel}
	_, err := (Service{Git: git}).Diff(ctx, DiffRequest{
		BeforeGit: &GitInput{Ref: "HEAD", Path: "process.bpmn"},
		After:     BPMNInput{Name: "broken.bpmn", Content: []byte("<broken>")},
	})
	if !errors.Is(err, context.Canceled) || git.calls != 1 {
		t.Fatalf("error = %#v, calls = %d", err, git.calls)
	}
}

func TestDiffComparesEveryProcess(t *testing.T) {
	after := strings.Replace(multiProcessBPMN, `<serviceTask id="secondTask"/>`, `<serviceTask id="secondTask" name="changed"/>`, 1)
	result, err := (Service{}).Diff(context.Background(), DiffRequest{
		Before: BPMNInput{Name: "before.bpmn", Content: []byte(multiProcessBPMN)},
		After:  BPMNInput{Name: "after.bpmn", Content: []byte(after)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || !result.Complete || len(result.Changes) != 1 ||
		result.Changes[0].BeforeProcessID != "two" || result.Changes[0].AfterProcessID != "two" ||
		result.Changes[0].Change == nil || result.Changes[0].Change.ID != "secondTask" {
		t.Fatalf("result = %+v", result)
	}
}

func TestDiffComparesDocumentMessagesOnce(t *testing.T) {
	after := strings.Replace(multiProcessBPMN, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><message id="notice"/>`, 1)
	result, err := (Service{}).Diff(context.Background(), DiffRequest{
		Before: BPMNInput{Name: "before.bpmn", Content: []byte(multiProcessBPMN)},
		After:  BPMNInput{Name: "after.bpmn", Content: []byte(after)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Change == nil || result.Changes[0].Change.ID != "notice" ||
		result.Changes[0].BeforeProcessID != "" || result.Changes[0].AfterProcessID != "" {
		t.Fatalf("changes = %+v", result.Changes)
	}
}

func TestDiffReportsProcessAddedAndRemoved(t *testing.T) {
	oneOnly := strings.Replace(multiProcessBPMN, `  <process id="two">
    <startEvent id="secondStart"/>
    <serviceTask id="secondTask"/>
  </process>
`, "", 1)
	result, err := (Service{}).Diff(context.Background(), DiffRequest{
		Before: BPMNInput{Name: "before.bpmn", Content: []byte(oneOnly)},
		After:  BPMNInput{Name: "after.bpmn", Content: []byte(multiProcessBPMN)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Changes) != 1 || result.Changes[0].Kind != ProcessAdded ||
		result.Changes[0].BeforeProcessID != "" || result.Changes[0].AfterProcessID != "two" ||
		result.Changes[0].Change != nil {
		t.Fatalf("changes = %+v", result.Changes)
	}
}

func TestExplainReturnsEveryProcess(t *testing.T) {
	result, err := (Service{}).Explain(context.Background(), ExplainRequest{
		Input: BPMNInput{Name: "multi.bpmn", Content: []byte(multiProcessBPMN)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || !result.Complete || len(result.Processes) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestReviewOptionalAIRecordsWarningAndRequiredAIFails(t *testing.T) {
	aiErr := errors.New("offline")
	service := Service{AI: stubAI{err: aiErr}}
	optional, err := service.Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "process.bpmn", Content: []byte(singleProcessBPMN)}},
		AI:     AIOptions{Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if optional.AIStatus != AIStatusFailed || optional.Status != StatusPartial || optional.Complete || len(optional.Warnings) != 1 {
		t.Fatalf("optional result = %+v", optional)
	}

	_, err = service.Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "process.bpmn", Content: []byte(singleProcessBPMN)}},
		AI:     AIOptions{Enabled: true, Required: true},
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorAI {
		t.Fatalf("required error = %#v", err)
	}
}

func TestReviewPolicyFailurePrecedesOptionalAIFailure(t *testing.T) {
	result, err := (Service{AI: stubAI{err: errors.New("offline")}}).Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "process.bpmn", Content: []byte(singleProcessBPMN)}},
		FailOn: "warning",
		AI:     AIOptions{Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.Complete || result.AIStatus != AIStatusFailed || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v", result)
	}
	if len(result.Findings) != 1 || result.Findings[0].ProcessID != "one" {
		t.Fatalf("findings = %+v", result.Findings)
	}
}

func TestReviewRunsAIForEveryProcess(t *testing.T) {
	client := &recordingAI{}
	source := strings.Replace(multiProcessBPMN, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><message id="m1" name="duplicate"/><message id="m2" name="duplicate"/>`, 1)
	result, err := (Service{AI: client}).Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "multi.bpmn", Content: []byte(source)}},
		AI:     AIOptions{Enabled: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || !result.Complete || result.AIStatus != AIStatusSucceeded || len(result.Processes) != 2 {
		t.Fatalf("result = %+v", result)
	}
	processes := map[string]bool{}
	for _, request := range client.requests {
		if len(request.Document.Processes) != 1 {
			t.Fatalf("AI request document = %+v", request.Document)
		}
		if len(request.Document.Messages) != 2 {
			t.Fatalf("AI request lost document messages: %+v", request.Document)
		}
		processes[request.Document.Processes[0].ID] = true
	}
	if len(client.requests) != 2 || !processes["one"] || !processes["two"] {
		t.Fatalf("AI requests = %+v", client.requests)
	}
	documentFindings := 0
	for _, finding := range result.Findings {
		if finding.Finding.Rule == "bpmn/duplicate-message-name" {
			documentFindings++
		}
	}
	if documentFindings != 1 {
		t.Fatalf("duplicate document findings = %d; findings = %+v", documentFindings, result.Findings)
	}
}

func TestReviewRequiredAIIsNotSilentlyDisabled(t *testing.T) {
	_, err := (Service{}).Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "process.bpmn", Content: []byte(singleProcessBPMN)}},
		AI:     AIOptions{Required: true},
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorAI {
		t.Fatalf("required error = %#v", err)
	}
}

func TestGenerateReturnsArtifactContents(t *testing.T) {
	result, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input:  BPMNInput{Name: "process.bpmn", Content: []byte(singleProcessBPMN)},
		OutDir: t.TempDir(),
		Lang:   "js",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || len(result.Artifacts[0].Content) == 0 || result.Artifacts[0].MediaType != "text/javascript" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
}

func TestGenerateReturnsArtifactForEveryProcess(t *testing.T) {
	result, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input:  BPMNInput{Name: "multi.bpmn", Content: []byte(multiProcessBPMN)},
		OutDir: t.TempDir(),
		Lang:   "js",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || !result.Complete || len(result.Artifacts) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestGenerateRejectsLanguageBeforeInputWork(t *testing.T) {
	_, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input: BPMNInput{Name: "broken.bpmn", Content: []byte("<broken>")},
		Lang:  "python",
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorInvalidRequest {
		t.Fatalf("error = %#v", err)
	}
}

func TestRejectsInvalidThresholdsBeforeInputWork(t *testing.T) {
	tests := []struct {
		name string
		run  func() error
	}{
		{
			name: "lint",
			run: func() error {
				_, err := (Service{}).Lint(context.Background(), LintRequest{
					Inputs: []BPMNInput{{Name: "broken.bpmn", Content: []byte("<broken>")}},
					FailOn: "warnings",
				})
				return err
			},
		},
		{
			name: "review",
			run: func() error {
				_, err := (Service{}).Review(context.Background(), ReviewRequest{
					Inputs: []BPMNInput{{Name: "broken.bpmn", Content: []byte("<broken>")}},
					FailOn: "warnings",
				})
				return err
			},
		},
		{
			name: "scan",
			run: func() error {
				_, err := (Service{}).Scan(context.Background(), ScanRequest{
					Roots:  []string{filepath.Join(t.TempDir(), "missing")},
					FailOn: "critical",
				})
				return err
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var serviceErr *Error
			if err := test.run(); !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorInvalidRequest {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestGenerateRejectsSanitizedPathCollisionWithoutPublishing(t *testing.T) {
	out := t.TempDir()
	source := strings.Replace(multiProcessBPMN, `id="one"`, `id="a-b"`, 1)
	source = strings.Replace(source, `id="two"`, `id="a_b"`, 1)
	_, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input:  BPMNInput{Name: "collision.bpmn", Content: []byte(source)},
		OutDir: out,
		Lang:   "js",
		Force:  true,
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorArtifact {
		t.Fatalf("error = %#v", err)
	}
	if files := regularFiles(t, out); len(files) != 0 {
		t.Fatalf("published files after collision: %v", files)
	}
}

func TestGenerateRollsBackWhenAnyFinalPathExists(t *testing.T) {
	out := t.TempDir()
	existing := filepath.Join(out, "js", "Two.test.js")
	write(t, existing, "keep")
	_, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input:  BPMNInput{Name: "multi.bpmn", Content: []byte(multiProcessBPMN)},
		OutDir: out,
		Lang:   "js",
	})
	if err == nil {
		t.Fatal("expected overwrite refusal")
	}
	if got, readErr := os.ReadFile(existing); readErr != nil || string(got) != "keep" {
		t.Fatalf("existing file changed: %q, %v", got, readErr)
	}
	if _, statErr := os.Stat(filepath.Join(out, "js", "One.test.js")); !os.IsNotExist(statErr) {
		t.Fatalf("first artifact published: %v", statErr)
	}
}

func TestPublishArtifactsRollsBackFilesAndDirectories(t *testing.T) {
	out := t.TempDir()
	blocker := filepath.Join(out, "blocker")
	write(t, blocker, "not a directory")
	first := filepath.Join(out, "created", "nested", "first.js")
	_, err := publishArtifacts(context.Background(), []preparedArtifact{
		{artifact: Artifact{Path: first, Content: []byte("first")}},
		{artifact: Artifact{Path: filepath.Join(blocker, "second.js"), Content: []byte("second")}},
	})
	if err == nil {
		t.Fatal("expected publish failure")
	}
	if _, statErr := os.Stat(first); !os.IsNotExist(statErr) {
		t.Fatalf("first artifact remains: %v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(out, "created")); !os.IsNotExist(statErr) {
		t.Fatalf("created directories remain: %v", statErr)
	}
	if got, readErr := os.ReadFile(blocker); readErr != nil || string(got) != "not a directory" {
		t.Fatalf("blocker changed: %q, %v", got, readErr)
	}
}

func TestScanContinuesAfterRootFailure(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "clean.txt"), "no secrets\n")
	missing := filepath.Join(root, "missing")
	result, err := (Service{}).Scan(context.Background(), ScanRequest{Roots: []string{missing, root}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusPartial || result.Complete || len(result.Warnings) != 1 || len(result.Findings) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestScanPolicyFailurePrecedesPartialRootFailure(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".env"), "CLIENT_SECRET=real-secret-value\n")
	result, err := (Service{}).Scan(context.Background(), ScanRequest{
		Roots:  []string{filepath.Join(root, "missing"), root},
		FailOn: "medium",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusFailed || result.Complete || len(result.FailedRoots) != 1 || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestScanPropagatesUnreadableFileAccounting(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "secret.env")); err != nil {
		t.Fatal(err)
	}
	result, err := (Service{}).Scan(context.Background(), ScanRequest{Roots: []string{root}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.Status != StatusPartial || len(result.Warnings) != 1 {
		t.Fatalf("result = %+v", result)
	}
}

func TestCanceledContextPrecedesDiscovery(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (Service{}).Lint(ctx, LintRequest{ProjectDir: filepath.Join(t.TempDir(), "missing")})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %#v", err)
	}
}

type stubGit struct {
	data []byte
	err  error
}

func (s stubGit) Read(_ context.Context, _, _ string) ([]byte, error) { return s.data, s.err }

type cancelingGit struct {
	cancel context.CancelFunc
	calls  int
}

func (g *cancelingGit) Read(_ context.Context, _, _ string) ([]byte, error) {
	g.calls++
	g.cancel()
	return []byte(singleProcessBPMN), nil
}

type stubAI struct {
	response ai.ChatResponse
	err      error
}

func (s stubAI) Complete(_ context.Context, _ ai.ChatRequest) (ai.ChatResponse, error) {
	return s.response, s.err
}

type recordingAI struct {
	requests []ai.ChatRequest
}

func (r *recordingAI) Complete(_ context.Context, request ai.ChatRequest) (ai.ChatResponse, error) {
	r.requests = append(r.requests, request)
	return ai.ChatResponse{Content: "reviewed"}, nil
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasLintElement(result LintResult, element string) bool {
	for _, finding := range result.Findings {
		if finding.Finding.Element == element {
			return true
		}
	}
	return false
}

func regularFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	if err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			files = append(files, path)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return files
}

const singleProcessBPMN = `<?xml version="1.0"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="one">
    <startEvent id="start"/>
    <serviceTask id="task"/>
    <endEvent id="end"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="task"/>
    <sequenceFlow id="f2" sourceRef="task" targetRef="end"/>
  </process>
</definitions>`

const multiProcessBPMN = `<?xml version="1.0"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="one">
    <startEvent id="firstStart"/>
  </process>
  <process id="two">
    <startEvent id="secondStart"/>
    <serviceTask id="secondTask"/>
  </process>
</definitions>`
