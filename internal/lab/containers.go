package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

// Container is a compose service row for the Lab UI / API.
type Container struct {
	Name    string `json:"name"`
	Service string `json:"service"`
	Image   string `json:"image"`
	State   string `json:"state"`
	Health  string `json:"health"`
	Status  string `json:"status"`
	Uptime  string `json:"uptime,omitempty"`
	Ports   string `json:"ports,omitempty"`
}

func (l *Lab) ListContainers(ctx context.Context) ([]Container, error) {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return nil, err
	}
	if cfg.Version == "" {
		return nil, fmt.Errorf("no lab configured — run install first")
	}
	workDir := paths.VersionDir(cfg.Version)
	raw, err := l.Engine.PsJSON(workDir, cfg.ComposeProject)
	if err != nil {
		return nil, err
	}
	rows, err := parsePSJSON(raw)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Service < rows[j].Service })
	out := make([]Container, 0, len(rows))
	for _, row := range rows {
		out = append(out, Container{
			Name:    row.Name,
			Service: row.Service,
			Image:   row.Image,
			State:   row.State,
			Health:  row.Health,
			Status:  prettyState(row),
			Uptime:  row.RunningFor,
			Ports:   publishedPorts(row),
		})
	}
	return out, nil
}

func (l *Lab) RestartService(ctx context.Context, service string) error {
	_ = ctx
	if service == "" {
		return fmt.Errorf("service name required")
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir, files, envFiles, err := l.resolve(cfg)
	if err != nil {
		return err
	}
	return l.Engine.UpService(workDir, files, envFiles, cfg.ComposeProject, service)
}

func (l *Lab) StreamLogs(ctx context.Context, service string, follow bool, w io.Writer) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir := paths.VersionDir(cfg.Version)
	return l.Engine.LogsTo(ctx, workDir, cfg.ComposeProject, service, follow, w)
}

// ContainersJSON returns compose ps as a JSON array (for debugging).
func ContainersJSON(containers []Container) ([]byte, error) {
	return json.Marshal(containers)
}
