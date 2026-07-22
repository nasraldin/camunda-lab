package cli

import (
	"errors"

	"github.com/spf13/cobra"
)

// ExitError carries an intentional process exit code without losing its cause.
type ExitError struct {
	Code int
	Err  error
}

func (e *ExitError) Error() string { return e.Err.Error() }
func (e *ExitError) Unwrap() error { return e.Err }

// ExitCode maps typed command outcomes:
//
//	0 — clean success (nil)
//	1 — findings / semantic diff / drift / policy incidents (or legacy unclassified)
//	2 — validation / upstream / partial / unknown tool failures (via ExitError)
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *ExitError
	if errors.As(err, &exitErr) {
		return exitErr.Code
	}
	return 1
}

func exitError(code int, err error) error {
	if err == nil {
		return nil
	}
	var classified *ExitError
	if errors.As(err, &classified) {
		return err
	}
	return &ExitError{Code: code, Err: err}
}

func classifyArgs(args cobra.PositionalArgs, code int) cobra.PositionalArgs {
	return func(command *cobra.Command, values []string) error {
		return exitError(code, args(command, values))
	}
}

func classifyRunE(run func(*cobra.Command, []string) error, code int) func(*cobra.Command, []string) error {
	return func(command *cobra.Command, values []string) error {
		return exitError(code, run(command, values))
	}
}
