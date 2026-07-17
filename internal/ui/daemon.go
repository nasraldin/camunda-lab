package ui

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

// EnsureOpts starts the Lab UI in the background when it is not already running.
type EnsureOpts struct {
	Options
	Open bool
}

// BaseURL returns the Lab UI root URL for the given options.
func BaseURL(opts Options) string {
	return fmt.Sprintf("http://%s/", net.JoinHostPort(opts.Host, strconv.Itoa(opts.Port)))
}

func apiOverviewURL(opts Options) string {
	return BaseURL(opts) + "api/v1/overview"
}

// IsRunning reports whether the Lab UI HTTP server is accepting requests.
func IsRunning(opts Options) bool {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(apiOverviewURL(opts))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// EnsureBackground starts the Lab UI detached when needed.
func EnsureBackground(opts EnsureOpts) error {
	if err := assertLoopback(opts.Host); err != nil {
		return err
	}
	if IsRunning(opts.Options) {
		fmt.Fprintf(os.Stderr, "Lab UI already running at %s\n", BaseURL(opts.Options))
		if opts.Open {
			_ = openBrowser(BaseURL(opts.Options))
		}
		return nil
	}
	if err := startBackground(opts.Options); err != nil {
		return err
	}
	if err := waitReady(opts.Options, 15*time.Second); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Lab UI running in background at %s\n", BaseURL(opts.Options))
	fmt.Fprintf(os.Stderr, "Logs: %s\n", uiLogPath())
	if opts.Open {
		_ = openBrowser(BaseURL(opts.Options))
	}
	return nil
}

// StopBackground stops a background Lab UI started by this CLI.
func StopBackground(opts Options) error {
	if err := assertLoopback(opts.Host); err != nil {
		return err
	}
	if !IsRunning(opts) {
		_ = os.Remove(paths.UIPidFile())
		return fmt.Errorf("lab UI is not running")
	}
	pid, _, err := readPIDFile()
	if err != nil {
		return fmt.Errorf("lab UI is running but no pid file at %s (started in another terminal?)", paths.UIPidFile())
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop lab UI (pid %d): %w", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if !IsRunning(opts) {
			_ = os.Remove(paths.UIPidFile())
			fmt.Fprintf(os.Stderr, "Lab UI stopped\n")
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("lab UI did not stop within 5s (pid %d)", pid)
}

func startBackground(opts Options) error {
	if err := os.MkdirAll(paths.LogsDir(), 0o755); err != nil {
		return fmt.Errorf("create logs dir: %w", err)
	}
	logFile, err := os.OpenFile(uiLogPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open ui log: %w", err)
	}
	defer logFile.Close()

	exe, err := os.Executable()
	if err != nil {
		return err
	}
	args := []string{
		"ui",
		"--foreground",
		"--no-open",
		"--host", opts.Host,
		"--port", strconv.Itoa(opts.Port),
	}
	cmd := exec.Command(exe, args...)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start lab UI: %w", err)
	}
	if err := writePIDFile(cmd.Process.Pid, opts.Port); err != nil {
		_ = cmd.Process.Kill()
		return err
	}
	return nil
}

func waitReady(opts Options, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if IsRunning(opts) {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("lab UI did not become ready within %s (see %s)", timeout, uiLogPath())
}

func uiLogPath() string {
	return filepath.Join(paths.LogsDir(), "ui.log")
}

func writePIDFile(pid, port int) error {
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		return err
	}
	body := fmt.Sprintf("%d\n%d\n", pid, port)
	return os.WriteFile(paths.UIPidFile(), []byte(body), 0o644)
}

func readPIDFile() (pid int, port int, err error) {
	data, err := os.ReadFile(paths.UIPidFile())
	if err != nil {
		return 0, 0, err
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 1 || lines[0] == "" {
		return 0, 0, fmt.Errorf("invalid pid file")
	}
	pid, err = strconv.Atoi(lines[0])
	if err != nil {
		return 0, 0, err
	}
	if len(lines) >= 2 {
		port, _ = strconv.Atoi(lines[1])
	}
	return pid, port, nil
}
