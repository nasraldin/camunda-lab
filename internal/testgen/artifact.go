package testgen

import (
	"archive/zip"
	"bytes"
	"fmt"
	"sort"
	"time"
)

// Fixed metadata keeps ZIP bytes reproducible for identical artifact sets.
var zipEntryModTime = time.Unix(0, 0).UTC()

// PackZIP builds a deterministic ZIP archive of relative artifacts.
// It never writes to the filesystem.
func PackZIP(artifacts []Artifact) ([]byte, error) {
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("pack zip: no artifacts")
	}
	if err := validateArtifactSet(artifacts); err != nil {
		return nil, err
	}
	sorted := append([]Artifact(nil), artifacts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })

	var buf bytes.Buffer
	writer := zip.NewWriter(&buf)
	for _, artifact := range sorted {
		header := &zip.FileHeader{
			Name:     artifact.Path,
			Method:   zip.Deflate,
			Modified: zipEntryModTime,
		}
		header.SetMode(artifactMode)
		entry, err := writer.CreateHeader(header)
		if err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("pack zip: create %q: %w", artifact.Path, err)
		}
		if _, err := entry.Write(artifact.Content); err != nil {
			_ = writer.Close()
			return nil, fmt.Errorf("pack zip: write %q: %w", artifact.Path, err)
		}
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("pack zip: close: %w", err)
	}
	return buf.Bytes(), nil
}
