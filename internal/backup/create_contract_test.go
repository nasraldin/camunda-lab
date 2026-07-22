package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestCreateIncludesProjectCamundaYAML(t *testing.T) {
	lab := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	writeFile(t, filepath.Join(proj, ".camunda.yaml"),
		"name: orders\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n", 0o644)
	writeFile(t, filepath.Join(proj, "bpmn", "order.bpmn"), "<xml/>", 0o644)

	out := filepath.Join(t.TempDir(), "bak.tar.gz")
	m, err := Create(context.Background(), Options{
		LabHome: lab, ProjectDir: proj, OutPath: out, LabVersion: "8.9", LabProfile: "light",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(m.Files, "project/.camunda.yaml") {
		t.Fatalf("manifest files = %v, want project/.camunda.yaml", m.Files)
	}
	archived, payload := readArchiveManifest(t, out)
	if !slices.Contains(payload, "project/.camunda.yaml") {
		t.Fatalf("archive payload = %v, want project/.camunda.yaml", payload)
	}
	if !slices.Equal(archived.Files, payload) {
		t.Fatalf("manifest/payload mismatch: %v vs %v", archived.Files, payload)
	}
}

func TestCreateUsesConfiguredRecursiveResourcePaths(t *testing.T) {
	lab := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	writeFile(t, filepath.Join(proj, ".camunda.yaml"), `name: custom
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: decisions
  forms: ui/forms
  tests: tests
`, 0o644)
	writeFile(t, filepath.Join(proj, "models", "nested", "order.bpmn"), "<bpmn/>", 0o644)
	writeFile(t, filepath.Join(proj, "decisions", "rule.dmn"), "<dmn/>", 0o644)
	writeFile(t, filepath.Join(proj, "ui", "forms", "start.form"), `{"type":"form"}`, 0o644)
	// Default folders must not be required / scanned when config overrides them.
	writeFile(t, filepath.Join(proj, "bpmn", "ignored.bpmn"), "<ignored/>", 0o644)

	out := filepath.Join(t.TempDir(), "bak.tar.gz")
	m, err := Create(context.Background(), Options{
		LabHome: lab, ProjectDir: proj, OutPath: out,
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"project/.camunda.yaml",
		"project/models/nested/order.bpmn",
		"project/decisions/rule.dmn",
		"project/ui/forms/start.form",
	}
	for _, name := range want {
		if !slices.Contains(m.Files, name) {
			t.Fatalf("manifest files = %v, missing %s", m.Files, name)
		}
	}
	if slices.Contains(m.Files, "project/bpmn/ignored.bpmn") {
		t.Fatalf("hardcoded bpmn/ path was scanned despite configured models/: %v", m.Files)
	}
}

func TestCreateRejectsOverlappingResourcePaths(t *testing.T) {
	lab := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)

	tests := []struct {
		name   string
		config string
	}{
		{
			name: "identical bpmn and dmn",
			config: `name: overlap
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: models
  forms: forms
  tests: tests
`,
		},
		{
			name: "nested dmn under bpmn",
			config: `name: nested
camundaVersion: "8.9"
paths:
  bpmn: models
  dmn: models/dmn
  forms: forms
  tests: tests
`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			proj := t.TempDir()
			writeFile(t, filepath.Join(proj, ".camunda.yaml"), tt.config, 0o644)
			writeFile(t, filepath.Join(proj, "models", "order.bpmn"), "<bpmn/>", 0o644)
			writeFile(t, filepath.Join(proj, "models", "dmn", "rule.dmn"), "<dmn/>", 0o644)
			writeFile(t, filepath.Join(proj, "forms", "start.form"), "{}", 0o644)

			out := filepath.Join(t.TempDir(), "bak.tar.gz")
			_, err := Create(context.Background(), Options{
				LabHome: lab, ProjectDir: proj, OutPath: out,
			})
			if err == nil {
				t.Fatal("Create() error = nil, want overlapping resource path rejection")
			}
			if !strings.Contains(err.Error(), "overlap") {
				t.Fatalf("Create() error = %v, want overlap message", err)
			}
			if _, statErr := os.Stat(out); !os.IsNotExist(statErr) {
				t.Fatalf("output archive published despite overlap: %v", statErr)
			}
		})
	}
}

func TestCreateOmitsSecretsUnlessOptedIn(t *testing.T) {
	lab := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	writeFile(t, filepath.Join(lab, "ai.env"), "SECRET_OPENAI_API_KEY=sk-test-not-real\n", 0o600)

	outOmit := filepath.Join(t.TempDir(), "omit.tar.gz")
	omitted, err := Create(context.Background(), Options{LabHome: lab, OutPath: outOmit})
	if err != nil {
		t.Fatal(err)
	}
	if omitted.IncludesSecrets {
		t.Fatal("IncludesSecrets = true without opt-in")
	}
	if !slices.Contains(omitted.AISecretKeys, "SECRET_OPENAI_API_KEY") {
		t.Fatalf("AISecretKeys = %v", omitted.AISecretKeys)
	}
	if slices.Contains(omitted.Files, "ai.env") {
		t.Fatal("ai.env present without include-secrets")
	}
	if !slices.Contains(omitted.Files, "ai.keys.json") {
		t.Fatal("ai.keys.json missing when secrets omitted")
	}
	_, payloadOmit := readArchiveManifest(t, outOmit)
	for _, name := range payloadOmit {
		if name == "ai.env" {
			t.Fatal("archive contains ai.env without opt-in")
		}
	}
	body := archiveFileBody(t, outOmit, "ai.keys.json")
	if strings.Contains(string(body), "sk-test-not-real") {
		t.Fatal("secret value leaked into ai.keys.json")
	}
	var meta map[string]any
	if err := json.Unmarshal(body, &meta); err != nil {
		t.Fatal(err)
	}

	outInclude := filepath.Join(t.TempDir(), "include.tar.gz")
	included, err := Create(context.Background(), Options{
		LabHome: lab, OutPath: outInclude, IncludeSecrets: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !included.IncludesSecrets {
		t.Fatal("IncludesSecrets = false with opt-in")
	}
	if !slices.Contains(included.Files, "ai.env") {
		t.Fatal("ai.env missing with include-secrets")
	}
	if slices.Contains(included.Files, "ai.keys.json") {
		t.Fatal("ai.keys.json should not be written when secrets are included")
	}
	if got := string(archiveFileBody(t, outInclude, "ai.env")); !strings.Contains(got, "sk-test-not-real") {
		t.Fatalf("ai.env body = %q", got)
	}
}

func TestCreateCancelsWithContext(t *testing.T) {
	lab := t.TempDir()
	proj := t.TempDir()
	writeFile(t, filepath.Join(lab, "config.yaml"), "version: \"8.9\"\n", 0o644)
	writeFile(t, filepath.Join(proj, ".camunda.yaml"),
		"name: cancel\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n", 0o644)
	for i := 0; i < 20; i++ {
		writeFile(t, filepath.Join(proj, "bpmn", "file-"+strings.Repeat("x", i+1)+".bpmn"), "<xml/>", 0o644)
	}
	out := filepath.Join(t.TempDir(), "cancel.tar.gz")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := Create(ctx, Options{LabHome: lab, ProjectDir: proj, OutPath: out})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Create() error = %v, want context.Canceled", err)
	}
	if _, err := os.Stat(out); !os.IsNotExist(err) {
		t.Fatalf("output exists after cancel: %v", err)
	}
}

func TestValidateArchiveDryRunDoesNotMutate(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	entries := []archiveEntry{
		{name: "config.yaml", body: []byte("new-config")},
		{name: "project/bpmn/order.bpmn", body: []byte("<new/>")},
	}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))

	manifest, err := ValidateArchive(RestoreOptions{
		ArchivePath: archive,
		LabHome:     labHome,
		ProjectDir:  projectDir,
	})
	if err != nil {
		t.Fatalf("ValidateArchive() error = %v", err)
	}
	if manifest.Version != 1 {
		t.Fatalf("version = %d", manifest.Version)
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)
	assertNoRestoreArtifacts(t, filepath.Dir(labHome), filepath.Dir(projectDir))

	bad := writeTestArchive(t,
		[]archiveEntry{{name: "../config.yaml", body: []byte("bad")}},
		Manifest{Version: 1, Files: []string{"../config.yaml"}},
	)
	if _, err := ValidateArchive(RestoreOptions{
		ArchivePath: bad, LabHome: labHome, ProjectDir: projectDir,
	}); err == nil {
		t.Fatal("ValidateArchive() accepted unsafe archive")
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)
}

func TestServiceRestoreRunningCheckErrors(t *testing.T) {
	labHome, projectDir := seededRestoreDestinations(t)
	entries := []archiveEntry{{name: "config.yaml", body: []byte("new-config")}}
	archive := writeTestArchive(t, entries, manifestForEntries(entries))
	svc := NewService(runningCheckerFunc(func(context.Context) (bool, error) {
		return true, nil
	}))

	_, err := svc.Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir,
	})
	const want = `lab is running; stop it first with "camunda down" or retry with --force`
	if err == nil || err.Error() != want {
		t.Fatalf("Service.Restore() error = %v, want %q", err, want)
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)

	failing := NewService(runningCheckerFunc(func(context.Context) (bool, error) {
		return false, errors.New("status unavailable")
	}))
	_, err = failing.Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir,
	})
	if err == nil || !strings.Contains(err.Error(), "could not determine whether the lab is running") {
		t.Fatalf("Service.Restore() error = %v, want running-check failure", err)
	}
	assertRestoreDestinationsUnchanged(t, labHome, projectDir)

	forced := NewService(runningCheckerFunc(func(context.Context) (bool, error) {
		return true, nil
	}))
	if _, err := forced.Restore(context.Background(), RestoreOptions{
		ArchivePath: archive, LabHome: labHome, ProjectDir: projectDir, Force: true,
	}); err != nil {
		t.Fatalf("forced Service.Restore() error = %v", err)
	}
	if got := readFile(t, filepath.Join(labHome, "config.yaml")); got != "new-config" {
		t.Fatalf("config.yaml = %q", got)
	}
}

func archiveFileBody(t *testing.T, archivePath, name string) []byte {
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
	for {
		header, err := tr.Next()
		if err == io.EOF {
			t.Fatalf("missing archive entry %s", name)
		}
		if err != nil {
			t.Fatal(err)
		}
		if header.Name == name {
			data, err := io.ReadAll(tr)
			if err != nil {
				t.Fatal(err)
			}
			return data
		}
	}
}
