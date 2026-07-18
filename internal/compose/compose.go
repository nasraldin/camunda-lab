package compose

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// BuildArgs constructs `docker compose ...` argv after the `docker` binary.
func BuildArgs(subcommand, project string, files, envFiles []string, extra ...string) []string {
	args := []string{"compose", "-p", project}
	for _, f := range files {
		args = append(args, "-f", f)
	}
	for _, e := range envFiles {
		args = append(args, "--env-file", e)
	}
	args = append(args, subcommand)
	args = append(args, extra...)
	return args
}

// Engine runs docker compose operations (injectable for tests).
type Engine interface {
	Up(workDir string, files, envFiles []string, project string) error
	UpService(workDir string, files, envFiles []string, project, service string) error
	RemoveByName(names ...string) error
	Down(workDir string, files []string, project string, volumes bool) error
	Ps(workDir, project string) (string, error)
	PsJSON(workDir, project string) (string, error)
	Logs(workDir, project, service string, follow bool) error
	LogsTo(ctx context.Context, workDir, project, service string, follow bool, w io.Writer) error
}

type Runner struct {
	// Exec runs a command in workDir; args[0] is the binary.
	Exec func(workDir string, args []string) (string, error)
}

func NewRunner() *Runner {
	return &Runner{Exec: defaultExec}
}

func (r *Runner) Up(workDir string, files, envFiles []string, project string) error {
	args := BuildArgs("up", project, files, envFiles, "-d")
	_, err := r.Exec(workDir, append([]string{"docker"}, args...))
	if err != nil {
		return fmt.Errorf("docker compose up: %w", err)
	}
	return nil
}

func (r *Runner) UpService(workDir string, files, envFiles []string, project, service string) error {
	args := BuildArgs("up", project, files, envFiles, "-d", "--force-recreate", "--no-deps", service)
	_, err := r.Exec(workDir, append([]string{"docker"}, args...))
	if err != nil {
		return fmt.Errorf("docker compose up %s: %w", service, err)
	}
	return nil
}

// RemoveByName force-removes containers by name (docker rm -f), ignoring any
// that don't exist. Used to tear down overlay add-ons whose services are no
// longer in the active compose file set. Containers must set container_name.
func (r *Runner) RemoveByName(names ...string) error {
	if len(names) == 0 {
		return nil
	}
	args := append([]string{"docker", "rm", "-f"}, names...)
	if _, err := r.Exec("", args); err != nil {
		return fmt.Errorf("docker rm -f: %w", err)
	}
	return nil
}

func (r *Runner) Down(workDir string, files []string, project string, volumes bool) error {
	extra := []string{}
	if volumes {
		extra = append(extra, "-v", "--remove-orphans")
	}
	args := BuildArgs("down", project, files, nil, extra...)
	_, err := r.Exec(workDir, append([]string{"docker"}, args...))
	if err != nil {
		return fmt.Errorf("docker compose down: %w", err)
	}
	return nil
}

func (r *Runner) Ps(workDir, project string) (string, error) {
	args := []string{"docker", "compose", "-p", project, "ps"}
	return r.Exec(workDir, args)
}

func (r *Runner) PsJSON(workDir, project string) (string, error) {
	args := []string{"docker", "compose", "-p", project, "ps", "--format", "json"}
	return r.Exec(workDir, args)
}

func (r *Runner) Logs(workDir, project, service string, follow bool) error {
	return r.LogsTo(context.Background(), workDir, project, service, follow, os.Stdout)
}

func (r *Runner) LogsTo(ctx context.Context, workDir, project, service string, follow bool, w io.Writer) error {
	if w == nil {
		w = os.Stdout
	}
	args := []string{"compose", "-p", project, "logs", "--tail", "200"}
	if follow {
		args = append(args, "-f")
	}
	if service != "" {
		args = append(args, service)
	}
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Dir = workDir
	cmd.Stdout = w
	cmd.Stderr = w
	return cmd.Run()
}

func defaultExec(workDir string, args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("empty command")
	}
	cmd := exec.Command(args[0], args[1:]...)
	cmd.Dir = workDir
	cmd.Stdin = os.Stdin
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	// For logs -f, stream to OS; for others capture.
	if len(args) >= 3 && args[1] == "compose" && contains(args, "logs") {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return "", err
		}
		return "", nil
	}
	err := cmd.Run()
	out := stdout.String()
	if err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return out, fmt.Errorf("%s", msg)
	}
	return out, nil
}

func contains(args []string, want string) bool {
	for _, a := range args {
		if a == want {
			return true
		}
	}
	return false
}
