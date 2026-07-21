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

func TestRemoveActiveProfileRollsBackWhenActivePersistenceFails(t *testing.T) {
	home := t.TempDir()
	profiles := filepath.Join(home, "envs")
	saveRemoteProfile(t, profiles, "prod")
	if err := SetActive(home, profiles, "prod"); err != nil {
		t.Fatal(err)
	}
	path, err := ProfilePath(profiles, "prod")
	if err != nil {
		t.Fatal(err)
	}
	persistErr := errors.New("injected active persistence failure")
	fallbackAttempted := false
	failFallback := func(gotHome, name string) error {
		fallbackAttempted = true
		if gotHome != home || name != "lab" {
			t.Fatalf("fallback write = (%q, %q), want (%q, lab)", gotHome, name, home)
		}
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("profile was not tombstoned before fallback write: %v", err)
		}
		tombstones, err := filepath.Glob(filepath.Join(profiles, ".prod.deleting-*"))
		if err != nil {
			t.Fatal(err)
		}
		if len(tombstones) != 1 {
			t.Fatalf("tombstones = %v, want exactly one", tombstones)
		}
		active, err := GetActive(home)
		if err != nil {
			t.Fatal(err)
		}
		if active != "prod" {
			t.Fatalf("active during failed fallback = %q, want prod", active)
		}
		return persistErr
	}

	err = removeProfile(home, profiles, "prod", failFallback)
	if !errors.Is(err, persistErr) {
		t.Fatalf("removeProfile error = %v, want injected persistence failure", err)
	}
	if !fallbackAttempted {
		t.Fatal("fallback active write was not attempted")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("profile was not rolled back: %v", err)
	}
	active, err := GetActive(home)
	if err != nil {
		t.Fatal(err)
	}
	if active != "prod" {
		t.Fatalf("active after rollback = %q, want prod", active)
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
