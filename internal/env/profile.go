package env

import (
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
)

var environmentReferencePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Profile is a named lab or remote environment.
type Profile struct {
	Name      string            `yaml:"name" json:"name"`
	Kind      string            `yaml:"kind" json:"kind"` // lab|remote
	Endpoints map[string]string `yaml:"endpoints,omitempty" json:"endpoints,omitempty"`
	Auth      AuthRefs          `yaml:"auth,omitempty" json:"auth,omitempty"`
}

// AuthRefs stores env var *names* only — never secret values.
type AuthRefs struct {
	ClientIDEnv     string `yaml:"clientIdEnv,omitempty" json:"clientIdEnv,omitempty"`
	ClientSecretEnv string `yaml:"clientSecretEnv,omitempty" json:"clientSecretEnv,omitempty"`
	TokenURL        string `yaml:"tokenUrl,omitempty" json:"tokenUrl,omitempty"`
	TokenURLEnv     string `yaml:"tokenUrlEnv,omitempty" json:"tokenUrlEnv,omitempty"`
	Audience        string `yaml:"audience,omitempty" json:"audience,omitempty"`
	Scope           string `yaml:"scope,omitempty" json:"scope,omitempty"`
}

// Validate checks profile rules.
func (p Profile) Validate() error {
	if err := ValidateName(p.Name); err != nil {
		return err
	}
	if p.Kind != "lab" && p.Kind != "remote" {
		return fmt.Errorf("kind must be lab or remote")
	}
	if p.Kind == "remote" {
		if strings.TrimSpace(p.Endpoints["orchestration"]) == "" {
			return fmt.Errorf("remote profile requires endpoints.orchestration")
		}
		if p.Auth.ClientIDEnv == "" || p.Auth.ClientSecretEnv == "" {
			return fmt.Errorf("remote profile requires auth.clientIdEnv and auth.clientSecretEnv")
		}
		if p.Auth.TokenURL != "" && p.Auth.TokenURLEnv != "" {
			return fmt.Errorf("remote profile auth must use tokenUrl or tokenUrlEnv, not both")
		}
		if p.Auth.Audience != "" && p.Auth.Scope != "" {
			return fmt.Errorf("remote profile auth must use audience or scope, not both")
		}
	}
	for field, reference := range map[string]string{
		"clientIdEnv": p.Auth.ClientIDEnv, "clientSecretEnv": p.Auth.ClientSecretEnv,
		"tokenUrlEnv": p.Auth.TokenURLEnv,
	} {
		if reference != "" && (reference != strings.TrimSpace(reference) ||
			!environmentReferencePattern.MatchString(reference)) {
			return fmt.Errorf("auth.%s must be a canonical environment variable identifier", field)
		}
	}
	// Reject inline secrets if someone stuffed values into endpoints under secret-like keys
	for k, v := range p.Endpoints {
		lk := strings.ToLower(k)
		if strings.Contains(lk, "secret") || strings.Contains(lk, "password") {
			return fmt.Errorf("endpoints must not include secret fields (%s)", k)
		}
		_ = v
	}
	return nil
}

// SaveProfile writes a profile yaml.
func SaveProfile(dir string, p Profile) error {
	root, err := openProfileRoot(dir, true, nil)
	if err != nil {
		return err
	}
	defer root.close()
	return root.save(p, true)
}

// LoadProfile reads one profile.
func LoadProfile(path string) (Profile, error) {
	return loadProfileAtPath(path)
}

// LoadNamedProfile reads a validated profile and verifies its embedded name.
func LoadNamedProfile(dir, name string) (Profile, error) {
	if err := ValidateName(name); err != nil {
		return Profile{}, err
	}
	root, err := openProfileRoot(dir, false, nil)
	if err != nil {
		return Profile{}, err
	}
	defer root.close()
	return root.load(name)
}

// ListProfiles loads all *.yaml from dir.
func ListProfiles(dir string) ([]Profile, error) {
	root, err := openProfileRoot(dir, false, nil)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer root.close()
	return root.list()
}

// DefaultLab returns the implicit lab profile.
func DefaultLab() Profile {
	return Profile{Name: "lab", Kind: "lab"}
}
