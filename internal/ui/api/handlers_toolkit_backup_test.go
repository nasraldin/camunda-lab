package api

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestBackupRestoreAPIMultipartRoundTrip(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)

	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")
	writeAPIFile(t, filepath.Join(home, "ai.env"), "SECRET_OPENAI_API_KEY=sk-api-roundtrip\n")
	writeAPIFile(t, filepath.Join(project, ".camunda.yaml"), `name: api-roundtrip
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: dmn
  forms: forms
  tests: tests
`)
	writeAPIFile(t, filepath.Join(project, "models", "nested", "order.bpmn"), "<api/>")

	archive := filepath.Join(t.TempDir(), "api-roundtrip.tar.gz")
	h := &handler{runningLab: runningCheckerFunc(func(context.Context) (bool, error) {
		return false, nil
	})}
	backupBody, _ := json.Marshal(map[string]any{
		"output": archive,
		"dir":    project,
	})
	backupReq := httptest.NewRequest(http.MethodPost, "/api/v1/backup", bytes.NewReader(backupBody))
	backupRec := httptest.NewRecorder()
	h.runBackup(backupRec, backupReq)
	if backupRec.Code != http.StatusOK {
		t.Fatalf("backup status=%d body=%s", backupRec.Code, backupRec.Body.String())
	}
	var backupResp struct {
		OK              bool `json:"ok"`
		IncludesSecrets bool `json:"includesSecrets"`
		Files           int  `json:"files"`
	}
	if err := json.Unmarshal(backupRec.Body.Bytes(), &backupResp); err != nil {
		t.Fatal(err)
	}
	if !backupResp.OK || backupResp.IncludesSecrets {
		t.Fatalf("backup response = %#v", backupResp)
	}
	if strings.Contains(backupRec.Body.String(), `"path"`) {
		t.Fatalf("backup JSON leaked path field: %s", backupRec.Body.String())
	}
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("archive mode = %o, want 600", got)
	}

	lab2 := t.TempDir()
	proj2 := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", lab2)
	paths.Reset()

	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	_ = writer.WriteField("yes", "true")
	_ = writer.WriteField("dir", proj2)
	part, err := writer.CreateFormFile("archive", "api-roundtrip.tar.gz")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.Open(archive)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(part, src); err != nil {
		_ = src.Close()
		t.Fatal(err)
	}
	_ = src.Close()
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/restore", &form)
	restoreReq.Header.Set("Content-Type", writer.FormDataContentType())
	restoreRec := httptest.NewRecorder()
	h.runRestore(restoreRec, restoreReq)
	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore status=%d body=%s", restoreRec.Code, restoreRec.Body.String())
	}
	if got := readAPIFile(t, filepath.Join(lab2, "config.yaml")); !strings.Contains(got, "8.9") {
		t.Fatalf("restored config = %q", got)
	}
	if _, err := os.Stat(filepath.Join(lab2, "ai.env")); !os.IsNotExist(err) {
		t.Fatalf("ai.env restored without includeSecrets: %v", err)
	}
	if got := readAPIFile(t, filepath.Join(proj2, ".camunda.yaml")); !strings.Contains(got, "api-roundtrip") {
		t.Fatalf(".camunda.yaml = %q", got)
	}
	if got := readAPIFile(t, filepath.Join(proj2, "models", "nested", "order.bpmn")); got != "<api/>" {
		t.Fatalf("restored bpmn = %q", got)
	}
}

func writeAPIFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func readAPIFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
