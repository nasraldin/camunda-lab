package lab

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/compose"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/overlay"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/prompt"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

type InstallOpts struct {
	Version   string
	Profile   string
	Resources string
	Yes       bool
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
		fmt.Fprintf(os.Stderr, "note: Camunda %s is marked preview in camunda-lab\n", version)
	}

	fmt.Printf("Fetching Camunda %s compose distribution...\n", version)
	if _, err := versions.Ensure(version, versions.DownloadOptions{SkipIfPresent: true}); err != nil {
		return err
	}

	cfg.Version = version
	cfg.Profile = profile
	cfg.Resources = resources
	if err := config.Save(cfg); err != nil {
		return err
	}

	return l.Up(ctx)
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
	fmt.Printf("Starting camunda-lab (%s / %s)...\n", cfg.Version, cfg.Profile)
	if err := l.Engine.Up(workDir, files, envFiles, cfg.ComposeProject); err != nil {
		return err
	}
	fmt.Println("Stack started. Run: camunda status | camunda urls | camunda wait")
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
	return l.Engine.Down(workDir, files, cfg.ComposeProject, volumes)
}

func (l *Lab) Status(ctx context.Context) (string, error) {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return "", err
	}
	workDir := paths.VersionDir(cfg.Version)
	out, err := l.Engine.Ps(workDir, cfg.ComposeProject)
	header := fmt.Sprintf("version=%s profile=%s resources=%s\nproject=%s\nworkdir=%s\n\n",
		cfg.Version, cfg.Profile, cfg.Resources, cfg.ComposeProject, workDir)
	return header + out, err
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
	overrides, err := overlay.ComposeOverrideFiles(cfg.Version, cfg.Profile)
	if err != nil {
		return "", nil, nil, err
	}
	files = append(files, overrides...)

	envPath, err := overlay.SyncResourcesEnv(cfg.Resources)
	if err != nil {
		return "", nil, nil, err
	}
	envFiles = []string{envPath}

	// Also load upstream .env from workDir if present (compose does this automatically when cwd is workDir)
	return workDir, files, envFiles, nil
}

func checkDocker() error {
	r := compose.NewRunner()
	_, err := r.Exec(".", []string{"docker", "compose", "version"})
	if err != nil {
		return fmt.Errorf("docker compose v2 required: %w\nOn macOS Apple Silicon, see https://github.com/nasraldin/docker-lab", err)
	}
	return nil
}
