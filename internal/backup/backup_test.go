package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
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
	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: out,
		LabHome:     lab2,
		ProjectDir:  proj2,
	}); err != nil {
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

func TestCreateUsesPrivateOutputAndExactManifest(t *testing.T) {
	lab := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	out := filepath.Join(t.TempDir(), "backup.tar.gz")
	writeFile(t, out, "old", 0o666)

	created, err := Create(Options{LabHome: lab, OutPath: out})
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(out)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("archive mode = %o, want 600", got)
	}

	archived, payload := readArchiveManifest(t, out)
	if !slices.Equal(archived.Files, payload) {
		t.Fatalf("archived manifest files = %v, payload = %v", archived.Files, payload)
	}
	if !slices.Equal(created.Files, archived.Files) {
		t.Fatalf("returned manifest files = %v, archived = %v", created.Files, archived.Files)
	}
}

func TestCreateRejectsProjectSymlinks(t *testing.T) {
	project := t.TempDir()
	if err := os.MkdirAll(filepath.Join(project, "bpmn"), 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "outside.bpmn")
	writeFile(t, target, "outside", 0o644)
	if err := os.Symlink(target, filepath.Join(project, "bpmn", "linked.bpmn")); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "backup.tar.gz")

	if _, err := Create(Options{ProjectDir: project, OutPath: out}); err == nil {
		t.Fatal("Create() accepted a project symlink")
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output exists after rejection: %v", err)
	}
}

func readArchiveManifest(t *testing.T, archivePath string) (Manifest, []string) {
	t.Helper()
	file, err := os.Open(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	gz, err := gzip.NewReader(file)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var (
		manifest Manifest
		payload  []string
	)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == manifestName {
			if err := json.NewDecoder(tr).Decode(&manifest); err != nil {
				t.Fatal(err)
			}
		} else {
			payload = append(payload, header.Name)
		}
	}
	return manifest, payload
}
