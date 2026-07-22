package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type archiveEntry struct {
	name     string
	body     []byte
	typeflag byte
	linkname string
	size     *int64
}

type runningCheckerFunc func(context.Context) (bool, error)

func (f runningCheckerFunc) Running(ctx context.Context) (bool, error) {
	return f(ctx)
}

func stoppedLabChecker() RunningChecker {
	return runningCheckerFunc(func(context.Context) (bool, error) { return false, nil })
}

func TestRestoreRejectsUnsafeArchivesWithoutMutation(t *testing.T) {
	tooLarge := int64(5)
	negative := int64(-1)
	cases := []struct {
		name    string
		entries []archiveEntry
		limits  Limits
	}{
		{name: "absolute path", entries: []archiveEntry{{name: "/config.yaml", body: []byte("bad")}}},
		{name: "parent traversal", entries: []archiveEntry{{name: "../config.yaml", body: []byte("bad")}}},
		{name: "backslash", entries: []archiveEntry{{name: `project\bpmn\bad.bpmn`, body: []byte("bad")}}},
		{name: "mixed separators", entries: []archiveEntry{{name: `project/bpmn\..\..\config.yaml`, body: []byte("bad")}}},
		{name: "symlink", entries: []archiveEntry{{name: "config.yaml", typeflag: tar.TypeSymlink, linkname: "elsewhere"}}},
		{name: "hardlink", entries: []archiveEntry{{name: "config.yaml", typeflag: tar.TypeLink, linkname: "elsewhere"}}},
		{name: "device", entries: []archiveEntry{{name: "config.yaml", typeflag: tar.TypeChar}}},
		{name: "fifo", entries: []archiveEntry{{name: "config.yaml", typeflag: tar.TypeFifo}}},
		{name: "duplicate destination", entries: []archiveEntry{
			{name: "config.yaml", body: []byte("first")},
			{name: "config.yaml", body: []byte("second")},
		}},
		{name: "negative size", entries: []archiveEntry{{name: "config.yaml", size: &negative}}},
		{name: "oversized entry", entries: []archiveEntry{{name: "config.yaml", body: []byte("12345"), size: &tooLarge}}, limits: Limits{MaxEntries: 10, MaxFileBytes: 4, MaxTotalBytes: 20}},
		{name: "too many entries", entries: []archiveEntry{
			{name: "config.yaml", body: []byte("bad")},
			{name: "ai.env", body: []byte("SECRET=bad")},
		}, limits: Limits{MaxEntries: 1, MaxFileBytes: 20, MaxTotalBytes: 20}},
		{name: "excessive total size", entries: []archiveEntry{
			{name: "config.yaml", body: []byte("1234")},
			{name: "ai.env", body: []byte("5678")},
		}, limits: Limits{MaxEntries: 10, MaxFileBytes: 10, MaxTotalBytes: 7}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labHome, projectDir := seededRestoreDestinations(t)
			archive := writeTestArchive(t, tc.entries, manifestForEntries(tc.entries))

			_, err := Restore(context.Background(), RestoreOptions{
				ArchivePath: archive,
				LabHome:     labHome,
				ProjectDir:  projectDir,
				Limits:      tc.limits,
				Lab:         stoppedLabChecker(),
			})
			if err == nil {
				t.Fatal("Restore() error = nil, want rejection")
			}
			assertRestoreDestinationsUnchanged(t, labHome, projectDir)
			assertNoRestoreArtifacts(t, filepath.Dir(labHome), filepath.Dir(projectDir))
		})
	}
}

func TestRestoreRejectsUnsupportedArchiveTypeWithoutMutation(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	archive := filepath.Join(t.TempDir(), "backup.zip")
	if err := os.WriteFile(archive, []byte("not a gzip tar archive"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive,
		LabHome:     labHome,
		ProjectDir:  projectDir,
		Lab:         stoppedLabChecker(),
	}); err == nil {
		t.Fatal("Restore() accepted an unsupported archive type")
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)
	assertNoRestoreArtifacts(t, filepath.Dir(labHome), filepath.Dir(projectDir))
}

func TestRestoreRejectsDestinationConflictWithoutMutation(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	writeFile(t, filepath.Join(projectDir, "dmn"), "not-a-directory", 0o644)
	entries := []archiveEntry{
		{name: "config.yaml", body: []byte("new-config")},
		{name: "project/dmn/rule.dmn", body: []byte("<new/>")},
	}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive,
		LabHome:     labHome,
		ProjectDir:  projectDir,
		Lab:         stoppedLabChecker(),
	}); err == nil {
		t.Fatal("Restore() accepted an incompatible destination")
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)
	if got := readFile(t, filepath.Join(projectDir, "dmn")); got != "not-a-directory" {
		t.Fatalf("destination blocker mutated: %q", got)
	}
	assertNoRestoreArtifacts(t, filepath.Dir(labHome), filepath.Dir(projectDir))
}

func TestRestoreValidatesManifestBeforeMutation(t *testing.T) {
	validPayload := []archiveEntry{{name: "config.yaml", body: []byte("new-config")}}
	cases := []struct {
		name     string
		manifest *Manifest
		raw      []byte
		entries  []archiveEntry
	}{
		{name: "missing manifest", entries: validPayload},
		{name: "invalid manifest", raw: []byte("{"), entries: validPayload},
		{name: "unsupported version", manifest: &Manifest{Version: 2, Files: []string{"config.yaml"}}, entries: validPayload},
		{name: "payload mismatch missing file", manifest: &Manifest{Version: 1}, entries: validPayload},
		{name: "payload mismatch extra file", manifest: &Manifest{Version: 1, Files: []string{"config.yaml", "ai.env"}}, entries: validPayload},
		{name: "duplicate manifest", manifest: &Manifest{Version: 1, Files: []string{"config.yaml"}}, entries: append(validPayload,
			archiveEntry{name: "manifest.json", body: mustJSON(t, Manifest{Version: 1, Files: []string{"config.yaml"}})})},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			labHome, projectDir := seededRestoreDestinations(t)
			archive := writeTestArchiveWithManifest(t, tc.entries, tc.manifest, tc.raw)

			if _, err := Restore(context.Background(), RestoreOptions{
				ArchivePath: archive,
				LabHome:     labHome,
				ProjectDir:  projectDir,
				Lab:         stoppedLabChecker(),
			}); err == nil {
				t.Fatal("Restore() error = nil, want manifest rejection")
			}
			assertRestoreDestinationsUnchanged(t, labHome, projectDir)
			assertNoRestoreArtifacts(t, filepath.Dir(labHome), filepath.Dir(projectDir))
		})
	}
}

func TestRestoreRequiresStoppedLabUnlessForced(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	entries := []archiveEntry{{name: "config.yaml", body: []byte("new-config")}}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))
	checker := runningCheckerFunc(func(context.Context) (bool, error) { return true, nil })

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir, Lab: checker,
	}); err == nil {
		t.Fatal("Restore() error = nil for running lab")
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir, Lab: checker, Force: true,
	}); err != nil {
		t.Fatalf("forced Restore() error = %v", err)
	}
	if got := readFile(t, filepath.Join(labHome, "config.yaml")); got != "new-config" {
		t.Fatalf("config.yaml = %q, want new-config", got)
	}
}

func TestRestoreRefusesNilLabUnlessForced(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	entries := []archiveEntry{{name: "config.yaml", body: []byte("new-config")}}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))

	_, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir,
	})
	if err == nil || !strings.Contains(err.Error(), "could not determine whether the lab is running") {
		t.Fatalf("Restore() error = %v, want missing running checker refusal", err)
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir, Force: true,
	}); err != nil {
		t.Fatalf("forced Restore() without Lab error = %v", err)
	}
	if got := readFile(t, filepath.Join(labHome, "config.yaml")); got != "new-config" {
		t.Fatalf("config.yaml = %q, want new-config", got)
	}
}

func TestRestoreRequiresSuccessfulRunningCheck(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	entries := []archiveEntry{{name: "config.yaml", body: []byte("new-config")}}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))
	checker := runningCheckerFunc(func(context.Context) (bool, error) {
		return false, errors.New("status unavailable")
	})

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir, Lab: checker,
	}); err == nil {
		t.Fatal("Restore() error = nil for failed running check")
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)
}

func TestRestoreValidatesArchiveEvenWhenForced(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	archive := writeTestArchive(t,
		[]archiveEntry{{name: "../config.yaml", body: []byte("bad")}},
		Manifest{Version: 1, Files: []string{"../config.yaml"}},
	)

	if _, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir, Force: true,
	}); err == nil {
		t.Fatal("forced Restore() accepted unsafe archive")
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)
}

func TestRestoreValidArchive(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	entries := []archiveEntry{
		{name: "config.yaml", body: []byte("new-config")},
		{name: "ai.env", body: []byte("SECRET=new")},
		{name: "project/bpmn/order.bpmn", body: []byte("<new/>")},
	}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))

	manifest, err := Restore(context.Background(), RestoreOptions{
		ArchivePath: archive,
		LabHome:     labHome,
		ProjectDir:  projectDir,
		Lab:         stoppedLabChecker(),
	})
	if err != nil {
		t.Fatalf("Restore() error = %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("manifest version = %d, want 1", manifest.Version)
	}
	if got := readFile(t, filepath.Join(labHome, "config.yaml")); got != "new-config" {
		t.Fatalf("config.yaml = %q", got)
	}
	if got := readFile(t, filepath.Join(labHome, "ai.env")); got != "SECRET=new" {
		t.Fatalf("ai.env = %q", got)
	}
	aiInfo, err := os.Stat(filepath.Join(labHome, "ai.env"))
	if err != nil {
		t.Fatal(err)
	}
	if got := aiInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("ai.env mode = %o, want 600", got)
	}
	if got := readFile(t, filepath.Join(projectDir, "bpmn", "order.bpmn")); got != "<new/>" {
		t.Fatalf("project file = %q", got)
	}
	assertNoRestoreArtifacts(t, filepath.Dir(labHome), filepath.Dir(projectDir))
}

func seededRestoreDestinations(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	labHome := filepath.Join(root, "lab")
	projectDir := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(projectDir, "bpmn"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(labHome, "config.yaml"), "old-config", 0o644)
	writeFile(t, filepath.Join(labHome, "ai.env"), "SECRET=old", 0o600)
	writeFile(t, filepath.Join(projectDir, "bpmn", "order.bpmn"), "<old/>", 0o644)
	return labHome, projectDir
}

func assertRestoreDestinationsUnchanged(t *testing.T, labHome, projectDir string) {
	t.Helper()
	if got := readFile(t, filepath.Join(labHome, "config.yaml")); got != "old-config" {
		t.Errorf("config.yaml mutated: %q", got)
	}
	if got := readFile(t, filepath.Join(labHome, "ai.env")); got != "SECRET=old" {
		t.Errorf("ai.env mutated: %q", got)
	}
	if got := readFile(t, filepath.Join(projectDir, "bpmn", "order.bpmn")); got != "<old/>" {
		t.Errorf("project file mutated: %q", got)
	}
}

func assertNoRestoreArtifacts(t *testing.T, dirs ...string) {
	t.Helper()
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if strings.Contains(entry.Name(), ".restore-") {
				t.Errorf("restore artifact remains: %s", entry.Name())
			}
		}
	}
}

func writeTestArchive(t *testing.T, entries []archiveEntry, manifest Manifest) string {
	t.Helper()
	return writeTestArchiveWithManifest(t, entries, &manifest, nil)
}

func writeTestArchiveWithManifest(t *testing.T, entries []archiveEntry, manifest *Manifest, rawManifest []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backup.tar.gz")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	for _, entry := range entries {
		size := int64(len(entry.body))
		if entry.size != nil {
			size = *entry.size
		}
		header := &tar.Header{
			Name: entry.name, Mode: 0o600, Size: size,
			Typeflag: entry.typeflag, Linkname: entry.linkname,
		}
		if err := tw.WriteHeader(header); err != nil {
			// A negative size is malformed before it reaches Restore. Emit a
			// syntactically valid archive with a negative PAX size instead.
			if size < 0 {
				header.Size = 0
				header.PAXRecords = map[string]string{"size": "-1"}
				if err := tw.WriteHeader(header); err != nil {
					t.Fatal(err)
				}
				continue
			}
			t.Fatal(err)
		}
		if len(entry.body) > 0 {
			if _, err := tw.Write(entry.body); err != nil {
				t.Fatal(err)
			}
		}
	}
	if manifest != nil || rawManifest != nil {
		data := rawManifest
		if rawManifest == nil {
			data = mustJSON(t, *manifest)
		}
		if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o600, Size: int64(len(data))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.size != nil && *entry.size < 0 {
			writeNegativeSizeHeader(t, path)
			break
		}
	}
	return path
}

func writeNegativeSizeHeader(t *testing.T, archivePath string) {
	t.Helper()
	compressed, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	if err := reader.Close(); err != nil {
		t.Fatal(err)
	}
	if len(raw) < 512 {
		t.Fatal("test archive has no tar header")
	}
	for index := 124; index < 136; index++ {
		raw[index] = 0xff
	}
	for index := 148; index < 156; index++ {
		raw[index] = ' '
	}
	var checksum int
	for _, value := range raw[:512] {
		checksum += int(value)
	}
	copy(raw[148:156], fmt.Sprintf("%06o\x00 ", checksum))

	var output bytes.Buffer
	writer := gzip.NewWriter(&output)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(archivePath, output.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
}

func manifestForEntries(entries []archiveEntry) Manifest {
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		files = append(files, entry.name)
	}
	return Manifest{Version: 1, Files: files}
}

func mustJSON(t *testing.T, value Manifest) []byte {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeFile(t *testing.T, path, body string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), mode); err != nil {
		t.Fatal(err)
	}
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(bytes.Clone(data))
}
