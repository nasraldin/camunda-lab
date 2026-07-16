package overlay_test

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/overlay"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}

func TestJavaToolOptions(t *testing.T) {
	got, err := overlay.JavaToolOptions("balanced")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "512m") {
		t.Fatalf("%q", got)
	}
}

func TestSyncResourcesEnv(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	path, err := overlay.SyncResourcesEnv("small")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "JAVA_TOOL_OPTIONS=") {
		t.Fatalf("%s", data)
	}
}

func TestComposeOverrideFiles810Full(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	files, err := overlay.ComposeOverrideFiles("8.10", "full")
	if err != nil {
		t.Fatal(err)
	}
	bases := basenames(files)
	want := []string{"elasticsearch-8.10.yaml", "elasticsearch-cors.yaml", "elasticvue.yaml"}
	if len(bases) != len(want) {
		t.Fatalf("got %v want %v", bases, want)
	}
	for i := range want {
		if bases[i] != want[i] {
			t.Fatalf("got %v want %v", bases, want)
		}
	}
}

func TestComposeOverrideFiles89Full(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	files, err := overlay.ComposeOverrideFiles("8.9", "full")
	if err != nil {
		t.Fatal(err)
	}
	bases := basenames(files)
	want := []string{"elasticsearch-cors.yaml", "elasticvue.yaml"}
	if len(bases) != len(want) {
		t.Fatalf("got %v want %v", bases, want)
	}
	for i := range want {
		if bases[i] != want[i] {
			t.Fatalf("got %v want %v", bases, want)
		}
	}
}

func TestComposeOverrideFiles810LightNone(t *testing.T) {
	files, err := overlay.ComposeOverrideFiles("8.10", "light")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%v", files)
	}
}

func TestComposeOverrideFiles89LightNone(t *testing.T) {
	files, err := overlay.ComposeOverrideFiles("8.9", "light")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%v", files)
	}
}

func TestComposeOverrideFilesModelerNone(t *testing.T) {
	files, err := overlay.ComposeOverrideFiles("8.9", "modeler")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%v", files)
	}
}

func TestOverlaysInSync(t *testing.T) {
	root := repoRoot(t)
	names := []string{"elasticsearch-8.10.yaml", "elasticsearch-cors.yaml", "elasticvue.yaml"}
	for _, name := range names {
		embedPath := filepath.Join(root, "internal", "overlay", "embed", name)
		repoPath := filepath.Join(root, "overlays", name)
		a, err := os.ReadFile(embedPath)
		if err != nil {
			t.Fatal(err)
		}
		b, err := os.ReadFile(repoPath)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(a, b) {
			t.Fatalf("overlays/%s must match internal/overlay/embed/%s", name, name)
		}
	}
}
