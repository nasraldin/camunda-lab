package review

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

func TestOfflineReview(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "lint", "broken.bpmn")
	m, err := bpmn.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(m, Options{File: "broken.bpmn"})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Findings) == 0 {
		t.Fatal("expected lint findings")
	}
	if res.AIText != "" {
		t.Fatal("offline should not include AI")
	}
	if !lint.ShouldFail(res.Findings, "error") {
		t.Fatal("should fail")
	}
}

func TestAIReviewMocked(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "order-v1.bpmn")
	m, err := bpmn.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := Run(m, Options{
		AI:       true,
		AIClient: &recordingClient{response: ai.ChatResponse{Content: "reviewed"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	text := FormatText(res)
	if !strings.Contains(text, "Review (lint)") {
		t.Fatal(text)
	}
	if !strings.Contains(text, "AI suggestions") {
		t.Fatal(text)
	}
	if res.AIText != "reviewed" || res.AIStatus != AIStatusSucceeded {
		t.Fatal(res.AIText)
	}
}

func TestPromptContainsCompleteNormalizedSemanticsAndFindings(t *testing.T) {
	document := bpmn.Document{
		Processes: []bpmn.Process{{
			ID: "orders", Name: "Orders",
			Elements: []bpmn.Element{
				{ID: "start", Type: "startEvent", Name: "Begin", MessageRef: "message"},
				{ID: "gateway", Type: "exclusiveGateway", Name: "Choice", DefaultFlow: "default"},
				{ID: "task", Type: "serviceTask", Name: "Charge", JobType: "charge", RetryCount: "5"},
				{ID: "timer", Type: "intermediateCatchEvent", Name: "Wait", TimerKind: "duration", Timer: "PT5M", EventDefs: []string{"timer"}},
				{ID: "end", Type: "endEvent", Name: "Done"},
			},
			Flows: []bpmn.Flow{
				{ID: "one", Source: "start", Target: "gateway"},
				{ID: "default", Source: "gateway", Target: "task"},
				{ID: "conditional", Name: "later", Source: "gateway", Target: "timer", Condition: "= retry"},
				{ID: "done", Source: "task", Target: "end"},
			},
		}},
		Messages: []bpmn.Message{{ID: "message", Name: "Order requested"}},
		Errors:   []bpmn.Error{{ID: "payment-error", Name: "Payment failed", ErrorCode: "PAYMENT"}},
		UnknownExtensions: []bpmn.Extension{{
			QName: xml.Name{Space: "urn:example", Local: "review-metadata"},
			Attributes: []bpmn.Attribute{
				{QName: xml.Name{Local: "mode"}, Value: "strict"},
				{QName: xml.Name{Local: "apiKey"}, Value: "must-not-leak"},
				{QName: xml.Name{Local: "name"}, Value: "token"},
				{QName: xml.Name{Local: "value"}, Value: "outer-designated-secret"},
			},
			InnerXML: `<risk level="high"><property name="apiKey" value="nested-property-secret"/>` +
				`<group><token>nested-element-secret</token></group>` +
				`<property key="password"><value>nested-value-secret</value></property></risk>`,
		}},
	}
	findings := []lint.Finding{{Rule: "bpmn/test-rule", Severity: lint.SeverityWarning, Element: "task", Message: "test"}}
	prompt, truncated, err := BuildPrompt(document, findings, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Fatal("small prompt unexpectedly truncated")
	}
	for _, semantic := range []string{
		`"processes"`, `"id":"orders"`, `"jobType":"charge"`, `"retryCount":"5"`,
		`"defaultFlow":"default"`, `"condition":"= retry"`, `"timerKind":"duration"`,
		`"messageRef":"message"`, `"errorCode":"PAYMENT"`, `"rule":"bpmn/test-rule"`,
		`"local":"review-metadata"`, `"mode"`, `"content"`, `"level"`,
		"infinite loops", "missing compensation", "unreachable paths", "duplicate messages",
	} {
		if !strings.Contains(prompt, semantic) {
			t.Fatalf("prompt missing %q:\n%s", semantic, prompt)
		}
	}
	for _, secret := range []string{
		"must-not-leak", "outer-designated-secret", "nested-property-secret",
		"nested-element-secret", "nested-value-secret",
	} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("nested credential %q leaked:\n%s", secret, prompt)
		}
	}
	if !strings.Contains(prompt, "[redacted]") {
		t.Fatalf("credential-like extension value was not redacted:\n%s", prompt)
	}
}

func TestPromptRedactsSensitiveRootExtensionContentRecursively(t *testing.T) {
	document := bpmn.Document{UnknownExtensions: []bpmn.Extension{
		{
			QName: xml.Name{Local: "secret"},
			Attributes: []bpmn.Attribute{
				{QName: xml.Name{Local: "id"}, Value: "safe-id"},
				{QName: xml.Name{Local: "data"}, Value: "root-attribute-secret"},
			},
			InnerXML: `root-text-secret<safe id="safe-child" data="child-attribute-secret">child-text-secret</safe>`,
		},
		{
			QName: xml.Name{Local: "property"},
			Attributes: []bpmn.Attribute{
				{QName: xml.Name{Local: "name"}, Value: "password"},
				{QName: xml.Name{Local: "data"}, Value: "designated-root-secret"},
			},
			InnerXML: `designated-text-secret<group data="nested-attribute-secret">nested-text-secret</group>`,
		},
	}}
	prompt, _, err := BuildPrompt(document, nil, 64*1024)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{
		"root-attribute-secret", "root-text-secret", "child-attribute-secret", "child-text-secret",
		"designated-root-secret", "designated-text-secret", "nested-attribute-secret", "nested-text-secret",
	} {
		if strings.Contains(prompt, secret) {
			t.Fatalf("root-sensitive secret %q leaked:\n%s", secret, prompt)
		}
	}
	for _, safeIdentifier := range []string{"safe-id", "safe-child", `"name","value":"password"`} {
		if !strings.Contains(prompt, safeIdentifier) {
			t.Fatalf("safe structural identifier %q was lost:\n%s", safeIdentifier, prompt)
		}
	}
}

func TestPromptCompactionIsStructuredAndPreservesEveryFinding(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{ID: "large"}}}
	for i := 0; i < 100; i++ {
		document.Processes[0].Elements = append(document.Processes[0].Elements,
			bpmn.Element{ID: strings.Repeat("x", 100) + string(rune(i)), Type: "task"})
	}
	var findings []lint.Finding
	for i := 0; i < 12; i++ {
		findings = append(findings, lint.Finding{
			Rule: fmt.Sprintf("rule/%02d", i), Severity: lint.SeverityWarning,
			Element: fmt.Sprintf("element-%02d", i), Message: "must remain",
		})
	}
	first, firstCompacted, err := BuildPrompt(document, findings, 4096)
	if err != nil {
		t.Fatal(err)
	}
	second, secondCompacted, err := BuildPrompt(document, findings, 4096)
	if err != nil {
		t.Fatal(err)
	}
	if !firstCompacted || !secondCompacted || first != second || len([]byte(first)) > 4096 {
		t.Fatalf("bad compaction (%d bytes):\n%s", len([]byte(first)), first)
	}
	var envelope struct {
		Instructions []string       `json:"instructions"`
		Findings     []lint.Finding `json:"findings"`
		Model        struct {
			Omission *struct {
				Count  int    `json:"count"`
				SHA256 string `json:"sha256"`
			} `json:"omission"`
		} `json:"model"`
	}
	if err := json.Unmarshal([]byte(first), &envelope); err != nil {
		t.Fatalf("prompt is not valid structured JSON: %v\n%s", err, first)
	}
	if len(envelope.Findings) != len(findings) {
		t.Fatalf("findings compacted: got %d want %d", len(envelope.Findings), len(findings))
	}
	if envelope.Model.Omission == nil || envelope.Model.Omission.Count != 101 ||
		len(envelope.Model.Omission.SHA256) != 64 {
		t.Fatalf("omission metadata = %+v", envelope.Model.Omission)
	}
	for _, finding := range findings {
		if !strings.Contains(first, finding.Rule) {
			t.Fatalf("missing finding %s", finding.Rule)
		}
	}
}

func TestPromptCompactionRejectsIrreducibleFindingOverflow(t *testing.T) {
	_, _, err := BuildPrompt(bpmn.Document{}, []lint.Finding{{
		Rule: "rule/large", Severity: lint.SeverityError,
		Message: strings.Repeat("finding detail ", 100),
	}}, 256)
	if err == nil || !strings.Contains(err.Error(), "findings") {
		t.Fatalf("irreducible prompt error = %v", err)
	}
}

func TestReviewAIStatesOptionalAndRequiredFailure(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}}}}}

	offline, err := RunContext(context.Background(), document, Options{})
	if err != nil || offline.AIStatus != AIStatusDisabled {
		t.Fatalf("offline=%+v err=%v", offline, err)
	}
	skipped, err := RunContext(context.Background(), document, Options{AI: true})
	if err != nil || skipped.AIStatus != AIStatusSkipped || len(skipped.Warnings) != 1 || len(skipped.Findings) == 0 {
		t.Fatalf("skipped=%+v err=%v", skipped, err)
	}

	providerErr := errors.New("provider unavailable")
	optional, err := RunContext(context.Background(), document, Options{
		AI: true, AIClient: &recordingClient{err: providerErr},
	})
	if err != nil || optional.AIStatus != AIStatusFailed || len(optional.Warnings) != 1 || len(optional.Findings) == 0 {
		t.Fatalf("optional=%+v err=%v", optional, err)
	}

	requiredResult, err := RunContext(context.Background(), document, Options{
		AI: true, AIRequired: true, AIClient: &recordingClient{err: providerErr},
	})
	var required *AIError
	if !errors.As(err, &required) || !errors.Is(err, providerErr) {
		t.Fatalf("required error=%#v", err)
	}
	if len(requiredResult.Findings) == 0 || requiredResult.AIStatus != AIStatusFailed {
		t.Fatalf("required result lost offline findings: %+v", requiredResult)
	}

	empty, err := RunContext(context.Background(), document, Options{
		AI: true, AIClient: &recordingClient{},
	})
	if err != nil || empty.AIStatus != AIStatusFailed || len(empty.Warnings) != 1 {
		t.Fatalf("empty response was silently successful: result=%+v err=%v", empty, err)
	}
}

func TestReviewDoesNotRenderUntrustedClientErrors(t *testing.T) {
	const secret = "provider-body-nested-secret"
	clientErr := errors.New(`provider failed: {"error":{"apiKey":"` + secret + `"}}`)
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}},
	}}}
	optional, err := RunContext(context.Background(), document, Options{
		AI: true, AIClient: &recordingClient{err: clientErr},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(optional.Warnings) != 1 || strings.Contains(optional.Warnings[0].Message, secret) {
		t.Fatalf("optional warning leaked provider body: %+v", optional.Warnings)
	}
	_, err = RunContext(context.Background(), document, Options{
		AI: true, AIRequired: true, AIClient: &recordingClient{err: clientErr},
	})
	if err == nil || strings.Contains(err.Error(), secret) || !errors.Is(err, clientErr) {
		t.Fatalf("required error leak/wrapping = %v", err)
	}
}

func TestReviewExposesActionableSanitizedAIErrors(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}},
	}}}
	_, unknownProviderErr := ai.NewChatClient(ai.ClientConfig{
		Provider: "mystery", Model: "test", APIKey: "not-rendered",
	})
	tests := []struct {
		name        string
		err         error
		wantCode    string
		wantDetails []string
	}{
		{
			name:     "unknown provider",
			err:      unknownProviderErr,
			wantCode: "ai_configuration_invalid", wantDetails: []string{"mystery", "openai", "anthropic"},
		},
		{
			name: "unauthorized", err: &ai.ProviderError{
				StatusCode: http.StatusUnauthorized,
				Err:        errors.New(`{"apiKey":"provider-body-secret"}`),
			},
			wantCode: "ai_http_401", wantDetails: []string{"401", "credentials"},
		},
		{
			name: "rate limited", err: &ai.ProviderError{
				StatusCode: http.StatusTooManyRequests,
				Err:        errors.New(`prompt=extension-secret`),
			},
			wantCode: "ai_http_429", wantDetails: []string{"429", "retry"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			optional, err := RunContext(context.Background(), document, Options{
				AI: true, AIClient: &recordingClient{err: test.err},
			})
			if err != nil || len(optional.Warnings) != 1 {
				t.Fatalf("optional=%+v err=%v", optional, err)
			}
			warning := optional.Warnings[0]
			if warning.Code != test.wantCode {
				t.Fatalf("warning code=%q want=%q", warning.Code, test.wantCode)
			}
			for _, detail := range test.wantDetails {
				if !strings.Contains(strings.ToLower(warning.Message), strings.ToLower(detail)) {
					t.Fatalf("warning missing %q: %+v", detail, warning)
				}
			}
			requiredResult, err := RunContext(context.Background(), document, Options{
				AI: true, AIRequired: true, AIClient: &recordingClient{err: test.err},
			})
			var aiErr *AIError
			if !errors.As(err, &aiErr) || !errors.Is(err, test.err) ||
				aiErr.Code != test.wantCode || aiErr.Message != warning.Message ||
				requiredResult.AIStatus != AIStatusFailed {
				t.Fatalf("required result=%+v aiErr=%+v err=%v", requiredResult, aiErr, err)
			}
			for _, forbidden := range []string{"not-rendered", "provider-body-secret", "extension-secret", `"apiKey"`, "prompt="} {
				if strings.Contains(warning.Message, forbidden) || strings.Contains(err.Error(), forbidden) {
					t.Fatalf("secret leaked through public AI detail: warning=%+v err=%v", warning, err)
				}
			}
		})
	}
}

func TestReviewMissingClientIncludesSafeRemediation(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}},
	}}}
	optional, err := RunContext(context.Background(), document, Options{AI: true})
	if err != nil || len(optional.Warnings) != 1 {
		t.Fatalf("optional=%+v err=%v", optional, err)
	}
	warning := optional.Warnings[0]
	if warning.Code != "ai_unavailable" ||
		!strings.Contains(strings.ToLower(warning.Message), "provider") ||
		!strings.Contains(strings.ToLower(warning.Message), "credentials") {
		t.Fatalf("missing-client warning is not actionable: %+v", warning)
	}
	_, err = RunContext(context.Background(), document, Options{AIRequired: true})
	var aiErr *AIError
	if !errors.As(err, &aiErr) || aiErr.Code != warning.Code ||
		aiErr.Message != warning.Message || !strings.Contains(err.Error(), warning.Message) {
		t.Fatalf("missing-client required error = %+v / %v", aiErr, err)
	}
}

func TestReviewTransportFailureKeepsCauseBehindSafeDetail(t *testing.T) {
	const secret = "review-transport-secret"
	cause := &reviewTransportError{secret: secret, cause: syscall.ECONNRESET}
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", APIKey: "not-rendered",
		BaseURL: "http://provider.invalid/v1",
		HTTPClient: &http.Client{Transport: reviewRoundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, cause
		})},
	})
	if err != nil {
		t.Fatal(err)
	}
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}},
	}}}
	optional, err := RunContext(context.Background(), document, Options{AI: true, AIClient: client})
	if err != nil || len(optional.Warnings) != 1 ||
		optional.Warnings[0].Code != "ai_transport_error" ||
		!strings.Contains(strings.ToLower(optional.Warnings[0].Message), "transport") {
		t.Fatalf("optional=%+v err=%v", optional, err)
	}
	_, err = RunContext(context.Background(), document, Options{
		AI: true, AIRequired: true, AIClient: client,
	})
	var aiErr *AIError
	var providerErr *ai.ProviderError
	var urlErr *url.Error
	var netErr net.Error
	var original *reviewTransportError
	if !errors.As(err, &aiErr) || aiErr.Code != "ai_transport_error" ||
		!errors.As(err, &providerErr) || !errors.As(err, &urlErr) ||
		!errors.As(err, &netErr) || !errors.As(err, &original) || original != cause ||
		!errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("review transport chain was not preserved: %#v", err)
	}
	for _, forbidden := range []string{secret, "provider.invalid", "not-rendered"} {
		if strings.Contains(optional.Warnings[0].Message, forbidden) || strings.Contains(err.Error(), forbidden) {
			t.Fatalf("review transport detail leaked: warning=%+v err=%v", optional.Warnings[0], err)
		}
	}
}

type reviewRoundTripFunc func(*http.Request) (*http.Response, error)

func (f reviewRoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type reviewTransportError struct {
	secret string
	cause  error
}

func (e *reviewTransportError) Error() string { return e.secret }
func (e *reviewTransportError) Unwrap() error { return e.cause }
func (e *reviewTransportError) Timeout() bool { return false }
func (e *reviewTransportError) Temporary() bool {
	return true
}

func TestReviewPassesStablePromptAndContextToChatClient(t *testing.T) {
	client := &recordingClient{response: ai.ChatResponse{Content: "AI result"}}
	document := bpmn.Document{Processes: []bpmn.Process{{ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}}}}}
	result, err := RunContext(context.Background(), document, Options{AI: true, AIClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if result.AIStatus != AIStatusSucceeded || result.AIText != "AI result" || len(client.requests) != 1 {
		t.Fatalf("result=%+v requests=%+v", result, client.requests)
	}
	if client.requests[0].Purpose != "review" || client.requests[0].Prompt == "" {
		t.Fatalf("request=%+v", client.requests[0])
	}
}

type recordingClient struct {
	requests []ai.ChatRequest
	response ai.ChatResponse
	err      error
}

func (c *recordingClient) Complete(_ context.Context, request ai.ChatRequest) (ai.ChatResponse, error) {
	c.requests = append(c.requests, request)
	return c.response, c.err
}
