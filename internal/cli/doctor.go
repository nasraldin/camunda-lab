package cli

import (
	"fmt"
	"os"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/smoke"
	"github.com/nasraldin/camunda-lab/internal/tools"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var fix bool
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run health diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			rep := doctor.Run(fix)
			for _, line := range rep.Lines {
				fmt.Fprintln(cmd.OutOrStdout(), line)
			}
			if rep.FixHint != "" {
				fmt.Fprintln(cmd.OutOrStdout(), "hint:", rep.FixHint)
			}
			if !rep.OK {
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Attempt common repairs")
	return cmd
}

func newWaitCmd() *cobra.Command {
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "wait",
		Short: "Wait until the lab is healthy",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Waiting up to %s for lab health...\n", timeout)
			return smoke.Wait(cmd.Context(), cfg, timeout)
		},
	}
	cmd.Flags().DurationVar(&timeout, "timeout", 10*time.Minute, "Max wait duration")
	return cmd
}

func newSmokeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "smoke",
		Short: "Run smoke checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if err := smoke.Run(cmd.Context(), cfg); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "smoke OK")
			return nil
		},
	}
}

func newNukeCmd() *cobra.Command {
	var yes bool
	cmd := &cobra.Command{
		Use:   "nuke",
		Short: "Wipe the lab completely",
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.New().Nuke(cmd.Context(), yes)
		},
	}
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Skip confirmation")
	return cmd
}

func newToolsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tools",
		Short: "Developer tool helpers",
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "c8ctl",
		Short: "c8ctl helpers",
	})
	c8 := cmd.Commands()[0]
	c8.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Check if c8ctl is installed",
		RunE: func(cmd *cobra.Command, args []string) error {
			ok, path, err := tools.C8ctlStatus()
			if err != nil {
				return err
			}
			if !ok {
				fmt.Fprintln(cmd.OutOrStdout(), "c8ctl: not installed (try: camunda tools c8ctl install)")
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "c8ctl: %s\n", path)
			return nil
		},
	})
	c8.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install c8ctl via npm (@camunda8/cli)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return tools.C8ctlInstall()
		},
	})

	modeler := &cobra.Command{Use: "modeler", Short: "Desktop Modeler helpers"}
	modeler.AddCommand(&cobra.Command{
		Use:   "profile",
		Short: "Write a Desktop Modeler connection profile for this lab",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			rest := urls.ModelerRESTBase(cfg)
			grpc := "localhost:26500"
			for _, e := range urls.List(cfg) {
				if e.Name == "grpc" {
					grpc = e.URL
				}
			}
			path, err := tools.WriteModelerProfile("camunda-lab", rest, grpc)
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Wrote Modeler profile 'camunda-lab' → %s\n", path)
			fmt.Fprintln(cmd.OutOrStdout(), "Open Desktop Modeler and select the camunda-lab profile.")
			_ = os.Stdout
			return nil
		},
	})
	cmd.AddCommand(modeler)
	return cmd
}
