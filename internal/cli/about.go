package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/spf13/cobra"
)

const (
	aboutName    = "Camunda Lab"
	aboutAuthor  = "Nasr Aldin"
	aboutWebsite = "https://nasraldin.com"
	aboutTagline = "Local Camunda 8 platform lab (Docker Compose)"
	aboutFooter  = "Unofficial community project — wraps Camunda's official Compose. Not affiliated with Camunda GmbH."
	aboutRepo    = "https://github.com/nasraldin/camunda-lab"
	aboutDocs    = "https://nasraldin.github.io/camunda-lab/"
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
		Short: "Project + runtime info card",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprint(cmd.OutOrStdout(), renderAbout(cmd.Root()))
		},
	}
}

func renderAbout(root *cobra.Command) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", aboutName)
	fmt.Fprintf(&b, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n\n")

	fmt.Fprintf(&b, "  Author      %s\n", aboutAuthor)
	fmt.Fprintf(&b, "  Website     %s\n\n", aboutWebsite)

	fmt.Fprintf(&b, "  Version     %s\n", appVersion)
	fmt.Fprintf(&b, "  Tagline     %s\n", aboutTagline)
	fmt.Fprintf(&b, "  CLI path    %s\n", cliPath())
	fmt.Fprintf(&b, "  Global      %s\n", globalLinkInfo())
	fmt.Fprintf(&b, "  Lab home    %s\n", paths.Home())
	fmt.Fprintf(&b, "  Config      %s\n", paths.ConfigFile())
	fmt.Fprintf(&b, "  Versions    %s\n", paths.VersionsDir())
	fmt.Fprintf(&b, "  Active      %s\n\n", activeLabInfo())

	fmt.Fprintf(&b, "  Docker      %s\n", dockerInfo())
	fmt.Fprintf(&b, "  Compose     %s\n", composeInfo())
	fmt.Fprintf(&b, "  Platform    %s\n", platformInfo())
	fmt.Fprintf(&b, "  Memory      %s\n", memoryInfo())
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		fmt.Fprintf(&b, "  DOCKER_HOST %s\n", host)
	}
	fmt.Fprintln(&b)

	fmt.Fprintf(&b, "  Features    compose · profiles · version-switch · overlays · c8ctl · modeler · doctor · smoke\n\n")
	fmt.Fprintf(&b, "  Repo        %s\n", aboutRepo)
	fmt.Fprintf(&b, "  Docs        %s\n\n", aboutDocs)
	fmt.Fprintf(&b, "  Commands    %d available — run: camunda help\n\n", countCommands(root))

	fmt.Fprintf(&b, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Fprintf(&b, "%s\n", aboutFooter)
	return b.String()
}

func cliPath() string {
	if exe, err := os.Executable(); err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			return resolved
		}
		return exe
	}
	if p, err := exec.LookPath("camunda"); err == nil {
		return p
	}
	return "(unknown)"
}

func globalLinkInfo() string {
	home, _ := os.UserHomeDir()
	candidates := []string{
		"/usr/local/bin/camunda",
		"/opt/homebrew/bin/camunda",
	}
	if home != "" {
		candidates = append([]string{filepath.Join(home, ".local", "bin", "camunda")}, candidates...)
	}
	for _, p := range candidates {
		fi, err := os.Lstat(p)
		if err != nil {
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(p)
			if err == nil {
				return fmt.Sprintf("%s -> %s", p, target)
			}
		}
		return p
	}
	return "(not linked globally — install via brew / install.sh, or use ./bin/camunda)"
}

func activeLabInfo() string {
	cfg, err := config.Load()
	if err != nil {
		return "(no config yet — run: camunda install)"
	}
	if cfg.Version == "" {
		return "(no config yet — run: camunda install)"
	}
	return fmt.Sprintf("version=%s profile=%s resources=%s project=%s",
		cfg.Version, cfg.Profile, cfg.Resources, cfg.ComposeProject)
}

func dockerInfo() string {
	out, err := exec.Command("docker", "version", "--format", "{{.Client.Version}}|{{.Server.Version}}").CombinedOutput()
	if err != nil {
		if _, lookErr := exec.LookPath("docker"); lookErr != nil {
			return "Docker CLI (not installed)"
		}
		return "Docker CLI present (daemon not reachable)"
	}
	parts := strings.Split(strings.TrimSpace(string(out)), "|")
	if len(parts) == 2 && parts[1] != "" && parts[1] != "<no value>" {
		return fmt.Sprintf("Docker %s (engine %s)", parts[0], parts[1])
	}
	ver, _ := exec.Command("docker", "--version").CombinedOutput()
	return strings.TrimSpace(string(ver))
}

func composeInfo() string {
	out, err := exec.Command("docker", "compose", "version", "--short").CombinedOutput()
	if err != nil {
		out, err = exec.Command("docker", "compose", "version").CombinedOutput()
		if err != nil {
			return "Compose v2 (not available)"
		}
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return "Compose v2 (unknown)"
	}
	if strings.HasPrefix(s, "Docker Compose version") {
		return s
	}
	return "Docker Compose " + s
}

func platformInfo() string {
	if runtime.GOOS == "darwin" {
		if chip := macHardwareField("Chip:"); chip != "" {
			return chip
		}
		if chip := macHardwareField("Processor Name:"); chip != "" {
			return chip
		}
	}
	return runtime.GOOS + "/" + runtime.GOARCH
}

func memoryInfo() string {
	if runtime.GOOS == "darwin" {
		if mem := macHardwareField("Memory:"); mem != "" {
			return mem
		}
	}
	return "(unknown)"
}

func macHardwareField(prefix string) string {
	out, err := exec.Command("system_profiler", "SPHardwareDataType").CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix))
		}
	}
	return ""
}

func countCommands(root *cobra.Command) int {
	if root == nil {
		return 0
	}
	n := 0
	var walk func(*cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if !sub.IsAvailableCommand() || sub.Hidden {
				continue
			}
			n++
			walk(sub)
		}
	}
	walk(root)
	return n
}
