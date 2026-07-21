package lab

import (
	"context"

	"github.com/nasraldin/camunda-lab/internal/compose"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/laberrors"
)

func (l *Lab) upOnce(workDir string, files, envFiles []string, project string) error {
	return l.Engine.Up(workDir, files, envFiles, project)
}

func (l *Lab) startStack(ctx context.Context, workDir string, files, envFiles []string, project string) error {
	_ = ctx
	// Idempotent: stop our compose project before starting (handles partial CLI runs).
	_ = l.Engine.Down(workDir, files, project, false)
	l.reconcileKnownStaleContainers(project)

	if err := l.upOnce(workDir, files, envFiles, project); err != nil {
		if fixed := l.tryRemediateNameConflicts(err, project); fixed {
			if retryErr := l.upOnce(workDir, files, envFiles, project); retryErr == nil {
				return nil
			}
		}
		return l.finalizeUpError(err, project)
	}
	return nil
}

func (l *Lab) reconcileKnownStaleContainers(project string) {
	for _, name := range compose.KnownFixedNames {
		info, err := compose.InspectContainerByName(name)
		if err != nil {
			continue
		}
		if compose.CanSafelyRemove(info, project) {
			_ = compose.RemoveContainer(name)
		}
	}
}

func (l *Lab) tryRemediateNameConflicts(err error, project string) bool {
	names := compose.ParseNameConflicts(err.Error())
	if len(names) == 0 {
		return false
	}
	removed := false
	for _, name := range names {
		info, ierr := compose.InspectContainerByName(name)
		if ierr != nil {
			continue
		}
		if compose.CanSafelyRemove(info, project) {
			if rmErr := compose.RemoveContainer(name); rmErr == nil {
				removed = true
			}
		}
	}
	return removed
}

func (l *Lab) finalizeUpError(err error, project string) error {
	names := compose.ParseNameConflicts(err.Error())
	if len(names) > 0 {
		for _, name := range names {
			info, ierr := compose.InspectContainerByName(name)
			if ierr != nil {
				continue
			}
			if !compose.CanSafelyRemove(info, project) {
				return laberrors.ForeignContainerConflict(names)
			}
		}
		return laberrors.ContainerConflict(names)
	}
	return laberrors.Wrap(err)
}

// Recover clears leftover Camunda-lab containers and compose state so a new start can succeed.
func (l *Lab) Recover(ctx context.Context) error {
	_ = ctx
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	workDir, files, _, err := l.resolve(cfg)
	if err != nil {
		return err
	}
	_ = l.Engine.Down(workDir, files, cfg.ComposeProject, true)
	l.reconcileKnownStaleContainers(cfg.ComposeProject)
	return nil
}
