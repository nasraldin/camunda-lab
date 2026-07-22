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

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestBackupDownloadStreamsGzipWithoutTempPathLeak(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")
	writeAPIFile(t, filepath.Join(project, ".camunda.yaml"), `name: dl
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: dmn
  forms: forms
`)
	writeAPIFile(t, filepath.Join(project, "models", "a.bpmn"), "<a/>")

	body, _ := json.Marshal(map[string]any{"dir": project})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", bytes.NewReader(body))
	rec := httptest.NewRecorder()
	(&handler{}).runBackupDownload(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	ct := rec.Header().Get("Content-Type")
	if !strings.Contains(ct, "gzip") && !strings.Contains(ct, "application/octet-stream") {
		t.Fatalf("Content-Type=%q", ct)
	}
	cd := rec.Header().Get("Content-Disposition")
	if !strings.Contains(cd, "attachment") || !strings.Contains(cd, ".tar.gz") {
		t.Fatalf("Content-Disposition=%q", cd)
	}
	if rec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("missing nosniff")
	}
	payload := rec.Body.String()
	if strings.Contains(payload, os.TempDir()) || strings.Contains(payload, home) {
		t.Fatalf("response leaked server path: %q", payload[:min(200, len(payload))])
	}
	if len(rec.Body.Bytes()) < 32 {
		t.Fatalf("download too small: %d bytes", rec.Body.Len())
	}
}

func TestBackupDownloadCreateFailureOmitsAbsolutePaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)

	leaky := filepath.Join(os.TempDir(), "camunda-lab-backup-dl-leak.tar.gz")
	h := &handler{backupCreate: func(context.Context, backup.Options) (backup.Manifest, error) {
		return backup.Manifest{}, &os.PathError{Op: "rename", Path: leaky, Err: os.ErrPermission}
	}}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup/download", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	h.runBackupDownload(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status=%d body=%s, want create failure", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "could not create backup archive") {
		t.Fatalf("body=%s, want opaque create failure", body)
	}
	for _, needle := range []string{os.TempDir(), home, leaky, "/Users/", "/var/", "/tmp/", "camunda-lab-backup-dl-leak"} {
		if strings.Contains(body, needle) {
			t.Fatalf("create failure leaked path pattern %q in body=%s", needle, body)
		}
	}
}

func TestBackupJSONRejectsImplicitTempPath(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")

	req := httptest.NewRequest(http.MethodPost, "/api/v1/backup", strings.NewReader(`{}`))
	rec := httptest.NewRecorder()
	(&handler{}).runBackup(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status=%d body=%s, want reject empty output", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), os.TempDir()) {
		t.Fatalf("error leaked temp path: %s", rec.Body.String())
	}
}

func TestRestoreAPIUsesRunningCheckerAndForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")
	archive := filepath.Join(t.TempDir(), "api.tar.gz")
	if _, err := backup.Create(context.Background(), backup.Options{
		LabHome: home, OutPath: archive, LabVersion: "8.9", LabProfile: "light",
	}); err != nil {
		t.Fatal(err)
	}

	checks := 0
	h := &handler{runningLab: runningCheckerFunc(func(context.Context) (bool, error) {
		checks++
		return true, nil
	})}

	restoreBody := multipartRestore(t, archive, map[string]string{"yes": "true"})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore", restoreBody.body)
	req.Header.Set("Content-Type", restoreBody.contentType)
	rec := httptest.NewRecorder()
	h.runRestore(rec, req)
	if rec.Code < 400 || !strings.Contains(rec.Body.String(), "lab is running") {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	if checks == 0 {
		t.Fatal("running checker not consulted")
	}

	lab2 := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", lab2)
	paths.Reset()
	checks = 0
	forceBody := multipartRestore(t, archive, map[string]string{"yes": "true", "force": "true"})
	req = httptest.NewRequest(http.MethodPost, "/api/v1/restore", forceBody.body)
	req.Header.Set("Content-Type", forceBody.contentType)
	rec = httptest.NewRecorder()
	h.runRestore(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("force restore status=%d body=%s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), os.TempDir()) {
		t.Fatalf("restore response leaked temp path: %s", rec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(lab2, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestRestoreAPIRejectsWithoutConfirmation(t *testing.T) {
	body := multipartRestore(t, writeTinyArchive(t), map[string]string{})
	req := httptest.NewRequest(http.MethodPost, "/api/v1/restore", body.body)
	req.Header.Set("Content-Type", body.contentType)
	rec := httptest.NewRecorder()
	(&handler{runningLab: runningCheckerFunc(func(context.Context) (bool, error) {
		return false, nil
	})}).runRestore(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status=%d, want confirmation rejection", rec.Code)
	}
}

func TestTraceFollowAPIIsBounded(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/process-instances/search" {
			_, _ = w.Write([]byte(`{"items":[],"page":{"totalItems":0}}`))
			return
		}
		polls++
		state := "ACTIVE"
		if polls >= 2 {
			state = "COMPLETED"
		}
		_, _ = w.Write([]byte(`{"items":[{
			"processInstanceKey":"42","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"` + state + `","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
		}],"page":{"totalItems":1,"endCursor":null}}`))
	}))
	t.Cleanup(server.Close)

	h := &handler{clusterFactory: &uiTestClusterFactory{baseURL: server.URL + "/v2"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trace/42?follow=1&interval=100ms&timeout=2s&maxEvents=3", nil)
	req.SetPathValue("instanceKey", "42")
	rec := httptest.NewRecorder()
	h.traceInstance(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK        bool             `json:"ok"`
		Timelines []map[string]any `json:"timelines"`
		Follow    bool             `json:"follow"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.Follow || len(body.Timelines) == 0 {
		t.Fatalf("body=%#v", body)
	}
	if polls < 2 {
		t.Fatalf("polls=%d, want follow polling", polls)
	}
}

type multipartRestoreBody struct {
	body        *bytes.Buffer
	contentType string
}

func multipartRestore(t *testing.T, archive string, fields map[string]string) multipartRestoreBody {
	t.Helper()
	var form bytes.Buffer
	writer := multipart.NewWriter(&form)
	for k, v := range fields {
		_ = writer.WriteField(k, v)
	}
	part, err := writer.CreateFormFile("archive", filepath.Base(archive))
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
	return multipartRestoreBody{body: &form, contentType: writer.FormDataContentType()}
}

func writeTinyArchive(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	writeAPIFile(t, filepath.Join(home, "config.yaml"), "version: \"8.9\"\nprofile: light\n")
	out := filepath.Join(t.TempDir(), "tiny.tar.gz")
	if _, err := backup.Create(context.Background(), backup.Options{
		LabHome: home, OutPath: out, LabVersion: "8.9", LabProfile: "light",
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

type runningCheckerFunc func(context.Context) (bool, error)

func (f runningCheckerFunc) Running(ctx context.Context) (bool, error) {
	return f(ctx)
}

type errSimple string

func (e errSimple) Error() string { return string(e) }
