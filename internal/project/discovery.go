package project

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// AssetKind identifies a configured project asset directory.
type AssetKind string

const (
	AssetBPMN  AssetKind = "bpmn"
	AssetDMN   AssetKind = "dmn"
	AssetForms AssetKind = "forms"
	AssetTests AssetKind = "tests"
)

// ErrProjectNotFound indicates that no .camunda.yaml exists at or above start.
var ErrProjectNotFound = errors.New("camunda project not found")

// Project is an opened, canonically authorized project.
type Project struct {
	Root   string
	Config Config
}

// FindRoot searches start and its parents for .camunda.yaml.
func FindRoot(start string) (string, error) {
	current, err := startDirectory(start)
	if err != nil {
		return "", err
	}
	for {
		if info, err := os.Stat(filepath.Join(current, ConfigFileName)); err == nil {
			if info.IsDir() {
				return "", fmt.Errorf("%s is a directory", filepath.Join(current, ConfigFileName))
			}
			return canonical(current)
		} else if !os.IsNotExist(err) {
			return "", err
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", fmt.Errorf("%w from %s", ErrProjectNotFound, start)
		}
		current = parent
	}
}

// Open loads the nearest project config. Without one, only ./bpmn is enabled.
func Open(start string) (Project, error) {
	root, err := FindRoot(start)
	if err != nil {
		if !errors.Is(err, ErrProjectNotFound) {
			return Project{}, err
		}
		root, err = startDirectory(start)
		if err != nil {
			return Project{}, err
		}
		root, err = canonical(root)
		if err != nil {
			return Project{}, err
		}
		return Project{
			Root: root,
			Config: Config{
				Paths: Paths{BPMN: "bpmn"},
			},
		}, nil
	}

	configPath, err := canonical(filepath.Join(root, ConfigFileName))
	if err != nil {
		return Project{}, err
	}
	if !isWithin(root, configPath) {
		return Project{}, fmt.Errorf("%s escapes project root", ConfigFileName)
	}
	cfg, err := Load(configPath)
	if err != nil {
		return Project{}, err
	}
	project := Project{Root: root, Config: cfg}
	for _, configured := range []struct {
		name string
		path string
	}{
		{name: "paths.bpmn", path: cfg.Paths.BPMN},
		{name: "paths.dmn", path: cfg.Paths.DMN},
		{name: "paths.forms", path: cfg.Paths.Forms},
		{name: "paths.tests", path: cfg.Paths.Tests},
	} {
		if _, err := project.Resolve(configured.path); err != nil {
			return Project{}, fmt.Errorf("%s: %w", configured.name, err)
		}
	}
	return project, nil
}

// Resolve authorizes a clean project-relative path and returns its canonical path.
func (p Project) Resolve(configuredPath string) (string, error) {
	if err := validateRelativePath("path", configuredPath); err != nil {
		return "", err
	}
	root, err := canonical(p.Root)
	if err != nil {
		return "", err
	}
	candidate := filepath.Join(root, configuredPath)
	resolved, err := canonicalAllowMissing(candidate)
	if err != nil {
		return "", err
	}
	if !isWithin(root, resolved) {
		return "", fmt.Errorf("path escapes project root")
	}
	return resolved, nil
}

// Discover recursively returns canonical files of kind in sorted order.
func (p Project) Discover(kind AssetKind) ([]string, error) {
	configured, ok := p.pathFor(kind)
	if !ok || configured == "" {
		return []string{}, nil
	}
	root, err := p.Resolve(configured)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s asset path is not a directory", kind)
	}

	files := make([]string, 0)
	err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Type()&os.ModeSymlink != 0 {
			return nil
		}
		if !entry.Type().IsRegular() || !kind.accepts(path) {
			return nil
		}
		resolved, err := canonical(path)
		if err != nil {
			return err
		}
		if !isWithin(p.Root, resolved) {
			return fmt.Errorf("discovered path escapes project root: %s", path)
		}
		files = append(files, resolved)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

// ResolveInput resolves an explicit input project-relative first, then relative
// to the configured directory for kind.
func (p Project) ResolveInput(kind AssetKind, input string) (string, error) {
	if err := validateRelativePath("input", input); err != nil {
		return "", err
	}
	projectCandidate, err := p.Resolve(input)
	if err != nil {
		return "", err
	}
	if exists, err := regularFile(projectCandidate); err != nil {
		return "", err
	} else if exists {
		if !kind.accepts(projectCandidate) {
			return "", fmt.Errorf("%s is not a %s file", input, kind)
		}
		return projectCandidate, nil
	}

	configured, ok := p.pathFor(kind)
	if !ok || configured == "" {
		return "", fmt.Errorf("%s input not found: %s", kind, input)
	}
	assetCandidate, err := p.Resolve(filepath.Join(configured, input))
	if err != nil {
		return "", err
	}
	if exists, err := regularFile(assetCandidate); err != nil {
		return "", err
	} else if !exists {
		return "", fmt.Errorf("%s input not found: %s", kind, input)
	}
	if !kind.accepts(assetCandidate) {
		return "", fmt.Errorf("%s is not a %s file", input, kind)
	}
	return assetCandidate, nil
}

func (p Project) pathFor(kind AssetKind) (string, bool) {
	switch kind {
	case AssetBPMN:
		return p.Config.Paths.BPMN, true
	case AssetDMN:
		return p.Config.Paths.DMN, true
	case AssetForms:
		return p.Config.Paths.Forms, true
	case AssetTests:
		return p.Config.Paths.Tests, true
	default:
		return "", false
	}
}

func (kind AssetKind) accepts(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	switch kind {
	case AssetBPMN:
		return ext == ".bpmn"
	case AssetDMN:
		return ext == ".dmn"
	case AssetForms:
		return ext == ".form"
	case AssetTests:
		return ext == ".java" || ext == ".js" || ext == ".ts"
	default:
		return false
	}
}

func startDirectory(start string) (string, error) {
	if strings.TrimSpace(start) == "" {
		return "", fmt.Errorf("start path is required")
	}
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	return filepath.Clean(abs), nil
}

func canonical(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	return filepath.Abs(resolved)
}

func canonicalAllowMissing(path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err == nil {
		return filepath.Abs(resolved)
	}
	if !os.IsNotExist(err) {
		return "", err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return "", err
	}
	resolvedParent, parentErr := canonicalAllowMissing(parent)
	if parentErr != nil {
		return "", parentErr
	}
	return filepath.Join(resolvedParent, filepath.Base(path)), nil
}

func isWithin(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))
}

func regularFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if !info.Mode().IsRegular() {
		return false, fmt.Errorf("%s is not a regular file", path)
	}
	return true, nil
}
