package env

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

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
	TokenURLEnv     string `yaml:"tokenUrlEnv,omitempty" json:"tokenUrlEnv,omitempty"`
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
	if err := p.Validate(); err != nil {
		return err
	}
	path, err := ProfilePath(dir, p.Name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("refusing profile symlink %q", p.Name)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// LoadProfile reads one profile.
func LoadProfile(path string) (Profile, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return Profile{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return Profile{}, fmt.Errorf("refusing profile symlink")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, err
	}
	var p Profile
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Profile{}, err
	}
	// Detect raw secrets mistakenly stored
	raw := string(data)
	if strings.Contains(strings.ToLower(raw), "clientsecret:") && !strings.Contains(strings.ToLower(raw), "clientsecretenv:") {
		return Profile{}, fmt.Errorf("refusing profile with inline clientSecret (use clientSecretEnv)")
	}
	if err := p.Validate(); err != nil {
		return Profile{}, err
	}
	filename := filepath.Base(path)
	if !strings.HasSuffix(filename, ".yaml") {
		return Profile{}, fmt.Errorf("profile filename must end in .yaml")
	}
	expectedName := strings.TrimSuffix(filename, ".yaml")
	if p.Name != expectedName {
		return Profile{}, fmt.Errorf("profile name %q does not match filename %q", p.Name, expectedName)
	}
	return p, nil
}

// LoadNamedProfile reads a validated profile and verifies its embedded name.
func LoadNamedProfile(dir, name string) (Profile, error) {
	path, err := ProfilePath(dir, name)
	if err != nil {
		return Profile{}, err
	}
	p, err := LoadProfile(path)
	if err != nil {
		return Profile{}, err
	}
	if p.Name != name {
		return Profile{}, fmt.Errorf("profile name %q does not match filename %q", p.Name, name)
	}
	return p, nil
}

// ListProfiles loads all *.yaml from dir.
func ListProfiles(dir string) ([]Profile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Profile
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".yaml")
		p, err := LoadNamedProfile(dir, name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, p)
	}
	return out, nil
}

// DefaultLab returns the implicit lab profile.
func DefaultLab() Profile {
	return Profile{Name: "lab", Kind: "lab"}
}
