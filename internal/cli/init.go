package cli

import (
	"os"
	"path/filepath"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/project"
	"github.com/nasraldin/camunda-lab/internal/prompt"
	"github.com/spf13/cobra"
)

func newInitCmd() *cobra.Command {
	var (
		name      string
		version   string
		profile   string
		resources string
		yes       bool
		force     bool
	)
	cmd := &cobra.Command{
		Use:   "init [dir]",
		Short: "Scaffold a Camunda application project",
		Long: `Create a Camunda project tree with bpmn/, dmn/, forms/, .camunda.yaml, and README.

Does not start the lab or deploy processes. Use camunda install for a local stack;
deploy with official Camunda tooling.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			dir := "."
			if len(args) > 0 {
				dir = args[0]
			}
			abs, err := filepath.Abs(dir)
			if err != nil {
				return err
			}

			opts := project.ScaffoldOpts{
				Dir:       abs,
				Name:      name,
				Version:   version,
				Profile:   profile,
				Resources: resources,
				Force:     force,
			}

			if opts.Name == "" {
				opts.Name = filepath.Base(abs)
			}
			if opts.Version == "" {
				opts.Version = defaultInitVersion()
			}
			if opts.Profile == "" {
				opts.Profile = "light"
			}
			if opts.Resources == "" {
				opts.Resources = "balanced"
			}

			if !yes && isInteractive() {
				opts, err = promptInit(opts)
				if err != nil {
					return err
				}
			}

			display.Step(cmd.OutOrStdout(), "Scaffolding project in %s...", abs)
			if err := project.Scaffold(opts); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "Created %s", filepath.Join(abs, project.ConfigFileName))
			display.Report{
				Title: "Project ready",
				Fields: []display.Field{
					display.KV("Directory", abs),
					display.KV("Name", opts.Name),
					display.KV("Version hint", opts.Version),
					display.KV("Profile hint", opts.Profile),
				},
				Footer: []string{
					"Next: camunda install --yes   (local lab)",
					"Deploy processes with official Camunda tooling.",
				},
			}.Write(cmd.OutOrStdout())
			return nil
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Project name (default: directory basename)")
	cmd.Flags().StringVar(&version, "version", "", "Camunda version hint (default: active lab or 8.9)")
	cmd.Flags().StringVar(&profile, "profile", "", "Lab profile hint (light|full|modeler)")
	cmd.Flags().StringVar(&resources, "resources", "", "Lab resources hint (small|balanced|power)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Non-interactive")
	cmd.Flags().BoolVar(&force, "force", false, "Allow non-empty target directory")
	return cmd
}

func defaultInitVersion() string {
	if _, err := os.Stat(paths.ConfigFile()); err != nil {
		return "8.9"
	}
	cfg, err := config.Load()
	if err != nil || cfg.Version == "" {
		return "8.9"
	}
	return cfg.Version
}

func promptInit(opts project.ScaffoldOpts) (project.ScaffoldOpts, error) {
	name, err := prompt.String(os.Stdin, os.Stderr, "Project name", opts.Name)
	if err != nil {
		return opts, err
	}
	opts.Name = name

	version, err := prompt.String(os.Stdin, os.Stderr, "Camunda version hint", opts.Version)
	if err != nil {
		return opts, err
	}
	opts.Version = version

	profile, err := prompt.Choose(os.Stdin, os.Stderr, "Lab profile hint", []string{"light", "full", "modeler"}, indexOf([]string{"light", "full", "modeler"}, opts.Profile))
	if err != nil {
		return opts, err
	}
	opts.Profile = profile

	resources, err := prompt.Choose(os.Stdin, os.Stderr, "Lab resources hint", []string{"small", "balanced", "power"}, indexOf([]string{"small", "balanced", "power"}, opts.Resources))
	if err != nil {
		return opts, err
	}
	opts.Resources = resources
	return opts, nil
}

func indexOf(opts []string, v string) int {
	for i, o := range opts {
		if o == v {
			return i
		}
	}
	return 0
}
