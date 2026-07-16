package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "camunda-lab %s\n", appVersion)
		},
	}
}

func newAboutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "about",
		Short: "Project info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), `Camunda Lab
  CLI       camunda %s
  Project   https://github.com/nasraldin/camunda-lab
  Note      Unofficial community project — not affiliated with Camunda GmbH
`, appVersion)
		},
	}
}
