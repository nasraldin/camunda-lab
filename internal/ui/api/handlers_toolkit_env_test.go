package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/env"
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

func TestEnvAPIHonorsAuthorizedProjectDir(t *testing.T) {
	home := t.TempDir()
	projectRoot := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := os.WriteFile(filepath.Join(projectRoot, ".camunda.yaml"), []byte("name: env-api\ncamundaVersion: \"8.9\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	h := &handler{}
	dirJSON, err := json.Marshal(projectRoot)
	if err != nil {
		t.Fatal(err)
	}

	add := httptest.NewRequest("POST", "/api/v1/env", strings.NewReader(`{
		"name":"project-staging","kind":"remote","orchestration":"https://staging.example/v2","dir":`+string(dirJSON)+`
	}`))
	add.Header.Set("Content-Type", "application/json")
	addRec := httptest.NewRecorder()
	h.envAdd(addRec, add)
	if addRec.Code != 200 {
		t.Fatalf("env add status=%d body=%s", addRec.Code, addRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "environments", "project-staging.yaml")); err != nil {
		t.Fatalf("expected project profile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "envs", "project-staging.yaml")); !os.IsNotExist(err) {
		t.Fatalf("global profile should not exist: %v", err)
	}

	use := httptest.NewRequest("POST", "/api/v1/env/use", strings.NewReader(`{"name":"project-staging","dir":`+string(dirJSON)+`}`))
	use.Header.Set("Content-Type", "application/json")
	useRec := httptest.NewRecorder()
	h.envUse(useRec, use)
	if useRec.Code != 200 {
		t.Fatalf("env use status=%d body=%s", useRec.Code, useRec.Body.String())
	}

	list := httptest.NewRequest("GET", "/api/v1/env?dir="+url.QueryEscape(projectRoot), nil)
	listRec := httptest.NewRecorder()
	h.envList(listRec, list)
	if listRec.Code != 200 {
		t.Fatalf("env list status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var listed struct {
		Active   string `json:"active"`
		Profiles []struct {
			Name   string `json:"name"`
			Source string `json:"source"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal(listRec.Body.Bytes(), &listed); err != nil {
		t.Fatal(err)
	}
	if listed.Active != "project-staging" {
		t.Fatalf("active=%q", listed.Active)
	}
	foundProject := false
	for _, profile := range listed.Profiles {
		if profile.Name == "project-staging" {
			foundProject = true
			if profile.Source != "project" {
				t.Fatalf("source=%q", profile.Source)
			}
		}
	}
	if !foundProject {
		t.Fatalf("profiles=%v", listed.Profiles)
	}

	remove := httptest.NewRequest("DELETE", "/api/v1/env/project-staging?dir="+url.QueryEscape(projectRoot), nil)
	remove.SetPathValue("name", "project-staging")
	removeRec := httptest.NewRecorder()
	h.envRemove(removeRec, remove)
	if removeRec.Code != 200 {
		t.Fatalf("env remove status=%d body=%s", removeRec.Code, removeRec.Body.String())
	}
	if _, err := os.Stat(filepath.Join(projectRoot, "environments", "project-staging.yaml")); !os.IsNotExist(err) {
		t.Fatalf("project profile still present: %v", err)
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

func TestEnvRemoveWithoutProjectRejectsUnknownReferences(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("version: \"8.9\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	profile := env.Profile{
		Name:      "prod",
		Kind:      "remote",
		Endpoints: map[string]string{"orchestration": "https://prod.example"},
		Auth: env.AuthRefs{
			ClientIDEnv:     "CAMUNDA_CLIENT_ID",
			ClientSecretEnv: "CAMUNDA_CLIENT_SECRET",
		},
	}
	if err := env.NewService(home).SaveGlobal(profile); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest("DELETE", "/api/v1/env/prod", nil)
	req.SetPathValue("name", "prod")
	rec := httptest.NewRecorder()
	(&handler{}).envRemove(rec, req)
	if rec.Code < 400 {
		t.Fatalf("status = %d, want conflict; body=%s", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rec.Body.String(), "completeness") {
		t.Fatalf("body = %v, want reference completeness error", body)
	}
	if _, err := os.Stat(filepath.Join(home, "envs", "prod.yaml")); err != nil {
		t.Fatalf("profile changed after rejected remove: %v", err)
	}
}
