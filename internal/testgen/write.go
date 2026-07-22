package testgen

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const artifactMode fs.FileMode = 0o600

// RecoveryError reports backups retained after a rollback restoration failed.
type RecoveryError struct {
	Err   error
	Paths []string
}

func (e *RecoveryError) Error() string {
	return fmt.Sprintf("%v; original artifact backups preserved at %s", e.Err, strings.Join(e.Paths, ", "))
}

func (e *RecoveryError) Unwrap() error { return e.Err }

type writerHooks struct {
	afterPrepare func() error
	afterPublish func() error
	fail         func(operation string, index int) error
}

type anchoredDir struct {
	fd      int
	parent  *anchoredDir
	name    string
	dev     uint64
	ino     uint64
	created bool
	display string
}

type secureTree struct {
	root      *anchoredDir
	opened    []*anchoredDir
	created   []*anchoredDir
	absRoot   string
	stage     *anchoredDir
	stageName string
	keepStage bool
}

type publishItem struct {
	artifact     Artifact
	parent       *anchoredDir
	base         string
	staged       string
	backup       string
	existed      bool
	originalDev  uint64
	originalIno  uint64
	backupMoved  bool
	published    bool
	publishedDev uint64
	publishedIno uint64
}

// Write atomically publishes a complete artifact set below root.
func Write(root string, artifacts []Artifact, force bool) ([]string, error) {
	return writeWithHooks(root, artifacts, force, writerHooks{})
}

func writeWithHooks(root string, artifacts []Artifact, force bool, hooks writerHooks) ([]string, error) {
	if strings.TrimSpace(root) == "" {
		return nil, errors.New("write artifacts: root is required")
	}
	if len(artifacts) == 0 {
		return nil, errors.New("write artifacts: no artifacts")
	}
	if err := validateArtifactSet(artifacts); err != nil {
		return nil, err
	}
	tree, err := openSecureTree(root)
	if err != nil {
		return nil, err
	}
	defer tree.close()

	items, err := tree.prepare(artifacts, force)
	if err != nil {
		return nil, tree.abort(items, err, hooks)
	}
	if hooks.afterPrepare != nil {
		if err := hooks.afterPrepare(); err != nil {
			return nil, tree.abort(items, fmt.Errorf("write artifacts: prepare hook: %w", err), hooks)
		}
	}
	for index := range items {
		if err := tree.verify(); err != nil {
			return nil, tree.abort(items, err, hooks)
		}
		item := &items[index]
		if err := verifyDestination(item); err != nil {
			return nil, tree.abort(items, err, hooks)
		}
		if item.existed {
			if err := runFailureHook(hooks, "backup", index); err != nil {
				return nil, tree.abort(items, err, hooks)
			}
			if err := unix.Renameat(item.parent.fd, item.base, tree.stage.fd, item.backup); err != nil {
				return nil, tree.abort(items, fmt.Errorf("backup %q: %w", item.artifact.Path, err), hooks)
			}
			item.backupMoved = true
		}
		if err := runFailureHook(hooks, "publish", index); err != nil {
			return nil, tree.abort(items, err, hooks)
		}
		if err := unix.Renameat(tree.stage.fd, item.staged, item.parent.fd, item.base); err != nil {
			return nil, tree.abort(items, fmt.Errorf("publish %q: %w", item.artifact.Path, err), hooks)
		}
		item.published = true
	}
	if hooks.afterPublish != nil {
		if err := hooks.afterPublish(); err != nil {
			return nil, tree.abort(items, fmt.Errorf("write artifacts: final publish hook: %w", err), hooks)
		}
	}
	if err := tree.verifyPublished(items); err != nil {
		return nil, tree.abort(items, err, hooks)
	}
	var retained []*publishItem
	var cleanupErr error
	for index := range items {
		if !items[index].backupMoved {
			continue
		}
		if err := unix.Unlinkat(tree.stage.fd, items[index].backup, 0); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("remove committed backup %q: %w", items[index].artifact.Path, err))
			retained = append(retained, &items[index])
			continue
		}
		items[index].backupMoved = false
	}
	if len(retained) > 0 {
		paths, preserveErr := tree.relocateRecovery(retained)
		cleanupErr = errors.Join(cleanupErr, preserveErr)
		return nil, &RecoveryError{Err: cleanupErr, Paths: paths}
	}

	paths := make([]string, len(items))
	for index, item := range items {
		paths[index] = filepath.Join(tree.absRoot, filepath.FromSlash(item.artifact.Path))
	}
	return paths, nil
}

func openSecureTree(root string) (*secureTree, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("write artifacts: resolve root: %w", err)
	}
	absolute = filepath.Clean(absolute)
	absolute = canonicalDarwinSystemAlias(absolute)
	if filepath.VolumeName(absolute) != "" && filepath.VolumeName(absolute) != "/" {
		return nil, fmt.Errorf("write artifacts: unsupported root volume %q", filepath.VolumeName(absolute))
	}
	fd, err := unix.Open(string(filepath.Separator), unix.O_RDONLY|unix.O_DIRECTORY|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, fmt.Errorf("write artifacts: open filesystem anchor: %w", err)
	}
	tree := &secureTree{absRoot: absolute}
	anchor, err := newAnchor(fd, nil, "", false, string(filepath.Separator))
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	tree.opened = append(tree.opened, anchor)
	current := anchor
	for _, component := range splitPath(absolute) {
		current, err = tree.openChild(current, component, true)
		if err != nil {
			tree.close()
			return nil, fmt.Errorf("write artifacts: open root component %q without symlinks: %w", component, err)
		}
	}
	tree.root = current
	return tree, nil
}

func canonicalDarwinSystemAlias(path string) string {
	if runtime.GOOS != "darwin" {
		return path
	}
	for _, alias := range []string{"/etc", "/tmp", "/var"} {
		if path == alias || strings.HasPrefix(path, alias+"/") {
			return "/private" + path
		}
	}
	return path
}

func (tree *secureTree) prepare(artifacts []Artifact, force bool) ([]publishItem, error) {
	stageName, err := randomName(".camunda-lab-stage-")
	if err != nil {
		return nil, err
	}
	stage, err := tree.openChild(tree.root, stageName, true)
	if err != nil {
		return nil, fmt.Errorf("write artifacts: create staging directory: %w", err)
	}
	tree.stage, tree.stageName = stage, stageName

	sorted := append([]Artifact(nil), artifacts...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Path < sorted[j].Path })
	items := make([]publishItem, 0, len(sorted))
	for index, artifact := range sorted {
		parts := strings.Split(filepath.ToSlash(filepath.Clean(filepath.FromSlash(artifact.Path))), "/")
		parent := tree.root
		for _, component := range parts[:len(parts)-1] {
			parent, err = tree.openChild(parent, component, true)
			if err != nil {
				return items, fmt.Errorf("write artifacts: open parent for %q: %w", artifact.Path, err)
			}
		}
		item := publishItem{
			artifact: artifact,
			parent:   parent,
			base:     parts[len(parts)-1],
			staged:   fmt.Sprintf("artifact-%06d", index),
			backup:   fmt.Sprintf("backup-%06d", index),
		}
		var stat unix.Stat_t
		err = unix.Fstatat(parent.fd, item.base, &stat, unix.AT_SYMLINK_NOFOLLOW)
		switch {
		case err == nil && stat.Mode&unix.S_IFMT == unix.S_IFLNK:
			return items, fmt.Errorf("write artifacts: destination %q is a symlink", artifact.Path)
		case err == nil && stat.Mode&unix.S_IFMT != unix.S_IFREG:
			return items, fmt.Errorf("write artifacts: destination %q is not a regular file", artifact.Path)
		case err == nil && !force:
			return items, fmt.Errorf("write artifacts: destination %q exists (use force to replace)", artifact.Path)
		case err == nil:
			item.existed = true
			item.originalDev, item.originalIno = uint64(stat.Dev), stat.Ino
		case errors.Is(err, unix.ENOENT):
		default:
			return items, fmt.Errorf("write artifacts: inspect %q: %w", artifact.Path, err)
		}
		if err := writeFileAt(stage.fd, item.staged, artifact.Content); err != nil {
			return items, fmt.Errorf("write artifacts: stage %q: %w", artifact.Path, err)
		}
		var stagedStat unix.Stat_t
		if err := unix.Fstatat(stage.fd, item.staged, &stagedStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return items, fmt.Errorf("write artifacts: inspect staged %q: %w", artifact.Path, err)
		}
		item.publishedDev, item.publishedIno = uint64(stagedStat.Dev), stagedStat.Ino
		items = append(items, item)
	}
	return items, nil
}

func (tree *secureTree) openChild(parent *anchoredDir, name string, create bool) (*anchoredDir, error) {
	var stat unix.Stat_t
	err := unix.Fstatat(parent.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW)
	created := false
	if errors.Is(err, unix.ENOENT) && create {
		if err := unix.Mkdirat(parent.fd, name, 0o700); err != nil {
			return nil, err
		}
		created = true
	} else if err != nil {
		return nil, err
	} else if stat.Mode&unix.S_IFMT == unix.S_IFLNK {
		return nil, errors.New("symbolic link is not allowed")
	} else if stat.Mode&unix.S_IFMT != unix.S_IFDIR {
		return nil, errors.New("component is not a directory")
	}
	fd, err := unix.Openat(parent.fd, name, unix.O_RDONLY|unix.O_DIRECTORY|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		if created {
			_ = unix.Unlinkat(parent.fd, name, unix.AT_REMOVEDIR)
		}
		return nil, err
	}
	display := filepath.Join(parent.display, name)
	child, err := newAnchor(fd, parent, name, created, display)
	if err != nil {
		_ = unix.Close(fd)
		return nil, err
	}
	tree.opened = append(tree.opened, child)
	if created {
		tree.created = append(tree.created, child)
	}
	return child, nil
}

func newAnchor(fd int, parent *anchoredDir, name string, created bool, display string) (*anchoredDir, error) {
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, err
	}
	return &anchoredDir{
		fd: fd, parent: parent, name: name, dev: uint64(stat.Dev), ino: stat.Ino,
		created: created, display: display,
	}, nil
}

func (tree *secureTree) verify() error {
	for _, dir := range tree.opened {
		if dir.parent == nil {
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(dir.parent.fd, dir.name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("write artifacts: anchored directory %q changed: %w", dir.display, err)
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFDIR || uint64(stat.Dev) != dir.dev || stat.Ino != dir.ino {
			return fmt.Errorf("write artifacts: anchored directory %q was replaced", dir.display)
		}
	}
	return nil
}

func (tree *secureTree) verifyPublished(items []publishItem) error {
	if err := tree.verify(); err != nil {
		return fmt.Errorf("write artifacts: final anchor verification: %w", err)
	}
	for index := range items {
		item := &items[index]
		if !item.published {
			continue
		}
		var stat unix.Stat_t
		if err := unix.Fstatat(item.parent.fd, item.base, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return fmt.Errorf("write artifacts: final destination %q changed: %w", item.artifact.Path, err)
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFREG ||
			uint64(stat.Dev) != item.publishedDev || stat.Ino != item.publishedIno {
			return fmt.Errorf("write artifacts: final destination %q was replaced", item.artifact.Path)
		}
	}
	return nil
}

func verifyDestination(item *publishItem) error {
	var stat unix.Stat_t
	err := unix.Fstatat(item.parent.fd, item.base, &stat, unix.AT_SYMLINK_NOFOLLOW)
	if item.existed {
		if err != nil {
			return fmt.Errorf("write artifacts: destination %q changed: %w", item.artifact.Path, err)
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFREG ||
			uint64(stat.Dev) != item.originalDev || stat.Ino != item.originalIno {
			return fmt.Errorf("write artifacts: destination %q was replaced", item.artifact.Path)
		}
		return nil
	}
	if err == nil {
		return fmt.Errorf("write artifacts: destination %q appeared during publication", item.artifact.Path)
	}
	if !errors.Is(err, unix.ENOENT) {
		return fmt.Errorf("write artifacts: inspect destination %q: %w", item.artifact.Path, err)
	}
	return nil
}

func (tree *secureTree) abort(items []publishItem, cause error, hooks writerHooks) error {
	joined := cause
	var recoveryItems []*publishItem
	for index := len(items) - 1; index >= 0; index-- {
		item := &items[index]
		if item.published {
			if err := runFailureHook(hooks, "remove", index); err == nil {
				err = unix.Unlinkat(item.parent.fd, item.base, 0)
				if err != nil && !errors.Is(err, unix.ENOENT) {
					joined = errors.Join(joined, fmt.Errorf("rollback remove %q: %w", item.artifact.Path, err))
				}
			} else {
				joined = errors.Join(joined, err)
			}
		}
		if item.backupMoved {
			err := runFailureHook(hooks, "restore", index)
			if err == nil {
				err = unix.Renameat(tree.stage.fd, item.backup, item.parent.fd, item.base)
			}
			if err != nil {
				joined = errors.Join(joined, fmt.Errorf("rollback restore %q: %w", item.artifact.Path, err))
				recoveryItems = append(recoveryItems, item)
			} else {
				item.backupMoved = false
			}
		}
	}
	if len(recoveryItems) > 0 {
		tree.cleanStage(items, recoveryItems)
		paths, preserveErr := tree.relocateRecovery(recoveryItems)
		if preserveErr != nil {
			joined = errors.Join(joined, preserveErr)
		}
		return &RecoveryError{Err: joined, Paths: paths}
	}
	tree.cleanStage(items, nil)
	return joined
}

func (tree *secureTree) cleanStage(items []publishItem, preserved []*publishItem) {
	keep := make(map[string]struct{}, len(preserved))
	for _, item := range preserved {
		keep[item.backup] = struct{}{}
	}
	for _, item := range items {
		_ = unix.Unlinkat(tree.stage.fd, item.staged, 0)
		if _, retained := keep[item.backup]; !retained {
			_ = unix.Unlinkat(tree.stage.fd, item.backup, 0)
		}
	}
}

func (tree *secureTree) relocateRecovery(items []*publishItem) ([]string, error) {
	anchor, anchorErr := tree.deepestVerifiedAnchor()
	if anchorErr != nil {
		tree.keepStage = true
		return nil, fmt.Errorf("find verified recovery anchor: %w", anchorErr)
	}
	recoveryName, err := randomName(".camunda-lab-recovery-")
	if err != nil {
		tree.keepStage = true
		return nil, err
	}
	recovery, err := tree.openChild(anchor, recoveryName, true)
	if err != nil {
		tree.keepStage = true
		return nil, fmt.Errorf("create recovery directory: %w", err)
	}
	var joined error
	var paths []string
	for index, item := range items {
		name := fmt.Sprintf("artifact-%06d.backup", index)
		if err := moveOrCopyBackup(tree.stage.fd, item.backup, recovery.fd, name); err != nil {
			joined = errors.Join(joined, fmt.Errorf("relocate backup %q: %w", item.artifact.Path, err))
			tree.keepStage = true
			continue
		}
		item.backupMoved = false
		if err := verifyRecoveryFile(recovery, name); err != nil {
			joined = errors.Join(joined, fmt.Errorf("verify relocated backup %q: %w", item.artifact.Path, err))
			continue
		}
		if err := verifyLineage(recovery); err != nil {
			joined = errors.Join(joined, fmt.Errorf("verify recovery path for %q: %w", item.artifact.Path, err))
			continue
		}
		paths = append(paths, filepath.Join(anchor.display, recoveryName, name))
	}
	sort.Strings(paths)
	return paths, joined
}

func (tree *secureTree) deepestVerifiedAnchor() (*anchoredDir, error) {
	var joined error
	for candidate := tree.root; candidate != nil; candidate = candidate.parent {
		if err := verifyLineage(candidate); err == nil {
			return candidate, nil
		} else {
			joined = errors.Join(joined, err)
		}
	}
	return nil, joined
}

func verifyLineage(dir *anchoredDir) error {
	var lineage []*anchoredDir
	for current := dir; current != nil; current = current.parent {
		lineage = append(lineage, current)
	}
	for index := len(lineage) - 2; index >= 0; index-- {
		child := lineage[index]
		var stat unix.Stat_t
		if err := unix.Fstatat(child.parent.fd, child.name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			return err
		}
		if stat.Mode&unix.S_IFMT != unix.S_IFDIR ||
			uint64(stat.Dev) != child.dev || stat.Ino != child.ino {
			return errors.New("directory identity changed")
		}
	}
	return nil
}

func moveOrCopyBackup(sourceFD int, source string, destinationFD int, destination string) error {
	if err := unix.Renameat(sourceFD, source, destinationFD, destination); err == nil {
		return nil
	} else if !errors.Is(err, unix.EXDEV) {
		return err
	}
	sourceFile, err := openFileAt(sourceFD, source, unix.O_RDONLY)
	if err != nil {
		return err
	}
	defer sourceFile.Close()
	destinationFile, err := createFileAt(destinationFD, destination)
	if err != nil {
		return err
	}
	copyOK := false
	defer func() {
		_ = destinationFile.Close()
		if !copyOK {
			_ = unix.Unlinkat(destinationFD, destination, 0)
		}
	}()
	if _, err := io.Copy(destinationFile, sourceFile); err != nil {
		return err
	}
	if err := destinationFile.Sync(); err != nil {
		return err
	}
	if err := destinationFile.Close(); err != nil {
		return err
	}
	copyOK = true
	if err := unix.Unlinkat(sourceFD, source, 0); err != nil {
		return fmt.Errorf("remove copied source backup: %w", err)
	}
	return nil
}

func verifyRecoveryFile(directory *anchoredDir, name string) error {
	var pathStat unix.Stat_t
	if err := unix.Fstatat(directory.fd, name, &pathStat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
		return err
	}
	if pathStat.Mode&unix.S_IFMT != unix.S_IFREG {
		return errors.New("recovery entry is not a regular file")
	}
	file, err := openFileAt(directory.fd, name, unix.O_RDONLY)
	if err != nil {
		return err
	}
	defer file.Close()
	var openStat unix.Stat_t
	if err := unix.Fstat(int(file.Fd()), &openStat); err != nil {
		return err
	}
	if uint64(pathStat.Dev) != uint64(openStat.Dev) || pathStat.Ino != openStat.Ino {
		return errors.New("recovery file identity changed")
	}
	return nil
}

func (tree *secureTree) close() {
	if tree.stage != nil && !tree.keepStage {
		_ = unix.Unlinkat(tree.root.fd, tree.stageName, unix.AT_REMOVEDIR)
	}
	for index := len(tree.created) - 1; index >= 0; index-- {
		dir := tree.created[index]
		if dir == tree.stage || dir.parent == nil {
			continue
		}
		_ = unix.Unlinkat(dir.parent.fd, dir.name, unix.AT_REMOVEDIR)
	}
	for index := len(tree.opened) - 1; index >= 0; index-- {
		_ = unix.Close(tree.opened[index].fd)
	}
}

func writeFileAt(dirfd int, name string, content []byte) error {
	file, err := createFileAt(dirfd, name)
	if err != nil {
		return err
	}
	complete := false
	defer func() {
		if !complete {
			_ = unix.Unlinkat(dirfd, name, 0)
		}
	}()
	if _, err := file.Write(content); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	complete = true
	return nil
}

func createFileAt(dirfd int, name string) (*os.File, error) {
	fd, err := unix.Openat(dirfd, name, unix.O_WRONLY|unix.O_CREAT|unix.O_EXCL|unix.O_NOFOLLOW|unix.O_CLOEXEC, uint32(artifactMode))
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("create file handle")
	}
	return file, nil
}

func openFileAt(dirfd int, name string, flags int) (*os.File, error) {
	fd, err := unix.Openat(dirfd, name, flags|unix.O_NOFOLLOW|unix.O_CLOEXEC, 0)
	if err != nil {
		return nil, err
	}
	file := os.NewFile(uintptr(fd), name)
	if file == nil {
		_ = unix.Close(fd)
		return nil, errors.New("open file handle")
	}
	return file, nil
}

func runFailureHook(hooks writerHooks, operation string, index int) error {
	if hooks.fail == nil {
		return nil
	}
	if err := hooks.fail(operation, index); err != nil {
		return fmt.Errorf("%s artifact %d: %w", operation, index, err)
	}
	return nil
}

func randomName(prefix string) (string, error) {
	var value [12]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return prefix + hex.EncodeToString(value[:]), nil
}

func splitPath(path string) []string {
	path = strings.TrimPrefix(filepath.Clean(path), string(filepath.Separator))
	if path == "" || path == "." {
		return nil
	}
	return strings.Split(path, string(filepath.Separator))
}

func validateArtifactSet(artifacts []Artifact) error {
	seen := make(map[string]string, len(artifacts))
	for _, artifact := range artifacts {
		if err := validateRelativePath(artifact.Path); err != nil {
			return err
		}
		key := strings.ToLower(filepath.Clean(filepath.FromSlash(artifact.Path)))
		if previous, exists := seen[key]; exists {
			return fmt.Errorf("write artifacts: path %q collides with %q", artifact.Path, previous)
		}
		seen[key] = artifact.Path
	}
	return nil
}

func validateRelativePath(path string) error {
	if strings.TrimSpace(path) == "" || filepath.IsAbs(path) {
		return fmt.Errorf("artifact path %q must be relative", path)
	}
	if strings.ContainsRune(path, '\\') || strings.ContainsRune(path, 0) {
		return fmt.Errorf("artifact path %q is not a portable slash-separated path", path)
	}
	clean := filepath.Clean(filepath.FromSlash(path))
	if clean == "." || clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return fmt.Errorf("artifact path %q escapes the output root", path)
	}
	return nil
}
