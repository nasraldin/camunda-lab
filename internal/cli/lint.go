package cli

import (
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func newLintCmd() *cobra.Command {
	return newLintCmdWith(Dependencies{})
}

func newLintCmdWith(deps Dependencies) *cobra.Command {
	var failOn string
	var ignores []string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "lint [files...]",
		Short: "Lint BPMN files (deterministic rules)",
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveBPMNArgs(args)
			if err != nil {
				return err
			}
			projectDir, _ := findProjectRoot()
			result, err := deps.toolkitService().Lint(cmd.Context(), toolkit.LintRequest{
				Inputs: toolkitInputs(files), ProjectDir: projectDir,
				FailOn: toolkit.LintThreshold(failOn), Ignore: ignores,
			})
			if err != nil {
				return err
			}
			result.Warnings = nonNilSlice(result.Warnings)
			result.Inputs = nonNilSlice(result.Inputs)
			result.Documents = nonNilSlice(result.Documents)
			result.Findings = nonNilSlice(result.Findings)
			if jsonOutput {
				if err := writeIndentedJSON(cmd, result); err != nil {
					return err
				}
			} else {
				findings := make([]lint.Finding, 0, len(result.Findings))
				for _, finding := range result.Findings {
					value := finding.Finding
					value.ProcessID = finding.ProcessID
					findings = append(findings, value)
				}
				fmt.Fprint(cmd.OutOrStdout(), lint.FormatText(lint.Result{
					Failed: result.Status == toolkit.StatusFailed, Findings: findings,
				}))
			}
			if result.Status == toolkit.StatusFailed {
				return policyFailure(toolkit.OperationLint)
			}
			return nil
		},
	}
	command.Flags().StringVar(&failOn, "fail-on", "error", "Fail on error|warning")
	command.Flags().StringArrayVar(&ignores, "ignore", nil, "Ignore lint rule (repeatable)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	return developerCommand(command)
}
