package overlay

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

func ValidateResources(resources string) error {
	switch resources {
	case "small", "balanced", "power":
		return nil
	default:
		return fmt.Errorf("unsupported resources %q (small|balanced|power)", resources)
	}
}

func JavaToolOptions(resources string) (string, error) {
	if err := ValidateResources(resources); err != nil {
		return "", err
	}
	switch resources {
	case "small":
		return "-Xms256m -Xmx512m", nil
	case "balanced":
		return "-Xms512m -Xmx1024m", nil
	case "power":
		return "-Xms1g -Xmx2g", nil
	default:
		return "", fmt.Errorf("unsupported resources %q", resources)
	}
}

// SyncResourcesEnv writes ~/.camunda-lab/resources.env for compose --env-file.
func SyncResourcesEnv(resources string) (string, error) {
	opts, err := JavaToolOptions(resources)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		return "", err
	}
	path := filepath.Join(paths.Home(), "resources.env")
	content := fmt.Sprintf("JAVA_TOOL_OPTIONS=%s\n", opts)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ComposeOverrideFiles returns extra -f compose files (absolute paths).
func ComposeOverrideFiles(minor, profile string) ([]string, error) {
	if !versions.NeedsElasticsearchOverlay(minor, profile) {
		return nil, nil
	}
	src, err := esOverlaySource()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(paths.OverlaysDir(), 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(paths.OverlaysDir(), "elasticsearch-8.10.yaml")
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, fmt.Errorf("read ES overlay: %w", err)
	}
	if err := os.WriteFile(dest, data, 0o644); err != nil {
		return nil, err
	}
	return []string{dest}, nil
}

func esOverlaySource() (string, error) {
	_, thisFile, _, ok := runtime.Caller(0)
	if ok {
		repoOverlay := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", "..", "overlays", "elasticsearch-8.10.yaml"))
		if _, err := os.Stat(repoOverlay); err == nil {
			return repoOverlay, nil
		}
	}
	if wd, err := os.Getwd(); err == nil {
		p := filepath.Join(wd, "overlays", "elasticsearch-8.10.yaml")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("overlays/elasticsearch-8.10.yaml not found (run from camunda-lab repo or install overlays)")
}
