package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}

func readDocs(t *testing.T, parts ...string) string {
	t.Helper()
	path := filepath.Join(append([]string{repoRoot(t), "docs"}, parts...)...)
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func TestToolkitCommandsDocumentedInCLIReference(t *testing.T) {
	doc := readDocs(t, "cli-reference.md")
	commands := []string{
		"lint", "diff", "explain", "review", "test generate", "scan",
		"doctor", "env", "plan", "drift", "backup", "restore",
		"incidents", "trace",
	}
	for _, command := range commands {
		if !strings.Contains(doc, "`"+command+"`") && !strings.Contains(doc, command) {
			t.Errorf("cli-reference.md missing command %q", command)
		}
	}
}

func TestToolkitFlagsDocumentedInCLIReference(t *testing.T) {
	doc := readDocs(t, "cli-reference.md")
	flags := []struct {
		command string
		flags   []string
	}{
		{command: "lint", flags: []string{"fail-on", "ignore", "json"}},
		{command: "diff", flags: []string{"from", "to", "against", "base", "json"}},
		{command: "explain", flags: []string{"json", "output"}},
		{command: "review", flags: []string{"fail-on", "ignore", "ai", "ai-required", "provider", "model", "json"}},
		{command: "test generate", flags: []string{"lang", "output", "force", "json"}},
		{command: "scan", flags: []string{"fail-on", "ignore", "json"}},
		{command: "doctor", flags: []string{"deep", "json", "timeout"}},
		{command: "env add", flags: []string{"kind", "orchestration", "client-id-env", "client-secret-env", "token-url", "token-url-env", "audience", "scope"}},
		{command: "plan", flags: []string{"dir", "env", "json"}},
		{command: "drift", flags: []string{"dir", "ref", "env", "json"}},
		{command: "backup", flags: []string{"output", "include-secrets"}},
		{command: "restore", flags: []string{"yes", "force", "project"}},
		{command: "incidents", flags: []string{"env", "limit"}},
		{command: "incidents retry", flags: []string{"yes", "dry-run"}},
		{command: "trace", flags: []string{"follow", "json", "env", "interval", "timeout", "idle-stop", "max-events"}},
	}
	for _, item := range flags {
		for _, flag := range item.flags {
			needle := "--" + flag
			if !strings.Contains(doc, needle) {
				t.Errorf("cli-reference.md missing %s for %s", needle, item.command)
			}
		}
	}
}

func TestCLIReferenceDocumentsExitCodes(t *testing.T) {
	doc := readDocs(t, "cli-reference.md")
	for _, phrase := range []string{
		"exit `0`", "exit `1`", "exit `2`",
		"exit with status `1`", "exit with status `2`",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("cli-reference.md missing exit-code phrase %q", phrase)
		}
	}
}

func TestCLIReferenceDocumentsOIDCAndBackupLimits(t *testing.T) {
	doc := readDocs(t, "cli-reference.md")
	for _, phrase := range []string{
		"OIDC",
		"Docker volumes",
		"64 MiB",
		"512 MiB",
		"RESTORE",
		"include-secrets",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("cli-reference.md missing %q", phrase)
		}
	}
}
