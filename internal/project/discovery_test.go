package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestOpenFindsConfiguredProjectFromNestedDirectory(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, Defaults("orders", "8.9"))
	nested := filepath.Join(root, "workers", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Open(nested)
	if err != nil {
		t.Fatal(err)
	}
	wantRoot := canonicalPath(t, root)
	if got.Root != wantRoot || got.Config.Name != "orders" {
		t.Fatalf("Open() = %+v, want root %q and configured project", got, wantRoot)
	}
}

func TestOpenWithoutConfigFallsBackToBPMNOnly(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "bpmn", "order.bpmn"))
	writeFile(t, filepath.Join(root, "dmn", "decision.dmn"))

	got, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	bpmn, err := got.Discover(AssetBPMN)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{canonicalPath(t, filepath.Join(root, "bpmn", "order.bpmn"))}; !reflect.DeepEqual(bpmn, want) {
		t.Fatalf("BPMN = %v, want %v", bpmn, want)
	}
	dmn, err := got.Discover(AssetDMN)
	if err != nil {
		t.Fatal(err)
	}
	if len(dmn) != 0 {
		t.Fatalf("fallback discovered DMN: %v", dmn)
	}
}

func TestConfiguredProjectDoesNotUseFallbackBPMN(t *testing.T) {
	root := t.TempDir()
	cfg := Defaults("orders", "8.9")
	cfg.Paths.BPMN = "models"
	writeProjectConfig(t, root, cfg)
	writeFile(t, filepath.Join(root, "bpmn", "fallback.bpmn"))
	writeFile(t, filepath.Join(root, "models", "configured.bpmn"))

	project, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := project.Discover(AssetBPMN)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{canonicalPath(t, filepath.Join(root, "models", "configured.bpmn"))}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %v, want %v", got, want)
	}
}

func TestDiscoverRecursesFiltersAndSortsByKind(t *testing.T) {
	root := t.TempDir()
	cfg := Defaults("toolkit", "8.9")
	cfg.Paths.BPMN = "assets/processes"
	writeProjectConfig(t, root, cfg)
	for _, name := range []string{
		"assets/processes/z-last.bpmn",
		"assets/processes/nested/a-first.bpmn",
		"assets/processes/nested/ignore.dmn",
		"assets/processes/readme.txt",
	} {
		writeFile(t, filepath.Join(root, filepath.FromSlash(name)))
	}

	project, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := project.Discover(AssetBPMN)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		canonicalPath(t, filepath.Join(root, "assets/processes/nested/a-first.bpmn")),
		canonicalPath(t, filepath.Join(root, "assets/processes/z-last.bpmn")),
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %v, want %v", got, want)
	}
}

func TestToolkitProjectFixture(t *testing.T) {
	root := filepath.Join("..", "..", "testdata", "projects", "toolkit")
	project, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	if project.Config.Paths.Tests != "process-tests" ||
		!reflect.DeepEqual(project.Config.Lint.Ignore, []string{"gateway-no-default"}) {
		t.Fatalf("fixture config = %+v", project.Config)
	}
	got, err := project.Discover(AssetBPMN)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{canonicalPath(t, filepath.Join(root, "models", "nested", "order.bpmn"))}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("Discover() = %v, want %v", got, want)
	}
}

func TestOpenRejectsConfiguredSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	cfg := Defaults("unsafe", "8.9")
	cfg.Paths.BPMN = "models"
	writeProjectConfig(t, root, cfg)
	if err := os.Symlink(outside, filepath.Join(root, "models")); err != nil {
		t.Fatal(err)
	}

	_, err := Open(root)
	if err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
	if !strings.Contains(err.Error(), "paths.bpmn") {
		t.Fatalf("error should identify paths.bpmn: %v", err)
	}
}

func TestOpenRejectsConfigSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), ConfigFileName)
	if err := Save(outside, Defaults("outside", "8.9")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, ConfigFileName)); err != nil {
		t.Fatal(err)
	}

	if _, err := Open(root); err == nil {
		t.Fatal("expected config symlink escape to be rejected")
	}
}

func TestResolveInputPrefersProjectRelativeThenConfiguredDirectory(t *testing.T) {
	root := t.TempDir()
	cfg := Defaults("orders", "8.9")
	cfg.Paths.BPMN = "models"
	writeProjectConfig(t, root, cfg)
	rootFile := filepath.Join(root, "shared.bpmn")
	configuredFile := filepath.Join(root, "models", "nested", "order.bpmn")
	writeFile(t, rootFile)
	writeFile(t, configuredFile)

	project, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	got, err := project.ResolveInput(AssetBPMN, "shared.bpmn")
	if err != nil {
		t.Fatal(err)
	}
	if got != canonicalPath(t, rootFile) {
		t.Fatalf("project-relative result %q", got)
	}
	got, err = project.ResolveInput(AssetBPMN, filepath.Join("nested", "order.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	if got != canonicalPath(t, configuredFile) {
		t.Fatalf("configured-relative result %q", got)
	}
}

func TestResolveInputRejectsUnsafeAndWrongKind(t *testing.T) {
	root := t.TempDir()
	writeProjectConfig(t, root, Defaults("orders", "8.9"))
	writeFile(t, filepath.Join(root, "bpmn", "decision.dmn"))
	for _, input := range []string{"../outside.bpmn", filepath.Join(string(filepath.Separator), "tmp", "outside.bpmn")} {
		if _, err := mustOpen(t, root).ResolveInput(AssetBPMN, input); err == nil {
			t.Fatalf("expected unsafe input %q to fail", input)
		}
	}
	if _, err := mustOpen(t, root).ResolveInput(AssetBPMN, "decision.dmn"); err == nil {
		t.Fatal("expected wrong-kind input to fail")
	}
}

func writeProjectConfig(t *testing.T, root string, cfg Config) {
	t.Helper()
	if err := Save(filepath.Join(root, ConfigFileName), cfg); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func canonicalPath(t *testing.T, path string) string {
	t.Helper()
	got, err := filepath.EvalSymlinks(path)
	if err != nil {
		t.Fatal(err)
	}
	got, err = filepath.Abs(got)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustOpen(t *testing.T, root string) Project {
	t.Helper()
	project, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	return project
}
