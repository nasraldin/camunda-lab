package env

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestValidateName(t *testing.T) {
	valid := []string{"prod", "prod.eu", "prod-eu", "prod_eu", "a1.b2", strings.Repeat("a", 64)}
	for _, name := range valid {
		t.Run("valid_"+name, func(t *testing.T) {
			if err := ValidateName(name); err != nil {
				t.Fatalf("ValidateName(%q) = %v", name, err)
			}
		})
	}

	invalid := []string{
		"", ".", "..", "prod.", ".prod", "prod..eu",
		"prod/eu", `prod\eu`, "prod%2feu", "prod%2Feu", "prod%5ceu",
		" prod", "prod ", "prod\teu", "prod\neu", "prod\x00eu", "Prod", "prod@eu",
		"-prod", "prod-", "_prod", "prod_",
		"lab", "config", "active-env", "envs",
		strings.Repeat("a", 65),
	}
	for _, name := range invalid {
		t.Run("invalid_"+strings.ReplaceAll(name, "/", "_"), func(t *testing.T) {
			if err := ValidateName(name); err == nil {
				t.Fatalf("ValidateName(%q) unexpectedly succeeded", name)
			}
		})
	}
}

func TestProfilePath(t *testing.T) {
	dir := t.TempDir()
	got, err := ProfilePath(dir, "prod.eu")
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "prod.eu.yaml"); got != want {
		t.Fatalf("ProfilePath = %q, want %q", got, want)
	}
	if _, err := ProfilePath(dir, "../escape"); err == nil {
		t.Fatal("ProfilePath accepted traversal name")
	}
}
