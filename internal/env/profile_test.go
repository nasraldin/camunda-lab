package env

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRejectInlineSecret(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.yaml")
	if err := os.WriteFile(path, []byte("name: bad\nkind: remote\nendpoints:\n  orchestration: https://x\nauth:\n  clientIdEnv: ID\n  clientSecret: leaked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadProfile(path)
	if err == nil || !strings.Contains(err.Error(), "clientSecret") {
		t.Fatalf("expected reject, got %v", err)
	}
}

func TestSaveLoadRemote(t *testing.T) {
	dir := t.TempDir()
	p := Profile{
		Name:      "prod",
		Kind:      "remote",
		Endpoints: map[string]string{"orchestration": "https://camunda.example"},
		Auth:      AuthRefs{ClientIDEnv: "CAMUNDA_CLIENT_ID", ClientSecretEnv: "CAMUNDA_CLIENT_SECRET"},
	}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadNamedProfile(dir, "prod")
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoints["orchestration"] != "https://camunda.example" {
		t.Fatalf("%+v", got)
	}
}

func TestRemoteRequiresAuth(t *testing.T) {
	p := Profile{Name: "x", Kind: "remote", Endpoints: map[string]string{"orchestration": "https://x"}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected auth error")
	}
}

func TestLoadNamedRejectsFilenameProfileNameMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "prod.yaml")
	data := []byte("name: staging\nkind: remote\nendpoints:\n  orchestration: https://x\nauth:\n  clientIdEnv: ID\n  clientSecretEnv: SECRET\n")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNamedProfile(dir, "prod"); err == nil {
		t.Fatal("LoadNamedProfile accepted mismatched embedded name")
	}
}

func TestLoadNamedRejectsProfileSymlink(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "target.yaml")
	data := []byte("name: prod\nkind: remote\nendpoints:\n  orchestration: https://x\nauth:\n  clientIdEnv: ID\n  clientSecretEnv: SECRET\n")
	if err := os.WriteFile(target, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "prod.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadNamedProfile(dir, "prod"); err == nil {
		t.Fatal("LoadNamedProfile accepted a profile symlink")
	}
}

func TestSaveProfileRejectsReservedName(t *testing.T) {
	p := Profile{Name: "lab", Kind: "lab"}
	if err := SaveProfile(t.TempDir(), p); err == nil {
		t.Fatal("SaveProfile accepted reserved stored name")
	}
}
