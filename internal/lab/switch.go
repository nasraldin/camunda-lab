package lab

import (
	"context"
	"fmt"
	"os"

	"github.com/nasraldin/camunda-lab/internal/config"
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
		fmt.Fprintf(os.Stderr, "warning: switching %s → %s without --wipe may leave incompatible volumes\n", cfg.Version, minor)
	}
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
	fmt.Printf("resources=%s (restart with: camunda restart)\n", resources)
	return nil
}
