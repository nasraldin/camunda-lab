package api

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
)

func TestTestGenerateDownloadStreamsZIPWithSecureHeaders(t *testing.T) {
	path := writeGenerateBPMN(t)
	body, _ := json.Marshal(map[string]any{"path": path, "lang": "js"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/test-generate/download", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	NewHandler("test", Dependencies{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := rec.Body.Bytes()
	if len(payload) < 4 || !bytes.Equal(payload[:4], []byte("PK\x03\x04")) {
		t.Fatalf("missing ZIP signature: %x", payload[:min(8, len(payload))])
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type=%q", ct)
	}
	assertSecureAttachment(t, rec, ".zip")
	if strings.Contains(rec.Header().Get("Content-Disposition"), path) ||
		strings.Contains(rec.Body.String(), path) ||
		strings.Contains(rec.Body.String(), os.TempDir()) {
		t.Fatal("download leaked absolute path")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(path), "js")); !os.IsNotExist(err) {
		t.Fatalf("download route wrote artifacts: %v", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(payload), int64(len(payload)))
	if err != nil {
		t.Fatal(err)
	}
	if len(reader.File) == 0 {
		t.Fatal("empty ZIP")
	}
	for _, file := range reader.File {
		if filepath.IsAbs(file.Name) || strings.Contains(file.Name, "..") {
			t.Fatalf("unsafe ZIP entry %q", file.Name)
		}
		if got := file.Mode().Perm(); got != 0o600 {
			t.Fatalf("%s mode=%o", file.Name, got)
		}
	}
}

func TestTestGenerateDownloadRejectsTraversalProjectDir(t *testing.T) {
	path := writeGenerateBPMN(t)
	body, _ := json.Marshal(map[string]any{
		"path": path, "lang": "python", "projectDir": "/etc",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/test-generate/download", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewHandler("test", Dependencies{}).ServeHTTP(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status=%d body=%s, want rejection", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "/etc") || strings.Contains(rec.Body.String(), os.TempDir()) {
		t.Fatalf("error leaked path: %s", rec.Body.String())
	}
}

func TestTestGenerateDownloadIgnoresWriteFields(t *testing.T) {
	path := writeGenerateBPMN(t)
	out := filepath.Join(t.TempDir(), "should-not-write")
	body, _ := json.Marshal(map[string]any{
		"path": path, "lang": "java", "write": true, "output": out, "force": true,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/test-generate/download", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	NewHandler("test", Dependencies{}).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("download route honored write/output: %v", err)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/zip" {
		t.Fatalf("Content-Type=%q, want zip stream not JSON write", ct)
	}
}

func TestBackupDownloadStreamsGzipSignatureAndHeaders(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")
	writeAPIFile(t, filepath.Join(home, "ai.env"), "SECRET_OPENAI_API_KEY=sk-download-omit\n")
	writeAPIFile(t, filepath.Join(project, ".camunda.yaml"), `name: dl
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: dmn
  forms: forms
`)
	writeAPIFile(t, filepath.Join(project, "models", "a.bpmn"), "<a/>")

	beforeTemps := listMatchingTemps(t, "camunda-lab-backup-dl-")

	body, _ := json.Marshal(map[string]any{"dir": project})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	(&handler{}).runBackupDownload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	payload := rec.Body.Bytes()
	if len(payload) < 2 || payload[0] != 0x1f || payload[1] != 0x8b {
		t.Fatalf("missing gzip signature: %x", payload[:min(4, len(payload))])
	}
	if !strings.Contains(rec.Header().Get("Content-Type"), "gzip") {
		t.Fatalf("Content-Type=%q", rec.Header().Get("Content-Type"))
	}
	assertSecureAttachment(t, rec, ".tar.gz")
	if rec.Header().Get("X-Camunda-Lab-Backup-Secrets") != "false" {
		t.Fatalf("secrets header=%q, want false without opt-in", rec.Header().Get("X-Camunda-Lab-Backup-Secrets"))
	}
	for _, needle := range []string{os.TempDir(), home, project} {
		if strings.Contains(string(payload), needle) || strings.Contains(rec.Header().Get("Content-Disposition"), needle) {
			t.Fatalf("response leaked path %q", needle)
		}
	}
	afterTemps := listMatchingTemps(t, "camunda-lab-backup-dl-")
	if len(afterTemps) > len(beforeTemps) {
		t.Fatalf("backup download left temp files: before=%v after=%v", beforeTemps, afterTemps)
	}

	names := gzipTarNames(t, payload)
	for _, name := range names {
		if name == "ai.env" {
			t.Fatal("ai.env included without includeSecrets")
		}
		if filepath.IsAbs(name) || strings.Contains(name, "..") {
			t.Fatalf("unsafe archive entry %q", name)
		}
	}
}

func TestBackupDownloadSecretsOptIn(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")
	writeAPIFile(t, filepath.Join(home, "ai.env"), "SECRET_OPENAI_API_KEY=sk-download-include\n")

	body, _ := json.Marshal(map[string]any{"includeSecrets": true})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	(&handler{}).runBackupDownload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Camunda-Lab-Backup-Secrets") != "true" {
		t.Fatalf("secrets header=%q", rec.Header().Get("X-Camunda-Lab-Backup-Secrets"))
	}
	names := gzipTarNames(t, rec.Body.Bytes())
	found := false
	for _, name := range names {
		if name == "ai.env" {
			found = true
		}
	}
	if !found {
		t.Fatalf("ai.env missing with includeSecrets; entries=%v", names)
	}
}

func TestBackupDownloadRejectsTraversalDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")

	body, _ := json.Marshal(map[string]any{"dir": "/etc"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	(&handler{}).runBackupDownload(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status=%d body=%s, want traversal rejection", rec.Code, rec.Body.String())
	}
	bodyText := rec.Body.String()
	for _, needle := range []string{"/etc", os.TempDir(), home} {
		if strings.Contains(bodyText, needle) {
			t.Fatalf("traversal error leaked %q in %s", needle, bodyText)
		}
	}
}

func TestSanitizeAttachmentFilename(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		{in: `camunda-lab-backup.tar.gz`, want: `camunda-lab-backup.tar.gz`},
		{in: "../evil\"name\r\n.zip", want: `evilname.zip`},
		{in: `/tmp/abs.zip`, want: `abs.zip`},
		{in: ``, want: `download`},
		{in: `...`, want: `download`},
	}
	for _, test := range tests {
		if got := toolkit.SanitizeAttachmentFilename(test.in); got != test.want {
			t.Fatalf("SanitizeAttachmentFilename(%q)=%q want %q", test.in, got, test.want)
		}
	}
}

func assertSecureAttachment(t *testing.T, rec *httptest.ResponseRecorder, ext string) {
	t.Helper()
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.HasPrefix(cd, "attachment; filename=") {
		t.Fatalf("Content-Disposition=%q", cd)
	}
	if strings.ContainsAny(cd, "/\\") || strings.Contains(cd, "\n") || strings.Contains(cd, "\r") {
		t.Fatalf("Content-Disposition not sanitized: %q", cd)
	}
	if !strings.Contains(cd, ext) {
		t.Fatalf("Content-Disposition missing %q: %q", ext, cd)
	}
	filename := strings.TrimPrefix(cd, `attachment; filename="`)
	filename = strings.TrimSuffix(filename, `"`)
	if filename != toolkit.SanitizeAttachmentFilename(filename) {
		t.Fatalf("filename not fully sanitized: %q", filename)
	}
}

func gzipTarNames(t *testing.T, payload []byte) []string {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var names []string
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, header.Name)
		if _, err := io.Copy(io.Discard, tr); err != nil {
			t.Fatal(err)
		}
	}
	return names
}

func listMatchingTemps(t *testing.T, prefix string) []string {
	t.Helper()
	entries, err := os.ReadDir(os.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var matched []string
	for _, entry := range entries {
		name := entry.Name()
		if strings.Contains(name, prefix) {
			matched = append(matched, filepath.Join(os.TempDir(), name))
		}
	}
	return matched
}

// Ensure the download route is registered on the real mux (not only a direct handler call).
func TestDownloadRoutesRegistered(t *testing.T) {
	h := NewHandler("test", Dependencies{})
	for _, path := range []string{
		"/api/v1/bpmn/test-generate/download",
		"/api/v1/backup/download",
	} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code == http.StatusNotFound && strings.Contains(rec.Body.String(), "404 page not found") {
			t.Fatalf("%s not registered", path)
		}
	}
}
