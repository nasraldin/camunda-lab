package env

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ActiveFile stores the active profile name.
func ActiveFile(labHome string) string {
	return filepath.Join(labHome, "active-env")
}

// GetActive returns the active profile, defaulting to the implicit lab only
// when no active-state file exists.
func GetActive(labHome string) (string, error) {
	data, err := os.ReadFile(ActiveFile(labHome))
	if errors.Is(err, os.ErrNotExist) {
		return "lab", nil
	}
	if err != nil {
		return "", fmt.Errorf("read active environment: %w", err)
	}
	name := strings.TrimSuffix(string(data), "\n")
	if name == "lab" {
		return name, nil
	}
	if err := ValidateName(name); err != nil {
		return "", fmt.Errorf("invalid active environment: %w", err)
	}
	return name, nil
}

// SetActive atomically selects the implicit lab or an existing stored profile.
func SetActive(labHome, profilesDir, name string) error {
	if name != "lab" {
		if err := ValidateName(name); err != nil {
			return err
		}
		if _, err := LoadNamedProfile(profilesDir, name); err != nil {
			return fmt.Errorf("activate environment %q: %w", name, err)
		}
	}
	return writeActive(labHome, name)
}

// RemoveProfile removes a validated stored profile. If it is active, the
// active state moves to the implicit lab before deletion.
func RemoveProfile(labHome, profilesDir, name string) error {
	return removeProfile(labHome, profilesDir, name, writeActive)
}

func removeProfile(labHome, profilesDir, name string, persistActive func(string, string) error) error {
	path, err := ProfilePath(profilesDir, name)
	if err != nil {
		return err
	}
	if _, err := LoadNamedProfile(profilesDir, name); err != nil {
		return fmt.Errorf("remove environment %q: %w", name, err)
	}
	active, err := GetActive(labHome)
	if err != nil {
		return err
	}

	tomb, err := reserveTombstone(profilesDir, name)
	if err != nil {
		return err
	}
	if err := os.Rename(path, tomb); err != nil {
		return fmt.Errorf("tombstone environment %q: %w", name, err)
	}
	rollbackProfile := func() error {
		if rollbackErr := os.Rename(tomb, path); rollbackErr != nil {
			return fmt.Errorf("restore environment %q: %w", name, rollbackErr)
		}
		return nil
	}

	if active == name {
		if err := persistActive(labHome, "lab"); err != nil {
			if rollbackErr := rollbackProfile(); rollbackErr != nil {
				return fmt.Errorf("persist lab fallback: %v; rollback failed: %w", err, rollbackErr)
			}
			return fmt.Errorf("persist lab fallback: %w", err)
		}
	}
	if err := os.Remove(tomb); err != nil {
		var rollbackErr error
		if active == name {
			rollbackErr = persistActive(labHome, name)
		}
		if profileErr := rollbackProfile(); profileErr != nil {
			rollbackErr = errors.Join(rollbackErr, profileErr)
		}
		if rollbackErr != nil {
			return fmt.Errorf("delete environment %q: %v; rollback failed: %w", name, err, rollbackErr)
		}
		return fmt.Errorf("delete environment %q: %w", name, err)
	}
	return nil
}

func writeActive(labHome, name string) error {
	if err := os.MkdirAll(labHome, 0o755); err != nil {
		return fmt.Errorf("create lab home: %w", err)
	}
	tmp, err := os.CreateTemp(labHome, ".active-env-*")
	if err != nil {
		return fmt.Errorf("create active environment temporary file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o644); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("set active environment permissions: %w", err)
	}
	if _, err := tmp.WriteString(name + "\n"); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write active environment: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("sync active environment: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close active environment: %w", err)
	}
	if err := os.Rename(tmpName, ActiveFile(labHome)); err != nil {
		return fmt.Errorf("replace active environment: %w", err)
	}
	return nil
}

func reserveTombstone(profilesDir, name string) (string, error) {
	tmp, err := os.CreateTemp(profilesDir, "."+name+".deleting-*")
	if err != nil {
		return "", fmt.Errorf("reserve environment tombstone: %w", err)
	}
	path := tmp.Name()
	if err := tmp.Close(); err != nil {
		_ = os.Remove(path)
		return "", fmt.Errorf("close environment tombstone: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("prepare environment tombstone: %w", err)
	}
	return path, nil
}
