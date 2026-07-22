package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGenerateJSONRejectsWrongTypesUnknownFieldsAndInvalidValues(t *testing.T) {
	path := writeGenerateBPMN(t)
	tests := []struct {
		name string
		body string
	}{
		{name: "wrong language type", body: fmt.Sprintf(`{"path":%q,"lang":true}`, path)},
		{name: "wrong force type", body: fmt.Sprintf(`{"path":%q,"force":"yes"}`, path)},
		{name: "unknown field", body: fmt.Sprintf(`{"path":%q,"unknown":true}`, path)},
		{name: "invalid language", body: fmt.Sprintf(`{"path":%q,"lang":"ruby"}`, path)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/test-generate", strings.NewReader(test.body))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			(&handler{}).bpmnTestGenerate(response, request)
			if response.Code < 400 || response.Code >= 500 {
				t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
			}
			var body struct {
				Code string `json:"code"`
			}
			if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Code != "invalid_request" {
				t.Fatalf("code = %q; body = %s", body.Code, response.Body.String())
			}
		})
	}
}

func TestGenerateJSONReturnsDownloadWithoutWritingArtifacts(t *testing.T) {
	path := writeGenerateBPMN(t)
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/bpmn/test-generate",
		strings.NewReader(fmt.Sprintf(`{"path":%q,"lang":"python"}`, path)),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&handler{}).bpmnTestGenerate(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Paths    []string          `json:"paths"`
		Contents map[string]string `json:"contents"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Paths) != 1 || filepath.IsAbs(body.Paths[0]) || body.Contents[body.Paths[0]] == "" {
		t.Fatalf("response = %+v", body)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), body.Paths[0])); !os.IsNotExist(err) {
		t.Fatalf("download mode wrote artifact: %v", err)
	}
}

func TestGenerateJSONDownloadWithConfiguredProjectTestsPathDoesNotWrite(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(root, ".camunda.yaml"),
		[]byte("name: api-purity\npaths:\n  bpmn: bpmn\n  tests: configured-tests\n"),
		0o600,
	); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "process.bpmn")
	source := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="one"><startEvent id="start"/></process></definitions>`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(
		http.MethodPost,
		"/api/v1/bpmn/test-generate",
		strings.NewReader(fmt.Sprintf(`{"path":%q,"projectDir":%q,"lang":"js","write":false}`, path, root)),
	)
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()

	(&handler{}).bpmnTestGenerate(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
	if _, err := os.Stat(filepath.Join(root, "configured-tests")); !os.IsNotExist(err) {
		t.Fatalf("download mode wrote configured tests output: %v", err)
	}
}

func TestGenerateMultipartRemainsSupported(t *testing.T) {
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	file, err := writer.CreateFormFile("file", "process.bpmn")
	if err != nil {
		t.Fatal(err)
	}
	source := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="one"><startEvent id="start"/></process></definitions>`
	if _, err := file.Write([]byte(source)); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("lang", "js"); err != nil {
		t.Fatal(err)
	}
	if err := writer.WriteField("force", "false"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/test-generate", &body)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	response := httptest.NewRecorder()
	(&handler{}).bpmnTestGenerate(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "js/One.spec.js") {
		t.Fatalf("status = %d; body = %s", response.Code, response.Body.String())
	}
}

func writeGenerateBPMN(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "process.bpmn")
	source := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="one"><startEvent id="start"/></process></definitions>`
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
