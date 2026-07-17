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
	if strings.TrimSpace(p.Name) == "" {
		return fmt.Errorf("name is required")
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
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(p)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, p.Name+".yaml"), data, 0o644)
}

// LoadProfile reads one profile.
func LoadProfile(path string) (Profile, error) {
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
		p, err := LoadProfile(filepath.Join(dir, e.Name()))
		if err != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), err)
		}
		out = append(out, p)
	}
	return out, nil
}

// ActiveFile stores the active profile name.
func ActiveFile(labHome string) string {
	return filepath.Join(labHome, "active-env")
}

func SetActive(labHome, name string) error {
	if err := os.MkdirAll(labHome, 0o755); err != nil {
		return err
	}
	return os.WriteFile(ActiveFile(labHome), []byte(name+"\n"), 0o644)
}

func GetActive(labHome string) string {
	data, err := os.ReadFile(ActiveFile(labHome))
	if err != nil {
		return "lab"
	}
	name := strings.TrimSpace(string(data))
	if name == "" {
		return "lab"
	}
	return name
}

// DefaultLab returns the implicit lab profile.
func DefaultLab() Profile {
	return Profile{Name: "lab", Kind: "lab"}
}
