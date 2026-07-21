package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

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

// Create writes a gzip tar backup.
func Create(opts Options) (Manifest, error) {
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

	if opts.ProjectDir != "" {
		for _, sub := range []string{"bpmn", "dmn", "forms"} {
			dir := filepath.Join(opts.ProjectDir, sub)
			if _, err := os.Stat(dir); err != nil {
				if os.IsNotExist(err) {
					continue
				}
				return Manifest{}, fmt.Errorf("inspect project resources: %w", err)
			}
			if err := filepath.WalkDir(dir, func(path string, d os.DirEntry, walkErr error) error {
				if walkErr != nil {
					return walkErr
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
				rel, err := filepath.Rel(opts.ProjectDir, path)
				if err != nil {
					return err
				}
				data, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				addFile(filepath.ToSlash(filepath.Join("project", rel)), data)
				return nil
			}); err != nil {
				return Manifest{}, fmt.Errorf("read project resources: %w", err)
			}
		}
	}

	manData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := writeBackupArchive(opts.OutPath, files, manData); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func writeBackupArchive(outPath string, files []backupFile, manifest []byte) (returnErr error) {
	parent := filepath.Dir(outPath)
	temp, err := os.CreateTemp(parent, ".camunda-lab-backup-*")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	defer func() {
		_ = temp.Close()
		if returnErr != nil {
			_ = os.Remove(tempPath)
		}
	}()
	if err := temp.Chmod(0o600); err != nil {
		return err
	}

	gz := gzip.NewWriter(temp)
	tw := tar.NewWriter(gz)
	writeFile := func(file backupFile) error {
		header := &tar.Header{
			Name: file.name, Mode: 0o600, Size: int64(len(file.data)),
			ModTime: time.Now(), Typeflag: tar.TypeReg,
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
			return err
		}
	}
	if err := writeFile(backupFile{name: "manifest.json", data: manifest}); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	if err := temp.Sync(); err != nil {
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, outPath); err != nil {
		return err
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
