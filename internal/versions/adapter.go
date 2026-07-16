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
