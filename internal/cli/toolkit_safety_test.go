package cli

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestRestoreRequiresExactInteractiveConfirmation(t *testing.T) {
	calls := 0
	restore := func(context.Context, backup.RestoreOptions) (backup.Manifest, error) {
		calls++
		return backup.Manifest{}, nil
	}

	cmd := newRestoreCmdWith(restore, runningCheckerFunc(func(context.Context) (bool, error) {
		return false, nil
	}))
	cmd.SetIn(strings.NewReader("restore\n"))
	cmd.SetArgs([]string{"backup.tar.gz"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("restore accepted confirmation other than exact RESTORE")
	}
	if calls != 0 {
		t.Fatalf("restore called %d times after invalid confirmation", calls)
	}

	cmd = newRestoreCmdWith(restore, runningCheckerFunc(func(context.Context) (bool, error) {
		return false, nil
	}))
	cmd.SetIn(strings.NewReader("RESTORE\n"))
	cmd.SetArgs([]string{"backup.tar.gz"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("restore with exact confirmation: %v", err)
	}
	if calls != 1 {
		t.Fatalf("restore called %d times, want 1", calls)
	}
}

func TestRestoreYesProjectAndForceFlags(t *testing.T) {
	var got backup.RestoreOptions
	restore := func(_ context.Context, opts backup.RestoreOptions) (backup.Manifest, error) {
		got = opts
		return backup.Manifest{}, nil
	}
	checkerCalls := 0
	checker := runningCheckerFunc(func(context.Context) (bool, error) {
		checkerCalls++
		return true, nil
	})
	projectDir := t.TempDir()

	cmd := newRestoreCmdWith(restore, checker)
	cmd.SetArgs([]string{"backup.tar.gz", "-y", "--force", "--project", projectDir})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if got.ArchivePath != "backup.tar.gz" {
		t.Fatalf("ArchivePath = %q", got.ArchivePath)
	}
	if got.ProjectDir != projectDir {
		t.Fatalf("ProjectDir = %q, want %q", got.ProjectDir, projectDir)
	}
	if !got.Force {
		t.Fatal("Force = false, want true")
	}
	if got.Lab == nil {
		t.Fatal("restore did not receive concrete running checker")
	}
	if _, err := got.Lab.Running(context.Background()); err != nil {
		t.Fatal(err)
	}
	if checkerCalls != 1 {
		t.Fatalf("running checker called %d times, want 1", checkerCalls)
	}
}

func TestRestoreReportsRunningLabRefusal(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	archive := writeRestoreArchive(t, "config.yaml", []byte("version: 8.9\n"))

	cmd := newRestoreCmdWith(backup.Restore, runningCheckerFunc(func(context.Context) (bool, error) {
		return true, nil
	}))
	cmd.SetArgs([]string{archive, "--yes"})
	err := cmd.Execute()
	const want = `lab is running; stop it first with "camunda down" or retry with --force`
	if err == nil || err.Error() != want {
		t.Fatalf("restore error = %v, want %q", err, want)
	}
}

func TestRestoreForceStillRejectsMaliciousArchive(t *testing.T) {
	home := t.TempDir()
	projectDir := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	archive := writeRestoreArchive(t, "../config.yaml", []byte("malicious"))

	checkerCalled := false
	cmd := newRestoreCmdWith(backup.Restore, runningCheckerFunc(func(context.Context) (bool, error) {
		checkerCalled = true
		return true, nil
	}))
	cmd.SetArgs([]string{archive, "--yes", "--force", "--project", projectDir})
	err := cmd.Execute()
	if err == nil || !strings.Contains(err.Error(), "unsafe path") {
		t.Fatalf("forced restore error = %v, want unsafe path rejection", err)
	}
	if checkerCalled {
		t.Fatal("--force did not bypass only the running-lab check")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(home), "config.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("malicious destination exists or stat failed: %v", err)
	}
}

func TestEnvUseRequiresExistingProfile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)

	cmd := newEnvCmd()
	cmd.SetArgs([]string{"use", "missing"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("env use accepted a missing profile")
	}
}

func TestEnvRemoveRejectsTraversalName(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	outside := filepath.Join(home, "escape.yaml")
	if err := os.WriteFile(outside, []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newEnvCmd()
	cmd.SetArgs([]string{"remove", "../escape"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("env remove accepted traversal name")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Fatalf("outside file was modified: %v", err)
	}
}

func TestLoadLintIgnoreUsesProjectConfiguration(t *testing.T) {
	root := t.TempDir()
	config := []byte("name: lint-test\nlint:\n  ignore:\n    - bpmn/service-task-retry\n    - bpmn/timer-reachable\n")
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), config, 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "bpmn", "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(nested)

	want := []string{"bpmn/service-task-retry", "bpmn/timer-reachable"}
	got, err := loadLintIgnore()
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("lint ignore = %v, want %v", got, want)
	}
}

func TestLintRejectsMalformedProjectConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte("name: ["), 0o644); err != nil {
		t.Fatal(err)
	}
	model := filepath.Join(root, "process.bpmn")
	source := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="valid"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`
	if err := os.WriteFile(model, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	cmd := newLintCmd()
	cmd.SetArgs([]string{model})
	if err := cmd.Execute(); err == nil || !strings.Contains(err.Error(), "parse") {
		t.Fatalf("lint error = %v, want malformed project configuration error", err)
	}
}

type runningCheckerFunc func(context.Context) (bool, error)

func (f runningCheckerFunc) Running(ctx context.Context) (bool, error) {
	return f(ctx)
}

func writeRestoreArchive(t *testing.T, name string, body []byte) string {
	t.Helper()
	archivePath := filepath.Join(t.TempDir(), "backup.tar.gz")
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	gz := gzip.NewWriter(file)
	tw := tar.NewWriter(gz)
	manifest, err := json.Marshal(backup.Manifest{Version: 1, Files: []string{name}})
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range []struct {
		name string
		body []byte
	}{
		{name: "manifest.json", body: manifest},
		{name: name, body: body},
	} {
		if err := tw.WriteHeader(&tar.Header{
			Name: entry.name, Mode: 0o600, Size: int64(len(entry.body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(entry.body); err != nil {
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
	return archivePath
}
