package env

import (
	"errors"
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

func TestRemoteRejectsNonCanonicalEnvironmentReferencesWithoutEcho(t *testing.T) {
	for _, field := range []string{"clientIdEnv", "clientSecretEnv", "tokenUrlEnv"} {
		t.Run(field, func(t *testing.T) {
			auth := AuthRefs{
				ClientIDEnv: "VALID_ID", ClientSecretEnv: "VALID_SECRET",
				TokenURL: "https://identity.example/token",
			}
			invalid := " INVALID-VALUE "
			switch field {
			case "clientIdEnv":
				auth.ClientIDEnv = invalid
			case "clientSecretEnv":
				auth.ClientSecretEnv = invalid
			case "tokenUrlEnv":
				auth.TokenURL = ""
				auth.TokenURLEnv = invalid
			}
			profile := Profile{
				Name: "prod", Kind: "remote",
				Endpoints: map[string]string{"orchestration": "https://cluster.example"},
				Auth:      auth,
			}
			err := profile.Validate()
			if err == nil || !strings.Contains(err.Error(), field) {
				t.Fatalf("Validate() error = %v", err)
			}
			if strings.Contains(err.Error(), invalid) || strings.Contains(err.Error(), "INVALID-VALUE") {
				t.Fatalf("validation error echoed invalid value: %v", err)
			}
		})
	}
}

func TestServiceResolveInvalidEnvironmentReferenceIsTypedAndRedacted(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, "envs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("activeEnv: prod\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	data := []byte("name: prod\nkind: remote\nendpoints:\n  orchestration: https://cluster.example\nauth:\n  clientIdEnv: INVALID-VALUE\n  clientSecretEnv: VALID_SECRET\n  tokenUrl: https://identity.example/token\n")
	if err := os.WriteFile(filepath.Join(home, "envs", "prod.yaml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := NewService(home).Resolve(ResolveRequest{})
	var serviceErr *Error
	if !errors.As(err, &serviceErr) || serviceErr.Kind != ErrorInvalid {
		t.Fatalf("Resolve() error = %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "clientIdEnv") || strings.Contains(err.Error(), "INVALID-VALUE") {
		t.Fatalf("typed error is not actionable/redacted: %v", err)
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
