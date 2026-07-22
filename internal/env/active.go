package env

import (
	"fmt"
	"path/filepath"
)

// ActiveFile stores the active profile name.
func ActiveFile(labHome string) string {
	return filepath.Join(labHome, "active-env")
}

// GetActive returns the active profile, defaulting to the implicit lab only
// when no active-state file exists.
func GetActive(labHome string) (string, error) {
	resolved, err := NewService(labHome).Resolve(ResolveRequest{})
	if err != nil {
		return "", err
	}
	return resolved.Profile.Name, nil
}

// SetActive atomically selects the implicit lab or an existing stored profile.
func SetActive(labHome, profilesDir, name string) error {
	if filepath.Clean(profilesDir) != filepath.Join(filepath.Clean(labHome), "envs") {
		return fmt.Errorf("profiles directory must be the canonical global profile root")
	}
	_, err := NewService(labHome).Use(name, "")
	return err
}

// RemoveProfile removes a validated stored profile. If it is active, the
// active state moves to the implicit lab before deletion.
func RemoveProfile(labHome, profilesDir, name string) error {
	if filepath.Clean(profilesDir) != filepath.Join(filepath.Clean(labHome), "envs") {
		return fmt.Errorf("profiles directory must be the canonical global profile root")
	}
	return NewService(labHome).Remove(name, "", ProfileSourceGlobal)
}
