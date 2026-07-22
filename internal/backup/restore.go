package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

const manifestName = "manifest.json"

// Limits bounds the amount of decompressed archive data Restore will process.
type Limits struct {
	MaxEntries    int
	MaxFileBytes  int64
	MaxTotalBytes int64
}

// RestoreOptions configures a transactional archive restore.
type RestoreOptions struct {
	ArchivePath string
	LabHome     string
	ProjectDir  string
	Force       bool
	Limits      Limits
	Lab         RunningChecker
}

// RunningChecker reports whether the lab is currently running.
type RunningChecker interface {
	Running(context.Context) (bool, error)
}

// DefaultLimits returns the restore safety bounds.
func DefaultLimits() Limits {
	return Limits{
		MaxEntries:    10_000,
		MaxFileBytes:  64 << 20,
		MaxTotalBytes: 512 << 20,
	}
}

type validatedEntry struct {
	name  string
	size  int64
	isDir bool
	root  destinationRoot
	rel   string
}

type destinationRoot uint8

const (
	noDestination destinationRoot = iota
	labDestination
	projectDestination
)

type stagedEntry struct {
	finalPath string
	stagePath string
	isDir     bool
}

type committedFile struct {
	finalPath  string
	backupPath string
	hadOld     bool
}

// Restore validates an entire gzip tar archive, extracts it into private
// staging directories, and only then replaces destination files.
func Restore(ctx context.Context, opts RestoreOptions) (Manifest, error) {
	limits, err := normalizeLimits(opts.Limits)
	if err != nil {
		return Manifest{}, err
	}
	if opts.ArchivePath == "" {
		return Manifest{}, errors.New("archive path required")
	}
	if opts.LabHome == "" {
		return Manifest{}, errors.New("lab home required")
	}
	if !opts.Force {
		if opts.Lab == nil {
			return Manifest{}, errors.New("could not determine whether the lab is running")
		}
		running, err := opts.Lab.Running(ctx)
		if err != nil {
			return Manifest{}, errors.New("could not determine whether the lab is running")
		}
		if running {
			return Manifest{}, errors.New(`lab is running; stop it first with "camunda down" or retry with --force`)
		}
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}

	archive, err := os.Open(opts.ArchivePath)
	if err != nil {
		return Manifest{}, errors.New("could not open backup archive")
	}
	defer archive.Close()

	manifest, entries, err := validateArchive(archive, opts, limits)
	if err != nil {
		return Manifest{}, err
	}
	if _, err := archive.Seek(0, io.SeekStart); err != nil {
		return Manifest{}, errors.New("could not prepare backup archive")
	}

	staged, cleanup, err := extractToStaging(ctx, archive, opts, entries)
	if err != nil {
		return Manifest{}, err
	}
	defer cleanup()
	if err := commitStaged(staged); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

func normalizeLimits(limits Limits) (Limits, error) {
	defaults := DefaultLimits()
	if limits.MaxEntries < 0 || limits.MaxFileBytes < 0 || limits.MaxTotalBytes < 0 {
		return Limits{}, errors.New("restore limits must not be negative")
	}
	if limits.MaxEntries == 0 {
		limits.MaxEntries = defaults.MaxEntries
	}
	if limits.MaxFileBytes == 0 {
		limits.MaxFileBytes = defaults.MaxFileBytes
	}
	if limits.MaxTotalBytes == 0 {
		limits.MaxTotalBytes = defaults.MaxTotalBytes
	}
	return limits, nil
}

func validateArchive(archive io.Reader, opts RestoreOptions, limits Limits) (Manifest, []validatedEntry, error) {
	gz, err := gzip.NewReader(archive)
	if err != nil {
		return Manifest{}, nil, errors.New("unsupported or invalid backup archive")
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	var (
		entries       []validatedEntry
		payloadNames  []string
		manifest      Manifest
		manifestFound bool
		totalBytes    int64
	)
	destinations := make(map[string]struct{})
	names := make(map[string]struct{})

	for count := 1; ; count++ {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Manifest{}, nil, errors.New("invalid backup archive")
		}
		if count > limits.MaxEntries {
			return Manifest{}, nil, errors.New("backup archive has too many entries")
		}
		if err := validateEntryName(header.Name); err != nil {
			return Manifest{}, nil, err
		}
		if _, exists := names[header.Name]; exists {
			return Manifest{}, nil, errors.New("backup archive contains a duplicate entry")
		}
		names[header.Name] = struct{}{}

		isDir := header.Typeflag == tar.TypeDir
		isFile := header.Typeflag == tar.TypeReg || header.Typeflag == tar.TypeRegA
		if !isDir && !isFile {
			return Manifest{}, nil, errors.New("backup archive contains an unsupported entry type")
		}
		if header.Size < 0 {
			return Manifest{}, nil, errors.New("backup archive contains a negative entry size")
		}
		if isDir && header.Size != 0 {
			return Manifest{}, nil, errors.New("backup archive contains an invalid directory")
		}
		if header.Size > limits.MaxFileBytes {
			return Manifest{}, nil, errors.New("backup archive entry exceeds the size limit")
		}
		if header.Size > limits.MaxTotalBytes-totalBytes {
			return Manifest{}, nil, errors.New("backup archive exceeds the total size limit")
		}
		totalBytes += header.Size

		if header.Name == manifestName {
			if !isFile {
				return Manifest{}, nil, errors.New("backup manifest must be a regular file")
			}
			data, err := readExactEntry(tr, header.Size)
			if err != nil {
				return Manifest{}, nil, errors.New("invalid backup manifest")
			}
			if err := json.Unmarshal(data, &manifest); err != nil {
				return Manifest{}, nil, errors.New("invalid backup manifest")
			}
			manifestFound = true
			continue
		}

		root, rel, err := mapDestination(header.Name, opts.ProjectDir != "")
		if err != nil {
			return Manifest{}, nil, err
		}
		destinationKey := fmt.Sprintf("%d:%s", root, rel)
		if _, exists := destinations[destinationKey]; exists {
			return Manifest{}, nil, errors.New("backup archive contains a duplicate destination")
		}
		destinations[destinationKey] = struct{}{}
		payloadNames = append(payloadNames, header.Name)
		entries = append(entries, validatedEntry{
			name: header.Name, size: header.Size, isDir: isDir, root: root, rel: rel,
		})
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return Manifest{}, nil, errors.New("invalid backup archive payload")
		}
	}

	if !manifestFound {
		return Manifest{}, nil, errors.New("backup manifest is missing")
	}
	if manifest.Version != 1 {
		return Manifest{}, nil, errors.New("unsupported backup manifest version")
	}
	if !slices.Equal(manifest.Files, payloadNames) {
		return Manifest{}, nil, errors.New("backup manifest does not match archive payload")
	}
	return manifest, entries, nil
}

func validateEntryName(name string) error {
	if name == "" || name == "." || strings.ContainsRune(name, '\\') ||
		strings.ContainsRune(name, '\x00') || path.IsAbs(name) ||
		path.Clean(name) != name {
		return errors.New("backup archive contains an unsafe path")
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return errors.New("backup archive contains an unsafe path")
		}
	}
	return nil
}

func mapDestination(name string, hasProject bool) (destinationRoot, string, error) {
	switch name {
	case "config.yaml", "ai.env", "ai.keys.json":
		return labDestination, name, nil
	case "project":
		if hasProject {
			return projectDestination, "", nil
		}
		return noDestination, name, nil
	default:
		if strings.HasPrefix(name, "project/") {
			rel := strings.TrimPrefix(name, "project/")
			if hasProject {
				return projectDestination, filepath.FromSlash(rel), nil
			}
			return noDestination, name, nil
		}
		return noDestination, "", errors.New("backup archive contains an unsupported destination")
	}
}

func readExactEntry(reader io.Reader, size int64) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(reader, size+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) != size {
		return nil, errors.New("entry size mismatch")
	}
	return data, nil
}

func extractToStaging(
	ctx context.Context,
	archive io.Reader,
	opts RestoreOptions,
	entries []validatedEntry,
) ([]stagedEntry, func(), error) {
	stageRoots := make(map[destinationRoot]string)
	cleanup := func() {
		for _, root := range stageRoots {
			_ = os.RemoveAll(root)
		}
	}
	createRoot := func(kind destinationRoot, finalRoot string) error {
		if kind == noDestination || stageRoots[kind] != "" {
			return nil
		}
		root, err := os.MkdirTemp(filepath.Dir(finalRoot), ".camunda-lab.restore-stage-*")
		if err != nil {
			return errors.New("could not create restore staging area")
		}
		if err := os.Chmod(root, 0o700); err != nil {
			_ = os.RemoveAll(root)
			return errors.New("could not secure restore staging area")
		}
		stageRoots[kind] = root
		return nil
	}
	for _, entry := range entries {
		switch entry.root {
		case labDestination:
			if err := createRoot(entry.root, opts.LabHome); err != nil {
				cleanup()
				return nil, func() {}, err
			}
		case projectDestination:
			if err := createRoot(entry.root, opts.ProjectDir); err != nil {
				cleanup()
				return nil, func() {}, err
			}
		}
	}

	gz, err := gzip.NewReader(archive)
	if err != nil {
		cleanup()
		return nil, func() {}, errors.New("backup archive changed during restore")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	entryIndex := 0
	var staged []stagedEntry
	for {
		if err := ctx.Err(); err != nil {
			cleanup()
			return nil, func() {}, err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			cleanup()
			return nil, func() {}, errors.New("backup archive changed during restore")
		}
		if header.Name == manifestName {
			continue
		}
		if entryIndex >= len(entries) {
			cleanup()
			return nil, func() {}, errors.New("backup archive changed during restore")
		}
		entry := entries[entryIndex]
		entryIndex++
		if header.Name != entry.name || header.Size != entry.size ||
			(header.Typeflag == tar.TypeDir) != entry.isDir {
			cleanup()
			return nil, func() {}, errors.New("backup archive changed during restore")
		}
		if entry.root == noDestination {
			continue
		}
		stagePath := filepath.Join(stageRoots[entry.root], entry.rel)
		if entry.isDir {
			if err := os.MkdirAll(stagePath, 0o700); err != nil {
				cleanup()
				return nil, func() {}, errors.New("could not extract backup archive")
			}
		} else {
			if err := os.MkdirAll(filepath.Dir(stagePath), 0o700); err != nil {
				cleanup()
				return nil, func() {}, errors.New("could not extract backup archive")
			}
			mode := os.FileMode(0o644)
			if entry.name == "ai.env" {
				mode = 0o600
			}
			file, err := os.OpenFile(stagePath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
			if err != nil {
				cleanup()
				return nil, func() {}, errors.New("could not extract backup archive")
			}
			_, copyErr := io.Copy(file, tr)
			closeErr := file.Close()
			if copyErr != nil || closeErr != nil {
				cleanup()
				return nil, func() {}, errors.New("could not extract backup archive")
			}
		}
		finalRoot := opts.LabHome
		if entry.root == projectDestination {
			finalRoot = opts.ProjectDir
		}
		staged = append(staged, stagedEntry{
			finalPath: filepath.Join(finalRoot, entry.rel),
			stagePath: stagePath,
			isDir:     entry.isDir,
		})
	}
	if entryIndex != len(entries) {
		cleanup()
		return nil, func() {}, errors.New("backup archive changed during restore")
	}
	return staged, cleanup, nil
}

func commitStaged(entries []stagedEntry) error {
	if err := preflightDestinations(entries); err != nil {
		return err
	}

	directories := requiredDirectories(entries)
	var createdDirectories []string
	for _, directory := range directories {
		if _, err := os.Lstat(directory); err == nil {
			continue
		} else if !os.IsNotExist(err) {
			removeDirectories(createdDirectories)
			return errors.New("could not prepare restore destination")
		}
		if err := os.Mkdir(directory, 0o700); err != nil {
			removeDirectories(createdDirectories)
			return errors.New("could not prepare restore destination")
		}
		createdDirectories = append(createdDirectories, directory)
	}

	var committed []committedFile
	rollback := func() bool {
		ok := true
		for i := len(committed) - 1; i >= 0; i-- {
			item := committed[i]
			if err := os.Remove(item.finalPath); err != nil && !os.IsNotExist(err) {
				ok = false
			}
			if item.hadOld {
				if err := os.Rename(item.backupPath, item.finalPath); err != nil {
					ok = false
				}
			}
		}
		removeDirectories(createdDirectories)
		return ok
	}

	for _, entry := range entries {
		if entry.isDir {
			continue
		}
		item := committedFile{finalPath: entry.finalPath}
		if _, err := os.Lstat(entry.finalPath); err == nil {
			backupFile, err := os.CreateTemp(filepath.Dir(entry.finalPath), ".camunda-lab.restore-backup-*")
			if err != nil {
				if !rollback() {
					return errors.New("restore commit failed and rollback was incomplete")
				}
				return errors.New("could not commit restored files")
			}
			item.backupPath = backupFile.Name()
			if closeErr := backupFile.Close(); closeErr != nil {
				_ = os.Remove(item.backupPath)
				if !rollback() {
					return errors.New("restore commit failed and rollback was incomplete")
				}
				return errors.New("could not commit restored files")
			}
			if err := os.Remove(item.backupPath); err != nil ||
				os.Rename(entry.finalPath, item.backupPath) != nil {
				_ = os.Remove(item.backupPath)
				if !rollback() {
					return errors.New("restore commit failed and rollback was incomplete")
				}
				return errors.New("could not commit restored files")
			}
			item.hadOld = true
		} else if !os.IsNotExist(err) {
			if !rollback() {
				return errors.New("restore commit failed and rollback was incomplete")
			}
			return errors.New("could not commit restored files")
		}
		committed = append(committed, item)
		if err := os.Rename(entry.stagePath, entry.finalPath); err != nil {
			if !rollback() {
				return errors.New("restore commit failed and rollback was incomplete")
			}
			return errors.New("could not commit restored files")
		}
	}
	for _, item := range committed {
		if item.hadOld {
			_ = os.Remove(item.backupPath)
		}
	}
	return nil
}

func preflightDestinations(entries []stagedEntry) error {
	for _, entry := range entries {
		info, err := os.Lstat(entry.finalPath)
		if err == nil {
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("restore destination contains a symbolic link")
			}
			if entry.isDir != info.IsDir() {
				return errors.New("restore destination has an incompatible file type")
			}
			if !entry.isDir && !info.Mode().IsRegular() {
				return errors.New("restore destination has an unsupported file type")
			}
		} else if !os.IsNotExist(err) {
			return errors.New("could not inspect restore destination")
		}
		for directory := filepath.Dir(entry.finalPath); ; directory = filepath.Dir(directory) {
			info, err := os.Lstat(directory)
			if err == nil {
				if info.Mode()&os.ModeSymlink != 0 {
					return errors.New("restore destination contains a symbolic link")
				}
				if !info.IsDir() {
					return errors.New("restore destination parent is not a directory")
				}
				break
			}
			if !os.IsNotExist(err) {
				return errors.New("could not inspect restore destination")
			}
			parent := filepath.Dir(directory)
			if parent == directory {
				return errors.New("could not find restore destination parent")
			}
		}
	}
	return nil
}

func requiredDirectories(entries []stagedEntry) []string {
	set := make(map[string]struct{})
	for _, entry := range entries {
		directory := entry.finalPath
		if !entry.isDir {
			directory = filepath.Dir(directory)
		}
		for {
			if _, err := os.Lstat(directory); err == nil {
				break
			}
			set[directory] = struct{}{}
			parent := filepath.Dir(directory)
			if parent == directory {
				break
			}
			directory = parent
		}
	}
	directories := make([]string, 0, len(set))
	for directory := range set {
		directories = append(directories, directory)
	}
	slices.SortFunc(directories, func(a, b string) int {
		return strings.Count(a, string(filepath.Separator)) - strings.Count(b, string(filepath.Separator))
	})
	return directories
}

func removeDirectories(directories []string) {
	for i := len(directories) - 1; i >= 0; i-- {
		_ = os.Remove(directories[i])
	}
}
