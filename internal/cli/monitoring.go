package cli

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/spf13/cobra"
)

func newMonitoringCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "monitoring",
		Short: "Prometheus + Grafana dashboards for the local lab",
	}
	cmd.AddCommand(newMonitoringEnableCmd())
	cmd.AddCommand(newMonitoringDisableCmd())
	cmd.AddCommand(newMonitoringStatusCmd())
	return cmd
}

func newMonitoringEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable",
		Short: "Start Prometheus + Grafana with pre-provisioned dashboards",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := lab.New().EnableMonitoring(cmd.Context()); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "Monitoring enabled.")
			fmt.Fprintln(cmd.OutOrStdout(), "Grafana: http://localhost:3000 (admin/admin) · Prometheus: http://localhost:9490")
			fmt.Fprintln(cmd.OutOrStdout(), "Next: camunda open grafana")
			return nil
		},
	}
}

func newMonitoringDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable",
		Short: "Stop and remove the monitoring containers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := lab.New().DisableMonitoring(cmd.Context()); err != nil {
				return err
			}
			display.Done(cmd.OutOrStdout(), "Monitoring disabled.")
			return nil
		},
	}
}

func newMonitoringStatusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show monitoring enablement and a Grafana health probe",
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			host := cfg.Host
			if host == "" {
				host = "localhost"
			}
			rep := display.Report{
				Title: "Camunda Lab Monitoring",
				Fields: []display.Field{
					display.KV("Monitoring enabled", fmt.Sprintf("%v", cfg.Monitoring.Enabled)),
					display.KV("Grafana", fmt.Sprintf("http://%s:3000", host)),
					display.KV("Prometheus", fmt.Sprintf("http://%s:9490", host)),
				},
			}
			if cfg.Monitoring.Enabled {
				rep.Sections = append(rep.Sections, display.Section{
					Title: "Health",
					Items: []string{probeGrafanaLine(host)},
				})
			}
			rep.Write(cmd.OutOrStdout())
			return nil
		},
	}
}

func probeGrafanaLine(host string) string {
	url := fmt.Sprintf("http://%s:3000/api/health", host)
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		return display.Warn(fmt.Sprintf("grafana — %s", err.Error()))
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, resp.Body)
	detail := fmt.Sprintf("HTTP %d", resp.StatusCode)
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return display.Success(fmt.Sprintf("grafana (%s)", detail))
	}
	return display.Fail(fmt.Sprintf("grafana — %s", detail))
}
