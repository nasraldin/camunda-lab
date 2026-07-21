package lab

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/compose"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/overlay"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/prompt"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

type InstallOpts struct {
	Version   string
	Profile   string
	Resources string
	Yes       bool
	AI        bool
	AISecrets ai.Secrets
}

type Lab struct {
	Engine compose.Engine
}

func New() *Lab {
	return &Lab{Engine: compose.NewRunner()}
}

func (l *Lab) Install(ctx context.Context, opts InstallOpts) error {
	_ = ctx
	if err := checkDocker(); err != nil {
		return err
	}

	cfg := config.Defaults()
	existing, _ := config.Load()
	if existing.Version != "" {
		cfg = existing
	}

	version := opts.Version
	profile := opts.Profile
	resources := opts.Resources

	if !opts.Yes {
		var err error
		if version == "" {
			labels := make([]string, len(versions.Supported))
			def := 1 // 8.8
			for i, v := range versions.Supported {
				labels[i] = v
				if versions.IsPreview(v) {
					labels[i] = v + " (preview)"
				}
				if v == "8.8" {
					def = i
				}
			}
			choice, err := prompt.Choose(os.Stdin, os.Stdout, "Camunda minor version:", labels, def)
			if err != nil {
				return err
			}
			version = strings.Fields(choice)[0]
		}
		if profile == "" {
			profile, err = prompt.Choose(os.Stdin, os.Stdout, "Profile:", []string{"light", "full", "modeler"}, 0)
			if err != nil {
				return err
			}
		}
		if resources == "" {
			resources, err = prompt.Choose(os.Stdin, os.Stdout, "Resources:", []string{"small", "balanced", "power"}, 1)
			if err != nil {
				return err
			}
		}
	}

	if version == "" {
		version = cfg.Version
	}
	if profile == "" {
		profile = cfg.Profile
	}
	if resources == "" {
		resources = cfg.Resources
	}

	if err := versions.ValidateMinor(version); err != nil {
		return err
	}
	if err := versions.ValidateProfile(profile); err != nil {
		return err
	}
	if err := overlay.ValidateResources(resources); err != nil {
		return err
	}

	if versions.IsPreview(version) {
		display.Note(os.Stderr, "Camunda %s is marked preview in camunda-lab", version)
	}

	display.Step(os.Stdout, "Fetching Camunda %s compose distribution...", version)
	if _, err := versions.Ensure(version, versions.DownloadOptions{SkipIfPresent: true}); err != nil {
		return err
	}
	display.Done(os.Stdout, "Distribution ready for Camunda %s.", version)

	cfg.Version = version
	cfg.Profile = profile
	cfg.Resources = resources
	if err := config.Save(cfg); err != nil {
		return err
	}

	if opts.AI {
		if err := ai.ValidateForEnable(version, profile, opts.AISecrets); err != nil {
			return err
		}
	}

	if err := l.Up(ctx); err != nil {
		return err
	}
	if opts.AI {
		return l.EnableAI(ctx, opts.AISecrets)
	}
	return nil
}

func (l *Lab) Up(ctx context.Context) error {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir, files, envFiles, err := l.resolve(cfg)
	if err != nil {
		return err
	}
	display.Step(os.Stdout, "Starting lab (%s / %s / %s)...", cfg.Version, cfg.Profile, cfg.Resources)
	if err := l.startStack(ctx, workDir, files, envFiles, cfg.ComposeProject); err != nil {
		return err
	}
	display.Done(os.Stdout, "Stack started.")
	fmt.Println("Next: camunda wait && camunda urls")
	fmt.Println("Lab UI: http://localhost:9090  (auto-started in background)")
	return nil
}

func (l *Lab) Down(ctx context.Context, volumes bool) error {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir, files, _, err := l.resolve(cfg)
	if err != nil {
		return err
	}
	if volumes {
		display.Step(os.Stdout, "Stopping lab and removing volumes...")
	} else {
		display.Step(os.Stdout, "Stopping lab (volumes kept)...")
	}
	if err := l.Engine.Down(workDir, files, cfg.ComposeProject, volumes); err != nil {
		return err
	}
	display.Done(os.Stdout, "Lab stopped.")
	return nil
}

func (l *Lab) Status(ctx context.Context) (string, error) {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	workDir := paths.VersionDir(cfg.Version)
	raw, err := l.Engine.PsJSON(workDir, cfg.ComposeProject)
	if err != nil {
		out, fallbackErr := l.Engine.Ps(workDir, cfg.ComposeProject)
		header := fmt.Sprintf("Camunda Lab Status\n==================\n\nVersion    %s\nProfile    %s\nResources  %s\nProject    %s\nWorkdir    %s\n\n",
			cfg.Version, cfg.Profile, cfg.Resources, cfg.ComposeProject, workDir)
		if fallbackErr != nil {
			return "", fallbackErr
		}
		return header + out, nil
	}
	return formatStatus(cfg, workDir, raw)
}

type composePSRow struct {
	Name       string `json:"Name"`
	Service    string `json:"Service"`
	Image      string `json:"Image"`
	State      string `json:"State"`
	Health     string `json:"Health"`
	Status     string `json:"Status"`
	RunningFor string `json:"RunningFor"`
	Publishers []struct {
		URL           string `json:"URL"`
		TargetPort    int    `json:"TargetPort"`
		PublishedPort int    `json:"PublishedPort"`
		Protocol      string `json:"Protocol"`
	} `json:"Publishers"`
}

func formatStatus(cfg config.Config, workDir, raw string) (string, error) {
	rows, err := parsePSJSON(raw)
	if err != nil {
		return "", err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Service < rows[j].Service })

	total := len(rows)
	healthy := 0
	running := 0
	for _, row := range rows {
		if row.State == "running" {
			running++
		}
		if row.Health == "healthy" || strings.Contains(strings.ToLower(row.Status), "healthy") {
			healthy++
		}
	}

	rep := display.Report{
		Title: "Camunda Lab Status",
		Fields: []display.Field{
			display.KV("Version", cfg.Version),
			display.KV("Profile", cfg.Profile),
			display.KV("Resources", cfg.Resources),
			display.KV("Project", cfg.ComposeProject),
			display.KV("Workdir", workDir),
			display.KV("Services", fmt.Sprintf("%d total, %d running, %d healthy", total, running, healthy)),
		},
	}
	if urlEntries := summarizeURLs(cfg); len(urlEntries) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Apps", Items: urlEntries})
	}

	var containerItems []string
	for _, row := range rows {
		containerItems = append(containerItems, "  - "+row.Service)
		containerItems = append(containerItems, "    state   "+prettyState(row))
		if row.RunningFor != "" {
			containerItems = append(containerItems, "    uptime  "+row.RunningFor)
		}
		if row.Image != "" {
			containerItems = append(containerItems, "    image   "+row.Image)
		}
		if ports := publishedPorts(row); ports != "" {
			containerItems = append(containerItems, "    ports   "+ports)
		}
	}
	if len(containerItems) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Containers", Items: containerItems, Raw: true})
	}

	var b strings.Builder
	rep.Write(&b)
	return b.String(), nil
}

func parsePSJSON(raw string) ([]composePSRow, error) {
	var rows []composePSRow
	for _, line := range strings.Split(strings.TrimSpace(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var row composePSRow
		if err := json.Unmarshal([]byte(line), &row); err != nil {
			return nil, fmt.Errorf("parse docker compose ps json: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func summarizeURLs(cfg config.Config) []string {
	wanted := []string{"operate", "tasklist", "admin", "console", "optimize", "identity", "web-modeler", "keycloak", "elasticvue", "connectors", "grpc", "mcp-cluster", "mcp-processes"}
	entries := ListStatusURLs(cfg)
	var lines []string
	for _, name := range wanted {
		if url, ok := entries[name]; ok {
			lines = append(lines, fmt.Sprintf("%s -> %s", name, url))
		}
	}
	return lines
}

func ListStatusURLs(cfg config.Config) map[string]string {
	out := map[string]string{}
	for _, entry := range urls.List(cfg) {
		switch entry.Name {
		case "operate", "tasklist", "admin", "console", "optimize", "identity", "web-modeler", "keycloak", "elasticvue", "connectors", "grpc", "mcp-cluster", "mcp-processes":
			out[entry.Name] = entry.URL
		}
	}
	return out
}

func prettyState(row composePSRow) string {
	switch {
	case row.Health != "":
		return row.Health
	case row.State != "":
		return row.State
	case row.Status != "":
		return row.Status
	default:
		return "unknown"
	}
}

func publishedPorts(row composePSRow) string {
	seen := map[string]bool{}
	var ports []string
	for _, p := range row.Publishers {
		if p.PublishedPort == 0 {
			continue
		}
		item := strconv.Itoa(p.PublishedPort) + "->" + strconv.Itoa(p.TargetPort) + "/" + p.Protocol
		if !seen[item] {
			seen[item] = true
			ports = append(ports, item)
		}
	}
	sort.Strings(ports)
	return strings.Join(ports, ", ")
}

func (l *Lab) resolve(cfg config.Config) (workDir string, files []string, envFiles []string, err error) {
	workDir = paths.VersionDir(cfg.Version)
	composeNames, err := versions.ComposeFiles(cfg.Version, cfg.Profile)
	if err != nil {
		return "", nil, nil, err
	}
	for _, name := range composeNames {
		p := filepath.Join(workDir, name)
		if _, statErr := os.Stat(p); statErr != nil {
			return "", nil, nil, fmt.Errorf("missing %s — run camunda install first", p)
		}
		files = append(files, p)
	}
	overrides, err := overlay.ComposeOverrideFiles(cfg.Version, cfg.Profile, cfg.AI.Enabled, cfg.Monitoring.Enabled)
	if err != nil {
		return "", nil, nil, err
	}
	files = append(files, overrides...)

	envPath, err := overlay.SyncResourcesEnv(cfg.Resources)
	if err != nil {
		return "", nil, nil, err
	}
	// Compose: any --env-file disables automatic loading of the project .env.
	// Always pass upstream .env first (image tags, secrets), then resources.env.
	envFiles = EnvFiles(workDir, envPath, cfg.AI.Enabled)

	return workDir, files, envFiles, nil
}

// EnvFiles returns compose --env-file paths: upstream Camunda .env (if present),
// then the lab resources.env, then ai.env when AI is enabled. Order matters —
// later files override earlier keys.
func EnvFiles(workDir, resourcesEnv string, aiEnabled bool) []string {
	var files []string
	upstream := filepath.Join(workDir, ".env")
	if _, err := os.Stat(upstream); err == nil {
		files = append(files, upstream)
	}
	if resourcesEnv != "" {
		files = append(files, resourcesEnv)
	}
	if aiEnabled {
		if _, err := os.Stat(paths.AIEnvFile()); err == nil {
			files = append(files, paths.AIEnvFile())
		}
	}
	return files
}

func (l *Lab) Logs(ctx context.Context, service string, follow bool) error {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir := paths.VersionDir(cfg.Version)
	return l.Engine.Logs(workDir, cfg.ComposeProject, service, follow)
}

func checkDocker() error {
	r := compose.NewRunner()
	_, err := r.Exec(".", []string{"docker", "compose", "version"})
	if err != nil {
		return fmt.Errorf("docker compose v2 required: %w\nOn macOS Apple Silicon, see https://github.com/nasraldin/docker-lab", err)
	}
	return nil
}
