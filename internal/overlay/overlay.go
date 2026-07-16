package overlay

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

//go:embed embed/elasticsearch-8.10.yaml
var elasticsearch810YAML []byte

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
	if err := os.MkdirAll(paths.OverlaysDir(), 0o755); err != nil {
		return nil, err
	}
	dest := filepath.Join(paths.OverlaysDir(), "elasticsearch-8.10.yaml")
	if err := os.WriteFile(dest, elasticsearch810YAML, 0o644); err != nil {
		return nil, err
	}
	return []string{dest}, nil
}
