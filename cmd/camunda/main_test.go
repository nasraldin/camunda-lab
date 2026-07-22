package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cli"
	"github.com/spf13/cobra"
)

func TestRunMapsDiffAndPreservesOtherExitCodes(t *testing.T) {
	root := t.TempDir()
	before := filepath.Join(root, "before.bpmn")
	after := filepath.Join(root, "after.bpmn")
	source := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/></process></definitions>`
	if err := os.WriteFile(before, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(after, []byte(strings.Replace(source, `id="start"`, `id="start" name="Changed"`, 1)), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := cli.Run(context.Background(), []string{"diff", before, after}, &stdout, &stderr); code != 1 {
		t.Fatalf("semantic exit = %d, stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run(context.Background(), []string{"diff", before, filepath.Join(root, "missing.bpmn")}, &stdout, &stderr); code != 2 {
		t.Fatalf("operational exit = %d, stderr=%q", code, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	if code := cli.Run(context.Background(), []string{"env", "use", "missing"}, &stdout, &stderr); code != 1 {
		t.Fatalf("other command exit = %d, stderr=%q", code, stderr.String())
	}
}

func TestRunCommandPreservesTypedCobraExitCodes(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want int
	}{
		{name: "policy", err: &cli.ExitError{Code: 1, Err: errors.New("policy")}, want: 1},
		{name: "tool", err: &cli.ExitError{Code: 2, Err: errors.New("tool")}, want: 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			command := &cobra.Command{
				Use:  "test",
				RunE: func(*cobra.Command, []string) error { return test.err },
			}
			var stdout, stderr bytes.Buffer
			command.SetArgs(nil)
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			err := command.Execute()
			if got := cli.ExitCode(err); got != test.want {
				t.Fatalf("exit = %d, want %d; stderr=%q", got, test.want, stderr.String())
			}
		})
	}
}
