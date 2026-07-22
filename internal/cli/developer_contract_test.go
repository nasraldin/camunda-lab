package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/spf13/cobra"
)

func TestDeveloperCommandsAdvertiseD10Flags(t *testing.T) {
	tests := map[string][]string{
		"lint":     {"fail-on", "ignore", "json"},
		"diff":     {"from", "to", "against", "base", "json"},
		"explain":  {"json", "output"},
		"review":   {"fail-on", "ignore", "ai", "ai-required", "provider", "model", "json"},
		"generate": {"lang", "output", "force", "json"},
		"scan":     {"fail-on", "ignore", "json"},
		"doctor":   {"deep", "json", "timeout"},
	}
	root := NewRoot()
	for name, flags := range tests {
		command, _, err := root.Find(commandPath(name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for _, flag := range flags {
			if command.Flags().Lookup(flag) == nil {
				t.Errorf("%s missing --%s", name, flag)
			}
		}
	}
}

func commandPath(name string) []string {
	if name == "generate" {
		return []string{"test", "generate"}
	}
	return []string{name}
}

func TestLintUsesPolicyAndUsageExitCodes(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bad.bpmn")
	content := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><serviceTask id="task"/></process></definitions>`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	command := newLintCmd()
	command.SetOut(&bytes.Buffer{})
	command.SetArgs([]string{path, "--fail-on", "warning"})
	err := command.Execute()
	if ExitCode(err) != 1 {
		t.Fatalf("policy exit = %d, error = %v", ExitCode(err), err)
	}

	command = newLintCmd()
	command.SetArgs([]string{path, "--fail-on", "invalid"})
	err = command.Execute()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || ExitCode(err) != 2 {
		t.Fatalf("usage exit = %d, error = %v", ExitCode(err), err)
	}
}

func TestExplainRejectsJSONWithOutputAsUsage(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validCLIBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	command := newExplainCmd()
	command.SetArgs([]string{path, "--json", "--output", filepath.Join(root, "explain.md")})
	if err := command.Execute(); ExitCode(err) != 2 {
		t.Fatalf("exit = %d, error = %v", ExitCode(err), err)
	}
}

func TestReviewValidatesAIModeAndConfigurationBeforeCredentials(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validCLIBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := [][]string{
		{path, "--provider", "anthropic"},
		{path, "--model", "custom-model"},
		{path, "--ai", "--provider", "unknown"},
		{path, "--ai", "--model", " "},
	}
	for _, args := range tests {
		command := newReviewCmd()
		command.SetArgs(args)
		if err := command.Execute(); ExitCode(err) != 2 {
			t.Errorf("args %v exit = %d, error = %v", args, ExitCode(err), err)
		}
	}
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := ai.WriteSecrets(ai.Secrets{OpenAIKey: "test-key", OpenAIBaseURL: "://invalid"}); err != nil {
		t.Fatal(err)
	}
	command := newReviewCmd()
	command.SetArgs([]string{path, "--ai", "--provider", "openai", "--model", "model"})
	if err := command.Execute(); ExitCode(err) != 2 {
		t.Fatalf("invalid endpoint exit = %d, error = %v", ExitCode(err), err)
	}
}

func TestReviewMissingOptionalCredentialsReturnsPartialJSON(t *testing.T) {
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validCLIBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	command := newReviewCmd()
	var stdout bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{path, "--ai", "--provider", "openai", "--model", "model", "--json"})
	if err := command.Execute(); err != nil {
		t.Fatalf("error = %v, output = %s", err, stdout.String())
	}
	var result struct {
		Status   string `json:"status"`
		Complete bool   `json:"complete"`
		AIStatus string `json:"aiStatus"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if result.Status != "partial" || result.Complete || result.AIStatus != "skipped" {
		t.Fatalf("result = %+v", result)
	}
}

func TestScanJSONRetainsToolkitEnvelopeOnPartialResult(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken.env")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	command := newScanCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.SetOut(&stdout)
	command.SetErr(&stderr)
	command.SetArgs([]string{root, "--json"})
	if err := command.Execute(); ExitCode(err) != 2 {
		t.Fatalf("exit = %d, error = %v", ExitCode(err), err)
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("stdout is not pure JSON: %q: %v", stdout.String(), err)
	}
	for _, key := range []string{
		"status", "complete", "warnings", "scannedRoots", "failedRoots", "findings", "issues", "stats",
	} {
		if _, ok := result[key]; !ok {
			t.Errorf("JSON missing %q: %s", key, stdout.String())
		}
	}
	if strings.TrimSpace(stderr.String()) != "" {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

func TestDeveloperCommandsExerciseRepresentativeSuccessPolicyAndUsage(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	changed := filepath.Join(root, "changed.bpmn")
	policy := filepath.Join(root, "policy.bpmn")
	if err := os.WriteFile(valid, []byte(validCLIBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(changed, []byte(strings.Replace(validCLIBPMN, `id="end"`, `id="end" name="changed"`, 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><serviceTask id="task"/></process></definitions>`), 0o600); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		command *cobra.Command
		args    []string
		exit    int
	}{
		{name: "lint success", command: newLintCmd(), args: []string{valid}, exit: 0},
		{name: "lint policy", command: newLintCmd(), args: []string{policy, "--fail-on", "warning"}, exit: 1},
		{name: "diff success", command: newDiffCmd(), args: []string{valid, valid}, exit: 0},
		{name: "diff policy", command: newDiffCmd(), args: []string{valid, changed}, exit: 1},
		{name: "diff usage", command: newDiffCmd(), args: []string{"--from", valid}, exit: 2},
		{name: "explain success", command: newExplainCmd(), args: []string{valid}, exit: 0},
		{name: "review success", command: newReviewCmd(), args: []string{valid}, exit: 0},
		{name: "review policy", command: newReviewCmd(), args: []string{policy, "--fail-on", "warning"}, exit: 1},
		{name: "generate success", command: newTestCmd(), args: []string{"generate", valid, "--output", filepath.Join(root, "generated")}, exit: 0},
		{name: "generate usage", command: newTestCmd(), args: []string{"generate", valid, "--lang", "ruby"}, exit: 2},
		{name: "doctor usage", command: newDoctorCmd(), args: []string{"--json"}, exit: 2},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.command.SetOut(&bytes.Buffer{})
			test.command.SetErr(&bytes.Buffer{})
			test.command.SetArgs(test.args)
			if code := ExitCode(test.command.Execute()); code != test.exit {
				t.Fatalf("exit = %d, want %d", code, test.exit)
			}
		})
	}
}

const validCLIBPMN = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`
