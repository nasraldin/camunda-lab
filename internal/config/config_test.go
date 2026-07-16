package config_test

import (
	"os"
	"path/filepath"
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
