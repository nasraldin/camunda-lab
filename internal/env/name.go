package env

import (
	"fmt"
	"path/filepath"
	"strings"
)

const maxNameBytes = 64

var reservedNames = map[string]struct{}{
	"lab":        {},
	"config":     {},
	"active-env": {},
	"envs":       {},
}

// ValidateName validates a stored environment profile name.
func ValidateName(name string) error {
	if name == "" {
		return fmt.Errorf("environment name is required")
	}
	if len(name) > maxNameBytes {
		return fmt.Errorf("environment name must be at most %d bytes", maxNameBytes)
	}
	if _, reserved := reservedNames[name]; reserved {
		return fmt.Errorf("environment name %q is reserved", name)
	}
	for _, label := range strings.Split(name, ".") {
		if label == "" {
			return fmt.Errorf("environment name must not contain empty dot segments")
		}
		for i := 0; i < len(label); i++ {
			c := label[i]
			alphanumeric := c >= 'a' && c <= 'z' || c >= '0' && c <= '9'
			if i == 0 || i == len(label)-1 {
				if !alphanumeric {
					return fmt.Errorf("environment name labels must begin and end with a lowercase letter or digit")
				}
				continue
			}
			if !alphanumeric && c != '-' && c != '_' {
				return fmt.Errorf("environment name contains invalid character %q", c)
			}
		}
	}
	return nil
}

// ProfilePath returns the canonical storage path for a validated profile name.
func ProfilePath(dir, name string) (string, error) {
	if err := ValidateName(name); err != nil {
		return "", err
	}
	return filepath.Join(dir, name+".yaml"), nil
}
