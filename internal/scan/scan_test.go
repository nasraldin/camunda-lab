package scan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkFindsClientSecret(t *testing.T) {
	dir := t.TempDir()
	dirty := filepath.Join(dir, "connectors", "secrets.env")
	if err := os.MkdirAll(filepath.Dir(dirty), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dirty, []byte("CLIENT_SECRET=supersecretvalue99\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	clean := filepath.Join(dir, "readme.txt")
	if err := os.WriteFile(clean, []byte("no secrets here\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Walk(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) == 0 {
		t.Fatal("expected finding")
	}
	if !strings.Contains(fs[0].Snippet, "…") && fs[0].Snippet != "****" {
		t.Fatalf("expected masked snippet, got %q", fs[0].Snippet)
	}
	if strings.Contains(FormatText(fs), "supersecretvalue99") {
		t.Fatal("raw secret leaked in output")
	}
	if !ShouldFail(fs, "medium") {
		t.Fatal("should fail")
	}
}

func TestWalkClean(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "ok.yaml"), []byte("name: demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := Walk(Options{Root: dir})
	if err != nil {
		t.Fatal(err)
	}
	if len(fs) != 0 {
		t.Fatalf("unexpected %#v", fs)
	}
}
