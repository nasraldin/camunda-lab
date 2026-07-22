package scan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWalkFindsClientSecret(t *testing.T) {
	dir := t.TempDir()
	dirty := filepath.Join(dir, "connectors", "secrets.env")
	if err := os.MkdirAll(filepath.Dir(dirty), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirty, []byte("CLIENT_SECRET=supersecretvalue99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(clean, []byte("no secrets here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Walk(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) == 0 {
		t.Fatal("expected finding")
	}
	if !strings.Contains(fs[0].Snippet, "…") && fs[0].Snippet != "****" {
		t.Fatalf("expected masked snippet, got %q", fs[0].Snippet)
	}
	if strings.Contains(FormatText(fs), "supersecretvalue99") {
		t.Fatal("raw secret leaked in output")
	}
	if !ShouldFail(fs, "medium") {
		t.Fatal("should fail")
	}
}

func TestWalkClean(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Walk(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Fatalf("unexpected %#v", fs)
	}
}

func TestWalkWithReportContextCancelsDuringTraversal(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "a.yaml", "name: first\n")
	writeScanFile(t, dir, "nested/b.yaml", "name: second\n")
	ctx, cancel := context.WithCancel(context.Background())
	visited := 0

	_, err := walkWithHooksContext(ctx, Options{Root: dir}, walkHooks{
		beforeOpen: func(string) error {
			visited++
			cancel()
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if visited == 0 {
		t.Fatal("scan canceled before traversal started")
	}
}

func TestWalkWithReportContextCancelsDuringCandidateRead(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "candidate.yaml", strings.Repeat("name: value\n", 1024))
	ctx, cancel := context.WithCancel(context.Background())
	opened := false

	_, err := walkWithHooksContext(ctx, Options{Root: dir}, walkHooks{
		afterOpen: func(string) error {
			opened = true
			cancel()
			return nil
		},
	})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if !opened {
		t.Fatal("scan canceled before candidate read")
	}
}

func TestWalkWithReportContextCancelsDuringNestedIgnoreRead(t *testing.T) {
	for _, test := range []struct {
		name       string
		ignore     string
		cancelLine int
		target     error
	}{
		{name: "canceled during read", ignore: "*.tmp\n*.log\n", cancelLine: 1, target: context.Canceled},
		{name: "canceled final entry", ignore: "*.tmp\n", cancelLine: 1, target: context.Canceled},
		{name: "deadline during read", ignore: "*.tmp\n*.log\n", cancelLine: 1, target: context.DeadlineExceeded},
		{name: "deadline final entry", ignore: "*.tmp\n", cancelLine: 1, target: context.DeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			dir := t.TempDir()
			writeScanFile(t, dir, "nested/.gitignore", test.ignore)
			writeScanFile(t, dir, "nested/candidate.yaml", "name: value\n")
			ctx := &controlledScanContext{Context: context.Background()}
			observed := 0

			result, err := walkWithHooksContext(ctx, Options{Root: dir}, walkHooks{
				afterIgnoreLine: func(source string, line int) error {
					if source == "nested/.gitignore" {
						observed = line
						if line == test.cancelLine {
							ctx.err = test.target
						}
					}
					return nil
				},
			})
			if !errors.Is(err, test.target) {
				t.Fatalf("error = %v; result = %+v", err, result)
			}
			if observed != test.cancelLine {
				t.Fatalf("observed line = %d, want %d", observed, test.cancelLine)
			}
			if len(result.Issues) != 0 {
				t.Fatalf("cancellation was converted to issues: %+v", result.Issues)
			}
		})
	}
}

type controlledScanContext struct {
	context.Context
	err error
}

func (ctx *controlledScanContext) Err() error { return ctx.err }

func TestWalkWithReportKeepsNestedIgnoreParseFailureAsIssue(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "nested/.gitignore", "../outside\n")
	writeScanFile(t, dir, "nested/candidate.yaml", "name: value\n")

	result, err := WalkWithReportContext(context.Background(), Options{Root: dir})
	if err != nil {
		t.Fatalf("genuine nested ignore failure returned as fatal: %v", err)
	}
	if result.Complete || len(result.Issues) != 1 ||
		!strings.Contains(result.Issues[0].Message, "load nested ignore") {
		t.Fatalf("result = %+v", result)
	}
}

func TestWalkWithReportAccountsForUnreadableFiles(t *testing.T) {
	dir := t.TempDir()
	broken := filepath.Join(dir, "secret.env")
	if err := os.Symlink(filepath.Join(dir, "missing-target"), broken); err != nil {
		t.Fatal(err)
	}
	result, err := WalkWithReport(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || len(result.Issues) != 1 || result.Issues[0].Path != "secret.env" {
		t.Fatalf("result = %+v", result)
	}
	if result.Complete || result.Stats.Discovered != 1 || result.Stats.Errored != 1 {
		t.Fatalf("accounting = %+v", result)
	}
	if result.Issues[0].Kind != IssueError || result.Issues[0].Message == "" {
		t.Fatalf("issue = %+v", result.Issues[0])
	}
}

func TestWalkRespectsIgnorePrecedenceAndNegation(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, ".gitignore", "ignored/\n*.env\n")
	writeScanFile(t, dir, ".camunda-scanignore", "!keep.env\nignored/\n")
	writeScanFile(t, dir, "keep.env", "CLIENT_SECRET=keep-secret-value\n")
	writeScanFile(t, dir, "drop.env", "CLIENT_SECRET=drop-secret-value\n")
	writeScanFile(t, dir, "ignored/nested.env", "CLIENT_SECRET=nested-secret-value\n")
	writeScanFile(t, dir, "user.txt", "password=user-secret-value\n")

	result, err := WalkWithReport(Options{Root: dir, Ignore: []string{"user.txt", "!drop.env"}})
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, finding := range result.Findings {
		files = append(files, finding.File)
	}
	if want := []string{"drop.env", "keep.env"}; !reflect.DeepEqual(files, want) {
		t.Fatalf("files = %v, want %v", files, want)
	}
	if !result.Complete || result.Stats != (Stats{Discovered: 4, Scanned: 2, Ignored: 2}) {
		t.Fatalf("result = %+v", result)
	}
	for _, issue := range result.Issues {
		if issue.Kind != IssueIgnored || issue.Reason == "" {
			t.Fatalf("ignored issue lacks reason: %+v", issue)
		}
	}
}

func TestProjectFixtureHasStableAccounting(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "scan", "project")
	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 2 ||
		result.Stats != (Stats{Discovered: 3, Scanned: 2, Ignored: 1}) ||
		!result.Complete {
		t.Fatalf("result = %+v", result)
	}
}

func TestBuiltInIgnoresCannotBeNegated(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "node_modules/leak.js", "api_key=module-secret-value\n")
	writeScanFile(t, dir, "build/leak.yaml", "password=build-secret-value\n")
	writeScanFile(t, dir, ".git/leak.txt", "password=git-secret-value\n")
	writeScanFile(t, dir, "source/app.yaml", "password=source-secret-value\n")

	result, err := WalkWithReport(Options{Root: dir, Ignore: []string{
		"!node_modules/**", "!build/**", "!.git/**",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].File != "source/app.yaml" {
		t.Fatalf("findings = %+v", result.Findings)
	}
	outsideIgnore := filepath.Join(t.TempDir(), ".gitignore")
	if err := os.WriteFile(outsideIgnore, []byte("!**\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsideIgnore, filepath.Join(dir, "node_modules", ".gitignore")); err != nil {
		t.Fatal(err)
	}
	targeted, err := WalkWithReport(Options{
		Root: filepath.Join(dir, "node_modules"), Ignore: []string{"!**"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(targeted.Findings) != 0 || targeted.Stats != (Stats{Discovered: 1, Ignored: 1}) {
		t.Fatalf("explicit built-in root was scanned: %+v", targeted)
	}
}

func TestDoubleStarIgnoreMatchesRootAndNestedPaths(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, ".gitignore", "**/generated.env\n")
	writeScanFile(t, dir, "generated.env", "password=root-generated-secret\n")
	writeScanFile(t, dir, "nested/generated.env", "password=nested-generated-secret\n")
	result, err := WalkWithReport(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 || result.Stats != (Stats{Discovered: 2, Ignored: 2}) {
		t.Fatalf("result = %+v", result)
	}
}

func TestRejectsUnsafeIgnorePattern(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "ok.yaml", "name: ok\n")
	result, err := WalkWithReport(Options{Root: dir, Ignore: []string{"../outside/**"}})
	if err == nil || !strings.Contains(err.Error(), "unsafe ignore pattern") {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestRejectsInvalidThresholdBeforeWalking(t *testing.T) {
	result, err := WalkWithReport(Options{Root: filepath.Join(t.TempDir(), "missing"), FailOn: "critical"})
	if err == nil || !strings.Contains(err.Error(), "fail threshold") {
		t.Fatalf("result = %+v, error = %v", result, err)
	}
}

func TestBinaryLargeAndUnsupportedFilesAreNeverScanned(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "binary.json", "CLIENT_SECRET=binary\x00secret\n")
	writeScanFile(t, dir, "large.yaml", "CLIENT_SECRET="+strings.Repeat("x", 80)+"\n")
	writeScanFile(t, dir, "archive.zip", "CLIENT_SECRET=archive-secret-value\n")
	writeScanFile(t, dir, "source.yaml", "CLIENT_SECRET=source-secret-value\n")

	result, err := WalkWithReport(Options{Root: dir, MaxFileSize: 64})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].File != "source.yaml" {
		t.Fatalf("findings = %+v", result.Findings)
	}
	if !result.Complete || result.Stats != (Stats{Discovered: 3, Scanned: 1, Ignored: 2}) {
		t.Fatalf("result = %+v", result)
	}
}

func TestLongLineIsReportedAsTruncatedAndPartial(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "long.yaml", strings.Repeat("x", 128)+"\n")
	result, err := WalkWithReport(Options{Root: dir, MaxLineBytes: 32})
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || result.Stats != (Stats{Discovered: 1, Errored: 1}) ||
		len(result.Issues) != 1 || result.Issues[0].Kind != IssueTruncated {
		t.Fatalf("result = %+v", result)
	}
	if strings.Contains(FormatText(result), "No secrets found") {
		t.Fatalf("partial report claimed clean:\n%s", FormatText(result))
	}
}

func TestFindingAttributionOrderingMaskingAndSeverity(t *testing.T) {
	dir := t.TempDir()
	writeScanFile(t, dir, "z.ts", "api_key=z-secret-token-value\n")
	writeScanFile(t, dir, "a.bpmn", "<property name=\"password\" value=\"a-secret-password-value\" />\n")
	writeScanFile(t, dir, "nested/b.yaml", "client_secret=b-secret-client-value\n")
	writeScanFile(t, dir, "suppressed.env", "CLIENT_SECRET=suppressed-secret-value # camunda-scan-ignore\n")

	first, err := WalkWithReport(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	second, err := WalkWithReport(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("results differ:\n%+v\n%+v", first, second)
	}
	if len(first.Findings) != 3 {
		t.Fatalf("findings = %+v", first.Findings)
	}
	for _, finding := range first.Findings {
		if filepath.IsAbs(finding.File) || strings.Contains(finding.File, `\`) ||
			finding.Line < 1 || finding.RuleID == "" || finding.SourceKind == "" ||
			strings.Contains(finding.Snippet, "secret") {
			t.Fatalf("finding attribution or masking = %+v", finding)
		}
	}
	if first.Findings[0].File != "a.bpmn" || first.Findings[0].SourceKind != SourceBPMN ||
		first.Findings[0].Severity != SeverityHigh {
		t.Fatalf("first finding = %+v", first.Findings[0])
	}
	if ShouldFail(first.Findings, "high") != true || ShouldFail(first.Findings[2:], "high") {
		t.Fatalf("severity threshold was not independent")
	}
}

func TestSymlinksDoNotEscapeOrLoop(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	writeScanFile(t, outside, "outside.yaml", "CLIENT_SECRET=outside-secret-value\n")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(root, filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	writeScanFile(t, root, "vendor/vendor.env", "CLIENT_SECRET=vendor-secret-value\n")
	if err := os.Symlink(
		filepath.Join(root, "vendor", "vendor.env"),
		filepath.Join(root, "vendor-alias.env"),
	); err != nil {
		t.Fatal(err)
	}
	writeScanFile(t, root, "inside.yaml", "CLIENT_SECRET=inside-secret-value\n")

	result, err := WalkWithReport(Options{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].File != "inside.yaml" {
		t.Fatalf("result = %+v", result)
	}
}

func TestStableJSONUsesArraysAndShowsPartialErrors(t *testing.T) {
	result := Result{
		Findings: []Finding{{
			Rule: "secret.client", RuleID: "secret.client", Severity: SeverityHigh,
			File: "app.yaml", Line: 2, Snippet: "****", SourceKind: SourceYAML,
		}},
		Issues: []Issue{{
			Path: "broken.yaml", Kind: IssueError, Message: "permission denied",
		}},
		Complete: false,
		Stats:    Stats{Discovered: 1, Errored: 1},
	}
	data, err := FormatJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["findings"].([]any); !ok {
		t.Fatalf("findings is not an array: %s", data)
	}
	if _, ok := decoded["issues"].([]any); !ok {
		t.Fatalf("issues is not an array: %s", data)
	}
	if !strings.Contains(string(data), `"complete": false`) ||
		!strings.Contains(string(data), `"rule": "secret.client"`) ||
		strings.Contains(string(data), `"ruleId"`) ||
		!strings.Contains(FormatText(result), "permission denied") {
		t.Fatalf("partial error missing: json=%s text=%s", data, FormatText(result))
	}
}

func writeScanFile(t *testing.T, root, relative, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
