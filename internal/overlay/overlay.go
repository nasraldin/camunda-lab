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

//go:embed embed/elasticsearch-cors.yaml
var elasticsearchCorsYAML []byte

//go:embed embed/elasticvue.yaml
var elasticvueYAML []byte

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
	// KEYCLOAK_HOST=keycloak: container→Keycloak on the compose network.
	// Browser issuer URLs still use HOST=localhost from Camunda's .env.
	content := fmt.Sprintf("JAVA_TOOL_OPTIONS=%s\nKEYCLOAK_HOST=keycloak\n", opts)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// ComposeOverrideFiles returns extra -f compose files (absolute paths).
func ComposeOverrideFiles(minor, profile string) ([]string, error) {
	if err := os.MkdirAll(paths.OverlaysDir(), 0o755); err != nil {
		return nil, err
	}
	var out []string
	write := func(name string, data []byte) error {
		dest := filepath.Join(paths.OverlaysDir(), name)
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			return err
		}
		out = append(out, dest)
		return nil
	}
	if versions.NeedsElasticsearchOverlay(minor, profile) {
		if err := write("elasticsearch-8.10.yaml", elasticsearch810YAML); err != nil {
			return nil, err
		}
	}
	if versions.HasHostElasticsearch(minor, profile) {
		if err := write("elasticsearch-cors.yaml", elasticsearchCorsYAML); err != nil {
			return nil, err
		}
		if err := write("elasticvue.yaml", elasticvueYAML); err != nil {
			return nil, err
		}
	}
	return out, nil
}
