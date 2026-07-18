package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"gopkg.in/yaml.v3"
)

type AIConfig struct {
	Enabled bool `yaml:"enabled"`
}

type MonitoringConfig struct {
	Enabled bool `yaml:"enabled"`
}

type Config struct {
	Version        string           `yaml:"version"`
	Profile        string           `yaml:"profile"`
	Resources      string           `yaml:"resources"`
	Host           string           `yaml:"host"`
	ComposeProject string           `yaml:"compose_project"`
	AI             AIConfig         `yaml:"ai"`
	Monitoring     MonitoringConfig `yaml:"monitoring"`
}

func Defaults() Config {
	return Config{
		Version:        "8.8",
		Profile:        "light",
		Resources:      "balanced",
		Host:           "localhost",
		ComposeProject: "camunda-lab",
	}
}

func Load() (Config, error) {
	path := paths.ConfigFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil
		}
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	d := Defaults()
	if c.Version == "" {
		c.Version = d.Version
	}
	if c.Profile == "" {
		c.Profile = d.Profile
	}
	if c.Resources == "" {
		c.Resources = d.Resources
	}
	if c.Host == "" {
		c.Host = d.Host
	}
	if c.ComposeProject == "" {
		c.ComposeProject = d.ComposeProject
	}
	return c, nil
}

func Save(c Config) error {
	if err := os.MkdirAll(filepath.Dir(paths.ConfigFile()), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigFile(), data, 0o644)
}
