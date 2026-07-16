package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var appVersion = "0.0.0-dev"

func SetVersion(v string) { appVersion = v }

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "camunda",
		Short: "Local Camunda 8 platform lab (Docker Compose)",
		Long: `camunda-lab — unofficial local Camunda 8 lab.

Wraps official Camunda Docker Compose distributions with install, version
switching, doctor, and developer tool helpers. Not affiliated with Camunda GmbH.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCmd())
	root.AddCommand(newAboutCmd())
	root.AddCommand(newInstallCmd())
	root.AddCommand(newUpCmd())
	root.AddCommand(newDownCmd())
	root.AddCommand(newRestartCmd())
	root.AddCommand(newStatusCmd())
	// stubs; real bodies in later tasks
	root.AddCommand(placeholder("switch", "Switch Camunda minor version"))
	root.AddCommand(placeholder("profile", "Set compose profile (light|full|modeler)"))
	root.AddCommand(placeholder("resources", "Set resource profile"))
	root.AddCommand(placeholder("urls", "Print component URLs"))
	root.AddCommand(placeholder("open", "Open a component URL in the browser"))
	root.AddCommand(placeholder("logs", "Show container logs"))
	root.AddCommand(placeholder("doctor", "Run health diagnostics"))
	root.AddCommand(placeholder("wait", "Wait until the lab is healthy"))
	root.AddCommand(placeholder("smoke", "Run smoke checks"))
	root.AddCommand(placeholder("nuke", "Wipe the lab completely"))
	root.AddCommand(placeholder("tools", "Developer tool helpers"))
	return root
}

func placeholder(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s: not implemented yet", use)
		},
	}
}
