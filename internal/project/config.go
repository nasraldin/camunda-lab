package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	configstore "github.com/nasraldin/camunda-lab/internal/config"
	"gopkg.in/yaml.v3"
)

const ConfigFileName = ".camunda.yaml"

// Paths holds relative directories for process assets.
type Paths struct {
	BPMN  string `yaml:"bpmn"`
	DMN   string `yaml:"dmn"`
	Forms string `yaml:"forms"`
	Tests string `yaml:"tests"`
}

// LintConfig configures deterministic project lint behavior.
type LintConfig struct {
	Ignore []string `yaml:"ignore,omitempty"`
}

// LabHints are optional hints for humans / future tooling (not lab home config).
type LabHints struct {
	Profile   string `yaml:"profile,omitempty"`
	Resources string `yaml:"resources,omitempty"`
}

// Config is the project-local .camunda.yaml schema (v1).
type Config struct {
	Name           string     `yaml:"name"`
	CamundaVersion string     `yaml:"camundaVersion"`
	Environment    string     `yaml:"environment,omitempty"`
	Paths          Paths      `yaml:"paths"`
	Lint           LintConfig `yaml:"lint,omitempty"`
	Lab            LabHints   `yaml:"lab,omitempty"`
}

// Defaults returns a Config with standard path defaults filled in.
func Defaults(name, version string) Config {
	if version == "" {
		version = "8.9"
	}
	return Config{
		Name:           name,
		CamundaVersion: version,
		Paths: Paths{
			BPMN:  "bpmn",
			DMN:   "dmn",
			Forms: "forms",
			Tests: "tests",
		},
		Lab: LabHints{
			Profile:   "light",
			Resources: "balanced",
		},
	}
}

// ApplyDefaults fills empty path / lab fields without overwriting set values.
func (c *Config) ApplyDefaults() {
	d := Defaults(c.Name, c.CamundaVersion)
	if c.CamundaVersion == "" {
		c.CamundaVersion = d.CamundaVersion
	}
	if c.Paths.BPMN == "" {
		c.Paths.BPMN = d.Paths.BPMN
	}
	if c.Paths.DMN == "" {
		c.Paths.DMN = d.Paths.DMN
	}
	if c.Paths.Forms == "" {
		c.Paths.Forms = d.Paths.Forms
	}
	if c.Paths.Tests == "" {
		c.Paths.Tests = d.Paths.Tests
	}
	if c.Lab.Profile == "" {
		c.Lab.Profile = d.Lab.Profile
	}
	if c.Lab.Resources == "" {
		c.Lab.Resources = d.Lab.Resources
	}
}

// Validate checks required fields after defaults are applied.
func (c Config) Validate() error {
	if strings.TrimSpace(c.Name) == "" {
		return fmt.Errorf("name is required")
	}
	for _, path := range []struct {
		name  string
		value string
	}{
		{name: "paths.bpmn", value: c.Paths.BPMN},
		{name: "paths.dmn", value: c.Paths.DMN},
		{name: "paths.forms", value: c.Paths.Forms},
		{name: "paths.tests", value: c.Paths.Tests},
	} {
		if err := validateRelativePath(path.name, path.value); err != nil {
			return err
		}
	}
	if err := validateResourcePathOverlap(c.Paths); err != nil {
		return err
	}
	return nil
}

// validateResourcePathOverlap rejects BPMN/DMN/Forms directories that resolve to the
// same path or where one contains another. Overlaps would duplicate archive
// entry names during backup create while restore rejects duplicates.
func validateResourcePathOverlap(p Paths) error {
	type named struct {
		name  string
		value string
	}
	paths := []named{
		{name: "paths.bpmn", value: p.BPMN},
		{name: "paths.dmn", value: p.DMN},
		{name: "paths.forms", value: p.Forms},
	}
	for i := 0; i < len(paths); i++ {
		for j := i + 1; j < len(paths); j++ {
			if resourcePathsOverlap(paths[i].value, paths[j].value) {
				return fmt.Errorf("%s (%q) overlaps %s (%q)",
					paths[i].name, paths[i].value, paths[j].name, paths[j].value)
			}
		}
	}
	return nil
}

func resourcePathsOverlap(a, b string) bool {
	a = filepath.ToSlash(filepath.Clean(a))
	b = filepath.ToSlash(filepath.Clean(b))
	if a == b {
		return true
	}
	return strings.HasPrefix(a+"/", b+"/") || strings.HasPrefix(b+"/", a+"/")
}

func validateRelativePath(name, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", name)
	}
	if value != strings.TrimSpace(value) {
		return fmt.Errorf("%s must not contain surrounding whitespace", name)
	}
	if filepath.IsAbs(value) {
		return fmt.Errorf("%s must be relative", name)
	}
	if filepath.Clean(value) != value || value == "." {
		return fmt.Errorf("%s must be clean and contained within the project", name)
	}
	if strings.Contains(value, `\`) {
		return fmt.Errorf("%s must use project-relative path separators", name)
	}
	for _, part := range strings.Split(filepath.ToSlash(value), "/") {
		if part == ".." {
			return fmt.Errorf("%s must be contained within the project", name)
		}
	}
	return nil
}

// Load reads and validates a .camunda.yaml file.
func Load(path string) (Config, error) {
	var data []byte
	err := configstore.WithLocks([]string{path}, func() error {
		state, err := configstore.SnapshotLocked(path)
		if err != nil {
			return err
		}
		if !state.Exists {
			return os.ErrNotExist
		}
		data = state.Data
		return nil
	})
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", path, err)
	}
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return Config{}, err
	}
	return c, nil
}

// Save writes config as YAML to path.
func Save(path string, c Config) error {
	c.ApplyDefaults()
	if err := c.Validate(); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	var desired yaml.Node
	if err := yaml.Unmarshal(data, &desired); err != nil {
		return err
	}
	return configstore.WithLocks([]string{path}, func() error {
		return configstore.UpdateNodeLocked(path, 0o644, func(mapping *yaml.Node) error {
			return configstore.MergeMapping(mapping, desired.Content[0])
		})
	})
}
