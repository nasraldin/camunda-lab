package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestDefaults(t *testing.T) {
	c := config.Defaults()
	if c.Profile != "light" {
		t.Fatalf("profile=%q", c.Profile)
	}
	if c.Resources != "balanced" {
		t.Fatalf("resources=%q", c.Resources)
	}
	if c.ComposeProject != "camunda-lab" {
		t.Fatalf("project=%q", c.ComposeProject)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", dir)
	paths.Reset()

	c := config.Defaults()
	c.Version = "8.8"
	if err := config.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "8.8" || got.Profile != "light" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}

func TestAIEnabledRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()

	c := config.Defaults()
	c.Version = "8.9"
	c.AI.Enabled = true
	if err := config.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if !got.AI.Enabled {
		t.Fatal("expected ai.enabled")
	}
}

func TestActiveEnvironmentRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()

	c := config.Defaults()
	c.ActiveEnv = "prod"
	if err := config.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ActiveEnv != "prod" {
		t.Fatalf("active environment = %q, want prod", got.ActiveEnv)
	}
	info, err := os.Stat(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("config permissions = %o, want 600", got)
	}
}

func TestLoadSurfacesCorruptConfig(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("activeEnv: ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(); err == nil {
		t.Fatal("Load reset corrupt config instead of surfacing it")
	}
}

func TestSavePreservesUnknownFieldsAndComments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	path := filepath.Join(home, "config.yaml")
	original := "# retained comment\nversion: \"8.9\"\ncustom:\n  nested: keep\nprofile: light\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.Profile = "full"
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# retained comment") || !strings.Contains(text, "nested: keep") {
		t.Fatalf("unknown data/comment lost:\n%s", text)
	}
}

func TestSaveRecursivelyPreservesNestedUnknownFieldsAndComments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	path := filepath.Join(home, "config.yaml")
	original := "version: \"8.9\"\nprofile: light\nresources: balanced\nhost: localhost\ncompose_project: camunda-lab\nai:\n  # nested ai comment\n  enabled: false\n  providerHint: keep\n"
	if err := os.WriteFile(path, []byte(original), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	cfg.AI.Enabled = true
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	text := readConfigText(t, path)
	if !strings.Contains(text, "# nested ai comment") || !strings.Contains(text, "providerHint: keep") ||
		!strings.Contains(text, "enabled: true") {
		t.Fatalf("nested config data was not recursively merged:\n%s", text)
	}
}

func TestConcurrentConfigUpdatesDoNotLoseFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := config.Save(config.Defaults()); err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		if err := config.UpdateScalar(paths.ConfigFile(), "activeEnv", "prod", 0o600); err != nil {
			t.Error(err)
		}
	}()
	go func() {
		defer wg.Done()
		cfg, err := config.Load()
		if err != nil {
			t.Error(err)
			return
		}
		cfg.Resources = "power"
		if err := config.Save(cfg); err != nil {
			t.Error(err)
		}
	}()
	wg.Wait()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ActiveEnv != "prod" || cfg.Resources != "power" {
		t.Fatalf("lost concurrent update: %+v", cfg)
	}
}

func TestConfigLockRejectsSymlink(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	outside := filepath.Join(t.TempDir(), "lock")
	if err := os.WriteFile(outside, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, paths.ConfigFile()+".lock"); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(config.Defaults()); err == nil {
		t.Fatal("Save accepted symlink config lock")
	}
}

func readConfigText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}
