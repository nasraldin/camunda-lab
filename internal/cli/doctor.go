package cli

import (
	"fmt"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/smoke"
	"github.com/nasraldin/camunda-lab/internal/tools"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/spf13/cobra"
)

func newDoctorCmd() *cobra.Command {
	var fix, deep bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run health diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			rep := doctor.Run(fix)
			if !deep {
				fmt.Fprint(cmd.OutOrStdout(), rep.Format())
				if !rep.OK {
					return fmt.Errorf("doctor found issues")
				}
				return nil
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			sections := doctor.Deep(cmd.Context(), cfg, doctor.DeepOptions{Timeout: timeout})
			fmt.Fprint(cmd.OutOrStdout(), doctor.FormatDeep(rep, sections))
			if !rep.OK || !doctor.DeepOK(sections) {
				return fmt.Errorf("doctor found issues")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Attempt common repairs")
	cmd.Flags().BoolVar(&deep, "deep", false, "Probe lab component endpoints")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "Per-probe timeout for --deep")
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
			display.Step(cmd.OutOrStdout(), "Waiting up to %s for healthy endpoints...", timeout)
			if err := smoke.Wait(cmd.Context(), cfg, timeout); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "Lab is healthy.")
			return nil
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
			res := smoke.Probe(cmd.Context(), cfg)
			fmt.Fprint(cmd.OutOrStdout(), res.Format(cfg))
			if !res.OK {
				return fmt.Errorf("smoke checks failed")
			}
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
			rep := display.Report{Title: "c8ctl"}
			if !ok {
				rep.Fields = []display.Field{display.KV("Status", "not installed")}
				rep.Footer = []string{"Install with: camunda tools c8ctl install"}
				rep.Write(cmd.OutOrStdout())
				return nil
			}
			rep.Fields = []display.Field{
				display.KV("Status", "installed"),
				display.KV("Path", path),
			}
			rep.Write(cmd.OutOrStdout())
			return nil
		},
	})
	c8.AddCommand(&cobra.Command{
		Use:   "install",
		Short: "Install c8ctl via npm (@camunda8/cli)",
		RunE: func(cmd *cobra.Command, args []string) error {
			display.Step(cmd.OutOrStdout(), "Installing @camunda8/cli via npm...")
			if err := tools.C8ctlInstall(); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "c8ctl installed.")
			return nil
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
			display.Report{
				Title: "Desktop Modeler Profile",
				Fields: []display.Field{
					display.KV("Name", "camunda-lab"),
					display.KV("REST", rest),
					display.KV("gRPC", grpc),
					display.KV("Wrote", path),
				},
				Footer: []string{"Open Desktop Modeler and select the camunda-lab profile."},
			}.Write(cmd.OutOrStdout())
			return nil
		},
	})
	cmd.AddCommand(modeler)
	return cmd
}
