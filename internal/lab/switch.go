package lab

import (
	"context"
	"os"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/overlay"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

func (l *Lab) Switch(ctx context.Context, minor string, wipe bool) error {
	if err := versions.ValidateMinor(minor); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if cfg.Version != minor && !wipe {
		display.Note(os.Stderr, "switching %s → %s without --wipe may leave incompatible volumes", cfg.Version, minor)
	}
	display.Step(os.Stdout, "Switching lab to Camunda %s...", minor)
	_ = l.Down(ctx, wipe)
	if err := config.Update(func(current *config.Config) error {
		current.Version = minor
		if current.AI.Enabled && versions.SupportsAIFeature(current.Version, current.Profile) != nil {
			display.Note(os.Stderr, "disabling AI/MCP — not supported on %s/%s", current.Version, current.Profile)
			current.AI.Enabled = false
		}
		cfg = *current
		return nil
	}); err != nil {
		return err
	}
	if _, err := versions.Ensure(minor, versions.DownloadOptions{SkipIfPresent: true}); err != nil {
		return err
	}
	return l.Up(ctx)
}

func (l *Lab) SetProfile(ctx context.Context, profile string) error {
	if err := versions.ValidateProfile(profile); err != nil {
		return err
	}
	display.Step(os.Stdout, "Switching profile to %s...", profile)
	_ = l.Down(ctx, false)
	if err := config.Update(func(current *config.Config) error {
		current.Profile = profile
		if current.AI.Enabled && versions.SupportsAIFeature(current.Version, current.Profile) != nil {
			display.Note(os.Stderr, "disabling AI/MCP — not supported on %s/%s", current.Version, current.Profile)
			current.AI.Enabled = false
		}
		return nil
	}); err != nil {
		return err
	}
	return l.Up(ctx)
}

func (l *Lab) SetResources(ctx context.Context, resources string) error {
	if err := overlay.ValidateResources(resources); err != nil {
		return err
	}
	if err := config.Update(func(current *config.Config) error {
		current.Resources = resources
		return nil
	}); err != nil {
		return err
	}
	if _, err := overlay.SyncResourcesEnv(resources); err != nil {
		return err
	}
	display.Done(os.Stdout, "Resources set to %s.", resources)
	display.Note(os.Stdout, "restart to apply: camunda restart")
	return nil
}
