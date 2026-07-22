package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/project"
)

// Fixed entry metadata keeps archive member ordering/modes reproducible.
var archiveEntryModTime = time.Unix(0, 0).UTC()

// Options for Create.
type Options struct {
	LabHome        string
	ProjectDir     string // optional
	OutPath        string
	IncludeSecrets bool
	LabVersion     string
	LabProfile     string
}

type backupFile struct {
	name string
	data []byte
}

// Create writes a gzip tar backup. It is context-aware and publishes the archive
// atomically with mode 0600.
func Create(ctx context.Context, opts Options) (Manifest, error) {
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}
	if opts.OutPath == "" {
		return Manifest{}, fmt.Errorf("out path required")
	}
	m := Manifest{
		Version:         1,
		CreatedAt:       time.Now().UTC().Format(time.RFC3339),
		Lab:             map[string]string{"version": opts.LabVersion, "profile": opts.LabProfile},
		IncludesSecrets: opts.IncludeSecrets,
	}

	var files []backupFile
	addFile := func(name string, data []byte) {
		files = append(files, backupFile{name: name, data: data})
		m.Files = append(m.Files, name)
	}

	cfgPath := filepath.Join(opts.LabHome, "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		addFile("config.yaml", data)
	} else if !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("read lab configuration: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}

	aiPath := filepath.Join(opts.LabHome, "ai.env")
	if data, err := os.ReadFile(aiPath); err == nil {
		keys := secretKeyNames(string(data))
		m.AISecretKeys = keys
		if opts.IncludeSecrets {
			addFile("ai.env", data)
		} else {
			meta, err := json.Marshal(map[string]any{"keys": keys, "note": "values omitted"})
			if err != nil {
				return Manifest{}, err
			}
			addFile("ai.keys.json", meta)
		}
	} else if !os.IsNotExist(err) {
		return Manifest{}, fmt.Errorf("read AI environment: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}

	if opts.ProjectDir != "" {
		if err := collectProjectFiles(ctx, opts.ProjectDir, addFile); err != nil {
			return Manifest{}, err
		}
	}
	if err := ctx.Err(); err != nil {
		return Manifest{}, err
	}

	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	m.Files = make([]string, len(files))
	for index, file := range files {
		m.Files[index] = file.name
	}

	manData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	files = append(files, backupFile{name: "manifest.json", data: manData})
	sort.Slice(files, func(i, j int) bool { return files[i].name < files[j].name })
	if err := writeBackupArchive(opts.OutPath, files); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func collectProjectFiles(ctx context.Context, projectDir string, addFile func(string, []byte)) error {
	configPath := filepath.Join(projectDir, project.ConfigFileName)
	resourceDirs := []string{"bpmn", "dmn", "forms"}

	if data, err := os.ReadFile(configPath); err == nil {
		info, err := os.Lstat(configPath)
		if err != nil {
			return fmt.Errorf("inspect project configuration: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("project symlinks are not supported")
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported project file type")
		}
		addFile(filepath.ToSlash(filepath.Join("project", project.ConfigFileName)), data)

		cfg, err := project.Load(configPath)
		if err != nil {
			return fmt.Errorf("load project configuration: %w", err)
		}
		resourceDirs = []string{cfg.Paths.BPMN, cfg.Paths.DMN, cfg.Paths.Forms}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("read project configuration: %w", err)
	}

	seenNames := make(map[string]struct{})
	addUnique := func(name string, data []byte) error {
		if _, exists := seenNames[name]; exists {
			return fmt.Errorf("duplicate archive entry %q from overlapping project resource paths", name)
		}
		seenNames[name] = struct{}{}
		addFile(name, data)
		return nil
	}

	for _, sub := range resourceDirs {
		if err := ctx.Err(); err != nil {
			return err
		}
		dir := filepath.Join(projectDir, sub)
		info, err := os.Lstat(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return fmt.Errorf("inspect project resources: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("project symlinks are not supported")
		}
		if !info.IsDir() {
			return fmt.Errorf("inspect project resources: %s is not a directory", sub)
		}
		if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("project symlinks are not supported")
			}
			if d.IsDir() {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return err
			}
			if !info.Mode().IsRegular() {
				return fmt.Errorf("unsupported project file type")
			}
			rel, err := filepath.Rel(projectDir, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			return addUnique(filepath.ToSlash(filepath.Join("project", rel)), data)
		}); err != nil {
			return fmt.Errorf("read project resources: %w", err)
		}
	}
	return nil
}

func writeBackupArchive(outPath string, files []backupFile) (returnErr error) {
	parent := filepath.Dir(outPath)
	temp, err := os.CreateTemp(parent, ".camunda-lab-backup-*")
	if err != nil {
		return errors.New("could not create backup archive")
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		if returnErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return errors.New("could not create backup archive")
	}

	gz := gzip.NewWriter(temp)
	tw := tar.NewWriter(gz)
	writeFile := func(file backupFile) error {
		header := &tar.Header{
			Name: file.name, Mode: 0o600, Size: int64(len(file.data)),
			ModTime: archiveEntryModTime, Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(header); err != nil {
			return err
		}
		if _, err := tw.Write(file.data); err != nil {
			return err
		}
		return nil
	}
	for _, file := range files {
		if err := writeFile(file); err != nil {
			return errors.New("could not create backup archive")
		}
	}
	if err := tw.Close(); err != nil {
		return errors.New("could not create backup archive")
	}
	if err := gz.Close(); err != nil {
		return errors.New("could not create backup archive")
	}
	if err := temp.Sync(); err != nil {
		return errors.New("could not create backup archive")
	}
	if err := temp.Close(); err != nil {
		return errors.New("could not create backup archive")
	}
	if err := os.Rename(tempPath, outPath); err != nil {
		return errors.New("could not create backup archive")
	}
	return nil
}

func secretKeyNames(envFile string) []string {
	var keys []string
	for _, line := range strings.Split(envFile, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if i := strings.IndexByte(line, '='); i > 0 {
			keys = append(keys, line[:i])
		}
	}
	return keys
}
