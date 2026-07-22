package env

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

type profileRoot struct {
	path     string
	file     *os.File
	info     os.FileInfo
	lock     *os.File
	lockInfo os.FileInfo
}

func openProfileRoot(path string, create bool, afterOpen func()) (*profileRoot, error) {
	if create {
		if info, err := os.Lstat(path); errors.Is(err, os.ErrNotExist) {
			if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
				return nil, err
			}
			if err := syncDirectory(filepath.Dir(path)); err != nil {
				return nil, err
			}
		} else if err != nil {
			return nil, err
		} else if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return nil, fmt.Errorf("profile root must be a non-symlink directory")
		}
	}
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	info, err := file.Stat()
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	root := &profileRoot{path: path, file: file, info: info}
	lockFD, err := unix.Openat(fd, ".profiles.lock", unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		_ = file.Close()
		return nil, err
	}
	root.lock = os.NewFile(uintptr(lockFD), ".profiles.lock")
	lockInfo, err := root.lock.Stat()
	if err != nil || !lockInfo.Mode().IsRegular() {
		_ = root.lock.Close()
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("profile lock must be regular")
	}
	if err := unix.Flock(lockFD, unix.LOCK_EX); err != nil {
		_ = root.lock.Close()
		_ = file.Close()
		return nil, err
	}
	root.lockInfo = lockInfo
	if afterOpen != nil {
		afterOpen()
	}
	if err := root.revalidate(); err != nil {
		_ = root.close()
		return nil, err
	}
	return root, nil
}

func syncDirectory(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}

func (r *profileRoot) close() error {
	if r.lock != nil {
		_ = unix.Flock(int(r.lock.Fd()), unix.LOCK_UN)
		_ = r.lock.Close()
	}
	return r.file.Close()
}

func (r *profileRoot) revalidate() error {
	current, err := os.Lstat(r.path)
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.IsDir() || !os.SameFile(current, r.info) {
		return fmt.Errorf("profile root changed during operation")
	}
	lockFD, err := unix.Openat(int(r.file.Fd()), ".profiles.lock", unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	lockFile := os.NewFile(uintptr(lockFD), ".profiles.lock")
	lockCurrent, err := lockFile.Stat()
	_ = lockFile.Close()
	if err != nil {
		return err
	}
	if !lockCurrent.Mode().IsRegular() || !os.SameFile(lockCurrent, r.lockInfo) {
		return fmt.Errorf("profile lock changed during operation")
	}
	return nil
}

func (r *profileRoot) load(name string) (Profile, error) {
	if err := ValidateName(name); err != nil {
		return Profile{}, err
	}
	fd, err := unix.Openat(int(r.file.Fd()), name+".yaml", unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return Profile{}, err
	}
	file := os.NewFile(uintptr(fd), name+".yaml")
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return Profile{}, err
	}
	if !info.Mode().IsRegular() {
		return Profile{}, fmt.Errorf("profile must be a regular non-symlink file")
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return Profile{}, err
	}
	profile, err := decodeProfile(data, name)
	if err != nil {
		return Profile{}, err
	}
	currentFD, err := unix.Openat(int(r.file.Fd()), name+".yaml", unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return Profile{}, err
	}
	currentFile := os.NewFile(uintptr(currentFD), name+".yaml")
	currentInfo, err := currentFile.Stat()
	_ = currentFile.Close()
	if err != nil {
		return Profile{}, err
	}
	if !currentInfo.Mode().IsRegular() || !os.SameFile(info, currentInfo) {
		return Profile{}, fmt.Errorf("profile changed during read")
	}
	if err := r.revalidate(); err != nil {
		return Profile{}, err
	}
	return profile, nil
}

func (r *profileRoot) list() ([]Profile, error) {
	entries, err := r.file.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	profiles := make([]Profile, 0)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".yaml") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".yaml")
		profile, err := r.load(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", entry.Name(), err)
		}
		profiles = append(profiles, profile)
	}
	if err := r.revalidate(); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (r *profileRoot) save(profile Profile, replace bool) error {
	if err := profile.Validate(); err != nil {
		return err
	}
	if !replace {
		if fd, err := unix.Openat(int(r.file.Fd()), profile.Name+".yaml", unix.O_RDONLY|unix.O_NOFOLLOW, 0); err == nil {
			_ = unix.Close(fd)
			return os.ErrExist
		} else if !errors.Is(err, unix.ENOENT) {
			return err
		}
	}
	data, err := yaml.Marshal(profile)
	if err != nil {
		return err
	}
	tmp := "." + profile.Name + "-" + secureSuffix()
	fd, err := unix.Openat(int(r.file.Fd()), tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), tmp)
	cleanup := func() {
		_ = file.Close()
		_ = unix.Unlinkat(int(r.file.Fd()), tmp, 0)
	}
	if _, err := file.Write(data); err != nil {
		cleanup()
		return err
	}
	if err := file.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := file.Close(); err != nil {
		_ = unix.Unlinkat(int(r.file.Fd()), tmp, 0)
		return err
	}
	if err := r.revalidate(); err != nil {
		_ = unix.Unlinkat(int(r.file.Fd()), tmp, 0)
		return err
	}
	if err := unix.Renameat(int(r.file.Fd()), tmp, int(r.file.Fd()), profile.Name+".yaml"); err != nil {
		_ = unix.Unlinkat(int(r.file.Fd()), tmp, 0)
		return err
	}
	return unix.Fsync(int(r.file.Fd()))
}

func (r *profileRoot) rename(oldName, newName string) error {
	if err := r.revalidate(); err != nil {
		return err
	}
	if err := unix.Renameat(int(r.file.Fd()), oldName, int(r.file.Fd()), newName); err != nil {
		return err
	}
	return unix.Fsync(int(r.file.Fd()))
}

func (r *profileRoot) renameExpected(oldName, newName string, expected os.FileInfo) error {
	current, err := r.stat(oldName)
	if err != nil {
		return err
	}
	if !os.SameFile(current, expected) {
		return fmt.Errorf("profile changed before rename")
	}
	return r.rename(oldName, newName)
}

func (r *profileRoot) stat(name string) (os.FileInfo, error) {
	fd, err := unix.Openat(int(r.file.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s must be regular", name)
	}
	return info, nil
}

func (r *profileRoot) unlink(name string) error {
	if err := r.revalidate(); err != nil {
		return err
	}
	if err := unix.Unlinkat(int(r.file.Fd()), name, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return unix.Fsync(int(r.file.Fd()))
}

func (r *profileRoot) exists(name string) (bool, error) {
	fd, err := unix.Openat(int(r.file.Fd()), name, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_ = unix.Close(fd)
	return true, r.revalidate()
}

func decodeProfile(data []byte, expectedName string) (Profile, error) {
	raw := strings.ToLower(string(data))
	if strings.Contains(raw, "clientsecret:") && !strings.Contains(raw, "clientsecretenv:") {
		return Profile{}, fmt.Errorf("refusing profile with inline clientSecret (use clientSecretEnv)")
	}
	var profile Profile
	if err := yaml.Unmarshal(data, &profile); err != nil {
		return Profile{}, err
	}
	if err := profile.Validate(); err != nil {
		return Profile{}, err
	}
	if profile.Name != expectedName {
		return Profile{}, fmt.Errorf("profile name %q does not match filename %q", profile.Name, expectedName)
	}
	return profile, nil
}

func secureSuffix() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}

func loadProfileAtPath(path string) (Profile, error) {
	root, err := openProfileRoot(filepath.Dir(path), false, nil)
	if err != nil {
		return Profile{}, err
	}
	defer root.close()
	name := strings.TrimSuffix(filepath.Base(path), ".yaml")
	if filepath.Ext(path) != ".yaml" {
		return Profile{}, fmt.Errorf("profile filename must end in .yaml")
	}
	return root.load(name)
}
