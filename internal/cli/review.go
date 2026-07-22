package cli

import (
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func newReviewCmd() *cobra.Command {
	return newReviewCmdWith(Dependencies{})
}

func newReviewCmdWith(deps Dependencies) *cobra.Command {
	var failOn, provider, model string
	var ignores []string
	var aiEnabled, aiRequired, jsonOutput bool
	command := &cobra.Command{
		Use:   "review [files...]",
		Short: "Lint + optional AI review of BPMN",
		RunE: func(cmd *cobra.Command, args []string) error {
			aiRequested := aiEnabled || aiRequired
			if !aiRequested && (cmd.Flags().Changed("provider") || cmd.Flags().Changed("model")) {
				return fmt.Errorf("--provider/--model require --ai or --ai-required")
			}
			files, err := resolveBPMNArgs(args)
			if err != nil {
				return err
			}
			projectDir, _ := findProjectRoot()
			request := toolkit.ReviewRequest{
				Inputs: toolkitInputs(files), ProjectDir: projectDir,
				FailOn: toolkit.LintThreshold(failOn), Ignore: ignores,
				AI: toolkit.AIOptions{Enabled: aiEnabled, Required: aiRequired},
			}
			var result toolkit.ReviewResult
			if deps.Toolkit != nil {
				result, err = deps.Toolkit.Review(cmd.Context(), request)
			} else {
				service := toolkit.Service{}
				if aiRequested {
					secrets, loadErr := ai.LoadSecrets()
					if loadErr != nil {
						return loadErr
					}
					if err := review.ValidateClientConfiguration(provider, model, secrets); err != nil {
						return err
					}
					service.AI, err = review.NewConfiguredClient(provider, model, secrets)
					if err != nil && (!review.IsMissingCredentials(err) || aiRequired) {
						return err
					}
					if review.IsMissingCredentials(err) {
						service.AI = nil
					}
				}
				result, err = service.Review(cmd.Context(), request)
			}
			if err != nil {
				return err
			}
			result.Warnings = nonNilSlice(result.Warnings)
			result.Inputs = nonNilSlice(result.Inputs)
			result.Documents = nonNilSlice(result.Documents)
			result.Processes = nonNilSlice(result.Processes)
			result.Findings = nonNilSlice(result.Findings)
			if jsonOutput {
				if err := writeIndentedJSON(cmd, result); err != nil {
					return err
				}
			} else {
				for _, process := range result.Processes {
					fmt.Fprintf(cmd.OutOrStdout(), "== %s ==\n%s\n", process.ProcessID, review.FormatText(process.Review))
				}
				for _, warning := range result.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "warning: %s\n", warning.Message)
				}
			}
			if result.Status == toolkit.StatusFailed || lint.ShouldFail(reviewFindings(result), failOn) {
				return policyFailure(toolkit.OperationReview)
			}
			return nil
		},
	}
	command.Flags().StringVar(&failOn, "fail-on", "error", "Fail on error|warning")
	command.Flags().StringArrayVar(&ignores, "ignore", nil, "Ignore lint rule (repeatable)")
	command.Flags().BoolVar(&aiEnabled, "ai", false, "Enable optional AI enrichment")
	command.Flags().BoolVar(&aiRequired, "ai-required", false, "Require AI enrichment to succeed")
	command.Flags().StringVar(&provider, "provider", "openai", "AI provider: openai|anthropic")
	command.Flags().StringVar(&model, "model", "gpt-4o-mini", "AI provider model")
	command.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	return developerCommand(command)
}

func reviewFindings(result toolkit.ReviewResult) []lint.Finding {
	findings := make([]lint.Finding, 0, len(result.Findings))
	for _, finding := range result.Findings {
		findings = append(findings, finding.Finding)
	}
	return findings
}
