package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/nasraldin/camunda-lab/internal/config"
)

type DiskUsage struct {
	Percent float64 `json:"percent"`
	Detail  string  `json:"detail"`
}

type VolumeState struct {
	Name    string `json:"name"`
	Present bool   `json:"present"`
}

// CommandRunner is injected so tests never require a live Docker daemon.
type CommandRunner interface {
	Run(context.Context, int64, string, ...string) (CommandOutput, error)
}

type execCommandRunner struct{}

type CommandOutput struct {
	Stdout   []byte
	Stderr   []byte
	Overflow bool
}

var ErrCommandOutputOverflow = fmt.Errorf("command output limit exceeded")

const defaultCommandOutputLimit = int64(1 << 20)

type boundedBuffer struct {
	mu       sync.Mutex
	data     []byte
	limit    int64
	overflow bool
}

func newBoundedBuffer(limit int64) *boundedBuffer {
	if limit < 0 {
		limit = 0
	}
	return &boundedBuffer{data: make([]byte, 0, minInt64(limit, 4096)), limit: limit}
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	remaining := b.limit - int64(len(b.data))
	if remaining > 0 {
		count := int64(len(p))
		if count > remaining {
			count = remaining
		}
		b.data = append(b.data, p[:count]...)
	}
	if int64(len(p)) > remaining {
		b.overflow = true
	}
	// Continue draining the process pipes after overflow. Returning len(p)
	// avoids blocking the child while retaining only the configured limit.
	return len(p), nil
}

func (b *boundedBuffer) Bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]byte(nil), b.data...)
}

func (b *boundedBuffer) Overflow() bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.overflow
}

func (execCommandRunner) Run(ctx context.Context, limit int64, name string, args ...string) (CommandOutput, error) {
	if limit <= 0 {
		limit = defaultCommandOutputLimit
	}
	cmd := exec.CommandContext(ctx, name, args...)
	stdout := newBoundedBuffer(limit)
	stderr := newBoundedBuffer(limit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err := cmd.Run()
	out := CommandOutput{
		Stdout: stdout.Bytes(), Stderr: stderr.Bytes(),
		Overflow: stdout.Overflow() || stderr.Overflow(),
	}
	if ctxErr := ctx.Err(); ctxErr != nil {
		contextError := fmt.Errorf("command canceled: %w", ctxErr)
		if out.Overflow {
			return out, errors.Join(contextError, ErrCommandOutputOverflow)
		}
		return out, contextError
	}
	if out.Overflow {
		return out, ErrCommandOutputOverflow
	}
	if err != nil {
		detail := strings.TrimSpace(string(out.Stderr))
		if detail == "" {
			detail = err.Error()
		}
		return out, fmt.Errorf("%s: %w", detail, err)
	}
	return out, nil
}

// DockerInspector reads Docker/Compose state without mutating it.
type DockerInspector struct {
	Commands    CommandRunner
	OutputLimit int64
}

func (d DockerInspector) runner() CommandRunner {
	if d.Commands != nil {
		return d.Commands
	}
	return execCommandRunner{}
}

func (d DockerInspector) outputLimit() int64 {
	if d.OutputLimit > 0 {
		return d.OutputLimit
	}
	return defaultCommandOutputLimit
}

func (d DockerInspector) ComposeServices(ctx context.Context, cfg config.Config) ([]ServiceState, error) {
	out, err := d.runner().Run(ctx, d.outputLimit(), "docker", "compose", "-p", cfg.ComposeProject, "ps", "--all", "--format", "json")
	if out.Overflow && !errors.Is(err, ErrCommandOutputOverflow) {
		err = errors.Join(err, ErrCommandOutputOverflow)
	}
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}
	return decodeComposeServices(out.Stdout)
}

func (d DockerInspector) DiskUsage(ctx context.Context) (DiskUsage, error) {
	out, err := d.runner().Run(ctx, d.outputLimit(), "docker", "system", "df", "--format", "json")
	if out.Overflow && !errors.Is(err, ErrCommandOutputOverflow) {
		err = errors.Join(err, ErrCommandOutputOverflow)
	}
	if err != nil {
		return DiskUsage{}, fmt.Errorf("docker system df: %w", err)
	}
	return decodeDiskUsage(out.Stdout)
}

func (d DockerInspector) Volumes(ctx context.Context, project string) ([]VolumeState, error) {
	out, err := d.runner().Run(ctx, d.outputLimit(), "docker", "volume", "ls",
		"--filter", "label=com.docker.compose.project="+project, "--format", "json")
	if out.Overflow && !errors.Is(err, ErrCommandOutputOverflow) {
		err = errors.Join(err, ErrCommandOutputOverflow)
	}
	if err != nil {
		return nil, fmt.Errorf("docker volume ls: %w", err)
	}
	var volumes []VolumeState
	for _, line := range bytes.Split(bytes.TrimSpace(out.Stdout), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var item struct {
			Name string `json:"Name"`
		}
		if err := json.Unmarshal(line, &item); err != nil {
			return nil, fmt.Errorf("decode docker volume: %w", err)
		}
		volumes = append(volumes, VolumeState{Name: item.Name, Present: true})
	}
	if volumes == nil {
		volumes = []VolumeState{}
	}
	sort.Slice(volumes, func(i, j int) bool { return volumes[i].Name < volumes[j].Name })
	return volumes, nil
}

func minInt64(a, b int64) int {
	if a < b {
		return int(a)
	}
	return int(b)
}

var percentPattern = regexp.MustCompile(`\(([0-9]+(?:\.[0-9]+)?)%\)`)

func decodeDiskUsage(data []byte) (DiskUsage, error) {
	var highest float64
	var details []string
	for _, line := range bytes.Split(bytes.TrimSpace(data), []byte{'\n'}) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var item map[string]any
		if err := json.Unmarshal(line, &item); err != nil {
			return DiskUsage{}, fmt.Errorf("decode docker disk usage: %w", err)
		}
		reclaimable, _ := item["Reclaimable"].(string)
		if match := percentPattern.FindStringSubmatch(reclaimable); len(match) == 2 {
			percent, _ := strconv.ParseFloat(match[1], 64)
			if percent > highest {
				highest = percent
			}
		}
		if kind, ok := item["Type"].(string); ok {
			details = append(details, kind)
		}
	}
	sort.Strings(details)
	return DiskUsage{Percent: highest, Detail: strings.Join(details, ", ")}, nil
}
