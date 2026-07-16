package lab

import (
	"context"
	"os"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
)

func (l *Lab) EnableAI(ctx context.Context, s ai.Secrets) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if err := ai.ValidateForEnable(cfg.Version, cfg.Profile, s); err != nil {
		return err
	}
	if err := ai.WriteSecrets(s); err != nil {
		return err
	}
	cfg.AI.Enabled = true
	if err := config.Save(cfg); err != nil {
		return err
	}
	return l.RecreateConnectors(ctx)
}

func (l *Lab) DisableAI(ctx context.Context, wipeSecrets bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	cfg.AI.Enabled = false
	if err := config.Save(cfg); err != nil {
		return err
	}
	if wipeSecrets {
		if err := ai.DeleteSecretsFile(); err != nil {
			return err
		}
	}
	return l.RecreateConnectors(ctx)
}

func (l *Lab) RecreateConnectors(ctx context.Context) error {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir, files, envFiles, err := l.resolve(cfg)
	if err != nil {
		return err
	}
	display.Step(os.Stdout, "Recreating connectors with AI secrets overlay...")
	if err := l.Engine.UpService(workDir, files, envFiles, cfg.ComposeProject, "connectors"); err != nil {
		return err
	}
	display.Done(os.Stdout, "Connectors recreated.")
	return nil
}
