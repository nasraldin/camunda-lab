package cli

import (
	"encoding/json"
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func toolkitInputs(paths []string) []toolkit.BPMNInput {
	inputs := make([]toolkit.BPMNInput, 0, len(paths))
	for _, path := range paths {
		inputs = append(inputs, toolkit.BPMNInput{Name: path, Path: path})
	}
	return inputs
}

func nonNilSlice[T any](value []T) []T {
	if value == nil {
		return []T{}
	}
	return value
}

func writeIndentedJSON(cmd *cobra.Command, value any) error {
	encoder := json.NewEncoder(cmd.OutOrStdout())
	encoder.SetIndent("", "  ")
	return encoder.Encode(value)
}

func developerCommand(command *cobra.Command) *cobra.Command {
	command.SilenceErrors = true
	command.SilenceUsage = true
	command.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return exitError(2, err)
	})
	if command.Args != nil {
		command.Args = classifyArgs(command.Args, 2)
	}
	if command.RunE != nil {
		run := command.RunE
		command.RunE = func(cmd *cobra.Command, args []string) error {
			err := run(cmd, args)
			if err == nil {
				return nil
			}
			return exitError(2, err)
		}
	}
	return command
}

func policyFailure(operation toolkit.Operation) error {
	return &ExitError{Code: 1, Err: fmt.Errorf("%s policy threshold reached", operation)}
}
