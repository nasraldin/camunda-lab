package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/trace"
	"github.com/spf13/cobra"
)

func newEnvCmd() *cobra.Command {
	return newEnvCmdWith(Dependencies{})
}

func newEnvCmdWith(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Environment profiles"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, _ := findProjectRoot()
			service := deps.envService()
			active, err := service.Resolve(env.ResolveRequest{ProjectRoot: projectRoot})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active: %s\n", active.Profile.Name)
			profiles, err := service.List(projectRoot)
			if err != nil {
				return err
			}
			for _, resolved := range profiles {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s, %s)\n", resolved.Profile.Name, resolved.Profile.Kind, resolved.Source)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use name",
		Short: "Set active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, _ := findProjectRoot()
			_, err := deps.envService().Use(args[0], projectRoot)
			return err
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show active profile name",
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, _ := findProjectRoot()
			active, err := deps.envService().Resolve(env.ResolveRequest{ProjectRoot: projectRoot})
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), active.Profile.Name)
			return nil
		},
	})
	var kind, orch, idEnv, secretEnv, tokenURL, tokenURLEnv, audience, scope string
	add := &cobra.Command{
		Use:   "add name",
		Short: "Add a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := env.Profile{Name: args[0], Kind: kind, Endpoints: map[string]string{}, Auth: env.AuthRefs{
				ClientIDEnv: idEnv, ClientSecretEnv: secretEnv, TokenURL: tokenURL,
				TokenURLEnv: tokenURLEnv, Audience: audience, Scope: scope,
			}}
			if orch != "" {
				p.Endpoints["orchestration"] = orch
			}
			return deps.envService().SaveGlobal(p)
		},
	}
	add.Flags().StringVar(&kind, "kind", "remote", "lab|remote")
	add.Flags().StringVar(&orch, "orchestration", "", "Orchestration base URL")
	add.Flags().StringVar(&idEnv, "client-id-env", "CAMUNDA_CLIENT_ID", "Env var name for client id")
	add.Flags().StringVar(&secretEnv, "client-secret-env", "CAMUNDA_CLIENT_SECRET", "Env var name for client secret")
	add.Flags().StringVar(&tokenURL, "token-url", "", "OIDC token URL (HTTPS)")
	add.Flags().StringVar(&tokenURLEnv, "token-url-env", "", "Env var name containing OIDC token URL")
	add.Flags().StringVar(&audience, "audience", "", "OIDC token audience")
	add.Flags().StringVar(&scope, "scope", "", "OIDC token scope")
	cmd.AddCommand(add)
	cmd.AddCommand(&cobra.Command{
		Use:   "remove name",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, _ := findProjectRoot()
			service := deps.envService()
			resolved, err := service.Resolve(env.ResolveRequest{Name: args[0], ProjectRoot: projectRoot})
			if err != nil {
				return err
			}
			return service.Remove(args[0], projectRoot, resolved.Source)
		},
	})
	return cmd
}

func newPlanCmd() *cobra.Command {
	return newPlanCmdWith(Dependencies{})
}

func newPlanCmdWith(deps Dependencies) *cobra.Command {
	var dir string
	var environment string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Deployment preview (does not deploy)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveProjectRoot(dir)
			if err != nil {
				return err
			}
			request := plan.Request{ProjectRoot: root, Environment: environment}
			var result plan.Result
			if deps.Plan != nil {
				result, err = deps.Plan(cmd.Context(), request)
			} else {
				cfg, loadErr := config.Load()
				if loadErr != nil {
					return loadErr
				}
				result, err = plan.NewService(cluster.NewFactory(paths.Home(), cfg)).Run(cmd.Context(), request)
			}
			if err != nil {
				return err
			}
			if jsonOutput {
				encoded, err := plan.FormatJSON(result)
				if err != nil {
					return err
				}
				fmt.Fprint(cmd.OutOrStdout(), string(encoded))
				if len(encoded) == 0 || encoded[len(encoded)-1] != '\n' {
					fmt.Fprintln(cmd.OutOrStdout())
				}
			} else {
				fmt.Fprint(cmd.OutOrStdout(), plan.FormatText(result))
			}
			if result.Policy.ExitCode != 0 {
				return &ExitError{Code: result.Policy.ExitCode, Err: fmt.Errorf("plan policy outcome: %s", result.Policy.Outcome)}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Project directory containing .camunda.yaml (default: walk up from cwd)")
	cmd.Flags().StringVar(&environment, "env", "", "Exact environment profile (default: resolved active/project profile)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable JSON")
	return developerCommand(cmd)
}

func newDriftCmd() *cobra.Command {
	return newDriftCmdWith(Dependencies{})
}

func newDriftCmdWith(deps Dependencies) *cobra.Command {
	var dir string
	var gitRef string
	var environment string
	var jsonOutput bool
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Compare a Git baseline, working project, and deployment",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveProjectRoot(dir)
			if err != nil {
				return err
			}
			request := drift.Request{
				ProjectRoot: root, GitRef: gitRef, Environment: environment,
			}
			var report drift.Report
			var runErr error
			if deps.Drift != nil {
				report, runErr = deps.Drift(cmd.Context(), request)
			} else {
				cfg, loadErr := config.Load()
				if loadErr != nil {
					return loadErr
				}
				service := drift.NewService(cluster.NewFactory(paths.Home(), cfg))
				report, runErr = service.Run(cmd.Context(), request)
			}
			if report.Baseline.Ref != "" || runErr == nil {
				if jsonOutput {
					if err := writeIndentedJSON(cmd, report); err != nil {
						return err
					}
				} else {
					fmt.Fprint(cmd.OutOrStdout(), drift.FormatText(report))
				}
			}
			if runErr != nil {
				return &ExitError{Code: 2, Err: runErr}
			}
			if report.Policy.ExitCode != 0 {
				return &ExitError{
					Code: report.Policy.ExitCode,
					Err:  fmt.Errorf("drift comparison outcome: %s", report.Policy.Outcome),
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Project directory containing .camunda.yaml (default: walk up from cwd)")
	cmd.Flags().StringVar(&gitRef, "ref", "HEAD", "Git baseline ref to resolve and pin to a commit")
	cmd.Flags().StringVar(&environment, "env", "", "Exact environment profile (default: resolved active/project profile)")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Emit stable JSON")
	return developerCommand(cmd)
}

func newBackupCmd() *cobra.Command {
	return newBackupCmdWith(Dependencies{})
}

func newBackupCmdWith(deps Dependencies) *cobra.Command {
	var out string
	var includeSecrets bool
	cmd := &cobra.Command{
		Use:   "backup",
		Short: "Backup lab config + project resources",
		RunE: func(cmd *cobra.Command, args []string) error {
			if out == "" {
				out = fmt.Sprintf("camunda-lab-backup-%s.tar.gz", time.Now().Format("20060102-150405"))
			}
			cfg, _ := config.Load()
			proj, _ := findProjectRoot()
			options := backup.Options{
				LabHome:        paths.Home(),
				ProjectDir:     proj,
				OutPath:        out,
				IncludeSecrets: includeSecrets,
				LabVersion:     cfg.Version,
				LabProfile:     cfg.Profile,
			}
			var m backup.Manifest
			var err error
			if deps.BackupCreate != nil {
				m, err = deps.BackupCreate(cmd.Context(), options)
			} else {
				m, err = backup.NewService(nil).Create(cmd.Context(), options)
			}
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote %s (%d files, secrets=%v)\n", out, len(m.Files), m.IncludesSecrets)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "Output archive path")
	cmd.Flags().BoolVar(&includeSecrets, "include-secrets", false, "Include ai.env values (dangerous)")
	return cmd
}

func newRestoreCmd() *cobra.Command {
	return newRestoreCmdWith(backup.NewService(lab.New()).Restore, lab.New())
}

type restoreFunc func(context.Context, backup.RestoreOptions) (backup.Manifest, error)

func newRestoreCmdWith(restore restoreFunc, runningLab backup.RunningChecker) *cobra.Command {
	var yes bool
	var force bool
	var projectDir string
	cmd := &cobra.Command{
		Use:   "restore archive.tar.gz",
		Short: "Restore a lab backup archive",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				fmt.Fprint(cmd.OutOrStdout(), "Type RESTORE to confirm: ")
				line, err := bufio.NewReader(cmd.InOrStdin()).ReadString('\n')
				if err != nil && err != io.EOF {
					return fmt.Errorf("read restore confirmation: %w", err)
				}
				line = strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r")
				if line != "RESTORE" {
					return fmt.Errorf("restore confirmation must be exactly RESTORE")
				}
			}
			project := projectDir
			if project == "" {
				project, _ = findProjectRoot()
			}
			_, err := restore(cmd.Context(), backup.RestoreOptions{
				ArchivePath: args[0],
				LabHome:     paths.Home(),
				ProjectDir:  project,
				Force:       force,
				Lab:         runningLab,
			})
			return err
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm restore")
	cmd.Flags().BoolVar(&force, "force", false, "Restore even if the lab is running")
	cmd.Flags().StringVar(&projectDir, "project", "", "Project directory to restore")
	return cmd
}

func newIncidentsCmd() *cobra.Command {
	return newIncidentsCmdWith(Dependencies{})
}

func newIncidentsCmdWith(deps Dependencies) *cobra.Command {
	cmd := &cobra.Command{Use: "incidents", Short: "List/retry incidents via Orchestration API"}
	var environment string
	var limit int
	listFn := func(cmd *cobra.Command, args []string) error {
		projectRoot, _ := findProjectRoot()
		request := incidents.ListRequest{
			Environment: environment, ProjectRoot: projectRoot, Limit: limit,
			Filter: incidents.ListFilter{State: "ACTIVE"},
		}
		var result incidents.Result
		var err error
		if deps.ListIncidents != nil {
			result, err = deps.ListIncidents(cmd.Context(), request)
		} else {
			cfg, loadErr := config.Load()
			if loadErr != nil {
				return loadErr
			}
			result, err = incidents.NewService(cluster.NewFactory(paths.Home(), cfg)).List(cmd.Context(), request)
		}
		if err != nil {
			return fmt.Errorf("incidents search (is the lab up?): %w", err)
		}
		fmt.Fprint(cmd.OutOrStdout(), incidents.FormatText(result))
		return nil
	}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List active incidents", RunE: listFn})
	show := &cobra.Command{
		Use: "show key", Short: "Show one incident with available context", Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, _ := findProjectRoot()
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			item, err := incidents.NewService(cluster.NewFactory(paths.Home(), cfg)).Show(
				cmd.Context(), environment, projectRoot, args[0],
			)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), incidents.FormatIncidentText(item))
			return nil
		},
	}
	cmd.AddCommand(show)
	var yes bool
	var dryRun bool
	retry := &cobra.Command{
		Use:   "retry id",
		Short: "Resolve an incident (POST /v2/incidents/{key}/resolution)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes && !dryRun {
				return fmt.Errorf("refusing retry without --yes")
			}
			projectRoot, _ := findProjectRoot()
			request := incidents.ResolveRequest{
				Environment: environment, ProjectRoot: projectRoot, Key: args[0], DryRun: dryRun,
			}
			var result incidents.Result
			var err error
			if deps.ResolveIncident != nil {
				result, err = deps.ResolveIncident(cmd.Context(), request)
			} else {
				cfg, loadErr := config.Load()
				if loadErr != nil {
					return loadErr
				}
				result, err = incidents.NewService(cluster.NewFactory(paths.Home(), cfg)).ResolveWithOptions(cmd.Context(), request)
			}
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), incidents.FormatText(result))
			return nil
		},
	}
	retry.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm resolve")
	retry.Flags().BoolVar(&dryRun, "dry-run", false, "Validate the resolve without mutation")
	cmd.AddCommand(retry)
	cmd.PersistentFlags().StringVar(&environment, "env", "", "Environment name")
	cmd.PersistentFlags().IntVar(&limit, "limit", 50, "Maximum incidents to return")
	cmd.RunE = listFn
	return cmd
}

func newTraceCmd() *cobra.Command {
	return newTraceCmdWith(Dependencies{})
}

func newTraceCmdWith(deps Dependencies) *cobra.Command {
	var follow bool
	var asJSON bool
	var environment string
	var interval, timeout, idleStop time.Duration
	var maxEvents int
	cmd := &cobra.Command{
		Use:   "trace instanceKey",
		Short: "Show process instance timeline from Orchestration API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			projectRoot, _ := findProjectRoot()
			request := trace.Request{
				Environment: environment, ProjectRoot: projectRoot,
				ProcessInstanceKey: args[0],
				Timeout:            timeout, MaxEvents: maxEvents, IdleStop: idleStop,
			}
			write := func(tl trace.Timeline) error {
				if asJSON {
					encoded, err := trace.FormatJSON(tl)
					if err != nil {
						return err
					}
					fmt.Fprint(cmd.OutOrStdout(), string(encoded))
					return nil
				}
				fmt.Fprint(cmd.OutOrStdout(), trace.FormatText(tl))
				return nil
			}
			if !follow {
				var tl trace.Timeline
				var err error
				if deps.TraceGet != nil {
					tl, err = deps.TraceGet(cmd.Context(), request)
				} else {
					cfg, loadErr := config.Load()
					if loadErr != nil {
						return loadErr
					}
					tl, err = trace.NewService(cluster.NewFactory(paths.Home(), cfg)).Get(cmd.Context(), request)
				}
				if err != nil {
					return fmt.Errorf("trace (is the lab up?): %w", err)
				}
				return write(tl)
			}
			var emitted int
			emit := func(tl trace.Timeline) error {
				if emitted > 0 && !asJSON {
					fmt.Fprintln(cmd.OutOrStdout())
				}
				emitted++
				return write(tl)
			}
			var err error
			if deps.TraceFollow != nil {
				err = deps.TraceFollow(cmd.Context(), request, interval, emit)
			} else {
				cfg, loadErr := config.Load()
				if loadErr != nil {
					return loadErr
				}
				err = trace.NewService(cluster.NewFactory(paths.Home(), cfg)).Follow(cmd.Context(), request, interval, emit)
			}
			if err != nil {
				return fmt.Errorf("trace follow (is the lab up?): %w", err)
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Poll until completed/timeout")
	cmd.Flags().BoolVar(&asJSON, "json", false, "Emit JSON timeline")
	cmd.Flags().StringVar(&environment, "env", "", "Environment name")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "Follow poll interval")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Follow timeout (CLI interactive default; API follow defaults to 30s)")
	cmd.Flags().DurationVar(&idleStop, "idle-stop", 0, "Stop follow after no changes for this duration (CLI-only)")
	cmd.Flags().IntVar(&maxEvents, "max-events", 0, "Maximum changed timelines to emit while following (0 uses domain default; API follow defaults to 20)")
	return cmd
}

func resolveBPMNArgs(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	root, err := findProjectRoot()
	if err != nil {
		return nil, fmt.Errorf("pass BPMN files or run inside a project with .camunda.yaml: %w", err)
	}
	cfg, err := project.Load(filepath.Join(root, project.ConfigFileName))
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(root, cfg.Paths.BPMN)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".bpmn") {
			files = append(files, filepath.Join(dir, e.Name()))
		}
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no .bpmn files in %s", dir)
	}
	return files, nil
}

func resolveProjectRoot(dir string) (string, error) {
	if strings.TrimSpace(dir) != "" {
		abs, err := filepath.Abs(dir)
		if err != nil {
			return "", err
		}
		if _, err := os.Stat(filepath.Join(abs, project.ConfigFileName)); err != nil {
			return "", fmt.Errorf("no .camunda.yaml in %s — run: camunda init %s", abs, abs)
		}
		return abs, nil
	}
	return findProjectRoot()
}

func findProjectRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, project.ConfigFileName)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("no .camunda.yaml found — run inside a project (camunda init) or pass --dir /path/to/project")
		}
		dir = parent
	}
}

func loadLintIgnore() ([]string, error) {
	root, err := findProjectRoot()
	if err != nil {
		return nil, nil
	}
	cfg, err := project.Load(filepath.Join(root, project.ConfigFileName))
	if err != nil {
		return nil, err
	}
	return append([]string(nil), cfg.Lint.Ignore...), nil
}
