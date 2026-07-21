package api

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

const maxUploadBytes = 10 << 20 // 10 MiB

// allowPath ensures dir/file is absolute and under an allowed root
// (user home, /tmp, CAMUNDA_LAB_HOME, or the active lab home).
func allowPath(path string) (string, error) {
	return allowPathWithin(path, allowedRoots())
}

func allowPathWithin(path string, roots []string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("path must be absolute (got %q)", path)
	}

	canonicalPath, err := canonicalizeForAuthorization(path)
	if err != nil {
		return "", fmt.Errorf("canonicalize path %q: %w", path, err)
	}
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" || !filepath.IsAbs(root) {
			continue
		}
		canonicalRoot, err := canonicalizeForAuthorization(root)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(canonicalRoot, canonicalPath)
		if err != nil {
			continue
		}
		if rel == "." || (rel != ".." &&
			!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
			!filepath.IsAbs(rel)) {
			return canonicalPath, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed roots (home, /tmp, lab home)", canonicalPath)
}

func canonicalizeForAuthorization(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	current := filepath.Clean(abs)
	var missing []string
	for {
		canonical, err := filepath.EvalSymlinks(current)
		if err == nil {
			for i := len(missing) - 1; i >= 0; i-- {
				canonical = filepath.Join(canonical, missing[i])
			}
			return filepath.Clean(canonical), nil
		}
		if !os.IsNotExist(err) {
			return "", err
		}
		if _, lstatErr := os.Lstat(current); lstatErr == nil {
			return "", err
		} else if !os.IsNotExist(lstatErr) {
			return "", lstatErr
		}

		parent := filepath.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append(missing, filepath.Base(current))
		current = parent
	}
}

func allowedRoots() []string {
	var roots []string
	if h, err := os.UserHomeDir(); err == nil {
		roots = append(roots, h)
	}
	roots = append(roots, "/tmp", os.TempDir(), paths.Home())
	if v := os.Getenv("CAMUNDA_LAB_HOME"); v != "" {
		if abs, err := filepath.Abs(v); err == nil {
			roots = append(roots, abs)
		}
	}
	return roots
}
