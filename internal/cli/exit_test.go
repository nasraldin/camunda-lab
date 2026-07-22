package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExitCodeMapsClasses(t *testing.T) {
	if ExitCode(nil) != 0 {
		t.Fatalf("nil = %d, want 0", ExitCode(nil))
	}
	if got := ExitCode(&ExitError{Code: 1, Err: errors.New("findings")}); got != 1 {
		t.Fatalf("findings/policy = %d, want 1", got)
	}
	if got := ExitCode(&ExitError{Code: 1, Err: errors.New("diff")}); got != 1 {
		t.Fatalf("diff = %d, want 1", got)
	}
	if got := ExitCode(&ExitError{Code: 1, Err: errors.New("drift")}); got != 1 {
		t.Fatalf("drift = %d, want 1", got)
	}
	if got := ExitCode(&ExitError{Code: 2, Err: errors.New("validation")}); got != 2 {
		t.Fatalf("validation = %d, want 2", got)
	}
	if got := ExitCode(&ExitError{Code: 2, Err: errors.New("upstream")}); got != 2 {
		t.Fatalf("upstream = %d, want 2", got)
	}
	if got := ExitCode(&ExitError{Code: 2, Err: errors.New("partial")}); got != 2 {
		t.Fatalf("partial = %d, want 2", got)
	}
	if got := ExitCode(&ExitError{Code: 2, Err: errors.New("unknown")}); got != 2 {
		t.Fatalf("unknown = %d, want 2", got)
	}
	if got := ExitCode(errors.New("legacy unclassified")); got != 1 {
		t.Fatalf("legacy = %d, want preserved 1", got)
	}
}

func TestExitRunCleanSuccess(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Run(context.Background(), []string{"version"}, &stdout, &stderr); code != 0 {
		t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
	}
	if stdout.Len() == 0 {
		t.Fatal("expected version output")
	}
}

func TestExitRunFindingsDiffAndDriftPolicy(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	invalidLint := filepath.Join(root, "invalid-lint.bpmn")
	before := filepath.Join(root, "before.bpmn")
	after := filepath.Join(root, "after.bpmn")
	writeExitBPMN(t, valid, exitValidBPMN)
	writeExitBPMN(t, invalidLint, exitDisconnectedBPMN)
	writeExitBPMN(t, before, exitValidBPMN)
	writeExitBPMN(t, after, strings.Replace(exitValidBPMN, `id="start"`, `id="start" name="Changed"`, 1))

	t.Run("lint findings", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"lint", invalidLint, "--json"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, stderr=%q stdout=%q", code, stderr.String(), stdout.String())
		}
	})
	t.Run("semantic diff", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{"diff", before, after, "--json"}, &stdout, &stderr)
		if code != 1 {
			t.Fatalf("exit = %d, stderr=%q stdout=%q", code, stderr.String(), stdout.String())
		}
	})
	t.Run("drift policy exit error", func(t *testing.T) {
		if got := ExitCode(&ExitError{Code: 1, Err: errors.New("drift comparison outcome: drift")}); got != 1 {
			t.Fatalf("drift policy = %d, want 1", got)
		}
	})
	t.Run("incident policy exit error", func(t *testing.T) {
		if got := ExitCode(&ExitError{Code: 1, Err: errors.New("incident policy")}); got != 1 {
			t.Fatalf("incident policy = %d, want 1", got)
		}
	})
	_ = valid
}

func TestExitRunValidationUpstreamPartialUnknown(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	writeExitBPMN(t, valid, exitValidBPMN)

	t.Run("validation missing file", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{
			"diff", valid, filepath.Join(root, "missing.bpmn"), "--json",
		}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
		}
	})
	t.Run("validation flag conflict", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{
			"explain", valid, "--json", "--output", filepath.Join(root, "out.md"),
		}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
		}
	})
	t.Run("plan validation missing project", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{
			"plan", "--dir", filepath.Join(root, "no-project"), "--json",
		}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
		}
	})
	t.Run("drift validation missing project", func(t *testing.T) {
		var stdout, stderr bytes.Buffer
		code := Run(context.Background(), []string{
			"drift", "--dir", filepath.Join(root, "no-project"), "--json",
		}, &stdout, &stderr)
		if code != 2 {
			t.Fatalf("exit = %d, stderr=%q", code, stderr.String())
		}
	})
	t.Run("upstream tool exit error", func(t *testing.T) {
		if got := ExitCode(&ExitError{Code: 2, Err: errors.New("upstream")}); got != 2 {
			t.Fatalf("upstream = %d, want 2", got)
		}
	})
	t.Run("partial tool exit error", func(t *testing.T) {
		if got := ExitCode(&ExitError{Code: 2, Err: errors.New("scan incomplete")}); got != 2 {
			t.Fatalf("partial = %d, want 2", got)
		}
	})
	t.Run("unknown tool exit error", func(t *testing.T) {
		if got := ExitCode(&ExitError{Code: 2, Err: errors.New("unknown remote state")}); got != 2 {
			t.Fatalf("unknown = %d, want 2", got)
		}
	})
}

func TestExitJSONHasNoHumanPreamble(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	writeExitBPMN(t, path, exitValidBPMN)

	commands := [][]string{
		{"lint", path, "--json"},
		{"diff", path, path, "--json"},
		{"explain", path, "--json"},
	}
	for _, args := range commands {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			_ = Run(context.Background(), args, &stdout, &stderr)
			raw := stdout.Bytes()
			if len(raw) == 0 {
				t.Fatalf("empty stdout; stderr=%q", stderr.String())
			}
			trimmed := bytes.TrimSpace(raw)
			if trimmed[0] != '{' && trimmed[0] != '[' {
				t.Fatalf("JSON preamble present: %q", raw)
			}
			var payload any
			if err := json.Unmarshal(trimmed, &payload); err != nil {
				t.Fatalf("stdout is not JSON: %v\n%s", err, raw)
			}
			if strings.Contains(stdout.String(), "error:") {
				t.Fatalf("human error leaked into JSON stdout: %q", stdout.String())
			}
		})
	}
}

const exitValidBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="start"/>
    <endEvent id="end"/>
    <sequenceFlow id="flow" sourceRef="start" targetRef="end"/>
  </process>
</definitions>`

const exitDisconnectedBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" isExecutable="true">
    <startEvent id="start"/>
    <endEvent id="end"/>
    <serviceTask id="orphan" name="Orphan"/>
    <sequenceFlow id="flow" sourceRef="start" targetRef="end"/>
  </process>
</definitions>`

func writeExitBPMN(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
