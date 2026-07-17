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
func allowPath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("path is required")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path must be absolute (got %q)", p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	abs = filepath.Clean(abs)
	roots := allowedRoots()
	for _, root := range roots {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, abs)
		if err != nil {
			continue
		}
		if rel == "." || (!strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel)) {
			return abs, nil
		}
	}
	return "", fmt.Errorf("path %q is outside allowed roots (home, /tmp, lab home)", abs)
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
