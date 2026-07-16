package lab_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/lab"
)

func TestEnvFilesIncludesUpstreamDotEnv(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("CAMUNDA_VERSION=8.9.13\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	resources := filepath.Join(dir, "resources.env")
	if err := os.WriteFile(resources, []byte("JAVA_TOOL_OPTIONS=-Xmx512m\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := lab.EnvFiles(dir, resources)
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
	if got[0] != filepath.Join(dir, ".env") {
		t.Fatalf("expected upstream .env first, got %q", got[0])
	}
	if got[1] != resources {
		t.Fatalf("expected resources.env second, got %q", got[1])
	}
}

func TestEnvFilesWithoutUpstream(t *testing.T) {
	dir := t.TempDir()
	resources := filepath.Join(dir, "resources.env")
	got := lab.EnvFiles(dir, resources)
	if len(got) != 1 || got[0] != resources {
		t.Fatalf("%#v", got)
	}
}
