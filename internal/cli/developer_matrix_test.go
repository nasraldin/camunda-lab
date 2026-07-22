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

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func TestDeveloperCLIJSONContractMatrix(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	changed := filepath.Join(root, "changed.bpmn")
	if err := os.WriteFile(valid, []byte(validCLIBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte(strings.Replace(
		validCLIBPMN,
		`id="end"`,
		`id="end" name="changed"`,
		1,
	)), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("lint", func(t *testing.T) {
		output, code := executeDeveloperJSON(t, newLintCmd(), valid, "--json")
		if code != 0 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(t, output, "status", "complete", "warnings", "inputs", "documents", "findings")
		var result toolkit.LintResult
		decodeJSONContract(t, output, &result)
		if result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || result.Inputs == nil || result.Documents == nil ||
			result.Findings == nil {
			t.Fatalf("incomplete lint schema: %+v", result)
		}
	})

	t.Run("diff", func(t *testing.T) {
		output, code := executeDeveloperJSON(t, newDiffCmd(), valid, changed, "--json")
		if code != 1 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(t, output, "status", "complete", "warnings", "before", "after", "changes")
		var result toolkit.DiffResult
		decodeJSONContract(t, output, &result)
		if result.Status != toolkit.StatusFailed || !result.Complete ||
			result.Warnings == nil || len(result.Changes) == 0 {
			t.Fatalf("incomplete diff schema: %+v", result)
		}
		assertMarshaledFields(t, result.Changes[0], "kind", "beforeProcessId", "afterProcessId", "change")
	})

	t.Run("explain", func(t *testing.T) {
		output, code := executeDeveloperJSON(t, newExplainCmd(), valid, "--json")
		if code != 0 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(t, output, "status", "complete", "warnings", "document", "processes")
		var result toolkit.ExplainResult
		decodeJSONContract(t, output, &result)
		if result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || len(result.Processes) != 1 {
			t.Fatalf("incomplete explain schema: %+v", result)
		}
		assertMarshaledFields(t, result.Processes[0], "processId", "explanation")
		assertMarshaledFields(
			t,
			result.Processes[0].Explanation,
			"business",
			"technical",
			"risks",
			"missingPaths",
			"optimizationSuggestions",
		)
	})

	t.Run("review", func(t *testing.T) {
		output, code := executeDeveloperJSON(t, newReviewCmd(), valid, "--json")
		if code != 0 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(
			t,
			output,
			"status",
			"complete",
			"warnings",
			"inputs",
			"documents",
			"processes",
			"findings",
			"aiStatus",
		)
		var result toolkit.ReviewResult
		decodeJSONContract(t, output, &result)
		if result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || result.Inputs == nil || result.Documents == nil ||
			result.Processes == nil || result.Findings == nil ||
			result.AIStatus != toolkit.AIStatusDisabled {
			t.Fatalf("incomplete review schema: %+v", result)
		}
		assertMarshaledFields(t, result.Processes[0], "processId", "review")
		assertMarshaledFields(t, result.Processes[0].Review, "findings", "aiStatus")
	})

	t.Run("generate", func(t *testing.T) {
		output, code := executeDeveloperJSON(
			t,
			newTestCmd(),
			"generate",
			valid,
			"--lang",
			"python",
			"--output",
			filepath.Join(root, "generated-json"),
			"--json",
		)
		if code != 0 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(t, output, "status", "complete", "warnings", "document", "artifacts")
		var result toolkit.GenerateResult
		decodeJSONContract(t, output, &result)
		if result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || len(result.Artifacts) == 0 {
			t.Fatalf("incomplete generate schema: %+v", result)
		}
		assertMarshaledFields(t, result.Artifacts[0], "path", "mediaType", "content")
	})

	t.Run("scan partial", func(t *testing.T) {
		scanRoot := filepath.Join(root, "partial-scan")
		if err := os.Mkdir(scanRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(scanRoot, "missing"), filepath.Join(scanRoot, "broken.env")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		output, code := executeDeveloperJSON(t, newScanCmd(), scanRoot, "--json")
		if code != 2 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(
			t,
			output,
			"status",
			"complete",
			"warnings",
			"scannedRoots",
			"failedRoots",
			"findings",
			"issues",
			"stats",
		)
		var result toolkit.ScanResult
		decodeJSONContract(t, output, &result)
		if result.Status != toolkit.StatusPartial || result.Complete ||
			result.Warnings == nil || result.ScannedRoots == nil || result.FailedRoots == nil ||
			result.Findings == nil || len(result.Issues) == 0 {
			t.Fatalf("incomplete scan schema: %+v", result)
		}
		assertMarshaledFields(t, result.Issues[0], "path", "kind", "message")
		assertMarshaledFields(t, result.Stats, "discovered", "scanned", "ignored", "errored")
	})

	t.Run("doctor", func(t *testing.T) {
		deps := deterministicDoctorDependencies(doctor.StatusPass)
		output, code := executeDeveloperJSON(
			t,
			newDoctorCmdWithDependencies(deps),
			"--deep",
			"--json",
		)
		if code != 0 {
			t.Fatalf("exit = %d, output = %s", code, output)
		}
		assertJSONFields(t, output, "ok", "checks")
		var result doctor.DeepReport
		decodeJSONContract(t, output, &result)
		if !result.OK || len(result.Checks) != 1 {
			t.Fatalf("incomplete doctor schema: %+v", result)
		}
		assertMarshaledFields(
			t,
			result.Checks[0],
			"id",
			"category",
			"status",
			"summary",
			"detail",
			"remediation",
			"durationNs",
			"required",
		)
	})
}

func TestDeveloperCLIExitMatrixIncludesEveryApplicableOutcome(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	policy := filepath.Join(root, "policy.bpmn")
	changed := filepath.Join(root, "changed.bpmn")
	secretDir := filepath.Join(root, "secret")
	if err := os.Mkdir(secretDir, 0o700); err != nil {
		t.Fatal(err)
	}
	for path, content := range map[string]string{
		valid:                                  validCLIBPMN,
		policy:                                 `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><serviceTask id="task"/></process></definitions>`,
		changed:                                strings.Replace(validCLIBPMN, `id="end"`, `id="end" name="changed"`, 1),
		filepath.Join(secretDir, "secret.env"): "CLIENT_SECRET=deterministic-client-secret-value",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	tests := []struct {
		name    string
		command func() *cobra.Command
		args    []string
		exit    int
	}{
		{name: "lint 0", command: newLintCmd, args: []string{valid}, exit: 0},
		{name: "lint 1", command: newLintCmd, args: []string{policy, "--fail-on", "warning"}, exit: 1},
		{name: "lint 2", command: newLintCmd, args: []string{valid, "--fail-on", "invalid"}, exit: 2},
		{name: "diff 0", command: newDiffCmd, args: []string{valid, valid}, exit: 0},
		{name: "diff 1", command: newDiffCmd, args: []string{valid, changed}, exit: 1},
		{name: "diff 2", command: newDiffCmd, args: []string{"--from", valid}, exit: 2},
		{name: "explain 0", command: newExplainCmd, args: []string{valid}, exit: 0},
		{name: "explain 2", command: newExplainCmd, args: []string{valid, "--json", "--output", "x"}, exit: 2},
		{name: "review 0", command: newReviewCmd, args: []string{valid}, exit: 0},
		{name: "review 1", command: newReviewCmd, args: []string{policy, "--fail-on", "warning"}, exit: 1},
		{name: "review 2", command: newReviewCmd, args: []string{valid, "--provider", "openai"}, exit: 2},
		{name: "generate 0", command: newTestCmd, args: []string{"generate", valid, "--output", filepath.Join(root, "generated")}, exit: 0},
		{name: "generate 2", command: newTestCmd, args: []string{"generate", valid, "--lang", "ruby"}, exit: 2},
		{name: "scan 0", command: newScanCmd, args: []string{root, "--ignore", "secret/**"}, exit: 0},
		{name: "scan 1", command: newScanCmd, args: []string{secretDir, "--fail-on", "low"}, exit: 1},
		{name: "scan 2", command: newScanCmd, args: []string{filepath.Join(root, "missing")}, exit: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, code := executeDeveloperJSON(t, test.command(), test.args...)
			if code != test.exit {
				t.Fatalf("exit = %d, want %d", code, test.exit)
			}
		})
	}

	t.Run("doctor 0 1 2", func(t *testing.T) {
		for name, test := range map[string]struct {
			deps doctorCommandDependencies
			exit int
		}{
			"success": {deps: deterministicDoctorDependencies(doctor.StatusPass), exit: 0},
			"policy":  {deps: deterministicDoctorDependencies(doctor.StatusFail), exit: 1},
			"tool": {
				deps: doctorCommandDependencies{
					runShallow: func(bool) doctor.Report { return doctor.Report{OK: true} },
					loadConfig: func() (config.Config, error) {
						return config.Config{}, errors.New("deterministic load failure")
					},
					runDeep: func(context.Context, config.Config, doctor.DeepOptions) (doctor.DeepReport, error) {
						return doctor.DeepReport{}, nil
					},
				},
				exit: 2,
			},
		} {
			t.Run(name, func(t *testing.T) {
				_, code := executeDeveloperJSON(
					t,
					newDoctorCmdWithDependencies(test.deps),
					"--deep",
					"--json",
				)
				if code != test.exit {
					t.Fatalf("exit = %d, want %d", code, test.exit)
				}
			})
		}
	})

	t.Run("shallow doctor policy is exit 1", func(t *testing.T) {
		deps := deterministicDoctorDependencies(doctor.StatusPass)
		deps.runShallow = func(bool) doctor.Report { return doctor.Report{OK: false} }
		_, code := executeDeveloperJSON(t, newDoctorCmdWithDependencies(deps))
		if code != 1 {
			t.Fatalf("exit = %d, want 1", code)
		}
	})

	t.Run("generate resolves configured tests output", func(t *testing.T) {
		projectRoot := t.TempDir()
		if err := os.WriteFile(
			filepath.Join(projectRoot, ".camunda.yaml"),
			[]byte("name: cli-output\npaths:\n  bpmn: bpmn\n  tests: configured-tests\n"),
			0o600,
		); err != nil {
			t.Fatal(err)
		}
		input := filepath.Join(projectRoot, "process.bpmn")
		if err := os.WriteFile(input, []byte(validCLIBPMN), 0o600); err != nil {
			t.Fatal(err)
		}
		_, code := executeDeveloperJSON(t, newTestCmd(), "generate", input, "--lang", "js")
		if code != 0 {
			t.Fatalf("exit = %d", code)
		}
		if _, err := os.Stat(filepath.Join(projectRoot, "configured-tests", "js")); err != nil {
			t.Fatalf("configured output was not written: %v", err)
		}
	})
}

func deterministicDoctorDependencies(status doctor.Status) doctorCommandDependencies {
	return doctorCommandDependencies{
		runShallow: func(bool) doctor.Report { return doctor.Report{OK: true} },
		loadConfig: func() (config.Config, error) { return config.Defaults(), nil },
		runDeep: func(context.Context, config.Config, doctor.DeepOptions) (doctor.DeepReport, error) {
			return doctor.DeepReport{Checks: []doctor.Check{{
				ID: "deterministic.check", Category: "test", Status: status,
				Summary: "summary", Detail: "detail", Remediation: "none", Required: true,
			}}}, nil
		},
	}
}

func executeDeveloperJSON(t *testing.T, command *cobra.Command, args ...string) ([]byte, int) {
	t.Helper()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err := command.Execute()
	return stdout.Bytes(), ExitCode(err)
}

func decodeJSONContract(t *testing.T, output []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(output, target); err != nil {
		t.Fatalf("invalid JSON %q: %v", output, err)
	}
}

func assertJSONFields(t *testing.T, output []byte, fields ...string) {
	t.Helper()
	var object map[string]json.RawMessage
	decodeJSONContract(t, output, &object)
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			t.Errorf("missing JSON field %q in %s", field, output)
		}
	}
}

func assertMarshaledFields(t *testing.T, value any, fields ...string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONFields(t, encoded, fields...)
}
