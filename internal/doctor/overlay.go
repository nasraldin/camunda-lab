package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	laboverlay "github.com/nasraldin/camunda-lab/internal/overlay"
)

type FileInfo struct {
	Path  string
	IsDir bool
}

type DirEntry struct {
	Name  string
	IsDir bool
}

// FileSystem is the read-only filesystem surface used by diagnostics.
type FileSystem interface {
	Stat(context.Context, string) (FileInfo, error)
	ReadDir(context.Context, string, int) ([]DirEntry, error)
	ReadFile(context.Context, string, int64) ([]byte, error)
}

type osFileSystem struct{}

// OS metadata/file syscalls are not asynchronously interruptible on every
// filesystem. These methods avoid goroutines (and therefore leaks), check the
// context before and after each syscall, and bound all variable-size reads.
func (osFileSystem) Stat(ctx context.Context, path string) (FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return FileInfo{}, err
	}
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	return FileInfo{Path: path, IsDir: info.IsDir()}, nil
}

func (osFileSystem) ReadDir(ctx context.Context, path string, maxEntries int) ([]DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.Readdir(maxEntries + 1)
	if err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(entries) > maxEntries {
		return nil, fmt.Errorf("directory entry limit exceeded (%d)", maxEntries)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, DirEntry{Name: entry.Name(), IsDir: entry.IsDir()})
	}
	return out, nil
}

func (osFileSystem) ReadFile(ctx context.Context, path string, maxBytes int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("file size limit exceeded (%d bytes)", maxBytes)
	}
	return data, nil
}

const (
	maxOverlayEntries = 256
	maxOverlayBytes   = int64(2 << 20)
)

func inspectOverlays(ctx context.Context, fs FileSystem, dir, version, profile string, aiEnabled bool) (Status, string) {
	expected, err := laboverlay.ExpectedFiles(version, profile, aiEnabled)
	if err != nil {
		return StatusFail, sanitize(err.Error())
	}
	entries, err := fs.ReadDir(ctx, dir, maxOverlayEntries)
	if err != nil {
		if len(expected) == 0 && os.IsNotExist(err) {
			return StatusPass, "No managed overlays are expected"
		}
		return StatusWarn, probeError(ctx, "Overlay directory is unavailable", err)
	}
	actual := map[string]bool{}
	for _, entry := range entries {
		if !entry.IsDir && strings.HasSuffix(entry.Name, ".yaml") {
			actual[entry.Name] = true
		}
	}
	var missing, stale, changed []string
	for _, name := range expected {
		if !actual[name] {
			missing = append(missing, name)
			continue
		}
		want, _ := laboverlay.ExpectedContent(name)
		got, readErr := fs.ReadFile(ctx, filepath.Join(dir, name), maxOverlayBytes)
		if readErr != nil {
			return StatusWarn, probeError(ctx, "Overlay "+name+" could not be read", readErr)
		}
		if !bytes.Equal(got, want) {
			changed = append(changed, name)
		}
		delete(actual, name)
	}
	for name := range actual {
		stale = append(stale, name)
	}
	sort.Strings(missing)
	sort.Strings(stale)
	sort.Strings(changed)
	if len(missing)+len(stale)+len(changed) == 0 {
		if len(expected) == 0 {
			return StatusPass, "No managed overlays are expected or present"
		}
		return StatusPass, fmt.Sprintf("%d expected managed overlays are current", len(expected))
	}
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	if len(stale) > 0 {
		parts = append(parts, "stale: "+strings.Join(stale, ", "))
	}
	if len(changed) > 0 {
		parts = append(parts, "outdated: "+strings.Join(changed, ", "))
	}
	return StatusWarn, strings.Join(parts, "; ")
}
