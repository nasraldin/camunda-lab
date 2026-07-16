package cli

import (
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/spf13/cobra"
)

func newInstallCmd() *cobra.Command {
	var (
		version      string
		profile      string
		resources    string
		yes          bool
		aiFlag       bool
		openaiKey    string
		anthropicKey string
		openaiBase   string
	)
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Install and start a Camunda lab",
		RunE: func(cmd *cobra.Command, args []string) error {
			opts := lab.InstallOpts{
				Version:   version,
				Profile:   profile,
				Resources: resources,
				Yes:       yes,
				AI:        aiFlag,
			}
			if aiFlag {
				s, err := resolveAISecrets(openaiKey, anthropicKey, openaiBase, !yes && isInteractive())
				if err != nil {
					return err
				}
				opts.AISecrets = s
			}
			return lab.New().Install(cmd.Context(), opts)
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Camunda minor (8.7|8.8|8.9|8.10)")
	cmd.Flags().StringVar(&profile, "profile", "", "Compose profile (light|full|modeler)")
	cmd.Flags().StringVar(&resources, "resources", "", "Resource profile (small|balanced|power)")
	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "Non-interactive (use flags/defaults)")
	cmd.Flags().BoolVar(&aiFlag, "ai", false, "Enable MCP URLs + AI Agent connector secrets (8.9+)")
	cmd.Flags().StringVar(&openaiKey, "openai-key", "", "OpenAI API key for connectors (SECRET_OPENAI_API_KEY)")
	cmd.Flags().StringVar(&anthropicKey, "anthropic-key", "", "Anthropic API key")
	cmd.Flags().StringVar(&openaiBase, "openai-base-url", "", "Optional OpenAI-compatible base URL")
	return cmd
}

func newUpCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "up",
		Aliases: []string{"start"},
		Short:   "Start the active lab",
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.New().Up(cmd.Context())
		},
	}
}

func newDownCmd() *cobra.Command {
	return &cobra.Command{
		Use:     "down",
		Aliases: []string{"stop"},
		Short:   "Stop the lab (keep volumes)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return lab.New().Down(cmd.Context(), false)
		},
	}
}

func newRestartCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "restart",
		Short: "Restart the lab",
		RunE: func(cmd *cobra.Command, args []string) error {
			l := lab.New()
			if err := l.Down(cmd.Context(), false); err != nil {
				return err
			}
			return l.Up(cmd.Context())
		},
	}
}

func newStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show lab status",
		RunE: func(cmd *cobra.Command, args []string) error {
			out, err := lab.New().Status(cmd.Context())
			if err != nil {
				return err
			}
			fmt.Fprint(cmd.OutOrStdout(), out)
			return nil
		},
	}
}
