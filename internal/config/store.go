package config

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
	"gopkg.in/yaml.v3"
)

// FileState is an exact config-file snapshot used by durable transactions.
type FileState struct {
	Exists bool
	Data   []byte
	Mode   os.FileMode
}

// WithLocks acquires config locks in canonical sorted order. Lock files are
// regular, no-follow files adjacent to each config.
func WithLocks(paths []string, fn func() error) error {
	unique := make(map[string]struct{}, len(paths))
	ordered := make([]string, 0, len(paths))
	for _, path := range paths {
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, exists := unique[path]; exists {
			continue
		}
		unique[path] = struct{}{}
		ordered = append(ordered, path)
	}
	locks := make([]*os.File, 0, len(ordered))
	for _, path := range ordered {
		lock, err := openLock(path + ".lock")
		if err != nil {
			releaseConfigLocks(locks)
			return err
		}
		locks = append(locks, lock)
	}
	defer releaseConfigLocks(locks)
	for _, lock := range locks {
		if err := revalidateConfigLock(lock); err != nil {
			return err
		}
	}
	err := fn()
	if err != nil {
		return err
	}
	for _, lock := range locks {
		if err := revalidateConfigLock(lock); err != nil {
			return err
		}
	}
	return nil
}

// UpdateScalar performs a node-preserving scalar update under the config lock.
func UpdateScalar(path, key, value string, mode os.FileMode) error {
	return WithLocks([]string{path}, func() error {
		return UpdateScalarLocked(path, key, value, mode)
	})
}

// UpdateScalarLocked updates a scalar while its config lock is held.
func UpdateScalarLocked(path, key, value string, mode os.FileMode) error {
	return UpdateNodeLocked(path, mode, func(mapping *yaml.Node) error {
		return setScalar(mapping, key, value)
	})
}

// RemoveScalarLocked removes a scalar while its config lock is held.
func RemoveScalarLocked(path, key string, mode os.FileMode) error {
	return UpdateNodeLocked(path, mode, func(mapping *yaml.Node) error {
		content := make([]*yaml.Node, 0, len(mapping.Content))
		found := false
		for i := 0; i < len(mapping.Content); i += 2 {
			if mapping.Content[i].Value == key {
				if found {
					return fmt.Errorf("duplicate %s field", key)
				}
				found = true
				continue
			}
			content = append(content, mapping.Content[i], mapping.Content[i+1])
		}
		mapping.Content = content
		return nil
	})
}

// ReadScalarLocked reads a scalar while its config lock is held.
func ReadScalarLocked(path, key string) (string, bool, error) {
	state, err := SnapshotLocked(path)
	if err != nil || !state.Exists {
		return "", false, err
	}
	document, err := parseDocument(state.Data)
	if err != nil {
		return "", false, err
	}
	return scalar(document.Content[0], key)
}

// UpdateNodeLocked preserves unknown fields, order, style, and comments.
func UpdateNodeLocked(path string, mode os.FileMode, mutate func(*yaml.Node) error) error {
	state, err := SnapshotLocked(path)
	if err != nil {
		return err
	}
	document := emptyDocument()
	if state.Exists {
		document, err = parseDocument(state.Data)
		if err != nil {
			return err
		}
	}
	if err := mutate(document.Content[0]); err != nil {
		return err
	}
	data, err := yaml.Marshal(document)
	if err != nil {
		return err
	}
	if state.Mode != 0 && mode != 0o600 {
		mode = state.Mode
	}
	return WriteLocked(path, data, mode)
}

// MergeMapping recursively updates known leaves while retaining unknown nested
// keys, existing key order, and comments.
func MergeMapping(destination, desired *yaml.Node) error {
	if destination.Kind != yaml.MappingNode || desired.Kind != yaml.MappingNode {
		return fmt.Errorf("recursive merge requires mapping nodes")
	}
	if err := validateUniqueKeys(destination); err != nil {
		return err
	}
	if err := validateUniqueKeys(desired); err != nil {
		return err
	}
	for i := 0; i < len(desired.Content); i += 2 {
		key := desired.Content[i]
		value := desired.Content[i+1]
		index := mappingIndex(destination, key.Value)
		if index < 0 {
			destination.Content = append(destination.Content, key, value)
			continue
		}
		existing := destination.Content[index+1]
		if existing.Kind == yaml.MappingNode && value.Kind == yaml.MappingNode {
			if err := MergeMapping(existing, value); err != nil {
				return err
			}
			continue
		}
		preserveNodeComments(value, existing)
		destination.Content[index+1] = value
	}
	return nil
}

func validateUniqueKeys(mapping *yaml.Node) error {
	seen := make(map[string]struct{}, len(mapping.Content)/2)
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i].Value
		if _, exists := seen[key]; exists {
			return fmt.Errorf("duplicate %s field", key)
		}
		seen[key] = struct{}{}
	}
	return nil
}

func mappingIndex(mapping *yaml.Node, key string) int {
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func preserveNodeComments(replacement, existing *yaml.Node) {
	if replacement.HeadComment == "" {
		replacement.HeadComment = existing.HeadComment
	}
	if replacement.LineComment == "" {
		replacement.LineComment = existing.LineComment
	}
	if replacement.FootComment == "" {
		replacement.FootComment = existing.FootComment
	}
}

// SnapshotLocked reads a regular config file without following its final
// component.
func SnapshotLocked(path string) (FileState, error) {
	parent, base, err := openParent(path)
	if err != nil {
		return FileState{}, err
	}
	defer parent.Close()
	fd, err := unix.Openat(int(parent.Fd()), base, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if errors.Is(err, unix.ENOENT) {
		return FileState{}, nil
	}
	if err != nil {
		return FileState{}, err
	}
	file := os.NewFile(uintptr(fd), path)
	defer file.Close()
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return FileState{}, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		return FileState{}, fmt.Errorf("%s must be a regular non-symlink file", path)
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return FileState{}, err
	}
	currentFD, err := unix.Openat(int(parent.Fd()), base, unix.O_RDONLY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return FileState{}, err
	}
	currentFile := os.NewFile(uintptr(currentFD), path)
	currentInfo, err := currentFile.Stat()
	_ = currentFile.Close()
	openedInfo, statErr := file.Stat()
	if err != nil {
		return FileState{}, err
	}
	if statErr != nil {
		return FileState{}, statErr
	}
	if !os.SameFile(currentInfo, openedInfo) {
		return FileState{}, fmt.Errorf("%s changed during read", path)
	}
	return FileState{Exists: true, Data: data, Mode: os.FileMode(stat.Mode & 0o777)}, nil
}

// WriteLocked atomically replaces a config file and fsyncs its parent.
func WriteLocked(path string, data []byte, mode os.FileMode) error {
	parent, base, err := openParent(path)
	if err != nil {
		return err
	}
	defer parent.Close()
	if fd, openErr := unix.Openat(int(parent.Fd()), base, unix.O_RDONLY|unix.O_NOFOLLOW, 0); openErr == nil {
		var stat unix.Stat_t
		checkErr := unix.Fstat(fd, &stat)
		_ = unix.Close(fd)
		if checkErr != nil {
			return checkErr
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFREG {
			return fmt.Errorf("%s must be a regular non-symlink file", path)
		}
	} else if !errors.Is(openErr, unix.ENOENT) {
		return openErr
	}
	tmp := "." + base + "-" + randomSuffix()
	fd, err := unix.Openat(int(parent.Fd()), tmp, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW, uint32(mode))
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), tmp)
	cleanup := func() {
		_ = file.Close()
		_ = unix.Unlinkat(int(parent.Fd()), tmp, 0)
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
		_ = unix.Unlinkat(int(parent.Fd()), tmp, 0)
		return err
	}
	if err := unix.Renameat(int(parent.Fd()), tmp, int(parent.Fd()), base); err != nil {
		_ = unix.Unlinkat(int(parent.Fd()), tmp, 0)
		return err
	}
	return unix.Fsync(int(parent.Fd()))
}

// RestoreLocked restores an exact snapshot and fsyncs the parent.
func RestoreLocked(path string, state FileState, fallbackMode os.FileMode) error {
	if state.Exists {
		mode := state.Mode
		if mode == 0 {
			mode = fallbackMode
		}
		return WriteLocked(path, state.Data, mode)
	}
	parent, base, err := openParent(path)
	if err != nil {
		return err
	}
	defer parent.Close()
	if err := unix.Unlinkat(int(parent.Fd()), base, 0); err != nil && !errors.Is(err, unix.ENOENT) {
		return err
	}
	return unix.Fsync(int(parent.Fd()))
}

func openLock(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CREAT|unix.O_NOFOLLOW, 0o600)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), path)
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		_ = file.Close()
		return nil, err
	}
	if stat.Mode&unix.S_IFMT != unix.S_IFREG {
		_ = file.Close()
		return nil, fmt.Errorf("lock %s is not regular", path)
	}
	if err := unix.Flock(fd, unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, err
	}
	openedInfo, err := file.Stat()
	if err != nil {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil || !os.SameFile(info, openedInfo) {
		_ = unix.Flock(fd, unix.LOCK_UN)
		_ = file.Close()
		if err != nil {
			return nil, err
		}
		return nil, fmt.Errorf("lock %s changed while acquiring", path)
	}
	return file, nil
}

func releaseConfigLocks(locks []*os.File) {
	for i := len(locks) - 1; i >= 0; i-- {
		_ = unix.Flock(int(locks[i].Fd()), unix.LOCK_UN)
		_ = locks[i].Close()
	}
}

func revalidateConfigLock(lock *os.File) error {
	opened, err := lock.Stat()
	if err != nil {
		return err
	}
	current, err := os.Lstat(lock.Name())
	if err != nil {
		return err
	}
	if current.Mode()&os.ModeSymlink != 0 || !current.Mode().IsRegular() || !os.SameFile(current, opened) {
		return fmt.Errorf("lock %s changed while held", lock.Name())
	}
	return nil
}

func openParent(path string) (*os.File, string, error) {
	parentPath := filepath.Dir(path)
	if err := os.MkdirAll(parentPath, 0o700); err != nil {
		return nil, "", err
	}
	fd, err := unix.Open(parentPath, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW, 0)
	if err != nil {
		return nil, "", err
	}
	return os.NewFile(uintptr(fd), parentPath), filepath.Base(path), nil
}

func scalar(mapping *yaml.Node, key string) (string, bool, error) {
	found := false
	value := ""
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if found {
			return "", false, fmt.Errorf("duplicate %s field", key)
		}
		node := mapping.Content[i+1]
		if node.Kind != yaml.ScalarNode || node.Tag != "!!str" {
			return "", false, fmt.Errorf("%s must be a string", key)
		}
		value, found = node.Value, true
	}
	return value, found, nil
}

func setScalar(mapping *yaml.Node, key, value string) error {
	found := false
	for i := 0; i < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value != key {
			continue
		}
		if found {
			return fmt.Errorf("duplicate %s field", key)
		}
		mapping.Content[i+1] = &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value}
		found = true
	}
	if !found {
		mapping.Content = append(mapping.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
			&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
		)
	}
	return nil
}

func emptyDocument() *yaml.Node {
	return &yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{{Kind: yaml.MappingNode, Tag: "!!map"}}}
}

func parseDocument(data []byte) (*yaml.Node, error) {
	var document yaml.Node
	if err := yaml.Unmarshal(data, &document); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if len(document.Content) != 1 || document.Content[0].Kind != yaml.MappingNode {
		return nil, fmt.Errorf("config root must be a mapping")
	}
	return &document, nil
}

func randomSuffix() string {
	var value [8]byte
	if _, err := rand.Read(value[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(value[:])
}
