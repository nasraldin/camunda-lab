package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestDeveloperAPIRejectsUnknownFields(t *testing.T) {
	for _, endpoint := range []string{
		"/api/v1/bpmn/lint",
		"/api/v1/bpmn/diff",
		"/api/v1/bpmn/explain",
		"/api/v1/bpmn/review",
		"/api/v1/bpmn/test-generate",
		"/api/v1/bpmn/scan",
	} {
		response := runDeveloperJSON(t, endpoint, `{"surprise":true}`)
		if response.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, body = %s", endpoint, response.Code, response.Body.String())
			continue
		}
		var body struct {
			Code string `json:"code"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Code != "invalid_request" {
			t.Errorf("%s code = %q", endpoint, body.Code)
		}
	}
}

func TestDeveloperAPIUsesStableResultEnvelope(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	content := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	Register(mux, "test", "token")
	request := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/lint",
		bytes.NewBufferString(`{"paths":[`+mustJSON(t, path)+`],"failOn":"error","ignore":[]}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		OK       bool            `json:"ok"`
		Status   string          `json:"status"`
		Complete bool            `json:"complete"`
		Findings json.RawMessage `json:"findings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Status != "completed" || !body.Complete || body.Findings == nil {
		t.Fatalf("body = %s", response.Body.String())
	}
}

func TestDeveloperDiffRequiresExactlyOneCompleteMode(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "first.bpmn")
	second := filepath.Join(root, "second.bpmn")
	for _, path := range []string{first, second} {
		if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	valid := []string{
		`{"paths":[` + mustJSON(t, first) + `,` + mustJSON(t, second) + `]}`,
		`{"from":` + mustJSON(t, first) + `,"to":` + mustJSON(t, second) + `}`,
		`{"path":` + mustJSON(t, first) + `,"against":` + mustJSON(t, second) + `}`,
	}
	for _, body := range valid {
		response := runDeveloperJSON(t, "/api/v1/bpmn/diff", body)
		if response.Code != http.StatusOK {
			t.Errorf("valid body %s status = %d: %s", body, response.Code, response.Body.String())
		}
	}
	invalid := []string{
		`{}`,
		`{"from":` + mustJSON(t, first) + `}`,
		`{"path":` + mustJSON(t, first) + `}`,
		`{"paths":[` + mustJSON(t, first) + `]}`,
		`{"paths":[` + mustJSON(t, first) + `,` + mustJSON(t, second) + `],"from":` + mustJSON(t, first) + `,"to":` + mustJSON(t, second) + `}`,
		`{"path":` + mustJSON(t, first) + `,"against":` + mustJSON(t, second) + `,"base":"HEAD","projectDir":` + mustJSON(t, root) + `}`,
	}
	for _, body := range invalid {
		response := runDeveloperJSON(t, "/api/v1/bpmn/diff", body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("invalid body %s status = %d: %s", body, response.Code, response.Body.String())
		}
	}
	request, service, err := buildDiffRequest(developerDiffRequest{
		Path: "first.bpmn", Base: "HEAD", ProjectDir: root,
	})
	if err != nil || request.BeforeGit == nil || service.Git == nil {
		t.Fatalf("valid Git mode request=%+v service=%+v error=%v", request, service, err)
	}
}

func TestDeveloperReviewRejectsAmbiguousOrInvalidAIControls(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []string{
		`{"path":` + mustJSON(t, path) + `,"provider":"anthropic"}`,
		`{"path":` + mustJSON(t, path) + `,"model":"custom"}`,
		`{"path":` + mustJSON(t, path) + `,"ai":true,"provider":"unknown","model":"model"}`,
		`{"path":` + mustJSON(t, path) + `,"ai":true,"provider":"openai","model":" "}`,
	}
	for _, body := range tests {
		response := runDeveloperJSON(t, "/api/v1/bpmn/review", body)
		if response.Code != http.StatusBadRequest {
			t.Errorf("body %s status = %d: %s", body, response.Code, response.Body.String())
		}
	}
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := ai.WriteSecrets(ai.Secrets{
		OpenAIKey: "test-key", OpenAIBaseURL: "://invalid",
	}); err != nil {
		t.Fatal(err)
	}
	response := runDeveloperJSON(t, "/api/v1/bpmn/review",
		`{"path":`+mustJSON(t, path)+`,"ai":true,"provider":"openai","model":"model"}`)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid endpoint status = %d: %s", response.Code, response.Body.String())
	}
}

func TestDeveloperReviewAllowsOnlyMissingOptionalCredentialsAsPartial(t *testing.T) {
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	response := runDeveloperJSON(t, "/api/v1/bpmn/review",
		`{"path":`+mustJSON(t, path)+`,"ai":true,"provider":"openai","model":"model"}`)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var result struct {
		Status   string `json:"status"`
		Complete bool   `json:"complete"`
		AIStatus string `json:"aiStatus"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.Complete || result.AIStatus != "skipped" {
		t.Fatalf("result = %+v: %s", result, response.Body.String())
	}
}

func TestDeveloperAPIOperationsReturnStableStatuses(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		endpoint string
		body     string
	}{
		{endpoint: "/api/v1/bpmn/lint", body: `{"path":` + mustJSON(t, path) + `}`},
		{endpoint: "/api/v1/bpmn/diff", body: `{"paths":[` + mustJSON(t, path) + `,` + mustJSON(t, path) + `]}`},
		{endpoint: "/api/v1/bpmn/explain", body: `{"path":` + mustJSON(t, path) + `,"format":"json"}`},
		{endpoint: "/api/v1/bpmn/review", body: `{"path":` + mustJSON(t, path) + `}`},
		{endpoint: "/api/v1/bpmn/test-generate", body: `{"path":` + mustJSON(t, path) + `,"lang":"python"}`},
		{endpoint: "/api/v1/bpmn/scan", body: `{"dir":` + mustJSON(t, root) + `}`},
	}
	for _, test := range cases {
		response := runDeveloperJSON(t, test.endpoint, test.body)
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d: %s", test.endpoint, response.Code, response.Body.String())
			continue
		}
		var envelope struct {
			Status   string `json:"status"`
			Complete bool   `json:"complete"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != "completed" || !envelope.Complete {
			t.Errorf("%s envelope = %+v: %s", test.endpoint, envelope, response.Body.String())
		}
	}
}

func TestDeveloperAPIPolicyOutcomesRemainHTTP200WithFailedStatus(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	changed := filepath.Join(root, "changed.bpmn")
	policy := filepath.Join(root, "policy.bpmn")
	if err := os.WriteFile(valid, []byte(validDeveloperBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte(strings.Replace(validDeveloperBPMN, `id="end"`, `id="end" name="changed"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><serviceTask id="task"/></process></definitions>`), 0o600); err != nil {
		t.Fatal(err)
	}
	scanRoot, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "scan", "project"))
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		endpoint string
		body     string
	}{
		{endpoint: "/api/v1/bpmn/lint", body: `{"path":` + mustJSON(t, policy) + `,"failOn":"warning"}`},
		{endpoint: "/api/v1/bpmn/diff", body: `{"paths":[` + mustJSON(t, valid) + `,` + mustJSON(t, changed) + `]}`},
		{endpoint: "/api/v1/bpmn/review", body: `{"path":` + mustJSON(t, policy) + `,"failOn":"warning"}`},
		{endpoint: "/api/v1/bpmn/scan", body: `{"dir":` + mustJSON(t, scanRoot) + `,"failOn":"low"}`},
	}
	for _, test := range cases {
		response := runDeveloperJSON(t, test.endpoint, test.body)
		if response.Code != http.StatusOK {
			t.Errorf("%s status = %d: %s", test.endpoint, response.Code, response.Body.String())
			continue
		}
		var envelope struct {
			Status string `json:"status"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &envelope); err != nil {
			t.Fatal(err)
		}
		if envelope.Status != "failed" {
			t.Errorf("%s status = %q: %s", test.endpoint, envelope.Status, response.Body.String())
		}
	}
}

func TestDeveloperDoctorDeepReturnsStableFatalContract(t *testing.T) {
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	cfg := config.Defaults()
	cfg.Version = "invalid"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	response := httptest.NewRecorder()
	(&handler{}).runDoctorDeep(response, httptest.NewRequest(http.MethodPost, "/api/v1/doctor/deep", nil))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d: %s", response.Code, response.Body.String())
	}
	var body developerErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Code != "invalid_configuration" || body.Error == "" {
		t.Fatalf("body = %+v", body)
	}
}

func runDeveloperJSON(t *testing.T, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	mux := http.NewServeMux()
	Register(mux, "test", "token")
	mux.ServeHTTP(response, request)
	return response
}

func mustJSON(t *testing.T, value string) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
