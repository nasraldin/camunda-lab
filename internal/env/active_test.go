package env

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestSetActiveRequiresExistingProfile(t *testing.T) {
	home := t.TempDir()
	profiles := filepath.Join(home, "envs")

	if err := SetActive(home, profiles, "missing"); err == nil {
		t.Fatal("SetActive accepted a missing profile")
	}
	active, err := GetActive(home)
	if err != nil {
		t.Fatal(err)
	}
	if active != "lab" {
		t.Fatalf("active = %q, want lab", active)
	}
}

func TestSetActiveAllowsImplicitLab(t *testing.T) {
	home := t.TempDir()
	if err := SetActive(home, filepath.Join(home, "envs"), "lab"); err != nil {
		t.Fatal(err)
	}
	active, err := GetActive(home)
	if err != nil {
		t.Fatal(err)
	}
	if active != "lab" {
		t.Fatalf("active = %q, want lab", active)
	}
}

func TestGetActiveRejectsCorruptState(t *testing.T) {
	home := t.TempDir()
	if err := os.WriteFile(filepath.Join(home, "active-env"), []byte("../escape\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := GetActive(home); err == nil {
		t.Fatal("GetActive accepted corrupt active state")
	}
}

func TestRemoveActiveProfileFallsBackToLab(t *testing.T) {
	home := t.TempDir()
	profiles := filepath.Join(home, "envs")
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("environmentReferences:\n  complete: true\n  projects: {}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	saveRemoteProfile(t, profiles, "prod")
	if err := SetActive(home, profiles, "prod"); err != nil {
		t.Fatal(err)
	}

	if err := RemoveProfile(home, profiles, "prod"); err != nil {
		t.Fatal(err)
	}
	active, err := GetActive(home)
	if err != nil {
		t.Fatal(err)
	}
	if active != "lab" {
		t.Fatalf("active = %q, want lab", active)
	}
	path, err := ProfilePath(profiles, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("removed profile stat error = %v", err)
	}
}

func saveRemoteProfile(t *testing.T, dir, name string) {
	t.Helper()
	p := Profile{
		Name:      name,
		Kind:      "remote",
		Endpoints: map[string]string{"orchestration": "https://camunda.example"},
		Auth:      AuthRefs{ClientIDEnv: "CAMUNDA_CLIENT_ID", ClientSecretEnv: "CAMUNDA_CLIENT_SECRET"},
	}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatal(err)
	}
}
