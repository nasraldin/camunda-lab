package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/config"
	bpmdiff "github.com/nasraldin/camunda-lab/internal/diff"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/explain"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/k8s"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/testgen"
	"github.com/nasraldin/camunda-lab/internal/trace"
	"github.com/spf13/cobra"
)

func newLintCmd() *cobra.Command {
	var failOn string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "lint [files...]",
		Short: "Lint BPMN files (deterministic rules)",
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveBPMNArgs(args)
			if err != nil {
				return err
			}
			var all []lint.Finding
			for _, f := range files {
				m, err := bpmn.ParseFile(f)
				if err != nil {
					return err
				}
				all = append(all, lint.Run(m, lint.Options{File: f, Ignore: loadLintIgnore()})...)
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(all)
			} else {
				fmt.Fprint(cmd.OutOrStdout(), lint.FormatText(all))
			}
			if lint.ShouldFail(all, failOn) {
				return fmt.Errorf("lint findings")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&failOn, "fail-on", "error", "Fail on error|warning")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newDiffCmd() *cobra.Command {
	var from, to, against, base string
	cmd := &cobra.Command{
		Use:   "diff [file] [file]",
		Short: "Semantic BPMN diff",
		Args:  cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if against != "" && to == "" {
				to = against
			}
			var a, b bpmn.Model
			var err error
			switch {
			case from != "" && to != "":
				a, err = bpmn.ParseFile(from)
				if err != nil {
					return err
				}
				b, err = bpmn.ParseFile(to)
				if err != nil {
					return err
				}
			case len(args) == 2:
				a, err = bpmn.ParseFile(args[0])
				if err != nil {
					return err
				}
				b, err = bpmn.ParseFile(args[1])
				if err != nil {
					return err
				}
			case len(args) == 1 && to != "":
				a, err = bpmn.ParseFile(args[0])
				if err != nil {
					return err
				}
				b, err = bpmn.ParseFile(to)
				if err != nil {
					return err
				}
			case len(args) == 1:
				b, err = bpmn.ParseFile(args[0])
				if err != nil {
					return err
				}
				if base == "" {
					base = "HEAD"
				}
				data, err := gitShow(base, args[0])
				if err != nil {
					return err
				}
				a, err = bpmn.Parse(strings.NewReader(data))
				if err != nil {
					return err
				}
			default:
				return fmt.Errorf("usage: camunda diff a.bpmn b.bpmn | camunda diff --from a.bpmn --to b.bpmn | camunda diff file.bpmn --against other.bpmn | camunda diff file.bpmn (--base HEAD)")
			}
			changes := bpmdiff.Compare(a, b)
			fmt.Fprint(cmd.OutOrStdout(), bpmdiff.FormatText(changes))
			if len(changes) > 0 {
				return fmt.Errorf("semantic changes found")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Base BPMN file")
	cmd.Flags().StringVar(&to, "to", "", "New BPMN file")
	cmd.Flags().StringVar(&against, "against", "", "Alias for --to (compare positional file against this)")
	cmd.Flags().StringVar(&base, "base", "HEAD", "Git ref for single-file diff")
	return cmd
}

func newScanCmd() *cobra.Command {
	var failOn string
	var jsonOut bool
	cmd := &cobra.Command{
		Use:   "scan [dir]",
		Short: "Scan project for hardcoded secrets",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			root := "."
			if len(args) > 0 {
				root = args[0]
			}
			fs, err := scan.Walk(scan.Options{Root: root, FailOn: failOn})
			if err != nil {
				return err
			}
			if jsonOut {
				enc := json.NewEncoder(cmd.OutOrStdout())
				enc.SetIndent("", "  ")
				_ = enc.Encode(fs)
			} else {
				fmt.Fprint(cmd.OutOrStdout(), scan.FormatText(fs))
			}
			if scan.ShouldFail(fs, failOn) {
				return fmt.Errorf("secrets found")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&failOn, "fail-on", "medium", "Fail on low|medium|high")
	cmd.Flags().BoolVar(&jsonOut, "json", false, "JSON output")
	return cmd
}

func newExplainCmd() *cobra.Command {
	var out string
	cmd := &cobra.Command{
		Use:   "explain file.bpmn",
		Short: "Explain a BPMN process (offline summary)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := bpmn.ParseFile(args[0])
			if err != nil {
				return err
			}
			md := explain.Offline(m).Markdown()
			if out != "" {
				return os.WriteFile(out, []byte(md), 0o644)
			}
			fmt.Fprint(cmd.OutOrStdout(), md)
			return nil
		},
	}
	cmd.Flags().StringVarP(&out, "output", "o", "", "Write markdown to file")
	return cmd
}

func newReviewCmd() *cobra.Command {
	var failOn string
	var ai bool
	cmd := &cobra.Command{
		Use:   "review [files...]",
		Short: "Lint + optional AI review of BPMN",
		RunE: func(cmd *cobra.Command, args []string) error {
			files, err := resolveBPMNArgs(args)
			if err != nil {
				return err
			}
			var failed bool
			for _, f := range files {
				m, err := bpmn.ParseFile(f)
				if err != nil {
					return err
				}
				res, err := review.Run(m, review.Options{File: f, FailOn: failOn, AI: ai})
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "== %s ==\n%s\n", f, review.FormatText(res))
				if lint.ShouldFail(res.Findings, failOn) {
					failed = true
				}
			}
			if failed {
				return fmt.Errorf("review findings")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&failOn, "fail-on", "error", "Fail on error|warning")
	cmd.Flags().BoolVar(&ai, "ai", false, "Enable AI enrichment (requires configured client)")
	return cmd
}

func newTestCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "test", Short: "Test helpers"}
	var lang string
	var force bool
	var outDir string
	gen := &cobra.Command{
		Use:   "generate file.bpmn",
		Short: "Generate test skeletons from BPMN",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			m, err := bpmn.ParseFile(args[0])
			if err != nil {
				return err
			}
			if outDir == "" {
				outDir = "tests"
			}
			paths, err := testgen.Generate(m, testgen.Options{Lang: lang, Force: force, OutDir: outDir})
			if err != nil {
				return err
			}
			for _, p := range paths {
				fmt.Fprintln(cmd.OutOrStdout(), p)
			}
			return nil
		},
	}
	gen.Flags().StringVar(&lang, "lang", "java", "java|js")
	gen.Flags().BoolVar(&force, "force", false, "Overwrite existing files")
	gen.Flags().StringVarP(&outDir, "output", "o", "tests", "Output directory for generated tests")
	cmd.AddCommand(gen)
	return cmd
}

func newEnvCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "env", Short: "Environment profiles"}
	cmd.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List profiles",
		RunE: func(cmd *cobra.Command, args []string) error {
			active, err := env.GetActive(paths.Home())
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "active: %s\n", active)
			fmt.Fprintln(cmd.OutOrStdout(), "lab (implicit)")
			ps, err := env.ListProfiles(filepath.Join(paths.Home(), "envs"))
			if err != nil {
				return err
			}
			for _, p := range ps {
				fmt.Fprintf(cmd.OutOrStdout(), "%s (%s)\n", p.Name, p.Kind)
			}
			return nil
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "use name",
		Short: "Set active profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return env.SetActive(paths.Home(), filepath.Join(paths.Home(), "envs"), args[0])
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "show",
		Short: "Show active profile name",
		RunE: func(cmd *cobra.Command, args []string) error {
			active, err := env.GetActive(paths.Home())
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), active)
			return nil
		},
	})
	var kind, orch, idEnv, secretEnv string
	add := &cobra.Command{
		Use:   "add name",
		Short: "Add a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			p := env.Profile{Name: args[0], Kind: kind, Endpoints: map[string]string{}, Auth: env.AuthRefs{ClientIDEnv: idEnv, ClientSecretEnv: secretEnv}}
			if orch != "" {
				p.Endpoints["orchestration"] = orch
			}
			return env.SaveProfile(filepath.Join(paths.Home(), "envs"), p)
		},
	}
	add.Flags().StringVar(&kind, "kind", "remote", "lab|remote")
	add.Flags().StringVar(&orch, "orchestration", "", "Orchestration base URL")
	add.Flags().StringVar(&idEnv, "client-id-env", "CAMUNDA_CLIENT_ID", "Env var name for client id")
	add.Flags().StringVar(&secretEnv, "client-secret-env", "CAMUNDA_CLIENT_SECRET", "Env var name for client secret")
	cmd.AddCommand(add)
	cmd.AddCommand(&cobra.Command{
		Use:   "remove name",
		Short: "Remove a profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return env.RemoveProfile(paths.Home(), filepath.Join(paths.Home(), "envs"), args[0])
		},
	})
	return cmd
}

func newPlanCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Deployment preview (does not deploy)",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveProjectRoot(dir)
			if err != nil {
				return err
			}
			local, err := plan.LocalInventory(root)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl, err := cluster.NewFromLab(paths.Home(), cfg)
			if err != nil {
				return err
			}
			remote, err := cl.RemoteInventory(cmd.Context())
			if err != nil {
				return fmt.Errorf("cluster inventory (is the lab up?): %w", err)
			}
			active, err := env.GetActive(paths.Home())
			if err != nil {
				return err
			}
			p := plan.Build(active, local, remote)
			fmt.Fprint(cmd.OutOrStdout(), plan.FormatText(p))
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Project directory containing .camunda.yaml (default: walk up from cwd)")
	return cmd
}

func newDriftCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "drift",
		Short: "Detect git/project vs cluster drift",
		RunE: func(cmd *cobra.Command, args []string) error {
			root, err := resolveProjectRoot(dir)
			if err != nil {
				return err
			}
			local, err := plan.LocalInventory(root)
			if err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl, err := cluster.NewFromLab(paths.Home(), cfg)
			if err != nil {
				return err
			}
			remote, err := cl.RemoteInventory(cmd.Context())
			if err != nil {
				return fmt.Errorf("cluster inventory (is the lab up?): %w", err)
			}
			active, err := env.GetActive(paths.Home())
			if err != nil {
				return err
			}
			r := drift.Compare(active, local, remote)
			fmt.Fprint(cmd.OutOrStdout(), drift.FormatText(r))
			if drift.HasDrift(r) {
				return fmt.Errorf("drift detected")
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "Project directory containing .camunda.yaml (default: walk up from cwd)")
	return cmd
}

func newBackupCmd() *cobra.Command {
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
			m, err := backup.Create(backup.Options{
				LabHome:        paths.Home(),
				ProjectDir:     proj,
				OutPath:        out,
				IncludeSecrets: includeSecrets,
				LabVersion:     cfg.Version,
				LabProfile:     cfg.Profile,
			})
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
	return newRestoreCmdWith(backup.Restore, lab.New())
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
	cmd := &cobra.Command{Use: "incidents", Short: "List/retry incidents via Orchestration API"}
	listFn := func(cmd *cobra.Command, args []string) error {
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cl, err := cluster.NewFromLab(paths.Home(), cfg)
		if err != nil {
			return err
		}
		raw, err := cl.SearchIncidents(cmd.Context(), 50)
		if err != nil {
			return fmt.Errorf("incidents search (is the lab up?): %w", err)
		}
		items := make([]incidents.Incident, 0, len(raw))
		for _, it := range raw {
			created, _ := time.Parse(time.RFC3339, it.CreationTime)
			items = append(items, incidents.Incident{
				ID:        it.Key,
				Created:   created,
				Error:     it.ErrorMessage,
				Process:   it.ProcessDefinitionID,
				JobWorker: it.ElementID,
				Key:       it.ProcessInstanceKey,
			})
		}
		fmt.Fprint(cmd.OutOrStdout(), incidents.FormatTable(items))
		return nil
	}
	cmd.AddCommand(&cobra.Command{Use: "list", Short: "List active incidents", RunE: listFn})
	var yes bool
	retry := &cobra.Command{
		Use:   "retry id",
		Short: "Resolve an incident (POST /v2/incidents/{key}/resolution)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing retry without --yes")
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl, err := cluster.NewFromLab(paths.Home(), cfg)
			if err != nil {
				return err
			}
			return cl.ResolveIncident(cmd.Context(), args[0])
		},
	}
	retry.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm resolve")
	cmd.AddCommand(retry)
	cmd.RunE = listFn
	return cmd
}

func newTraceCmd() *cobra.Command {
	var follow bool
	var interval, timeout time.Duration
	cmd := &cobra.Command{
		Use:   "trace instanceKey",
		Short: "Show process instance timeline from Orchestration API",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			cl, err := cluster.NewFromLab(paths.Home(), cfg)
			if err != nil {
				return err
			}
			render := func() (trace.Timeline, error) {
				pi, err := cl.GetProcessInstance(cmd.Context(), args[0])
				if err != nil {
					return trace.Timeline{}, err
				}
				els, err := cl.SearchElementInstances(cmd.Context(), args[0], 200)
				if err != nil {
					return trace.Timeline{}, err
				}
				steps := make([]trace.Step, 0, len(els))
				for _, el := range els {
					name := el.ElementName
					if name == "" {
						name = el.ElementID
					}
					st := el.State
					detail := ""
					if el.IncidentKey != "" {
						st = "INCIDENT"
						detail = el.IncidentKey
					}
					steps = append(steps, trace.Step{Name: name, State: st, Detail: detail})
				}
				state := pi.State
				if pi.HasIncident {
					state = "INCIDENT"
				}
				return trace.FromActivities(args[0], state, steps), nil
			}
			tl, err := render()
			if err != nil {
				return fmt.Errorf("trace (is the lab up?): %w", err)
			}
			fmt.Fprint(cmd.OutOrStdout(), trace.RenderASCII(tl))
			if !follow {
				return nil
			}
			deadline := time.Now().Add(timeout)
			prev := tl
			for time.Now().Before(deadline) {
				time.Sleep(interval)
				next, err := render()
				if err != nil {
					return err
				}
				if updated, changed := trace.FollowOnce(prev, next); changed {
					fmt.Fprint(cmd.OutOrStdout(), "\n"+trace.RenderASCII(updated))
					prev = updated
				}
				if next.State == "COMPLETED" || next.State == "TERMINATED" || next.State == "CANCELED" {
					return nil
				}
			}
			return fmt.Errorf("follow timeout")
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Poll until completed/timeout")
	cmd.Flags().DurationVar(&interval, "interval", 2*time.Second, "Follow poll interval")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Minute, "Follow timeout")
	return cmd
}

func newK8sCmd() *cobra.Command {
	var context, namespace, release string
	var yes bool
	cmd := &cobra.Command{Use: "k8s", Short: "Kubernetes helpers (kubectl wrappers)"}
	opts := func() k8s.Options {
		return k8s.Options{Context: context, Namespace: namespace, Release: release}
	}
	cmd.PersistentFlags().StringVar(&context, "context", "", "kube context")
	cmd.PersistentFlags().StringVar(&namespace, "namespace", "camunda", "namespace")
	cmd.PersistentFlags().StringVar(&release, "release", "camunda", "Helm release name")

	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "kubectl get pods,svc for release",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := k8s.Status(opts())
			fmt.Fprint(cmd.OutOrStdout(), out)
			return err
		},
	})
	var follow bool
	var tail int
	logs := &cobra.Command{
		Use:   "logs component",
		Short: "kubectl logs for a Camunda component",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := k8s.Logs(opts(), args[0], follow, tail)
			fmt.Fprint(cmd.OutOrStdout(), out)
			return err
		},
	}
	logs.Flags().BoolVarP(&follow, "follow", "f", false, "Follow")
	logs.Flags().IntVar(&tail, "tail", 100, "Tail lines")
	cmd.AddCommand(logs)

	restart := &cobra.Command{
		Use:   "restart component",
		Short: "Rollout restart deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing restart without --yes")
			}
			out, err := k8s.Restart(opts(), args[0])
			fmt.Fprint(cmd.OutOrStdout(), out)
			return err
		},
	}
	restart.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm")
	cmd.AddCommand(restart)

	var replicas int
	scale := &cobra.Command{
		Use:   "scale component",
		Short: "Scale a deployment",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if !yes {
				return fmt.Errorf("refusing scale without --yes")
			}
			out, err := k8s.Scale(opts(), args[0], replicas)
			fmt.Fprint(cmd.OutOrStdout(), out)
			return err
		},
	}
	scale.Flags().BoolVarP(&yes, "yes", "y", false, "Confirm")
	scale.Flags().IntVar(&replicas, "replicas", 1, "Replica count")
	cmd.AddCommand(scale)
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

func loadLintIgnore() []string {
	root, err := findProjectRoot()
	if err != nil {
		return nil
	}
	cfg, err := project.Load(filepath.Join(root, project.ConfigFileName))
	if err != nil {
		return nil
	}
	_ = cfg
	// v1 config has no lint.ignore yet — reserved
	return nil
}

func gitShow(ref, path string) (string, error) {
	out, err := exec.Command("git", "show", ref+":"+path).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git show %s:%s: %s", ref, path, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}
