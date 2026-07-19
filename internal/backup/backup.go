package backup

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Manifest describes a backup archive.
type Manifest struct {
	Version         int               `json:"version"`
	CreatedAt       string            `json:"createdAt"`
	Lab             map[string]string `json:"lab"`
	IncludesSecrets bool              `json:"includesSecrets"`
	Files           []string          `json:"files"`
	AISecretKeys    []string          `json:"aiSecretKeys,omitempty"`
}

// Options for Create.
type Options struct {
	LabHome        string
	ProjectDir     string // optional
	OutPath        string
	IncludeSecrets bool
	LabVersion     string
	LabProfile     string
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

	f, err := os.Create(opts.OutPath)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	addFile := func(name string, data []byte) error {
		hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: time.Now()}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		_, err := tw.Write(data)
		if err == nil {
			m.Files = append(m.Files, name)
		}
		return err
	}

	cfgPath := filepath.Join(opts.LabHome, "config.yaml")
	if data, err := os.ReadFile(cfgPath); err == nil {
		if err := addFile("config.yaml", data); err != nil {
			return Manifest{}, err
		}
	}

	aiPath := filepath.Join(opts.LabHome, "ai.env")
	if data, err := os.ReadFile(aiPath); err == nil {
		keys := secretKeyNames(string(data))
		m.AISecretKeys = keys
		if opts.IncludeSecrets {
			if err := addFile("ai.env", data); err != nil {
				return Manifest{}, err
			}
		} else {
			meta, _ := json.Marshal(map[string]any{"keys": keys, "note": "values omitted"})
			if err := addFile("ai.keys.json", meta); err != nil {
				return Manifest{}, err
			}
		}
	}

	if opts.ProjectDir != "" {
		for _, sub := range []string{"bpmn", "dmn", "forms"} {
			dir := filepath.Join(opts.ProjectDir, sub)
			_ = filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
				if err != nil || d.IsDir() {
					return nil
				}
				rel, _ := filepath.Rel(opts.ProjectDir, path)
				data, err := os.ReadFile(path)
				if err != nil {
					return nil
				}
				_ = addFile(filepath.ToSlash(filepath.Join("project", rel)), data)
				return nil
			})
		}
	}

	manData, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return Manifest{}, err
	}
	if err := addFile("manifest.json", manData); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

// Restore extracts archive into labHome (and optional projectDir).
func Restore(archive, labHome, projectDir string) (Manifest, error) {
	f, err := os.Open(archive)
	if err != nil {
		return Manifest{}, err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return Manifest{}, err
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	var man Manifest
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Manifest{}, err
		}
		data, err := io.ReadAll(tr)
		if err != nil {
			return Manifest{}, err
		}
		switch {
		case hdr.Name == "manifest.json":
			_ = json.Unmarshal(data, &man)
		case hdr.Name == "config.yaml":
			if err := os.MkdirAll(labHome, 0o755); err != nil {
				return Manifest{}, err
			}
			if err := os.WriteFile(filepath.Join(labHome, "config.yaml"), data, 0o644); err != nil {
				return Manifest{}, err
			}
		case hdr.Name == "ai.env":
			if err := os.WriteFile(filepath.Join(labHome, "ai.env"), data, 0o600); err != nil {
				return Manifest{}, err
			}
		case strings.HasPrefix(hdr.Name, "project/") && projectDir != "":
			rel := strings.TrimPrefix(hdr.Name, "project/")
			dest := filepath.Join(projectDir, rel)
			if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
				return Manifest{}, err
			}
			if err := os.WriteFile(dest, data, 0o644); err != nil {
				return Manifest{}, err
			}
		}
	}
	return man, nil
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
