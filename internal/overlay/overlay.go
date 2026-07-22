package overlay

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"

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

//go:embed embed/csrf-disabled.yaml
var csrfDisabledYAML []byte

//go:embed embed/connectors-ai-secrets.yaml
var connectorsAISecretsYAML []byte

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
func ComposeOverrideFiles(minor, profile string, aiEnabled bool) ([]string, error) {
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
	// Light + full orchestration UIs: avoid new-tab CSRF 401 → fake login wall.
	if profile == "light" || profile == "full" {
		if err := write("csrf-disabled.yaml", csrfDisabledYAML); err != nil {
			return nil, err
		}
	}
	if aiEnabled && versions.SupportsAIFeature(minor, profile) == nil {
		if err := write("connectors-ai-secrets.yaml", connectorsAISecretsYAML); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ExpectedFiles reports the managed overlay filenames for a configuration
// without creating or modifying anything on disk.
func ExpectedFiles(minor, profile string, aiEnabled bool) ([]string, error) {
	if err := versions.ValidateMinor(minor); err != nil {
		return nil, err
	}
	if err := versions.ValidateProfile(profile); err != nil {
		return nil, err
	}
	var out []string
	if versions.NeedsElasticsearchOverlay(minor, profile) {
		out = append(out, "elasticsearch-8.10.yaml")
	}
	if versions.HasHostElasticsearch(minor, profile) {
		out = append(out, "elasticsearch-cors.yaml", "elasticvue.yaml")
	}
	if profile == "full" {
		out = append(out, "http-headers.yaml")
	}
	if profile == "light" || profile == "full" {
		out = append(out, "csrf-disabled.yaml")
	}
	if aiEnabled && versions.SupportsAIFeature(minor, profile) == nil {
		out = append(out, "connectors-ai-secrets.yaml")
	}
	sort.Strings(out)
	return out, nil
}

// ExpectedContent returns the embedded managed content for an overlay filename.
func ExpectedContent(name string) ([]byte, bool) {
	content := map[string][]byte{
		"elasticsearch-8.10.yaml":    elasticsearch810YAML,
		"elasticsearch-cors.yaml":    elasticsearchCorsYAML,
		"elasticvue.yaml":            elasticvueYAML,
		"http-headers.yaml":          httpHeadersYAML,
		"csrf-disabled.yaml":         csrfDisabledYAML,
		"connectors-ai-secrets.yaml": connectorsAISecretsYAML,
	}
	data, ok := content[name]
	if !ok {
		return nil, false
	}
	return append([]byte(nil), data...), true
}
