package toolkit

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
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

func TestDiffKeepsProcessAndDocumentChangeAttribution(t *testing.T) {
	before := strings.Replace(
		multiProcessBPMN,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><error id="failure" name="Before" errorCode="OLD"/>`,
		1,
	)
	after := strings.Replace(
		before,
		`<error id="failure" name="Before" errorCode="OLD"/>`,
		`<error id="failure" name="After" errorCode="NEW"/>`,
		1,
	)
	after = strings.Replace(after, `<process id="two">`, `<process id="two" name="Renamed">`, 1)

	result, err := (Service{}).Diff(context.Background(), DiffRequest{
		Before: BPMNInput{Name: "before.bpmn", Content: []byte(before)},
		After:  BPMNInput{Name: "after.bpmn", Content: []byte(after)},
	})
	if err != nil {
		t.Fatal(err)
	}

	var processChanges, documentChanges int
	for _, change := range result.Changes {
		switch change.Kind {
		case ProcessModified:
			processChanges++
			if change.BeforeProcessID != "two" || change.AfterProcessID != "two" ||
				change.Change == nil || change.Change.Field != "name" {
				t.Fatalf("process attribution = %+v", change)
			}
		case DocumentChanged:
			documentChanges++
			if change.BeforeProcessID != "" || change.AfterProcessID != "" || change.Change == nil ||
				change.Change.ElementType != "error" {
				t.Fatalf("document attribution = %+v", change)
			}
		}
	}
	if processChanges != 1 || documentChanges != 2 {
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

func TestDiffUsesDocumentSemanticIdentityAcrossRenamedIDs(t *testing.T) {
	before := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><message id="old-message" name="Notice"><documentation>ignored</documentation></message><process id="old-process" name="Process"><startEvent id="old-start"><messageEventDefinition messageRef="old-message"/></startEvent><endEvent id="old-end"/><sequenceFlow id="old-flow" sourceRef="old-start" targetRef="old-end"/></process></definitions>`
	after := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" exporter="ignored"><process name="Process" id="new-process"><endEvent id="new-end"/><startEvent id="new-start"><messageEventDefinition messageRef="new-message"/></startEvent><sequenceFlow targetRef="new-end" id="new-flow" sourceRef="new-start"/></process><message name="Notice" id="new-message"/></definitions>`
	result, err := (Service{}).Diff(context.Background(), DiffRequest{
		Before: BPMNInput{Name: "before.bpmn", Content: []byte(before)},
		After:  BPMNInput{Name: "after.bpmn", Content: []byte(after)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || len(result.Changes) != 0 {
		t.Fatalf("result = %+v", result)
	}
}

func TestExplainReturnsEveryProcess(t *testing.T) {
	source := strings.Replace(multiProcessBPMN,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`+
			`<message id="unreferenced-message" name="Only once"/>`+
			`<error id="unreferenced-error" name="Only one error" errorCode="ONCE"/>`, 1)
	result, err := (Service{}).Explain(context.Background(), ExplainRequest{
		Input: BPMNInput{Name: "multi.bpmn", Content: []byte(source)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusCompleted || !result.Complete || len(result.Processes) != 2 {
		t.Fatalf("result = %+v", result)
	}
	var technical strings.Builder
	for _, process := range result.Processes {
		technical.WriteString(process.Explanation.Technical)
	}
	for _, definition := range []string{"Only once", "Only one error", "errorCode=\"ONCE\""} {
		if strings.Count(technical.String(), definition) != 1 {
			t.Fatalf("definition %q count != 1:\n%s", definition, technical.String())
		}
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

	requiredSource := strings.Replace(multiProcessBPMN,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`,
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><message id="m1" name="duplicate"/><message id="m2" name="duplicate"/>`, 1)
	required, err := service.Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "multi.bpmn", Content: []byte(requiredSource)}},
		AI:     AIOptions{Enabled: true, Required: true},
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorAI {
		t.Fatalf("required error = %#v", err)
	}
	var domainAIError *review.AIError
	if !errors.As(err, &domainAIError) || domainAIError.Code != "ai_provider_failed" ||
		!strings.Contains(strings.ToLower(err.Error()), "endpoint") {
		t.Fatalf("required provider detail = %+v / %v", domainAIError, err)
	}
	if len(required.Documents) != 1 || len(required.Processes) != 2 || len(required.Findings) < 3 {
		t.Fatalf("required AI discarded offline review: %+v", required)
	}
	if required.AIStatus != AIStatusFailed || required.Status != StatusFailed {
		t.Fatalf("required AI result status = %+v", required)
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
		documentPromptFindings := 0
		for _, finding := range request.Findings {
			if finding.Rule == "bpmn/duplicate-message-name" {
				documentPromptFindings++
			}
		}
		if documentPromptFindings != 1 {
			t.Fatalf("document findings in AI request = %d; request=%+v", documentPromptFindings, request)
		}
		if strings.Count(request.Prompt, `"rule":"bpmn/duplicate-message-name"`) != 1 {
			t.Fatalf("document finding prompt count != 1:\n%s", request.Prompt)
		}
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
	result, err := (Service{}).Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "multi.bpmn", Content: []byte(multiProcessBPMN)}},
		AI:     AIOptions{Required: true},
	})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorAI {
		t.Fatalf("required error = %#v", err)
	}
	var domainAIError *review.AIError
	if !errors.As(err, &domainAIError) || domainAIError.Code != "ai_unavailable" ||
		!strings.Contains(strings.ToLower(err.Error()), "credentials") {
		t.Fatalf("required missing-client detail = %+v / %v", domainAIError, err)
	}
	if len(result.Documents) != 1 || len(result.Processes) != 2 || len(result.Findings) == 0 ||
		result.AIStatus != AIStatusFailed {
		t.Fatalf("missing client discarded offline review: %+v", result)
	}
}

func TestReviewTransportFailurePreservesCauseThroughToolkitError(t *testing.T) {
	const secret = "toolkit-transport-secret"
	cause := &toolkitTransportError{secret: secret, cause: syscall.ECONNRESET}
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", APIKey: "not-rendered",
		BaseURL: "http://provider.invalid/v1",
		HTTPClient: &http.Client{Transport: toolkitRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, cause
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	result, err := (Service{AI: client}).Review(context.Background(), ReviewRequest{
		Inputs: []BPMNInput{{Name: "process.bpmn", Content: []byte(singleProcessBPMN)}},
		AI:     AIOptions{Required: true},
	})
	var serviceErr *Error
	var domainErr *review.AIError
	var providerErr *ai.ProviderError
	var urlErr *url.Error
	var netErr net.Error
	var original *toolkitTransportError
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorAI ||
		!errors.As(err, &domainErr) || domainErr.Code != "ai_transport_error" ||
		!errors.As(err, &providerErr) || !errors.As(err, &urlErr) ||
		!errors.As(err, &netErr) || !errors.As(err, &original) || original != cause ||
		!errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("toolkit transport chain was not preserved: %#v", err)
	}
	if len(result.Findings) == 0 || result.AIStatus != AIStatusFailed {
		t.Fatalf("offline result was lost: %+v", result)
	}
	for _, forbidden := range []string{secret, "provider.invalid", "not-rendered"} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("toolkit transport detail leaked: %v", err)
		}
	}
}

func TestGenerateReturnsArtifactContents(t *testing.T) {
	out := t.TempDir()
	result, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input:  BPMNInput{Name: "process.bpmn", Content: []byte(singleProcessBPMN)},
		OutDir: out,
		Lang:   "js",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || len(result.Artifacts[0].Content) == 0 || result.Artifacts[0].MediaType != "text/javascript" {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
}

func TestGenerateCanReturnDownloadableContentWithoutServerWrites(t *testing.T) {
	result, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input: BPMNInput{Name: "process.bpmn", Content: []byte(singleProcessBPMN)},
		Lang:  "python",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || result.Artifacts[0].Path != "python/test_one.py" ||
		result.Artifacts[0].MediaType != "text/x-python" || len(result.Artifacts[0].Content) == 0 {
		t.Fatalf("artifacts = %+v", result.Artifacts)
	}
	if filepath.IsAbs(result.Artifacts[0].Path) {
		t.Fatalf("download artifact path is absolute: %q", result.Artifacts[0].Path)
	}
}

func TestGenerateWithProjectConfigDoesNotPublishWithoutOutDir(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, ".camunda.yaml"),
		[]byte("name: purity\npaths:\n  bpmn: bpmn\n  tests: configured-tests\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	input := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(input, []byte(singleProcessBPMN), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := (Service{}).Generate(context.Background(), GenerateRequest{
		Input:      BPMNInput{Name: input, Path: input},
		ProjectDir: root,
		Lang:       "js",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Artifacts) != 1 || filepath.IsAbs(result.Artifacts[0].Path) {
		t.Fatalf("download artifacts = %+v", result.Artifacts)
	}
	if _, err := os.Stat(filepath.Join(root, "configured-tests")); !os.IsNotExist(err) {
		t.Fatalf("configured tests path was written in render-only mode: %v", err)
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
		Lang:  "ruby",
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

func TestScanPreservesCancellationThroughTypedError(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		name   string
		cancel func(context.Context) (context.Context, func())
		target error
	}{
		{
			name: "canceled",
			cancel: func(parent context.Context) (context.Context, func()) {
				return context.WithCancel(parent)
			},
			target: context.Canceled,
		},
		{
			name: "deadline",
			cancel: func(parent context.Context) (context.Context, func()) {
				ctx := &controlledContext{Context: parent}
				return ctx, func() { ctx.err = context.DeadlineExceeded }
			},
			target: context.DeadlineExceeded,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, cancel := test.cancel(context.Background())
			service := Service{
				scan: func(callCtx context.Context, options scan.Options) (scan.Result, error) {
					if options.Root != root {
						t.Fatalf("root = %q", options.Root)
					}
					cancel()
					return scan.Result{}, callCtx.Err()
				},
			}

			_, err := service.Scan(ctx, ScanRequest{Roots: []string{root}, FailOn: "medium"})
			var serviceErr *Error
			if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorScan {
				t.Fatalf("typed error = %#v", err)
			}
			if !errors.Is(err, test.target) {
				t.Fatalf("cancellation chain was lost: %v", err)
			}
		})
	}
}

type controlledContext struct {
	context.Context
	err error
}

func (ctx *controlledContext) Err() error { return ctx.err }

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
	existing := filepath.Join(out, "js", "Two.spec.js")
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
	if _, statErr := os.Stat(filepath.Join(out, "js", "One.spec.js")); !os.IsNotExist(statErr) {
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

func TestPublishArtifactsRestoresForceOverwrittenArtifactAfterLaterFailure(t *testing.T) {
	out := t.TempDir()
	first := filepath.Join(out, "first.js")
	write(t, first, "original")
	blocker := filepath.Join(out, "blocker")
	write(t, blocker, "not a directory")

	_, err := publishArtifacts(context.Background(), []preparedArtifact{
		{
			artifact: Artifact{Path: first, Content: []byte("replacement")},
			existed:  true,
			original: []byte("original"),
			mode:     0o644,
		},
		{artifact: Artifact{Path: filepath.Join(blocker, "second.js"), Content: []byte("second")}},
	})
	if err == nil {
		t.Fatal("expected later publish failure")
	}
	if got, readErr := os.ReadFile(first); readErr != nil || string(got) != "original" {
		t.Fatalf("force-overwritten artifact not restored: %q, %v", got, readErr)
	}
}

func TestRollbackContinuesAfterOneArtifactRollbackFails(t *testing.T) {
	out := t.TempDir()
	first := filepath.Join(out, "first.js")
	second := filepath.Join(out, "second.js")
	third := filepath.Join(out, "third.js")
	write(t, first, "original")
	publishErr := errors.New("publish failed")
	removeErr := errors.New("remove failed")

	_, err := publishArtifactsWithOps(context.Background(), []preparedArtifact{
		{
			artifact: Artifact{Path: first, Content: []byte("replacement")},
			existed:  true,
			original: []byte("original"),
			mode:     0o644,
		},
		{artifact: Artifact{Path: second, Content: []byte("second")}},
		{artifact: Artifact{Path: third, Content: []byte("third")}},
	}, artifactFileOps{
		write: func(path string, content []byte, mode os.FileMode) error {
			if path == third {
				return publishErr
			}
			return atomicWrite(path, content, mode)
		},
		remove: func(path string) error {
			if path == second {
				return removeErr
			}
			return os.Remove(path)
		},
	})
	if !errors.Is(err, publishErr) || !errors.Is(err, removeErr) {
		t.Fatalf("joined error = %v", err)
	}
	if got, readErr := os.ReadFile(first); readErr != nil || string(got) != "original" {
		t.Fatalf("earlier artifact was not restored: %q, %v", got, readErr)
	}
	if !strings.Contains(err.Error(), "second.js") || strings.Contains(err.Error(), ".camunda-lab-artifact-") {
		t.Fatalf("rollback error lacks safe path context: %v", err)
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

func TestScanAppliesRequestIgnoreRulesAfterProjectRules(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".gitignore"), "*.env\n")
	write(t, filepath.Join(root, "keep.env"), "CLIENT_SECRET=kept-secret-value\n")
	result, err := (Service{}).Scan(context.Background(), ScanRequest{
		Roots: []string{root}, Ignore: []string{"!keep.env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 || result.Findings[0].File != "keep.env" {
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
	if result.Stats.Discovered != 1 || result.Stats.Errored != 1 ||
		len(result.Issues) != 1 || result.Issues[0].Path != "secret.env" {
		t.Fatalf("scan accounting was not propagated: %+v", result)
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

type toolkitRoundTripFunc func(*http.Request) (*http.Response, error)

func (f toolkitRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type toolkitTransportError struct {
	secret string
	cause  error
}

func (e *toolkitTransportError) Error() string { return e.secret }
func (e *toolkitTransportError) Unwrap() error { return e.cause }
func (e *toolkitTransportError) Timeout() bool { return false }
func (e *toolkitTransportError) Temporary() bool {
	return true
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
