package config

import (
	"fmt"

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
	ActiveEnv      string           `yaml:"activeEnv,omitempty"`
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
	var c Config
	err := WithLocks([]string{path}, func() error {
		state, err := SnapshotLocked(path)
		if err != nil {
			return err
		}
		if !state.Exists {
			c = Defaults()
			return nil
		}
		if _, err := parseDocument(state.Data); err != nil {
			return err
		}
		if err := yaml.Unmarshal(state.Data, &c); err != nil {
			return fmt.Errorf("parse config: %w", err)
		}
		return nil
	})
	if err != nil {
		return Config{}, err
	}
	if c == (Config{}) {
		return Defaults(), nil
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
	path := paths.ConfigFile()
	return WithLocks([]string{path}, func() error {
		state, err := SnapshotLocked(path)
		if err != nil {
			return err
		}
		return saveLocked(path, c, state)
	})
}

// Update atomically loads, mutates, and node-preservingly saves user config.
func Update(mutate func(*Config) error) error {
	path := paths.ConfigFile()
	return WithLocks([]string{path}, func() error {
		state, err := SnapshotLocked(path)
		if err != nil {
			return err
		}
		current := Defaults()
		if state.Exists {
			if _, err := parseDocument(state.Data); err != nil {
				return err
			}
			if err := yaml.Unmarshal(state.Data, &current); err != nil {
				return fmt.Errorf("parse config: %w", err)
			}
			applyDefaults(&current)
		}
		if err := mutate(&current); err != nil {
			return err
		}
		return saveLocked(path, current, state)
	})
}

func saveLocked(path string, c Config, state FileState) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	var desired yaml.Node
	if err := yaml.Unmarshal(data, &desired); err != nil {
		return err
	}
	return UpdateNodeLocked(path, 0o600, func(mapping *yaml.Node) error {
		source := desired.Content[0]
		updates := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for i := 0; i < len(source.Content); i += 2 {
			key := source.Content[i].Value
			if key == "activeEnv" && c.ActiveEnv == "" {
				continue
			}
			updates.Content = append(updates.Content, source.Content[i], source.Content[i+1])
		}
		if err := MergeMapping(mapping, updates); err != nil {
			return err
		}
		if !state.Exists {
			references := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			references.Content = append(references.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "complete"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: "true"},
				&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: "projects"},
				&yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"},
			)
			if err := setNode(mapping, "environmentReferences", references); err != nil {
				return err
			}
		}
		return nil
	})
}

func applyDefaults(c *Config) {
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
}

func setNode(mapping *yaml.Node, key string, value *yaml.Node) error {
	found := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if found {
			return fmt.Errorf("duplicate %s field", key)
		}
		mapping.Content[i+1] = value
		found = true
	}
	if !found {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			value,
		)
	}
	return nil
}
