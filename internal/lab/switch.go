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
	cfg.Version = minor
	if err := config.Save(cfg); err != nil {
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	display.Step(os.Stdout, "Switching profile to %s...", profile)
	_ = l.Down(ctx, false)
	cfg.Profile = profile
	if err := config.Save(cfg); err != nil {
		return err
	}
	return l.Up(ctx)
}

func (l *Lab) SetResources(ctx context.Context, resources string) error {
	if err := overlay.ValidateResources(resources); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.Resources = resources
	if err := config.Save(cfg); err != nil {
		return err
	}
	if _, err := overlay.SyncResourcesEnv(resources); err != nil {
		return err
	}
	display.Done(os.Stdout, "Resources set to %s.", resources)
	display.Note(os.Stdout, "restart to apply: camunda restart")
	return nil
}
