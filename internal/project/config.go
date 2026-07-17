package project

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

const ConfigFileName = ".camunda.yaml"

// Paths holds relative directories for process assets.
type Paths struct {
	BPMN  string `yaml:"bpmn"`
	DMN   string `yaml:"dmn"`
	Forms string `yaml:"forms"`
}

// LabHints are optional hints for humans / future tooling (not lab home config).
type LabHints struct {
	Profile   string `yaml:"profile,omitempty"`
	Resources string `yaml:"resources,omitempty"`
}

// Config is the project-local .camunda.yaml schema (v1).
type Config struct {
	Name           string   `yaml:"name"`
	CamundaVersion string   `yaml:"camundaVersion"`
	Paths          Paths    `yaml:"paths"`
	Lab            LabHints `yaml:"lab,omitempty"`
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
	if strings.TrimSpace(c.Paths.BPMN) == "" {
		return fmt.Errorf("paths.bpmn is required")
	}
	if strings.TrimSpace(c.Paths.DMN) == "" {
		return fmt.Errorf("paths.dmn is required")
	}
	if strings.TrimSpace(c.Paths.Forms) == "" {
		return fmt.Errorf("paths.forms is required")
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
