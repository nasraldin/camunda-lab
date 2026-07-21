package project

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ScaffoldOpts configures project scaffolding.
type ScaffoldOpts struct {
	Dir       string
	Name      string
	Version   string
	Profile   string
	Resources string
	Force     bool
}

// Validate checks scaffold inputs shared by all callers.
func (o ScaffoldOpts) Validate() error {
	if strings.TrimSpace(o.Dir) == "" {
		return fmt.Errorf("directory is required")
	}
	if o.Name != "" {
		name := strings.TrimSpace(o.Name)
		if name == "" || name != o.Name || name == "." || name == ".." ||
			strings.ContainsAny(name, `/\`) {
			return fmt.Errorf("name must be a plain project name")
		}
	}
	if o.Version != "" && strings.TrimSpace(o.Version) != o.Version {
		return fmt.Errorf("version must not contain surrounding whitespace")
	}
	if o.Profile != "" && !oneOf(o.Profile, "light", "full", "modeler") {
		return fmt.Errorf("profile must be one of light, full, or modeler")
	}
	if o.Resources != "" && !oneOf(o.Resources, "small", "balanced", "power") {
		return fmt.Errorf("resources must be one of small, balanced, or power")
	}
	return nil
}

var scaffoldDirs = []string{
	"bpmn",
	"dmn",
	"forms",
	"workers",
	"connectors",
	"scripts",
	"tests",
	"environments",
	"helm",
}

// Scaffold creates a Camunda application project tree under opts.Dir.
func Scaffold(opts ScaffoldOpts) error {
	if err := opts.Validate(); err != nil {
		return err
	}
	dir, err := filepath.Abs(opts.Dir)
	if err != nil {
		return err
	}

	if err := ensureEmptyOrForce(dir, opts.Force); err != nil {
		return err
	}

	name := strings.TrimSpace(opts.Name)
	if name == "" {
		name = filepath.Base(dir)
	}
	version := opts.Version
	if version == "" {
		version = "8.9"
	}
	profile := opts.Profile
	if profile == "" {
		profile = "light"
	}
	resources := opts.Resources
	if resources == "" {
		resources = "balanced"
	}

	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, d := range scaffoldDirs {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "environments", ".gitkeep"), []byte{}, 0o644); err != nil {
		return err
	}

	cfg := Defaults(name, version)
	cfg.Lab.Profile = profile
	cfg.Lab.Resources = resources
	if err := Save(filepath.Join(dir, ConfigFileName), cfg); err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readmeContent(name, version)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "helm", "README.md"), []byte(helmReadme), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "docker-compose.yml"), []byte(composeStub), 0o644); err != nil {
		return err
	}
	return nil
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}

func ensureEmptyOrForce(dir string, force bool) error {
	fi, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if !fi.IsDir() {
		return fmt.Errorf("%s exists and is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	if force {
		return nil
	}
	return fmt.Errorf("directory %s is not empty (use --force to proceed)", dir)
}

func readmeContent(name, version string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", name)
	b.WriteString("Camunda 8 project scaffold (Camunda Lab).\n\n")
	b.WriteString("## Local lab\n\n")
	b.WriteString("Use [Camunda Lab](https://github.com/nasraldin/camunda-lab) to run the official Docker Compose stack locally:\n\n")
	b.WriteString("```bash\n")
	fmt.Fprintf(&b, "camunda install --version %s --yes\n", version)
	b.WriteString("camunda ui\n")
	b.WriteString("```\n\n")
	b.WriteString("## Project layout\n\n")
	b.WriteString("| Path | Purpose |\n")
	b.WriteString("|------|---------|\n")
	b.WriteString("| bpmn/ | Process models |\n")
	b.WriteString("| dmn/ | Decision tables |\n")
	b.WriteString("| forms/ | Forms |\n")
	b.WriteString("| workers/ | Job workers |\n")
	b.WriteString("| connectors/ | Connector configs |\n")
	b.WriteString("| scripts/ | Helper scripts |\n")
	b.WriteString("| tests/ | Process / worker tests |\n")
	b.WriteString("| environments/ | Environment profiles (Phase 3) |\n")
	b.WriteString("| helm/ | Production Helm notes (not managed by Lab) |\n\n")
	b.WriteString("Project config: `.camunda.yaml`.\n\n")
	b.WriteString("## Deploy\n\n")
	b.WriteString("Deploy BPMN/DMN/forms with **official Camunda tooling** (for example the Camunda 8 CLI) against your lab or remote cluster. Camunda Lab gets the stack up; it does not replace resource deploy commands.\n")
	return b.String()
}

const helmReadme = `# Helm (production)

Camunda Lab runs **Docker Compose** for local development and evaluation.

For production / Kubernetes, use Camunda’s official Helm charts and your own values.
This directory is a placeholder — Lab does not generate or manage Helm releases.
`

const composeStub = `# This file is a stub — do not use it to run Camunda.
#
# Camunda Lab wraps the official Camunda Docker Compose distributions:
#
#   camunda install
#   camunda up
#   camunda ui
#
# See https://github.com/nasraldin/camunda-lab
`
