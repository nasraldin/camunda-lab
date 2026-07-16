package tools

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
)

func C8ctlStatus() (installed bool, path string, err error) {
	path, err = exec.LookPath("c8ctl")
	if err != nil {
		path, err = exec.LookPath("c8")
	}
	if err != nil {
		return false, "", nil
	}
	return true, path, nil
}

func C8ctlInstall() error {
	if _, err := exec.LookPath("npm"); err != nil {
		return fmt.Errorf("npm not found; install Node.js or: npm install -g @camunda8/cli")
	}
	cmd := exec.Command("npm", "install", "-g", "@camunda8/cli")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

type modelerProfiles map[string]any

func WriteModelerProfile(name, restURL, grpcAddr string) (string, error) {
	path, err := modelerProfilesPath()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	profiles := modelerProfiles{}
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		_ = json.Unmarshal(data, &profiles)
	}
	profiles[name] = map[string]any{
		"name": name,
		"zeebe": map[string]any{
			"gatewayAddress": grpcAddr,
			"restAddress":    restURL,
		},
	}
	out, err := json.MarshalIndent(profiles, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		return "", err
	}
	return path, nil
}

func modelerProfilesPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "camunda-modeler", "resources", "profiles.json"), nil
	case "linux":
		return filepath.Join(home, ".config", "camunda-modeler", "resources", "profiles.json"), nil
	default:
		return "", fmt.Errorf("modeler profile path unsupported on %s", runtime.GOOS)
	}
}
