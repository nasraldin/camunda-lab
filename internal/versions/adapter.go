package versions

import "fmt"

var Supported = []string{"8.7", "8.8", "8.9", "8.10"}

func IsPreview(minor string) bool { return minor == "8.10" }

func ValidateMinor(minor string) error {
	for _, s := range Supported {
		if s == minor {
			return nil
		}
	}
	return fmt.Errorf("unsupported version %q (supported: %v)", minor, Supported)
}

func ValidateProfile(profile string) error {
	switch profile {
	case "light", "full", "modeler":
		return nil
	default:
		return fmt.Errorf("unsupported profile %q (light|full|modeler)", profile)
	}
}

func ComposeFiles(minor, profile string) ([]string, error) {
	if err := ValidateMinor(minor); err != nil {
		return nil, err
	}
	if err := ValidateProfile(profile); err != nil {
		return nil, err
	}
	switch minor {
	case "8.7":
		switch profile {
		case "light":
			return []string{"docker-compose-core.yaml"}, nil
		case "full":
			return []string{"docker-compose.yaml"}, nil
		case "modeler":
			return []string{"docker-compose-web-modeler.yaml"}, nil
		}
	default: // 8.8, 8.9, 8.10
		switch profile {
		case "light":
			return []string{"docker-compose.yaml"}, nil
		case "full":
			return []string{"docker-compose-full.yaml"}, nil
		case "modeler":
			return []string{"docker-compose-web-modeler.yaml"}, nil
		}
	}
	return nil, fmt.Errorf("internal: unhandled %s/%s", minor, profile)
}

func NeedsElasticsearchOverlay(minor, profile string) bool {
	return minor == "8.10" && profile == "full"
}

// HasHostElasticsearch reports whether this profile publishes Elasticsearch on host :9200.
// Official light compose includes ES through 8.8; 8.9+ light does not. Full includes ES
// (8.10 via our elasticsearch overlay). Modeler never does.
func HasHostElasticsearch(minor, profile string) bool {
	if profile == "modeler" {
		return false
	}
	if profile == "full" {
		return true
	}
	if profile == "light" {
		switch minor {
		case "8.7", "8.8":
			return true
		default:
			return false
		}
	}
	return false
}

// SupportsClusterMCP is true for Camunda 8.9+ (Orchestration Cluster MCP).
func SupportsClusterMCP(minor string) bool {
	switch minor {
	case "8.9", "8.10":
		return true
	default:
		return false
	}
}

// SupportsProcessesMCP is true for Camunda 8.10+ (Processes MCP /mcp/processes).
func SupportsProcessesMCP(minor string) bool {
	return minor == "8.10"
}

// SupportsAIFeature gates camunda ai / --ai (8.9+ light|full).
func SupportsAIFeature(minor, profile string) error {
	if !SupportsClusterMCP(minor) {
		return fmt.Errorf("AI/MCP requires Camunda 8.9+ (got %s)", minor)
	}
	if profile == "modeler" {
		return fmt.Errorf("AI/MCP is not available on the modeler profile")
	}
	if profile != "light" && profile != "full" {
		return fmt.Errorf("AI/MCP requires profile light or full (got %s)", profile)
	}
	return nil
}

func ReleaseTag(minor string) string {
	return "docker-compose-" + minor
}

func ZipURL(minor string) string {
	tag := ReleaseTag(minor)
	return fmt.Sprintf(
		"https://github.com/camunda/camunda-distributions/releases/download/%s/%s.zip",
		tag, tag,
	)
}

func CosignBundleURL(minor string) string {
	tag := ReleaseTag(minor)
	return fmt.Sprintf(
		"https://github.com/camunda/camunda-distributions/releases/download/%s/%s.cosign.bundle",
		tag, tag,
	)
}
