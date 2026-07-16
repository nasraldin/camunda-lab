package cli

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/spf13/cobra"
)

func newSwitchCmd() *cobra.Command {
	var wipe bool
	cmd := &cobra.Command{
		Use:   "switch <8.7|8.8|8.9|8.10>",
		Short: "Switch Camunda minor version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.New().Switch(cmd.Context(), args[0], wipe)
		},
	}
	cmd.Flags().BoolVar(&wipe, "wipe", false, "Remove volumes before switching")
	return cmd
}

func newProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profile <light|full|modeler>",
		Short: "Set compose profile (light|full|modeler)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.New().SetProfile(cmd.Context(), args[0])
		},
	}
}

func newResourcesCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "resources <small|balanced|power>",
		Short: "Set resource profile",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.New().SetResources(cmd.Context(), args[0])
		},
	}
}

func newLogsCmd() *cobra.Command {
	var follow bool
	cmd := &cobra.Command{
		Use:   "logs [service]",
		Short: "Show container logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			service := ""
			if len(args) == 1 {
				service = args[0]
			}
			return lab.New().Logs(cmd.Context(), service, follow)
		},
	}
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output")
	return cmd
}

func newURLsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "urls",
		Short: "Print component URLs",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			for _, e := range urls.List(cfg) {
				if e.Notes != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s %s  (%s)\n", e.Name, e.URL, e.Notes)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "%-14s %s\n", e.Name, e.URL)
				}
			}
			return nil
		},
	}
}

func newOpenCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "open [app]",
		Short: "Open a component URL in the browser",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			name := "operate"
			if len(args) == 1 {
				name = args[0]
			}
			e, err := urls.Find(cfg, name)
			if err != nil {
				return err
			}
			return openBrowser(e.URL)
		},
	}
}

func openBrowser(url string) error {
	var c *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		c = exec.Command("open", url)
	case "linux":
		c = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("open not supported on %s", runtime.GOOS)
	}
	return c.Start()
}
