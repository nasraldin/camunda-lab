package cli

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/prompt"
	"github.com/spf13/cobra"
)

func newAICmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "ai",
		Short: "Enable Camunda MCP + AI Agent connector secrets",
	}
	cmd.AddCommand(newAIEnableCmd())
	cmd.AddCommand(newAIDisableCmd())
	cmd.AddCommand(newAIStatusCmd())
	cmd.AddCommand(newAIConfigCmd())
	return cmd
}

func newAIEnableCmd() *cobra.Command {
	var openaiKey, anthropicKey, openaiBase string
	cmd := &cobra.Command{
		Use:   "enable",
		Short: "Enable MCP URLs and inject AI Agent connector secrets",
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := resolveAISecrets(openaiKey, anthropicKey, openaiBase, isInteractive())
			if err != nil {
				return err
			}
			if err := lab.New().EnableAI(cmd.Context(), s); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "AI/MCP enabled.")
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out, err := ai.MCPClientConfig(cfg)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout())
			fmt.Fprintln(cmd.OutOrStdout(), "MCP client config (paste into Cursor / Claude):")
			fmt.Fprint(cmd.OutOrStdout(), out)
			fmt.Fprintln(cmd.OutOrStdout(), "Next: camunda urls && camunda ai status")
			return nil
		},
	}
	cmd.Flags().StringVar(&openaiKey, "openai-key", "", "OpenAI API key (SECRET_OPENAI_API_KEY)")
	cmd.Flags().StringVar(&anthropicKey, "anthropic-key", "", "Anthropic API key (SECRET_ANTHROPIC_API_KEY)")
	cmd.Flags().StringVar(&openaiBase, "openai-base-url", "", "Optional OpenAI-compatible base URL")
	return cmd
}

func newAIDisableCmd() *cobra.Command {
	var wipe bool
	cmd := &cobra.Command{
		Use:   "disable",
		Short: "Disable AI overlay (optionally wipe secrets)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := lab.New().DisableAI(cmd.Context(), wipe); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "AI/MCP disabled.")
			return nil
		},
	}
	cmd.Flags().BoolVar(&wipe, "wipe-secrets", false, "Delete ~/.camunda-lab/ai.env")
	return cmd
}

func newAIStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show AI/MCP enablement and MCP HTTP probe",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			s, err := ai.LoadSecrets()
			if err != nil {
				return err
			}
			rep := display.Report{
				Title: "Camunda Lab AI / MCP",
				Fields: []display.Field{
					display.KV("Version", cfg.Version),
					display.KV("Profile", cfg.Profile),
					display.KV("AI enabled", fmt.Sprintf("%v", cfg.AI.Enabled)),
					display.KV("OpenAI key", ai.Mask(s.OpenAIKey)),
					display.KV("Anthropic key", ai.Mask(s.AnthropicKey)),
					display.KV("OpenAI base URL", maskURL(s.OpenAIBaseURL)),
				},
			}
			if cfg.AI.Enabled {
				rep.Sections = append(rep.Sections, display.Section{
					Title: "MCP probe",
					Items: []string{probeMCPLine(cfg)},
				})
			}
			rep.Write(cmd.OutOrStdout())
			return nil
		},
	}
}

func newAIConfigCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "config",
		Short: "Print MCP client JSON for Cursor / Claude",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			out, err := ai.MCPClientConfig(cfg)
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}

func resolveAISecrets(openaiKey, anthropicKey, openaiBase string, allowPrompt bool) (ai.Secrets, error) {
	existing, err := ai.LoadSecrets()
	if err != nil {
		return ai.Secrets{}, err
	}
	s := ai.Secrets{
		OpenAIKey:     firstNonEmpty(openaiKey, os.Getenv("SECRET_OPENAI_API_KEY"), existing.OpenAIKey),
		AnthropicKey:  firstNonEmpty(anthropicKey, os.Getenv("SECRET_ANTHROPIC_API_KEY"), existing.AnthropicKey),
		OpenAIBaseURL: firstNonEmpty(openaiBase, os.Getenv("SECRET_OPENAI_BASE_URL"), existing.OpenAIBaseURL),
	}
	if s.Configured() {
		return s, nil
	}
	if !allowPrompt {
		return s, fmt.Errorf("set at least one of --openai-key, --anthropic-key, --openai-base-url (or SECRET_* env / ai.env)")
	}
	fmt.Fprintln(os.Stderr, "Configure at least one AI provider for the connectors runtime.")
	if v, err := prompt.String(os.Stdin, os.Stderr, "OpenAI API key (blank to skip)", ""); err == nil {
		s.OpenAIKey = v
	}
	if v, err := prompt.String(os.Stdin, os.Stderr, "Anthropic API key (blank to skip)", ""); err == nil {
		s.AnthropicKey = v
	}
	if v, err := prompt.String(os.Stdin, os.Stderr, "OpenAI-compatible base URL (blank to skip)", ""); err == nil {
		s.OpenAIBaseURL = v
	}
	if !s.Configured() {
		return s, fmt.Errorf("set at least one of --openai-key, --anthropic-key, --openai-base-url (or SECRET_* env / ai.env)")
	}
	return s, nil
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func isInteractive() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func maskURL(v string) string {
	if v == "" {
		return "(not set)"
	}
	return v
}

func probeMCPLine(cfg config.Config) string {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := 8080
	if cfg.Version == "8.8" {
		port = 8088
	}
	url := fmt.Sprintf("http://%s:%d/mcp/cluster", host, port)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return display.Warn(fmt.Sprintf("mcp-cluster — %s", err.Error()))
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	detail := fmt.Sprintf("HTTP %d", resp.StatusCode)
	switch {
	case resp.StatusCode == 401 && cfg.Profile == "full":
		return display.Warn(fmt.Sprintf("mcp-cluster (%s) — OIDC required; use camunda ai config", detail))
	case resp.StatusCode >= 200 && resp.StatusCode < 300:
		return display.Success(fmt.Sprintf("mcp-cluster (%s)", detail))
	default:
		return display.Fail(fmt.Sprintf("mcp-cluster — %s", detail))
	}
}
