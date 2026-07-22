package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func newExplainCmd() *cobra.Command {
	return newExplainCmdWith(Dependencies{})
}

func newExplainCmdWith(deps Dependencies) *cobra.Command {
	var output string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "explain file.bpmn",
		Short: "Explain a BPMN process (offline summary)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput && output != "" {
				return fmt.Errorf("--json and --output cannot be used together")
			}
			result, err := deps.toolkitService().Explain(cmd.Context(), toolkit.ExplainRequest{
				Input: toolkit.BPMNInput{Name: args[0], Path: args[0]},
			})
			if err != nil {
				return err
			}
			result.Warnings = nonNilSlice(result.Warnings)
			result.Processes = nonNilSlice(result.Processes)
			if jsonOutput {
				return writeIndentedJSON(cmd, result)
			}
			var rendered strings.Builder
			for index, process := range result.Processes {
				if index > 0 {
					rendered.WriteString("\n")
				}
				rendered.WriteString(process.Explanation.Markdown())
			}
			if output != "" {
				if err := os.WriteFile(output, []byte(rendered.String()), 0o644); err != nil {
					return err
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), rendered.String())
			}
			return nil
		},
	}
	command.Flags().StringVarP(&output, "output", "o", "", "Write markdown to file")
	command.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	return developerCommand(command)
}
