package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/config"
)

// Inspector supplies the state required by deep diagnostics.
type Inspector interface {
	ComposeServices(context.Context, config.Config) ([]ServiceState, error)
	DiskUsage(context.Context) (DiskUsage, error)
	Volumes(context.Context, string) ([]VolumeState, error)
}

type ServiceState struct {
	Name     string `json:"name"`
	State    string `json:"state"`
	Health   string `json:"health"`
	ExitCode int    `json:"exitCode"`
}

type composeServiceJSON struct {
	Name     string      `json:"Name"`
	Service  string      `json:"Service"`
	State    string      `json:"State"`
	Health   string      `json:"Health"`
	ExitCode json.Number `json:"ExitCode"`
}

func decodeComposeServices(data []byte) ([]ServiceState, error) {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return []ServiceState{}, nil
	}
	var raw []composeServiceJSON
	if data[0] == '[' {
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("decode compose services: %w", err)
		}
	} else {
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			line = bytes.TrimSpace(line)
			if len(line) == 0 {
				continue
			}
			var item composeServiceJSON
			if err := json.Unmarshal(line, &item); err != nil {
				return nil, fmt.Errorf("decode compose service: %w", err)
			}
			raw = append(raw, item)
		}
	}
	out := make([]ServiceState, 0, len(raw))
	for _, item := range raw {
		name := item.Service
		if name == "" {
			name = item.Name
		}
		exitCode, _ := strconv.Atoi(item.ExitCode.String())
		out = append(out, ServiceState{
			Name: name, State: strings.ToLower(item.State),
			Health: strings.ToLower(item.Health), ExitCode: exitCode,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
