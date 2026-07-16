package cli

import (
	"github.com/nasraldin/camunda-lab/internal/ui"
	"github.com/spf13/cobra"
)

func newUICmd() *cobra.Command {
	opts := ui.DefaultOptions()
	var noOpen bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Start the local Lab UI (http://localhost:9090)",
		Long: `Start an embedded web UI to manage the lab without living in the terminal.

Binds to loopback only (no auth). Default http://localhost:9090 so the browser
can share Keycloak SSO cookies with Camunda apps. Serves Overview, Setup, Apps,
Containers, Logs, AI/MCP, Tools, and Danger (nuke).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Open = !noOpen
			opts.Version = appVersion
			return ui.Run(opts)
		},
	}
	cmd.Flags().StringVar(&opts.Host, "host", opts.Host, "Listen address (loopback only)")
	cmd.Flags().IntVar(&opts.Port, "port", opts.Port, "Listen port (env CAMUNDA_LAB_UI_PORT)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open a browser")
	return cmd
}
