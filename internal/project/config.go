package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
	return nil
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
	data, err := os.ReadFile(path)
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
	return os.WriteFile(path, data, 0o644)
}
