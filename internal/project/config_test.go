package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
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
	orig.Paths.Tests = "spec/process"
	orig.Lint.Ignore = []string{"gateway-no-default", "task-no-retries"}
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
	if got.Paths.Tests != "spec/process" {
		t.Fatalf("tests path %q", got.Paths.Tests)
	}
	if !reflect.DeepEqual(got.Lint.Ignore, orig.Lint.Ignore) {
		t.Fatalf("lint ignore got %v, want %v", got.Lint.Ignore, orig.Lint.Ignore)
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
	if got.Paths.Tests != "tests" {
		t.Fatalf("expected default tests path, got %q", got.Paths.Tests)
	}
}

func TestConfigValidateRejectsUnsafePaths(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{name: "absolute", path: filepath.Join(string(filepath.Separator), "tmp", "bpmn")},
		{name: "traversal", path: "../bpmn"},
		{name: "unclean", path: "assets/../bpmn"},
		{name: "dot", path: "."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := Defaults("unsafe", "8.9")
			cfg.Paths.BPMN = tt.path
			err := cfg.Validate()
			if err == nil {
				t.Fatalf("expected %q to be rejected", tt.path)
			}
			if !strings.Contains(err.Error(), "paths.bpmn") {
				t.Fatalf("error should identify paths.bpmn: %v", err)
			}
		})
	}
}
