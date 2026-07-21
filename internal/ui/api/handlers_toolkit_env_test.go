package api

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestEnvUseRequiresExistingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)

	req := httptest.NewRequest("POST", "/api/v1/env/use", strings.NewReader(`{"name":"missing"}`))
	rec := httptest.NewRecorder()
	(&handler{}).envUse(rec, req)
	if rec.Code < 400 {
		t.Fatalf("env use status = %d, want error; body=%s", rec.Code, rec.Body.String())
	}
}

func TestEnvRemoveRejectsTraversalName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	outside := filepath.Join(home, "escape.yaml")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/env/../escape", nil)
	req.SetPathValue("name", "../escape")
	rec := httptest.NewRecorder()
	(&handler{}).envRemove(rec, req)
	if rec.Code < 400 {
		t.Fatalf("env remove status = %d, want error; body=%s", rec.Code, rec.Body.String())
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was modified: %v", err)
	}
}
