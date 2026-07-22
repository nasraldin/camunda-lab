package lab

import (
	"context"
	"os"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
)

// monitoringServices are the compose services added by the monitoring overlay.
var monitoringServices = []string{"prometheus", "grafana", "elasticsearch-exporter"}

// monitoringContainers are the explicit container_name values from
// embed/monitoring.yaml, used to tear the add-on down once the overlay is
// dropped from the active compose file set.
var monitoringContainers = []string{
	"camunda-lab-prometheus",
	"camunda-lab-grafana",
	"camunda-lab-es-exporter",
}

func (l *Lab) EnableMonitoring(ctx context.Context) error {
	if err := config.Update(func(current *config.Config) error {
		current.Monitoring.Enabled = true
		return nil
	}); err != nil {
		return err
	}
	return l.RecreateMonitoring(ctx)
}

func (l *Lab) DisableMonitoring(ctx context.Context) error {
	if err := config.Update(func(current *config.Config) error {
		current.Monitoring.Enabled = false
		return nil
	}); err != nil {
		return err
	}
	// Stop and remove the monitoring containers now that the overlay is dropped.
	display.Step(os.Stdout, "Stopping monitoring...")
	if err := l.Engine.RemoveByName(monitoringContainers...); err != nil {
		return err
	}
	display.Done(os.Stdout, "Monitoring disabled.")
	return nil
}

func (l *Lab) RecreateMonitoring(ctx context.Context) error {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir, files, envFiles, err := l.resolve(cfg)
	if err != nil {
		return err
	}
	display.Step(os.Stdout, "Starting monitoring (Prometheus + Grafana)...")
	for _, svc := range monitoringServices {
		if err := l.Engine.UpService(workDir, files, envFiles, cfg.ComposeProject, svc); err != nil {
			return err
		}
	}
	display.Done(os.Stdout, "Monitoring enabled — Grafana on http://localhost:3000 (admin/admin).")
	return nil
}
