package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestPlatformCommandsAdvertiseFlags(t *testing.T) {
	tests := map[string][]string{
		"env add":         {"kind", "orchestration", "client-id-env", "client-secret-env", "token-url", "token-url-env", "audience", "scope"},
		"plan":            {"dir", "env", "json"},
		"drift":           {"dir", "ref", "env", "json"},
		"backup":          {"output", "include-secrets"},
		"restore":         {"yes", "force", "project"},
		"incidents":       {"env", "limit"},
		"incidents retry": {"yes", "dry-run"},
		"trace":           {"follow", "json", "env", "interval", "timeout", "idle-stop", "max-events"},
	}
	root := NewRoot()
	for name, flags := range tests {
		parts := strings.Fields(name)
		command, _, err := root.Find(parts)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, flag := range flags {
			if command.Flags().Lookup(flag) == nil && command.PersistentFlags().Lookup(flag) == nil {
				t.Errorf("%s missing --%s", name, flag)
			}
		}
	}
}

func TestRestoreUsesServiceRunningCheckerAndForce(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("version: \"8.9\"\nprofile: light\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	archive := filepath.Join(t.TempDir(), "lab.tar.gz")
	if _, err := backup.Create(context.Background(), backup.Options{
		LabHome: home, OutPath: archive, LabVersion: "8.9", LabProfile: "light",
	}); err != nil {
		t.Fatal(err)
	}

	calls := 0
	checker := runningCheckerFunc(func(context.Context) (bool, error) {
		calls++
		return true, nil
	})
	service := backup.NewService(checker)
	cmd := newRestoreCmdWith(service.Restore, checker)
	cmd.SetArgs([]string{archive, "--yes"})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "lab is running") {
		t.Fatalf("restore without force error = %v", err)
	}
	if calls == 0 {
		t.Fatal("running checker was not consulted")
	}

	lab2 := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", lab2)
	paths.Reset()
	calls = 0
	cmd = newRestoreCmdWith(service.Restore, checker)
	cmd.SetArgs([]string{archive, "--yes", "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("force restore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(lab2, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}
