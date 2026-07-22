package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func newTestCmd() *cobra.Command {
	return newTestCmdWith(Dependencies{})
}

func newTestCmdWith(deps Dependencies) *cobra.Command {
	command := &cobra.Command{Use: "test", Short: "Test helpers"}
	var language, output string
	var force, jsonOutput bool
	generate := &cobra.Command{
		Use:   "generate file.bpmn",
		Short: "Generate test skeletons from BPMN",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			outDir := strings.TrimSpace(output)
			if outDir == "" {
				opened, err := project.Open(filepath.Dir(args[0]))
				if err != nil {
					return err
				}
				configured := opened.Config.Paths.Tests
				if configured == "" {
					configured = "tests"
				}
				outDir, err = opened.Resolve(configured)
				if err != nil {
					return err
				}
			}
			result, err := deps.toolkitService().Generate(cmd.Context(), toolkit.GenerateRequest{
				Input:  toolkit.BPMNInput{Name: args[0], Path: args[0]},
				OutDir: outDir, Lang: toolkit.GenerateLanguage(language), Force: force,
			})
			if err != nil {
				return err
			}
			result.Warnings = nonNilSlice(result.Warnings)
			result.Artifacts = nonNilSlice(result.Artifacts)
			if jsonOutput {
				return writeIndentedJSON(cmd, result)
			}
			for _, artifact := range result.Artifacts {
				fmt.Fprintln(cmd.OutOrStdout(), artifact.Path)
			}
			return nil
		},
	}
	generate.Flags().StringVar(&language, "lang", "java", "java|js|python")
	generate.Flags().StringVarP(&output, "output", "o", "", "Output directory (defaults to configured paths.tests)")
	generate.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	generate.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	command.AddCommand(developerCommand(generate))
	return command
}
