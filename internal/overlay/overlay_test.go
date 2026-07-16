package overlay_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/overlay"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

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
	if len(files) != 1 {
		t.Fatalf("%v", files)
	}
	if filepath.Base(files[0]) != "elasticsearch-8.10.yaml" {
		t.Fatalf("%v", files)
	}
}

func TestComposeOverrideFiles89FullNone(t *testing.T) {
	files, err := overlay.ComposeOverrideFiles("8.9", "full")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 0 {
		t.Fatalf("%v", files)
	}
}
