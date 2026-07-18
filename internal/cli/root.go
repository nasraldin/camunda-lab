package cli

import (
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
	root.AddCommand(newSwitchCmd())
	root.AddCommand(newProfileCmd())
	root.AddCommand(newResourcesCmd())
	root.AddCommand(newURLsCmd())
	root.AddCommand(newOpenCmd())
	root.AddCommand(newLogsCmd())
	root.AddCommand(newDoctorCmd())
	root.AddCommand(newWaitCmd())
	root.AddCommand(newSmokeCmd())
	root.AddCommand(newNukeCmd())
	root.AddCommand(newToolsCmd())
	root.AddCommand(newAICmd())
	root.AddCommand(newMonitoringCmd())
	root.AddCommand(newUICmd())
	return root
}
