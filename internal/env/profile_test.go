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
		Name: "prod",
		Kind: "remote",
		Endpoints: map[string]string{"orchestration": "https://camunda.example"},
		Auth:      AuthRefs{ClientIDEnv: "CAMUNDA_CLIENT_ID", ClientSecretEnv: "CAMUNDA_CLIENT_SECRET"},
	}
	if err := SaveProfile(dir, p); err != nil {
		t.Fatal(err)
	}
	got, err := LoadProfile(filepath.Join(dir, "prod.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Endpoints["orchestration"] != "https://camunda.example" {
		t.Fatalf("%+v", got)
	}
}

func TestActive(t *testing.T) {
	home := t.TempDir()
	if GetActive(home) != "lab" {
		t.Fatal("default")
	}
	if err := SetActive(home, "prod"); err != nil {
		t.Fatal(err)
	}
	if GetActive(home) != "prod" {
		t.Fatal(GetActive(home))
	}
}

func TestRemoteRequiresAuth(t *testing.T) {
	p := Profile{Name: "x", Kind: "remote", Endpoints: map[string]string{"orchestration": "https://x"}}
	if err := p.Validate(); err == nil {
		t.Fatal("expected auth error")
	}
}
