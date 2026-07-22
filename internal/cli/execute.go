package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"
)

// Run executes the camunda CLI with the given args and writers, returning a
// process exit code: 0 success, 1 policy/findings/diff/drift, 2 tool failures.
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	return runRoot(ctx, NewRoot(), args, stdout, stderr)
}

func runRoot(ctx context.Context, command *cobra.Command, args []string, stdout, stderr io.Writer) int {
	if ctx == nil {
		ctx = context.Background()
	}
	command.SetArgs(args)
	command.SetOut(stdout)
	command.SetErr(stderr)
	if err := command.ExecuteContext(ctx); err != nil {
		fmt.Fprintf(stderr, "error: %v\n", err)
		return ExitCode(err)
	}
	return 0
}
