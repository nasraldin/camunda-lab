package backup

import (
	"errors"
	"os"
)

// ValidateArchive validates a backup archive without restoring it or mutating
// destinations. It reuses the same safety validation as Restore.
func ValidateArchive(opts RestoreOptions) (Manifest, error) {
	limits, err := normalizeLimits(opts.Limits)
	if err != nil {
		return Manifest{}, err
	}
	if opts.ArchivePath == "" {
		return Manifest{}, errors.New("archive path required")
	}

	archive, err := os.Open(opts.ArchivePath)
	if err != nil {
		return Manifest{}, errors.New("could not open backup archive")
	}
	defer archive.Close()

	manifest, _, err := validateArchive(archive, opts, limits)
	if err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}
