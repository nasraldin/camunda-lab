package api

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAllowPathHomeAndTmp(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(home, "camunda-lab-test-allow")
	got, err := allowPath(p)
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Clean(p) {
		t.Fatalf("got %q", got)
	}
	tmp := filepath.Join(os.TempDir(), "camunda-x")
	if _, err := allowPath(tmp); err != nil {
		t.Fatal(err)
	}
	if _, err := allowPath("relative/path"); err == nil {
		t.Fatal("expected relative reject")
	}
	if _, err := allowPath("/etc/passwd"); err == nil {
		t.Fatal("expected /etc reject")
	}
}
