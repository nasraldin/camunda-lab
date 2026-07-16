package lab

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"gopkg.in/yaml.v3"
)

type Active struct {
	Version string   `yaml:"version"`
	Profile string   `yaml:"profile"`
	WorkDir string   `yaml:"workdir"`
	Files   []string `yaml:"files"`
}

func WriteActive(a Active) error {
	data, err := yaml.Marshal(a)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(paths.ActiveFile(), data, 0o644)
}

func (l *Lab) Nuke(ctx context.Context, yes bool) error {
	if !yes && os.Getenv("CONFIRM") != "yes" {
		fmt.Fprint(os.Stderr, "This will delete ~/.camunda-lab volumes and config. Type 'yes' to continue: ")
		line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
		if strings.TrimSpace(line) != "yes" {
			return fmt.Errorf("aborted")
		}
	}
	_ = l.Down(ctx, true)
	home := paths.Home()
	if err := os.RemoveAll(home); err != nil {
		return err
	}
	fmt.Printf("Removed %s\n", home)
	return nil
}
