package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const diffBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/></process></definitions>`

func TestDiffSupportsExplicitFromToAndAgainstModes(t *testing.T) {
	root := t.TempDir()
	before := filepath.Join(root, "before.bpmn")
	after := filepath.Join(root, "after.bpmn")
	writeDiffFile(t, before, diffBPMN)
	writeDiffFile(t, after, diffBPMN)

	for _, args := range [][]string{
		{before, after},
		{"--from", before, "--to", after},
		{before, "--against", after},
	} {
		command := newDiffCmd()
		var output bytes.Buffer
		command.SetOut(&output)
		command.SetErr(&output)
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("diff %v: %v", args, err)
		}
		if output.String() != "No semantic changes.\n" {
			t.Fatalf("diff %v output = %q", args, output.String())
		}
	}
}

func TestDiffBaseReadsProjectRelativeFileFromGit(t *testing.T) {
	root := t.TempDir()
	runDiffGit(t, root, "init", "-q")
	writeDiffFile(t, filepath.Join(root, ".camunda.yaml"), "name: diff\n")
	writeDiffFile(t, filepath.Join(root, "models", "process.bpmn"), diffBPMN)
	runDiffGit(t, root, "add", ".")
	runDiffGit(t, root, "-c", "user.name=Test", "-c", "user.email=test@example.com", "commit", "-m", "base")
	t.Chdir(root)

	command := newDiffCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{"models/process.bpmn", "--base", "HEAD"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if output.String() != "No semantic changes.\n" {
		t.Fatalf("output = %q", output.String())
	}
}

func TestDiffRejectsBulkUnsupportedAndConflictingModes(t *testing.T) {
	root := t.TempDir()
	bpmnPath := filepath.Join(root, "process.bpmn")
	writeDiffFile(t, bpmnPath, diffBPMN)
	for _, args := range [][]string{
		{root, bpmnPath},
		{filepath.Join(root, "decision.dmn"), bpmnPath},
		{bpmnPath, "--from", bpmnPath, "--to", bpmnPath},
		{bpmnPath, bpmnPath, "--base", "HEAD"},
		{bpmnPath, "--against", bpmnPath, "--to", bpmnPath},
	} {
		command := newDiffCmd()
		command.SetArgs(args)
		err := command.Execute()
		if err == nil {
			t.Fatalf("diff accepted %v", args)
		}
		if strings.Contains(err.Error(), root+root) {
			t.Fatalf("unsafe/unbounded error = %v", err)
		}
	}
}

func TestDiffBaseRejectsWorkingTreeSymlinkOutsideProject(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.bpmn")
	writeDiffFile(t, filepath.Join(root, ".camunda.yaml"), "name: diff\n")
	writeDiffFile(t, outside, diffBPMN)
	link := filepath.Join(root, "models", "process.bpmn")
	if err := os.MkdirAll(filepath.Dir(link), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	t.Chdir(root)

	command := newDiffCmd()
	command.SetArgs([]string{"models/process.bpmn", "--base", "HEAD"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "escapes the project") {
		t.Fatalf("error = %v", err)
	}
}

func TestDiffReturnsTypedExitCodes(t *testing.T) {
	root := t.TempDir()
	before := filepath.Join(root, "before.bpmn")
	after := filepath.Join(root, "after.bpmn")
	writeDiffFile(t, before, diffBPMN)
	writeDiffFile(t, after, strings.Replace(diffBPMN, `id="start"`, `id="start" name="Changed"`, 1))

	tests := []struct {
		name string
		args []string
		code int
	}{
		{name: "semantic difference", args: []string{before, after}, code: 1},
		{name: "validation failure", args: []string{before, filepath.Join(root, "missing.bpmn")}, code: 2},
		{name: "mode failure", args: []string{before, "--from", before}, code: 2},
		{name: "flag failure", args: []string{"--not-a-flag"}, code: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := newDiffCmd()
			command.SetArgs(test.args)
			err := command.Execute()
			var exitErr *ExitError
			if !errors.As(err, &exitErr) || ExitCode(err) != test.code || exitErr.Code != test.code {
				t.Fatalf("error = %#v, exit code = %d", err, ExitCode(err))
			}
		})
	}
	if code := ExitCode(errors.New("other command failure")); code != 1 {
		t.Fatalf("default exit code = %d, want preserved code 1", code)
	}
}

func writeDiffFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runDiffGit(t *testing.T, root string, args ...string) {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = root
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, output)
	}
}
