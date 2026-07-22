package cli

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/spf13/cobra"
)

func newSwitchCmd() *cobra.Command {
	var wipe bool
	var aiFlag bool
	var openaiKey, anthropicKey, openaiBase string
	cmd := &cobra.Command{
		Use:   "switch <8.7|8.8|8.9|8.10>",
		Short: "Switch Camunda minor version",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			minor := args[0]
			if aiFlag {
				cfg, err := config.Load()
				if err != nil {
					return err
				}
				s, err := resolveAISecrets(openaiKey, anthropicKey, openaiBase, isInteractive())
				if err != nil {
					return err
				}
				if err := ai.ValidateForEnable(minor, cfg.Profile, s); err != nil {
					return err
				}
				if err := lab.New().Switch(cmd.Context(), minor, wipe); err != nil {
					return err
				}
				if err := lab.New().EnableAI(cmd.Context(), s); err != nil {
					return err
				}
				ensureUIBackground(false)
				return nil
			}
			if err := lab.New().Switch(cmd.Context(), minor, wipe); err != nil {
				return err
			}
			ensureUIBackground(false)
			return nil
		},
	}
	cmd.Flags().BoolVar(&wipe, "wipe", false, "Remove volumes before switching")
	cmd.Flags().BoolVar(&aiFlag, "ai", false, "Enable MCP URLs + AI Agent connector secrets (8.9+)")
	cmd.Flags().StringVar(&openaiKey, "openai-key", "", "OpenAI API key for connectors")
	cmd.Flags().StringVar(&anthropicKey, "anthropic-key", "", "Anthropic API key")
	cmd.Flags().StringVar(&openaiBase, "openai-base-url", "", "Optional OpenAI-compatible base URL")
	return cmd
}

func newProfileCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "profile <light|full|modeler>",
		Short: "Set compose profile (light|full|modeler)",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := lab.New().SetProfile(cmd.Context(), args[0]); err != nil {
				return err
			}
			ensureUIBackground(false)
			return nil
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
			printURLs(cmd, cfg)
			return nil
		},
	}
}

func printURLs(cmd *cobra.Command, cfg config.Config) {
	entries := urls.List(cfg)
	webApps := []string{"operate", "tasklist", "admin", "console", "optimize", "identity", "web-modeler", "keycloak", "elasticvue"}
	apis := []string{"rest", "orchestration", "grpc", "zeebe-http", "connectors", "elasticsearch", "mcp-cluster", "mcp-processes"}
	monitoring := []string{"grafana", "prometheus"}

	index := map[string]urls.Entry{}
	for _, entry := range entries {
		index[entry.Name] = entry
	}

	rep := display.Report{
		Title: "Camunda Lab URLs",
		Fields: []display.Field{
			display.KV("Version", cfg.Version),
			display.KV("Profile", cfg.Profile),
			display.KV("Host", cfg.Host),
		},
	}
	if lines := formatURLSection(index, webApps); len(lines) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Web apps", Items: lines})
	}
	if lines := formatURLSection(index, apis); len(lines) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "APIs and infra", Items: lines})
	}
	if lines := formatURLSection(index, monitoring); len(lines) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Monitoring", Items: lines})
	}
	if notes := collectAuthNotes(entries); len(notes) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Auth", Items: notes})
	}
	rep.Write(cmd.OutOrStdout())
}

func formatURLSection(index map[string]urls.Entry, names []string) []string {
	var lines []string
	for _, name := range names {
		entry, ok := index[name]
		if !ok {
			continue
		}
		lines = append(lines, fmt.Sprintf("%s -> %s", entry.Name, entry.URL))
	}
	return lines
}

func collectAuthNotes(entries []urls.Entry) []string {
	seen := map[string]bool{}
	var notes []string
	for _, entry := range entries {
		if entry.Notes == "" {
			continue
		}
		note := strings.TrimSpace(entry.Notes)
		switch note {
		case "demo/demo":
			note = "Camunda app login: demo / demo"
		case "admin/admin":
			note = "Keycloak admin: admin / admin"
		default:
			continue
		}
		if !seen[note] {
			seen[note] = true
			notes = append(notes, note)
		}
	}
	return notes
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
			display.Step(cmd.OutOrStdout(), "Opening %s → %s", name, e.URL)
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
