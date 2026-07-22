package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	bpmndiff "github.com/nasraldin/camunda-lab/internal/diff"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/spf13/cobra"
)

func newDiffCmd() *cobra.Command {
	return newDiffCmdWith(Dependencies{})
}

func newDiffCmdWith(deps Dependencies) *cobra.Command {
	var from, to, against, base string
	var jsonOutput bool
	command := &cobra.Command{
		Use:   "diff [file] [file]",
		Short: "Semantic BPMN diff",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			request := toolkit.DiffRequest{}
			service := deps.toolkitService()
			baseSet := cmd.Flags().Changed("base")
			switch {
			case from != "" || to != "":
				if from == "" || to == "" || len(args) != 0 || against != "" || baseSet {
					return diffUsageError()
				}
				request.Before = toolkit.BPMNInput{Name: from, Path: from}
				request.After = toolkit.BPMNInput{Name: to, Path: to}
			case against != "":
				if len(args) != 1 || baseSet {
					return diffUsageError()
				}
				request.Before = toolkit.BPMNInput{Name: args[0], Path: args[0]}
				request.After = toolkit.BPMNInput{Name: against, Path: against}
			case len(args) == 2:
				if baseSet {
					return diffUsageError()
				}
				request.Before = toolkit.BPMNInput{Name: args[0], Path: args[0]}
				request.After = toolkit.BPMNInput{Name: args[1], Path: args[1]}
			case len(args) == 1:
				root, err := findProjectRoot()
				if err != nil {
					return fmt.Errorf("single-file Git diff requires a project: %w", err)
				}
				relative, err := safeProjectBPMNPath(args[0])
				if err != nil {
					return err
				}
				working, err := secureProjectWorkingPath(root, relative)
				if err != nil {
					return err
				}
				request.ProjectDir = root
				request.BeforeGit = &toolkit.GitInput{Ref: base, Path: relative}
				request.After = toolkit.BPMNInput{Name: relative, Path: working}
				if deps.Toolkit == nil {
					service = toolkit.Service{Git: bpmndiff.NewGitReader(root)}
				}
			default:
				return diffUsageError()
			}
			result, err := service.Diff(cmd.Context(), request)
			if err != nil {
				return err
			}
			result.Warnings = nonNilSlice(result.Warnings)
			result.Changes = nonNilSlice(result.Changes)
			if jsonOutput {
				if err := writeIndentedJSON(cmd, result); err != nil {
					return err
				}
			} else {
				changes := make([]bpmndiff.Change, 0, len(result.Changes))
				for _, change := range result.Changes {
					if change.Change != nil {
						changes = append(changes, *change.Change)
						continue
					}
					processID := change.AfterProcessID
					summary := "Added process " + processID
					if change.Kind == toolkit.ProcessRemoved {
						processID = change.BeforeProcessID
						summary = "Removed process " + processID
					}
					changes = append(changes, bpmndiff.Change{
						Kind: string(change.Kind), ProcessID: processID, Summary: summary,
					})
				}
				fmt.Fprint(cmd.OutOrStdout(), bpmndiff.FormatText(changes))
			}
			if result.Status == toolkit.StatusFailed {
				return policyFailure(toolkit.OperationDiff)
			}
			return nil
		},
	}
	command.Flags().StringVar(&from, "from", "", "Base BPMN file")
	command.Flags().StringVar(&to, "to", "", "New BPMN file")
	command.Flags().StringVar(&against, "against", "", "Compare positional file against this BPMN file")
	command.Flags().StringVar(&base, "base", "HEAD", "Git ref for single-file diff")
	command.Flags().BoolVar(&jsonOutput, "json", false, "JSON output")
	return developerCommand(command)
}

func safeProjectBPMNPath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return "", fmt.Errorf("single-file Git diff requires a project-relative BPMN path")
	}
	clean := filepath.Clean(path)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("single-file Git diff path escapes the project")
	}
	if !strings.EqualFold(filepath.Ext(clean), ".bpmn") {
		return "", fmt.Errorf("diff supports only .bpmn files")
	}
	return filepath.ToSlash(clean), nil
}

func secureProjectWorkingPath(projectRoot, relative string) (string, error) {
	root, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(projectRoot, filepath.FromSlash(relative)))
	if err != nil {
		return "", err
	}
	inside, err := filepath.Rel(root, path)
	if err != nil || inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("single-file Git diff path escapes the project")
	}
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("diff input must be a regular BPMN file")
	}
	return path, nil
}

func diffUsageError() error {
	return fmt.Errorf("usage: camunda diff a.bpmn b.bpmn | camunda diff --from a.bpmn --to b.bpmn | camunda diff file.bpmn --against other.bpmn | camunda diff project/file.bpmn --base REF")
}
