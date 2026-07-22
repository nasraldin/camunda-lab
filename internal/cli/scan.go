package cli

import (
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func newScanCmd() *cobra.Command {
	return newScanCmdWith(Dependencies{})
}

func newScanCmdWith(deps Dependencies) *cobra.Command {
	var failOn string
	var ignores []string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "scan [dir]",
		Short: "Scan project for hardcoded secrets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) == 1 {
				root = args[0]
			}
			result, err := deps.toolkitService().Scan(cmd.Context(), toolkit.ScanRequest{
				Roots: []string{root}, FailOn: toolkit.ScanThreshold(failOn), Ignore: ignores,
			})
			if err != nil {
				return err
			}
			domainResult := scan.Result{
				Complete: result.Complete, Findings: result.Findings, Issues: result.Issues, Stats: result.Stats,
			}
			if domainResult.Findings == nil {
				domainResult.Findings = []scan.Finding{}
			}
			if domainResult.Issues == nil {
				domainResult.Issues = []scan.Issue{}
			}
			if jsonOutput {
				if result.Warnings == nil {
					result.Warnings = []toolkit.Warning{}
				}
				if result.ScannedRoots == nil {
					result.ScannedRoots = []string{}
				}
				if result.FailedRoots == nil {
					result.FailedRoots = []string{}
				}
				result.Findings = domainResult.Findings
				result.Issues = domainResult.Issues
				if err := writeIndentedJSON(cmd, result); err != nil {
					return err
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), scan.FormatText(domainResult))
			}
			if result.Status == toolkit.StatusFailed {
				return policyFailure(toolkit.OperationScan)
			}
			if result.Status == toolkit.StatusPartial {
				return fmt.Errorf("scan incomplete")
			}
			return nil
		},
	}
	command.Flags().StringVar(&failOn, "fail-on", "medium", "Fail on low|medium|high")
	command.Flags().StringArrayVar(&ignores, "ignore", nil, "Additional project-relative ignore pattern (repeatable)")
	command.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	return developerCommand(command)
}
