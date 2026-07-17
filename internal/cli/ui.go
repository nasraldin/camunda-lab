package cli

import (
	"github.com/nasraldin/camunda-lab/internal/ui"
	"github.com/spf13/cobra"
)

func newUICmd() *cobra.Command {
	opts := ui.DefaultOptions()
	var noOpen, foreground, stop bool
	cmd := &cobra.Command{
		Use:   "ui",
		Short: "Lab UI control panel (http://localhost:9090)",
		Long: `Manage the embedded Lab UI that mirrors the CLI in the browser.

By default starts the UI in the background (if not already running) and opens
your browser. Use --foreground to run in the current terminal (Ctrl+C to stop).

Binds to loopback only (no auth). Serves Overview, Setup, Apps, Containers,
Logs, AI/MCP, Tools, and Danger (nuke).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			opts.Open = !noOpen
			opts.Version = appVersion
			if stop {
				return ui.StopBackground(opts)
			}
			if foreground {
				return ui.Run(opts)
			}
			return ui.EnsureBackground(ui.EnsureOpts{Options: opts, Open: opts.Open})
		},
	}
	cmd.Flags().StringVar(&opts.Host, "host", opts.Host, "Listen address (loopback only)")
	cmd.Flags().IntVar(&opts.Port, "port", opts.Port, "Listen port (env CAMUNDA_LAB_UI_PORT)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "Do not open a browser")
	cmd.Flags().BoolVarP(&foreground, "foreground", "f", false, "Run in the foreground (blocks until Ctrl+C)")
	cmd.Flags().BoolVar(&stop, "stop", false, "Stop the background Lab UI")
	return cmd
}
