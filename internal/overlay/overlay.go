package overlay

import (
	"bytes"
	"embed"
	_ "embed"
	"fmt"
	"io/fs"
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

//go:embed embed/http-headers.yaml
var httpHeadersYAML []byte

//go:embed embed/connectors-ai-secrets.yaml
var connectorsAISecretsYAML []byte

//go:embed embed/monitoring.yaml
var monitoringYAML []byte

//go:embed embed/monitoring
var monitoringAssets embed.FS

// overlaysDirPlaceholder is replaced with the absolute overlays dir at write
// time so Compose bind mounts resolve (Compose resolves relative mounts against
// the project dir, not the override file's dir).
const overlaysDirPlaceholder = "__OVERLAYS_DIR__"

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
func ComposeOverrideFiles(minor, profile string, aiEnabled, monitoringEnabled bool) ([]string, error) {
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
	if profile == "full" {
		if err := write("http-headers.yaml", httpHeadersYAML); err != nil {
			return nil, err
		}
	}
	if aiEnabled && versions.SupportsAIFeature(minor, profile) == nil {
		if err := write("connectors-ai-secrets.yaml", connectorsAISecretsYAML); err != nil {
			return nil, err
		}
	}
	if monitoringEnabled {
		if err := writeMonitoringAssets(); err != nil {
			return nil, err
		}
		// Template the absolute overlays dir into bind-mount sources.
		yaml := bytes.ReplaceAll(monitoringYAML, []byte(overlaysDirPlaceholder), []byte(paths.OverlaysDir()))
		if err := write("monitoring.yaml", yaml); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// writeMonitoringAssets extracts the embedded monitoring/ tree (prometheus
// config, Grafana provisioning + dashboards) into ~/.camunda-lab/overlays/.
func writeMonitoringAssets() error {
	return fs.WalkDir(monitoringAssets, "embed/monitoring", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		// Strip the leading "embed/" so files land under overlays/monitoring/...
		rel, err := filepath.Rel("embed", p)
		if err != nil {
			return err
		}
		dest := filepath.Join(paths.OverlaysDir(), filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}
		data, err := monitoringAssets.ReadFile(p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, 0o644)
	})
}
