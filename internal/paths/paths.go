package paths

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	mu   sync.Mutex
	home string
)

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	home = ""
}

func Home() string {
	mu.Lock()
	defer mu.Unlock()
	if home != "" {
		return home
	}
	if v := os.Getenv("CAMUNDA_LAB_HOME"); v != "" {
		home = v
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		home = filepath.Join(".", ".camunda-lab")
		return home
	}
	home = filepath.Join(userHome, ".camunda-lab")
	return home
}

func ConfigFile() string         { return filepath.Join(Home(), "config.yaml") }
func VersionsDir() string        { return filepath.Join(Home(), "versions") }
func VersionDir(v string) string { return filepath.Join(VersionsDir(), v) }
func OverlaysDir() string        { return filepath.Join(Home(), "overlays") }
func LogsDir() string            { return filepath.Join(Home(), "logs") }
func ActiveFile() string         { return filepath.Join(Home(), "active.yaml") }
