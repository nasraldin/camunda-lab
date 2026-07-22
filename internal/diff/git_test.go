package diff

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestGitReaderReadsProjectRelativeBPMNAtRevision(t *testing.T) {
	root := initRepository(t)
	path := filepath.Join(root, "models", "process.bpmn")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, root, "add", "models/process.bpmn")
	runGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "base")
	if err := os.WriteFile(path, []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}

	content, err := NewGitReader(root).Read(context.Background(), "HEAD", "models/process.bpmn")
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "before" {
		t.Fatalf("content = %q", content)
	}
}

func TestGitReaderRejectsUnsafeAndUnsupportedPaths(t *testing.T) {
	reader := NewGitReader(t.TempDir())
	for _, path := range []string{"../escape.bpmn", "/absolute.bpmn", "models", "model.dmn", "form.form"} {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			_, err := reader.Read(context.Background(), "HEAD", path)
			if err == nil {
				t.Fatalf("Read accepted %q", path)
			}
			var gitErr *GitError
			if !errors.As(err, &gitErr) {
				t.Fatalf("error = %T %v", err, err)
			}
		})
	}
}

func TestGitReaderRejectsUnsafeRefsBeforeRunningGit(t *testing.T) {
	reader := NewGitReader(t.TempDir())
	for _, ref := range []string{"", " HEAD ", "-HEAD", "HEAD:other", "HEAD\nmain"} {
		_, err := reader.Read(context.Background(), ref, "process.bpmn")
		var gitErr *GitError
		if !errors.As(err, &gitErr) || gitErr.Operation != "validate Git ref" {
			t.Fatalf("ref %q error = %#v", ref, err)
		}
	}
}

func TestGitReaderBoundsCommandErrors(t *testing.T) {
	root := initRepository(t)
	_, err := NewGitReader(root).Read(context.Background(), strings.Repeat("missing", 1000), "missing.bpmn")
	if err == nil {
		t.Fatal("expected Git failure")
	}
	if len(err.Error()) > maxGitErrorBytes+256 {
		t.Fatalf("Git error is unbounded: %d bytes", len(err.Error()))
	}
	if !strings.Contains(err.Error(), "git show failed") {
		t.Fatalf("error = %v", err)
	}
}

func TestGitReaderPreservesCanceledContext(t *testing.T) {
	root := initRepository(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := NewGitReader(root).Read(ctx, "HEAD", "process.bpmn")
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %T %v, want context.Canceled", err, err)
	}
}

func TestGitReaderDoesNotInterpretRefAsShell(t *testing.T) {
	root := initRepository(t)
	marker := filepath.Join(t.TempDir(), "injected")
	ref := "HEAD;touch " + marker

	_, err := NewGitReader(root).Read(context.Background(), ref, "process.bpmn")
	if err == nil {
		t.Fatal("expected invalid revision to fail")
	}
	if _, statErr := os.Stat(marker); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("ref executed as shell input: %v", statErr)
	}
}

func initRepository(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	runGit(t, root, "init", "-q")
	return root
}

func runGit(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
