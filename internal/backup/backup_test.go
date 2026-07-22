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
	"strings"
	"testing"
	"time"
)

func TestBackupRestoreRoundTrip(t *testing.T) {
	lab := t.TempDir()
	proj := t.TempDir()
	_ = os.WriteFile(filepath.Join(lab, "config.yaml"), []byte("version: \"8.9\"\nprofile: light\n"), 0o644)
	_ = os.WriteFile(filepath.Join(lab, "ai.env"), []byte("SECRET_OPENAI_API_KEY=sk-test-not-real\n"), 0o600)
	_ = os.MkdirAll(filepath.Join(proj, "bpmn"), 0o755)
	_ = os.WriteFile(filepath.Join(proj, "bpmn", "order.bpmn"), []byte("<xml/>"), 0o644)

	out := filepath.Join(t.TempDir(), "bak.tar.gz")
	m, err := Create(context.Background(), Options{
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
		Lab: runningCheckerFunc(func(context.Context) (bool, error) {
			return false, nil
		}),
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

func TestCreateArchiveEntriesAreDeterministicAndPrivate(t *testing.T) {
	lab := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	writeFile(t, filepath.Join(lab, "ai.env"), "SECRET=value\n", 0o600)
	writeFile(t, filepath.Join(proj, "bpmn", "z.bpmn"), "<z/>", 0o644)
	writeFile(t, filepath.Join(proj, "bpmn", "a.bpmn"), "<a/>", 0o644)

	out1 := filepath.Join(t.TempDir(), "one.tar.gz")
	out2 := filepath.Join(t.TempDir(), "two.tar.gz")
	opts := Options{LabHome: lab, ProjectDir: proj, OutPath: out1, LabVersion: "8.9", LabProfile: "light"}
	if _, err := Create(context.Background(), opts); err != nil {
		t.Fatal(err)
	}
	opts.OutPath = out2
	if _, err := Create(context.Background(), opts); err != nil {
		t.Fatal(err)
	}

	entries1 := readArchiveEntries(t, out1)
	entries2 := readArchiveEntries(t, out2)
	if len(entries1) == 0 {
		t.Fatal("no archive entries")
	}
	for i := range entries1 {
		if i > 0 && entries1[i-1].name >= entries1[i].name {
			t.Fatalf("entries not sorted: %q then %q", entries1[i-1].name, entries1[i].name)
		}
		if entries1[i].mode != 0o600 {
			t.Fatalf("%s mode=%o", entries1[i].name, entries1[i].mode)
		}
		if !entries1[i].modTime.Equal(time.Unix(0, 0).UTC()) {
			t.Fatalf("%s ModTime=%v, want unix epoch UTC", entries1[i].name, entries1[i].modTime)
		}
		if filepath.IsAbs(entries1[i].name) || strings.Contains(entries1[i].name, "..") {
			t.Fatalf("unsafe entry %q", entries1[i].name)
		}
	}
	if len(entries1) != len(entries2) {
		t.Fatalf("entry count drift: %d vs %d", len(entries1), len(entries2))
	}
	for i := range entries1 {
		if entries1[i].name != entries2[i].name || entries1[i].mode != entries2[i].mode {
			t.Fatalf("entry metadata drift at %d: %#v vs %#v", i, entries1[i], entries2[i])
		}
	}
	info, err := os.Stat(out1)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("archive mode = %o, want 600", got)
	}
}

func TestCreateUsesPrivateOutputAndExactManifest(t *testing.T) {
	lab := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	out := filepath.Join(t.TempDir(), "backup.tar.gz")
	writeFile(t, out, "old", 0o666)

	created, err := Create(context.Background(), Options{LabHome: lab, OutPath: out})
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

	if _, err := Create(context.Background(), Options{ProjectDir: project, OutPath: out}); err == nil {
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

type archiveEntryMeta struct {
	name    string
	mode    int64
	modTime time.Time
}

func readArchiveEntries(t *testing.T, archivePath string) []archiveEntryMeta {
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
	var entries []archiveEntryMeta
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, archiveEntryMeta{
			name: header.Name, mode: header.Mode, modTime: header.ModTime.UTC(),
		})
		if _, err := io.Copy(io.Discard, tr); err != nil {
			t.Fatal(err)
		}
	}
	return entries
}
