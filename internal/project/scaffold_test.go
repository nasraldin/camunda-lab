package project

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestScaffoldCreatesLayout(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "orders")
	if err := Scaffold(ScaffoldOpts{
		Dir:       dir,
		Name:      "orders",
		Version:   "8.9",
		Profile:   "light",
		Resources: "balanced",
	}); err != nil {
		t.Fatal(err)
	}

	for _, d := range scaffoldDirs {
		fi, err := os.Stat(filepath.Join(dir, d))
		if err != nil || !fi.IsDir() {
			t.Fatalf("missing dir %s: %v", d, err)
		}
	}
	for _, f := range []string{
		ConfigFileName,
		"README.md",
		"docker-compose.yml",
		filepath.Join("helm", "README.md"),
		filepath.Join("environments", ".gitkeep"),
	} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Fatalf("missing file %s: %v", f, err)
		}
	}

	cfg, err := Load(filepath.Join(dir, ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "orders" || cfg.CamundaVersion != "8.9" {
		t.Fatalf("config %+v", cfg)
	}

	readme, err := os.ReadFile(filepath.Join(dir, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(readme)
	for _, want := range []string{"camunda install", "camunda ui", "official Camunda tooling"} {
		if !strings.Contains(text, want) {
			t.Fatalf("README missing %q", want)
		}
	}
}

func TestScaffoldRefusesNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := Scaffold(ScaffoldOpts{Dir: dir, Name: "x"})
	if err == nil {
		t.Fatal("expected error for non-empty dir")
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Fatalf("error should mention --force: %v", err)
	}
}

func TestScaffoldForceAllowsNonEmpty(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Scaffold(ScaffoldOpts{Dir: dir, Name: "forced", Force: true}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, ConfigFileName)); err != nil {
		t.Fatal(err)
	}
}

func TestScaffoldDefaultNameFromDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "my-service")
	if err := Scaffold(ScaffoldOpts{Dir: dir}); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(filepath.Join(dir, ConfigFileName))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Name != "my-service" {
		t.Fatalf("name %q", cfg.Name)
	}
}

func TestScaffoldOptsValidate(t *testing.T) {
	valid := ScaffoldOpts{
		Dir:       filepath.Join(t.TempDir(), "valid"),
		Name:      "orders",
		Version:   "8.9",
		Profile:   "light",
		Resources: "balanced",
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid options rejected: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*ScaffoldOpts)
		want   string
	}{
		{name: "empty directory", mutate: func(o *ScaffoldOpts) { o.Dir = " " }, want: "directory"},
		{name: "invalid name", mutate: func(o *ScaffoldOpts) { o.Name = "../orders" }, want: "name"},
		{name: "invalid profile", mutate: func(o *ScaffoldOpts) { o.Profile = "huge" }, want: "profile"},
		{name: "invalid resources", mutate: func(o *ScaffoldOpts) { o.Resources = "tiny" }, want: "resources"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := valid
			tt.mutate(&opts)
			err := opts.Validate()
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Validate() error = %v, want error containing %q", err, tt.want)
			}
		})
	}
}

func TestScaffoldOutputOpensAsProject(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "orders")
	if err := Scaffold(ScaffoldOpts{Dir: dir, Name: "orders"}); err != nil {
		t.Fatal(err)
	}
	got, err := Open(filepath.Join(dir, "bpmn"))
	if err != nil {
		t.Fatalf("Open(scaffold): %v", err)
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Root != root || got.Config.Name != "orders" {
		t.Fatalf("project = %+v, want root %q and name orders", got, root)
	}
}
