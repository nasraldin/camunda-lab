package project

import (
	"os"
	"path/filepath"
	"testing"
)

func TestConfigValidateEmptyName(t *testing.T) {
	c := Config{Paths: Paths{BPMN: "bpmn", DMN: "dmn", Forms: "forms"}}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestConfigValidateMissingPaths(t *testing.T) {
	c := Config{Name: "demo"}
	if err := c.Validate(); err == nil {
		t.Fatal("expected error for missing paths")
	}
}

func TestConfigRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	orig := Defaults("orders", "8.9")
	orig.Lab.Profile = "full"
	if err := Save(path, orig); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "orders" || got.CamundaVersion != "8.9" {
		t.Fatalf("got %+v", got)
	}
	if got.Paths.BPMN != "bpmn" || got.Paths.DMN != "dmn" || got.Paths.Forms != "forms" {
		t.Fatalf("paths %+v", got.Paths)
	}
	if got.Lab.Profile != "full" || got.Lab.Resources != "balanced" {
		t.Fatalf("lab %+v", got.Lab)
	}
}

func TestLoadAppliesPathDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ConfigFileName)
	if err := os.WriteFile(path, []byte("name: bare\ncamundaVersion: \"8.8\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Paths.BPMN != "bpmn" {
		t.Fatalf("expected default bpmn path, got %q", got.Paths.BPMN)
	}
}
