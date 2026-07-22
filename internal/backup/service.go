package backup

import "context"

// Service exposes backup create/restore/validate over shared domain helpers.
type Service struct {
	Lab RunningChecker
}

// NewService returns a backup service that can inject a running-lab checker.
func NewService(lab RunningChecker) *Service {
	return &Service{Lab: lab}
}

// Create writes a backup archive using the package Create helper.
func (s *Service) Create(ctx context.Context, opts Options) (Manifest, error) {
	return Create(ctx, opts)
}

// Restore restores an archive, injecting the service running checker when opts
// do not already supply one. Safety-plan validation is not duplicated or weakened.
func (s *Service) Restore(ctx context.Context, opts RestoreOptions) (Manifest, error) {
	if opts.Lab == nil && s != nil {
		opts.Lab = s.Lab
	}
	return Restore(ctx, opts)
}

// ValidateArchive dry-validates an archive without mutation.
func (s *Service) ValidateArchive(opts RestoreOptions) (Manifest, error) {
	return ValidateArchive(opts)
}
