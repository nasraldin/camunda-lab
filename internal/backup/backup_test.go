package backup

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	lab := t.TempDir()
	proj := t.TempDir()
	_ = os.WriteFile(filepath.Join(lab, "config.yaml"), []byte("version: \"8.9\"\nprofile: light\n"), 0o644)
	_ = os.WriteFile(filepath.Join(lab, "ai.env"), []byte("SECRET_OPENAI_API_KEY=sk-test-not-real\n"), 0o600)
	_ = os.MkdirAll(filepath.Join(proj, "bpmn"), 0o755)
	_ = os.WriteFile(filepath.Join(proj, "bpmn", "order.bpmn"), []byte("<xml/>"), 0o644)

	out := filepath.Join(t.TempDir(), "bak.tar.gz")
	m, err := Create(Options{
		LabHome:    lab,
		ProjectDir: proj,
		OutPath:    out,
		LabVersion: "8.9",
		LabProfile: "light",
	})
	if err != nil {
		t.Fatal(err)
	}
	if m.IncludesSecrets {
		t.Fatal("should omit secrets by default")
	}
	if len(m.AISecretKeys) == 0 {
		t.Fatal("expected key names")
	}

	lab2 := t.TempDir()
	proj2 := t.TempDir()
	if _, err := Restore(out, lab2, proj2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lab2, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(proj2, "bpmn", "order.bpmn")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(lab2, "ai.env")); err == nil {
		t.Fatal("ai.env should not restore without include-secrets")
	}
}
