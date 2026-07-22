package scan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
)

type secureDir struct {
	fd     int
	parent *secureDir
	name   string
	dev    uint64
	ino    uint64
}

type secureFS struct {
	root   *secureDir
	opened []*secureDir
}

type secureFile struct {
	file *os.File
	dev  uint64
	ino  uint64
}

func openSecureFS(root string) (*secureFS, error) {
	absolute := canonicalScanSystemAlias(filepath.Clean(root))
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("open filesystem anchor: %w", err)
	}
	filesystem := &secureFS{}
	anchor, err := newSecureDir(fd, nil, "")
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	filesystem.opened = append(filesystem.opened, anchor)
	current := anchor
	for _, component := range splitScanPath(absolute) {
		current, err = filesystem.openChild(current, component)
		if err != nil {
			filesystem.close()
			return nil, fmt.Errorf("open root component %q without following symlinks: %w", component, err)
		}
	}
	filesystem.root = current
	return filesystem, nil
}

func (filesystem *secureFS) close() {
	for index := len(filesystem.opened) - 1; index >= 0; index-- {
		_ = unix.Close(filesystem.opened[index].fd)
	}
}

func (filesystem *secureFS) release(directory *secureDir) {
	last := len(filesystem.opened) - 1
	if last < 0 || filesystem.opened[last] != directory {
		return
	}
	_ = unix.Close(directory.fd)
	filesystem.opened = filesystem.opened[:last]
}

func (filesystem *secureFS) openChild(parent *secureDir, name string) (*secureDir, error) {
	var before unix.Stat_t
	if err := unix.Fstatat(parent.fd, name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, err
	}
	if before.Mode&unix.S_IFMT == unix.S_IFLNK {
		return nil, errors.New("symbolic link is not allowed")
	}
	if before.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, errors.New("component is not a directory")
	}
	fd, err := unix.Openat(parent.fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	child, err := newSecureDir(fd, parent, name)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	if child.dev != uint64(before.Dev) || child.ino != before.Ino {
		_ = unix.Close(fd)
		return nil, errors.New("directory changed while opening")
	}
	filesystem.opened = append(filesystem.opened, child)
	return child, nil
}

func newSecureDir(fd int, parent *secureDir, name string) (*secureDir, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, errors.New("opened component is not a directory")
	}
	return &secureDir{
		fd: fd, parent: parent, name: name, dev: uint64(stat.Dev), ino: stat.Ino,
	}, nil
}

func (filesystem *secureFS) openFile(parent *secureDir, name string) (*secureFile, error) {
	var before unix.Stat_t
	if err := unix.Fstatat(parent.fd, name, &before, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return nil, err
	}
	if before.Mode&unix.S_IFMT == unix.S_IFLNK {
		return nil, errors.New("symbolic link is not allowed")
	}
	if before.Mode&unix.S_IFMT != unix.S_IFREG {
		return nil, errors.New("candidate is not a regular file")
	}
	fd, err := unix.Openat(parent.fd, name, unix.O_RDONLY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create file handle")
	}
	var opened unix.Stat_t
	if err := unix.Fstat(fd, &opened); err != nil {
		_ = file.Close()
		return nil, err
	}
	if opened.Mode&unix.S_IFMT != unix.S_IFREG ||
		uint64(opened.Dev) != uint64(before.Dev) || opened.Ino != before.Ino {
		_ = file.Close()
		return nil, errors.New("candidate changed while opening")
	}
	return &secureFile{file: file, dev: uint64(opened.Dev), ino: opened.Ino}, nil
}

func (filesystem *secureFS) verify() error {
	for _, directory := range filesystem.opened {
		if directory.parent == nil {
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(directory.parent.fd, directory.name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
			uint64(stat.Dev) != directory.dev || stat.Ino != directory.ino {
			return errors.New("anchored directory was replaced")
		}
	}
	return nil
}

func verifySecureFile(parent *secureDir, name string, opened *secureFile) error {
	var stat unix.Stat_t
	if err := unix.Fstatat(parent.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG ||
		uint64(stat.Dev) != opened.dev || stat.Ino != opened.ino {
		return errors.New("candidate path was replaced")
	}
	return nil
}

func readSecureDir(directory *secureDir) ([]os.DirEntry, error) {
	fd, err := unix.Dup(directory.fd)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), directory.name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create directory handle")
	}
	defer file.Close()
	return file.ReadDir(-1)
}

func splitScanPath(value string) []string {
	trimmed := strings.TrimPrefix(filepath.Clean(value), string(filepath.Separator))
	if trimmed == "" || trimmed == "." {
		return nil
	}
	return strings.Split(trimmed, string(filepath.Separator))
}

func canonicalScanSystemAlias(value string) string {
	if runtime.GOOS != "darwin" {
		return value
	}
	for _, alias := range []string{"/etc", "/tmp", "/var"} {
		if value == alias || strings.HasPrefix(value, alias+"/") {
			return "/private" + value
		}
	}
	return value
}
