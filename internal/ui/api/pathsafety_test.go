package api

import (
	"os"
	"path/filepath"
	"runtime"
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
	canonicalHome, err := filepath.EvalSymlinks(home)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalHome, "camunda-lab-test-allow")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
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

func TestAllowPathRejectsExistingSymlinkEscape(t *testing.T) {
	root := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink("/etc", link); err != nil {
		t.Fatal(err)
	}

	if _, err := allowPath(filepath.Join(link, "passwd")); err == nil {
		t.Fatal("expected symlink escape reject")
	}
}

func TestAllowPathRejectsNonexistentChildThroughSymlink(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "approved")
	outside := filepath.Join(base, "outside")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(root, "escape", "missing", "file.txt")
	if _, err := allowPathWithin(target, []string{root}); err == nil {
		t.Fatal("expected nonexistent child through escaping symlink reject")
	}
}

func TestAllowPathAllowsSymlinkResolvingInsideRoot(t *testing.T) {
	root := t.TempDir()
	realDir := filepath.Join(root, "real")
	if err := os.Mkdir(realDir, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "inside")
	if err := os.Symlink(realDir, link); err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(link, "new", "file.txt")
	got, err := allowPathWithin(target, []string{root})
	if err != nil {
		t.Fatal(err)
	}
	canonicalRealDir, err := filepath.EvalSymlinks(realDir)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(canonicalRealDir, "new", "file.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAllowPathRejectsRootPrefixCollision(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "approved")
	collision := filepath.Join(base, "approved-escape")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(collision, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := allowPathWithin(filepath.Join(collision, "file.txt"), []string{root}); err == nil {
		t.Fatal("expected root-prefix collision reject")
	}
}

func TestAllowPathCanonicalizesDarwinTmp(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("Darwin-specific /tmp canonicalization")
	}

	target := filepath.Join("/tmp", "camunda-lab-path-safety", "missing.txt")
	got, err := allowPathWithin(target, []string{"/private/tmp"})
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join("/private/tmp", "camunda-lab-path-safety", "missing.txt")
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
