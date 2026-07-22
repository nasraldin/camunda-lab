package cli

import (
	"context"
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
	return newDoctorCmdWithDependencies(doctorCommandDependencies{
		runShallow: doctor.Run,
		loadConfig: config.Load,
		runDeep:    doctor.RunDeep,
	})
}

type doctorCommandDependencies struct {
	runShallow func(bool) doctor.Report
	loadConfig func() (config.Config, error)
	runDeep    func(context.Context, config.Config, doctor.DeepOptions) (doctor.DeepReport, error)
}

func newDoctorCmdWithDependencies(deps doctorCommandDependencies) *cobra.Command {
	var fix, deep, jsonOutput bool
	var timeout time.Duration
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run health diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			if jsonOutput && !deep {
				return fmt.Errorf("--json requires --deep")
			}
			rep := deps.runShallow(fix)
			if !deep {
				fmt.Fprint(cmd.OutOrStdout(), rep.Format())
				if !rep.OK {
					return policyFailure("doctor")
				}
				return nil
			}
			cfg, err := deps.loadConfig()
			if err != nil {
				return err
			}
			deepReport, err := deps.runDeep(cmd.Context(), cfg, doctor.DeepOptions{PerCheckTimeout: timeout})
			if err != nil {
				return err
			}
			if jsonOutput {
				checks := append([]doctor.Check(nil), deepReport.Checks...)
				if !rep.OK {
					checks = append(checks, doctor.Check{
						ID: "shallow.prerequisites", Category: "configuration", Status: doctor.StatusFail,
						Summary: "Basic prerequisites have issues", Detail: "One or more basic prerequisite checks failed",
						Remediation: rep.FixHint, Required: true,
					})
				}
				report := doctor.DeepReport{Checks: checks}
				content, marshalErr := report.JSON()
				if marshalErr != nil {
					return marshalErr
				}
				_, _ = cmd.OutOrStdout().Write(append(content, '\n'))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), doctor.FormatDeep(rep, deepReport))
			}
			if !rep.OK || !doctor.DeepOK(deepReport) {
				return policyFailure("doctor")
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&fix, "fix", false, "Attempt common repairs")
	cmd.Flags().BoolVar(&deep, "deep", false, "Probe lab component endpoints")
	cmd.Flags().BoolVar(&jsonOutput, "json", false, "Structured JSON output (requires --deep)")
	cmd.Flags().DurationVar(&timeout, "timeout", 5*time.Second, "Per-probe timeout for --deep")
	return developerCommand(cmd)
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
