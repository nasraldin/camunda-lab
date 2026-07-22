package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestAuthorizedInputsStayInsideDeclaredProject(t *testing.T) {
	parent := t.TempDir()
	projectDir := filepath.Join(parent, "project")
	outsideDir := filepath.Join(parent, "project-evil")
	nested := filepath.Join(projectDir, "bpmn", "nested")
	for _, dir := range []string{nested, outsideDir} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	inside := filepath.Join(nested, "inside.bpmn")
	outside := filepath.Join(outsideDir, "outside.bpmn")
	for _, path := range []string{inside, outside} {
		if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	link := filepath.Join(projectDir, "bpmn", "escape.bpmn")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	valid := []string{"bpmn/nested/inside.bpmn", inside}
	canonicalInside, err := filepath.EvalSymlinks(inside)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range valid {
		inputs, root, err := authorizedInputs([]string{path}, projectDir)
		if err != nil {
			t.Fatalf("valid path %q: %v", path, err)
		}
		if root == "" || len(inputs) != 1 || inputs[0].Path != canonicalInside {
			t.Fatalf("path %q resolved to root=%q inputs=%+v", path, root, inputs)
		}
	}

	for name, path := range map[string]string{
		"traversal":        filepath.Join("..", "project-evil", "outside.bpmn"),
		"prefix collision": outside,
		"symlink escape":   link,
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := authorizedInputs([]string{path}, projectDir); err == nil {
				t.Fatalf("accepted %q outside %q", path, projectDir)
			}
		})
	}
}

func TestDeveloperMultipartRejectsAggregateAndFileCountLimits(t *testing.T) {
	t.Run("aggregate", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for _, name := range []string{"first.bpmn", "second.bpmn"} {
			part, err := writer.CreateFormFile("files", name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write(bytes.Repeat([]byte("x"), maxUploadBytes)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		response := runDeveloperMultipart(t, "/api/v1/bpmn/lint", &body, writer.FormDataContentType())
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		assertErrorEnvelope(t, response, http.StatusRequestEntityTooLarge, "payload_too_large")
	})

	t.Run("count", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		for index := 0; index <= maxDeveloperUploadFiles; index++ {
			part, err := writer.CreateFormFile("files", "process.bpmn")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := part.Write([]byte(validDeveloperBPMN)); err != nil {
				t.Fatal(err)
			}
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		response := runDeveloperMultipart(t, "/api/v1/bpmn/lint", &body, writer.FormDataContentType())
		if response.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("per file", func(t *testing.T) {
		var body bytes.Buffer
		writer := multipart.NewWriter(&body)
		part, err := writer.CreateFormFile("file", "large.bpmn")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write(bytes.Repeat([]byte("x"), maxUploadBytes+1)); err != nil {
			t.Fatal(err)
		}
		if err := writer.Close(); err != nil {
			t.Fatal(err)
		}
		response := runDeveloperMultipart(t, "/api/v1/bpmn/lint", &body, writer.FormDataContentType())
		if response.Code != http.StatusRequestEntityTooLarge {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
		assertErrorEnvelope(t, response, http.StatusRequestEntityTooLarge, "payload_too_large")
	})
}

func TestDeveloperMultipartRemovesTemporaryFiles(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv("TMPDIR", tempDir)
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "process.bpmn")
	if err != nil {
		t.Fatal(err)
	}
	padding := strings.Repeat(" ", developerMultipartMemoryBytes+1)
	if _, err := part.Write([]byte(validDeveloperBPMN + padding)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	_ = runDeveloperMultipart(t, "/api/v1/bpmn/lint", &body, writer.FormDataContentType())
	entries, err := os.ReadDir(tempDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("multipart temp files leaked: %v", entries)
	}
}

func TestDeveloperMultipartOptionalBooleansAreStrict(t *testing.T) {
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	tests := []struct {
		name     string
		endpoint string
		field    string
		values   []string
	}{
		{name: "ai malformed", endpoint: "/api/v1/bpmn/review", field: "ai", values: []string{"TRUE"}},
		{name: "ai duplicate", endpoint: "/api/v1/bpmn/review", field: "ai", values: []string{"true", "false"}},
		{name: "required malformed", endpoint: "/api/v1/bpmn/review", field: "required", values: []string{"1"}},
		{name: "required duplicate", endpoint: "/api/v1/bpmn/review", field: "required", values: []string{"false", "false"}},
		{name: "write malformed", endpoint: "/api/v1/bpmn/test-generate", field: "write", values: []string{""}},
		{name: "write duplicate", endpoint: "/api/v1/bpmn/test-generate", field: "write", values: []string{"true", "false"}},
		{name: "force malformed", endpoint: "/api/v1/bpmn/test-generate", field: "force", values: []string{"yes"}},
		{name: "force duplicate", endpoint: "/api/v1/bpmn/test-generate", field: "force", values: []string{"false", "true"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runDeveloperMultipartFields(t, test.endpoint, map[string][]string{
				test.field: test.values,
			})
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			var body developerErrorResponse
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if body.Code != "invalid_request" {
				t.Fatalf("code = %q, body = %s", body.Code, response.Body.String())
			}
		})
	}
	t.Run("required aliases conflict", func(t *testing.T) {
		response := runDeveloperMultipartFields(t, "/api/v1/bpmn/review", map[string][]string{
			"required": {"false"}, "aiRequired": {"true"},
		})
		if response.Code != http.StatusBadRequest ||
			!strings.Contains(response.Body.String(), `"code":"invalid_request"`) {
			t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
		}
	})
}

func TestDeveloperMultipartOptionalBooleanExactValuesDoNotFallback(t *testing.T) {
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	tests := []struct {
		name     string
		endpoint string
		fields   map[string][]string
		status   int
	}{
		{
			name: "review false values stay offline", endpoint: "/api/v1/bpmn/review",
			fields: map[string][]string{"ai": {"false"}, "required": {"false"}},
			status: http.StatusOK,
		},
		{
			name: "review true enables optional partial", endpoint: "/api/v1/bpmn/review",
			fields: map[string][]string{
				"ai": {"true"}, "aiRequired": {"false"}, "provider": {"openai"}, "model": {"model"},
			},
			status: http.StatusOK,
		},
		{
			name: "review exact required true is enforced", endpoint: "/api/v1/bpmn/review",
			fields: map[string][]string{
				"ai": {"false"}, "required": {"true"}, "provider": {"openai"}, "model": {"model"},
			},
			status: http.StatusBadRequest,
		},
		{
			name: "generate false remains download", endpoint: "/api/v1/bpmn/test-generate",
			fields: map[string][]string{"write": {"false"}, "force": {"false"}},
			status: http.StatusOK,
		},
		{
			name: "generate exact force true accepted", endpoint: "/api/v1/bpmn/test-generate",
			fields: map[string][]string{"write": {"false"}, "force": {"true"}},
			status: http.StatusOK,
		},
		{
			name: "generate exact write true accepted", endpoint: "/api/v1/bpmn/test-generate",
			fields: map[string][]string{
				"write": {"true"}, "force": {"false"}, "output": {filepath.Join(t.TempDir(), "generated")},
			},
			status: http.StatusOK,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runDeveloperMultipartFields(t, test.endpoint, test.fields)
			if response.Code != test.status {
				t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
			}
			if test.name == "review true enables optional partial" &&
				!strings.Contains(response.Body.String(), `"status":"partial"`) {
				t.Fatalf("AI true fell back instead of partial: %s", response.Body.String())
			}
			if test.name == "review exact required true is enforced" &&
				!strings.Contains(response.Body.String(), `"code":"ai_configuration_invalid"`) {
				t.Fatalf("required=true fell back instead of failing: %s", response.Body.String())
			}
			if strings.Contains(test.name, "generate") &&
				test.name != "generate exact write true accepted" &&
				!strings.Contains(response.Body.String(), `"mode":"download"`) {
				t.Fatalf("write=false did not remain download: %s", response.Body.String())
			}
			if test.name == "generate exact write true accepted" &&
				!strings.Contains(response.Body.String(), `"mode":"written"`) {
				t.Fatalf("write=true fell back instead of writing: %s", response.Body.String())
			}
		})
	}
}

func runDeveloperMultipart(
	t *testing.T,
	path string,
	body *bytes.Buffer,
	contentType string,
) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, body)
	request.Header.Set("Content-Type", contentType)
	response := httptest.NewRecorder()
	(&handler{}).bpmnLint(response, request)
	return response
}

func runDeveloperMultipartFields(
	t *testing.T,
	endpoint string,
	fields map[string][]string,
) *httptest.ResponseRecorder {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", "process.bpmn")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := part.Write([]byte(validDeveloperBPMN)); err != nil {
		t.Fatal(err)
	}
	for name, values := range fields {
		for _, value := range values {
			if err := writer.WriteField(name, value); err != nil {
				t.Fatal(err)
			}
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, endpoint, &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	switch endpoint {
	case "/api/v1/bpmn/review":
		(&handler{}).bpmnReview(response, request)
	case "/api/v1/bpmn/test-generate":
		(&handler{}).bpmnTestGenerate(response, request)
	default:
		t.Fatalf("unsupported multipart endpoint %q", endpoint)
	}
	return response
}

const validDeveloperBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`
