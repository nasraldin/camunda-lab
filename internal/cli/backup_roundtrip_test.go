package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestBackupRestoreCLIRoundTrip(t *testing.T) {
	home := t.TempDir()
	project := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)

	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("version: \"8.9\"\nprofile: light\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "ai.env"), []byte("SECRET_OPENAI_API_KEY=sk-cli-roundtrip\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, ".camunda.yaml"), []byte(`name: cli-roundtrip
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: dmn
  forms: forms
  tests: tests
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "models", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "models", "nested", "order.bpmn"), []byte("<cli/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	archive := filepath.Join(t.TempDir(), "cli-roundtrip.tar.gz")
	backupCmd := newBackupCmd()
	var backupOut bytes.Buffer
	backupCmd.SetOut(&backupOut)
	backupCmd.SetArgs([]string{"-o", archive})
	t.Chdir(project)
	if err := backupCmd.Execute(); err != nil {
		t.Fatalf("backup: %v", err)
	}
	info, err := os.Stat(archive)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("archive mode = %o, want 600", got)
	}

	lab2 := t.TempDir()
	proj2 := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", lab2)
	paths.Reset()

	restoreCmd := newRestoreCmdWith(backup.Restore, runningCheckerFunc(func(context.Context) (bool, error) {
		return false, nil
	}))
	restoreCmd.SetArgs([]string{archive, "--yes", "--project", proj2})
	if err := restoreCmd.Execute(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lab2, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lab2, "ai.env")); !os.IsNotExist(err) {
		t.Fatalf("ai.env restored without include-secrets: %v", err)
	}
	got := readCLIFile(t, filepath.Join(proj2, ".camunda.yaml"))
	if !strings.Contains(got, "cli-roundtrip") {
		t.Fatalf(".camunda.yaml = %q", got)
	}
	got = readCLIFile(t, filepath.Join(proj2, "models", "nested", "order.bpmn"))
	if got != "<cli/>" {
		t.Fatalf("restored bpmn = %q", got)
	}
}

func readCLIFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
