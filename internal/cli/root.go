package cli

import (
	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/spf13/cobra"
)

var appVersion = "0.0.0-dev"

func SetVersion(v string) { appVersion = v }

func NewRoot() *cobra.Command {
	return NewRootWithDependencies(Dependencies{})
}

// NewRootWithDependencies builds the CLI with injectable collaborators for tests.
func NewRootWithDependencies(deps Dependencies) *cobra.Command {
	if deps.Doctor.runShallow == nil && deps.Doctor.loadConfig == nil && deps.Doctor.runDeep == nil {
		deps.Doctor = doctorCommandDependencies{
			runShallow: doctor.Run,
			loadConfig: config.Load,
			runDeep:    doctor.RunDeep,
		}
	}
	if deps.Restore == nil {
		checker := deps.RunningLab
		if checker == nil {
			checker = lab.New()
		}
		deps.Restore = backup.NewService(checker).Restore
		deps.RunningLab = checker
	} else if deps.RunningLab == nil {
		deps.RunningLab = lab.New()
	}

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
	root.AddCommand(newInitCmd())
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
	root.AddCommand(newDoctorCmdWithDependencies(deps.Doctor))
	root.AddCommand(newWaitCmd())
	root.AddCommand(newSmokeCmd())
	root.AddCommand(newNukeCmd())
	root.AddCommand(newToolsCmd())
	root.AddCommand(newAICmd())
	root.AddCommand(newUICmd())
	root.AddCommand(newLintCmdWith(deps))
	root.AddCommand(newDiffCmdWith(deps))
	root.AddCommand(newScanCmdWith(deps))
	root.AddCommand(newExplainCmdWith(deps))
	root.AddCommand(newReviewCmdWith(deps))
	root.AddCommand(newTestCmdWith(deps))
	root.AddCommand(newEnvCmdWith(deps))
	root.AddCommand(newPlanCmdWith(deps))
	root.AddCommand(newDriftCmdWith(deps))
	root.AddCommand(newBackupCmdWith(deps))
	root.AddCommand(newRestoreCmdWith(deps.Restore, deps.RunningLab))
	root.AddCommand(newIncidentsCmdWith(deps))
	root.AddCommand(newTraceCmdWith(deps))
	return root
}
