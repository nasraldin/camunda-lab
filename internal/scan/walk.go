package scan

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/sys/unix"
)

const (
	defaultMaxFileSize  = int64(1024 * 1024)
	defaultMaxLineBytes = 256 * 1024
	binaryProbeSize     = 8 * 1024
)

var builtInDirectories = map[string]bool{
	".git": true, ".hg": true, ".svn": true, ".camunda-lab": true,
	"node_modules": true, "vendor": true,
	"build": true, "dist": true, "target": true, "coverage": true,
	"generated": true, "gen": true, "out": true, "bin": true, "obj": true,
	".next": true,
}

type walkHooks struct {
	beforeOpen      func(string) error
	afterOpen       func(string) error
	afterIgnoreLine func(string, int) error
}

// Walk scans a directory tree and retains the legacy findings-only API.
func Walk(options Options) ([]Finding, error) {
	result, err := WalkWithReport(options)
	return result.Findings, err
}

// WalkWithReport recursively scans supported sources with complete accounting.
func WalkWithReport(options Options) (Result, error) {
	return WalkWithReportContext(context.Background(), options)
}

// WalkWithReportContext recursively scans supported sources and stops promptly
// when ctx is canceled.
func WalkWithReportContext(ctx context.Context, options Options) (Result, error) {
	return walkWithHooksContext(ctx, options, walkHooks{})
}

func walkWithHooks(options Options, hooks walkHooks) (Result, error) {
	return walkWithHooksContext(context.Background(), options, hooks)
}

func walkWithHooksContext(ctx context.Context, options Options, hooks walkHooks) (Result, error) {
	result := Result{Findings: []Finding{}, Issues: []Issue{}, Complete: true}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	switch options.FailOn {
	case "", "low", "medium", "high":
	default:
		return result, errors.New("fail threshold must be low, medium, or high")
	}
	root := options.Root
	if strings.TrimSpace(root) == "" {
		root = "."
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return result, fmt.Errorf("resolve scan root: %w", err)
	}
	absolute = filepath.Clean(absolute)
	if err := ctx.Err(); err != nil {
		return result, err
	}
	rootInfo, err := os.Lstat(absolute)
	if err != nil {
		return result, fmt.Errorf("inspect scan root: %w", err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return result, errors.New("scan root must not be a symbolic link")
	}
	if !rootInfo.IsDir() && !rootInfo.Mode().IsRegular() {
		return result, errors.New("scan root must be a directory or regular file")
	}

	projectRoot, err := governingProjectRootContext(ctx, absolute, rootInfo)
	if err != nil {
		return result, err
	}
	relative, err := filepath.Rel(projectRoot, absolute)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return result, errors.New("scan root escapes governing project")
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	filesystem, err := openSecureFS(projectRoot)
	if err != nil {
		return result, fmt.Errorf("secure scan root: %w", err)
	}
	defer filesystem.close()

	explicitBuiltIn := isBuiltInPath(relative) ||
		(relative == "." && builtInDirectories[strings.ToLower(filepath.Base(absolute))]) ||
		(rootInfo.Mode().IsRegular() && builtInDirectories[strings.ToLower(filepath.Base(projectRoot))])
	if explicitBuiltIn {
		result.Stats.Discovered++
		reason := "built-in excluded file"
		if rootInfo.IsDir() {
			reason = "built-in excluded subtree"
		}
		addIgnored(&result, relative, reason)
		return result, nil
	}

	state, err := rootIgnoreState(ctx, filesystem, options.Ignore)
	if err != nil {
		return result, err
	}
	maxFileSize := options.MaxFileSize
	if maxFileSize <= 0 {
		maxFileSize = defaultMaxFileSize
	}
	maxLineBytes := options.MaxLineBytes
	if maxLineBytes <= 0 {
		maxLineBytes = defaultMaxLineBytes
	}

	walker := secureWalker{
		filesystem: filesystem, result: &result, maxFileSize: maxFileSize,
		maxLineBytes: maxLineBytes, hooks: hooks, ctx: ctx,
	}
	components := relativeComponents(relative)
	parentComponents := components
	if len(components) > 0 {
		parentComponents = components[:len(components)-1]
	}
	requestedParent, state, err := openRequestedPathWithHooks(
		ctx, filesystem, parentComponents, state, hooks.afterIgnoreLine,
	)
	if err != nil {
		return result, fmt.Errorf("secure requested root: %w", err)
	}
	if rootInfo.IsDir() {
		requestedDir := requestedParent
		if len(components) > 0 {
			requestedDir, err = filesystem.openChild(requestedParent, components[len(components)-1])
			if err != nil {
				return result, fmt.Errorf("secure requested directory: %w", err)
			}
			state, err = addNestedGitignore(
				ctx, filesystem, requestedDir, relative, state, hooks.afterIgnoreLine,
			)
			if err != nil {
				return result, err
			}
		}
		err = walker.walkDirectory(requestedDir, cleanRelativeDir(relative), state)
	} else {
		err = walker.scanRegular(requestedParent, filepath.Base(absolute), relative, state)
	}
	if err != nil {
		return result, err
	}
	sort.SliceStable(result.Findings, func(left, right int) bool {
		a, b := result.Findings[left], result.Findings[right]
		if a.File != b.File {
			return a.File < b.File
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.RuleID < b.RuleID
	})
	sort.SliceStable(result.Issues, func(left, right int) bool {
		a, b := result.Issues[left], result.Issues[right]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		return a.Kind < b.Kind
	})
	return result, nil
}

type secureWalker struct {
	filesystem   *secureFS
	result       *Result
	maxFileSize  int64
	maxLineBytes int
	hooks        walkHooks
	ctx          context.Context
}

func (walker *secureWalker) walkDirectory(directory *secureDir, base string, state ignoreState) error {
	if err := walker.ctx.Err(); err != nil {
		return err
	}
	entries, err := readSecureDir(directory)
	if err != nil {
		path := base
		if path == "" {
			path = "."
		}
		walker.result.Stats.Discovered++
		addError(walker.result, path, IssueError, "walk directory", err)
		return nil
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].Name() < entries[right].Name() })
	for _, entry := range entries {
		if err := walker.ctx.Err(); err != nil {
			return err
		}
		name := entry.Name()
		if name == ".gitignore" || name == ".camunda-scanignore" {
			continue
		}
		relative := joinRelative(base, name)
		var stat unix.Stat_t
		if err := unix.Fstatat(directory.fd, name, &stat, unix.AT_SYMLINK_NOFOLLOW); err != nil {
			walker.result.Stats.Discovered++
			addError(walker.result, relative, IssueError, "inspect candidate", err)
			continue
		}
		switch stat.Mode & unix.S_IFMT {
		case unix.S_IFDIR:
			if builtInDirectories[strings.ToLower(name)] {
				walker.result.Stats.Discovered++
				addIgnored(walker.result, relative, "built-in excluded subtree")
				continue
			}
			child, err := walker.filesystem.openChild(directory, name)
			if err != nil {
				walker.result.Stats.Discovered++
				addError(walker.result, relative, IssueError, "open directory", err)
				continue
			}
			childState, err := addNestedGitignore(
				walker.ctx,
				walker.filesystem,
				child,
				relative,
				state,
				walker.hooks.afterIgnoreLine,
			)
			if err != nil {
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					walker.filesystem.release(child)
					return err
				}
				walker.result.Stats.Discovered++
				addError(walker.result, relative, IssueError, "load nested ignore", err)
				walker.filesystem.release(child)
				continue
			}
			err = walker.walkDirectory(child, relative, childState)
			walker.filesystem.release(child)
			if err != nil {
				return err
			}
		case unix.S_IFREG:
			if _, candidate := sourceKind(relative); candidate {
				if err := walker.scanRegular(directory, name, relative, state); err != nil {
					return err
				}
			}
		case unix.S_IFLNK:
			walker.result.Stats.Discovered++
			if _, candidate := sourceKind(relative); candidate {
				addError(walker.result, relative, IssueError, "inspect candidate", errors.New("symbolic link is not allowed"))
			} else {
				addIgnored(walker.result, relative, "symbolic link subtree is not traversed")
			}
		default:
			if _, candidate := sourceKind(relative); candidate {
				walker.result.Stats.Discovered++
				addIgnored(walker.result, relative, "candidate is not a regular file")
			}
		}
	}
	return nil
}

func (walker *secureWalker) scanRegular(
	parent *secureDir,
	name, relative string,
	state ignoreState,
) error {
	if err := walker.ctx.Err(); err != nil {
		return err
	}
	kind, candidate := sourceKind(relative)
	if !candidate {
		return nil
	}
	walker.result.Stats.Discovered++
	if isBuiltInPath(relative) {
		addIgnored(walker.result, relative, "built-in excluded file")
		return nil
	}
	if ignored, reason := state.match(relative); ignored {
		addIgnored(walker.result, relative, reason)
		return nil
	}
	if walker.hooks.beforeOpen != nil {
		if err := walker.hooks.beforeOpen(relative); err != nil {
			addError(walker.result, relative, IssueError, "before open", err)
			return nil
		}
	}
	if err := walker.ctx.Err(); err != nil {
		return err
	}
	opened, err := walker.filesystem.openFile(parent, name)
	if err != nil {
		addError(walker.result, relative, IssueError, "secure open", err)
		return nil
	}
	defer opened.file.Close()
	if walker.hooks.afterOpen != nil {
		if err := walker.hooks.afterOpen(relative); err != nil {
			addError(walker.result, relative, IssueError, "after open", err)
			return nil
		}
	}
	if err := walker.ctx.Err(); err != nil {
		return err
	}
	if err := walker.filesystem.verify(); err != nil {
		addError(walker.result, relative, IssueError, "verify parent", err)
		return nil
	}
	if err := verifySecureFile(parent, name, opened); err != nil {
		addError(walker.result, relative, IssueError, "verify candidate", err)
		return nil
	}
	findings, disposition, scanErr := inspectFileContext(
		walker.ctx, opened.file, relative, kind, walker.maxFileSize, walker.maxLineBytes,
	)
	if errors.Is(scanErr, context.Canceled) || errors.Is(scanErr, context.DeadlineExceeded) {
		return scanErr
	}
	if err := walker.filesystem.verify(); err != nil {
		addError(walker.result, relative, IssueError, "final parent verification", err)
		return nil
	}
	if err := verifySecureFile(parent, name, opened); err != nil {
		addError(walker.result, relative, IssueError, "final candidate verification", err)
		return nil
	}
	walker.result.Findings = append(walker.result.Findings, findings...)
	switch disposition {
	case IssueIgnored:
		addIgnored(walker.result, relative, scanErr.Error())
	case IssueTruncated:
		addError(walker.result, relative, IssueTruncated, "scan", scanErr)
	case IssueError:
		addError(walker.result, relative, IssueError, "scan", scanErr)
	default:
		walker.result.Stats.Scanned++
	}
	return nil
}

func inspectFile(
	file *os.File,
	displayPath string,
	kind SourceKind,
	maxFileSize int64,
	maxLineBytes int,
) ([]Finding, IssueKind, error) {
	return inspectFileContext(context.Background(), file, displayPath, kind, maxFileSize, maxLineBytes)
}

func inspectFileContext(
	ctx context.Context,
	file *os.File,
	displayPath string,
	kind SourceKind,
	maxFileSize int64,
	maxLineBytes int,
) ([]Finding, IssueKind, error) {
	if err := ctx.Err(); err != nil {
		return nil, IssueError, err
	}
	info, err := file.Stat()
	if err != nil {
		return nil, IssueError, err
	}
	if !info.Mode().IsRegular() {
		return nil, IssueIgnored, errors.New("not a regular file")
	}
	if info.Size() > maxFileSize {
		return nil, IssueIgnored, fmt.Errorf("large file (%d bytes exceeds %d)", info.Size(), maxFileSize)
	}
	probe := make([]byte, binaryProbeSize)
	count, readErr := file.Read(probe)
	if readErr != nil && !errors.Is(readErr, io.EOF) {
		return nil, IssueError, readErr
	}
	if strings.IndexByte(string(probe[:count]), 0) >= 0 {
		return nil, IssueIgnored, errors.New("binary file")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, IssueError, err
	}

	findings := []Finding{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 0, min(maxLineBytes, 64*1024)), maxLineBytes)
	line := 0
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return findings, IssueError, err
		}
		line++
		text := scanner.Text()
		scanText, directive := inlineSuppression(kind, text)
		lineFindings := matchLine(displayPath, kind, line, scanText)
		if directive && len(lineFindings) > 0 {
			continue
		}
		if directive {
			lineFindings = matchLine(displayPath, kind, line, text)
		}
		findings = append(findings, lineFindings...)
	}
	if err := scanner.Err(); err != nil {
		return findings, IssueTruncated, fmt.Errorf("line exceeds %d-byte scan limit: %w", maxLineBytes, err)
	}
	if err := ctx.Err(); err != nil {
		return findings, IssueError, err
	}
	return findings, "", nil
}

func governingProjectRoot(absolute string, info os.FileInfo) (string, error) {
	return governingProjectRootContext(context.Background(), absolute, info)
}

func governingProjectRootContext(ctx context.Context, absolute string, info os.FileInfo) (string, error) {
	start := absolute
	if !info.IsDir() {
		start = filepath.Dir(absolute)
	}
	for current := start; ; current = filepath.Dir(current) {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		marker := filepath.Join(current, ".camunda.yaml")
		markerInfo, err := os.Lstat(marker)
		switch {
		case err == nil && markerInfo.Mode()&os.ModeSymlink != 0:
			return "", errors.New("governing project marker must not be a symbolic link")
		case err == nil && !markerInfo.Mode().IsRegular():
			return "", errors.New("governing project marker must be a regular file")
		case err == nil:
			return current, nil
		case !os.IsNotExist(err):
			return "", fmt.Errorf("inspect governing project: %w", err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return start, nil
		}
	}
}

func rootIgnoreState(ctx context.Context, filesystem *secureFS, user []string) (ignoreState, error) {
	git, err := readIgnoreAt(ctx, filesystem, filesystem.root, ".gitignore", ".gitignore", "", nil)
	if err != nil {
		return ignoreState{}, err
	}
	camunda, err := readIgnoreAt(
		ctx, filesystem, filesystem.root, ".camunda-scanignore", ".camunda-scanignore", "", nil,
	)
	if err != nil {
		return ignoreState{}, err
	}
	userRules, err := parseIgnoreLinesContext(ctx, user, "user", "")
	if err != nil {
		return ignoreState{}, err
	}
	return ignoreState{git: git, camunda: camunda, user: userRules}, nil
}

func openRequestedPath(
	filesystem *secureFS,
	components []string,
	state ignoreState,
) (*secureDir, ignoreState, error) {
	return openRequestedPathContext(context.Background(), filesystem, components, state)
}

func openRequestedPathContext(
	ctx context.Context,
	filesystem *secureFS,
	components []string,
	state ignoreState,
) (*secureDir, ignoreState, error) {
	return openRequestedPathWithHooks(ctx, filesystem, components, state, nil)
}

func openRequestedPathWithHooks(
	ctx context.Context,
	filesystem *secureFS,
	components []string,
	state ignoreState,
	afterLine func(string, int) error,
) (*secureDir, ignoreState, error) {
	current := filesystem.root
	var base string
	for _, component := range components {
		if err := ctx.Err(); err != nil {
			return nil, state, err
		}
		child, err := filesystem.openChild(current, component)
		if err != nil {
			return nil, state, err
		}
		base = joinRelative(base, component)
		state, err = addNestedGitignore(ctx, filesystem, child, base, state, afterLine)
		if err != nil {
			return nil, state, err
		}
		current = child
	}
	return current, state, nil
}

func addNestedGitignore(
	ctx context.Context,
	filesystem *secureFS,
	directory *secureDir,
	base string,
	state ignoreState,
	afterLine func(string, int) error,
) (ignoreState, error) {
	source := joinRelative(base, ".gitignore")
	rules, err := readIgnoreAt(ctx, filesystem, directory, ".gitignore", source, base, afterLine)
	if err != nil {
		return state, err
	}
	state.git = append(append(ignoreRules(nil), state.git...), rules...)
	return state, nil
}

func relativeComponents(relative string) []string {
	if relative == "." || relative == "" {
		return nil
	}
	return strings.Split(relative, "/")
}

func cleanRelativeDir(relative string) string {
	if relative == "." {
		return ""
	}
	return relative
}

func joinRelative(base, name string) string {
	if base == "" || base == "." {
		return name
	}
	return base + "/" + name
}

func sourceKind(relative string) (SourceKind, bool) {
	base := strings.ToLower(filepath.Base(relative))
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return SourceEnv, true
	}
	switch strings.ToLower(filepath.Ext(base)) {
	case ".bpmn":
		return SourceBPMN, true
	case ".dmn":
		return SourceDMN, true
	case ".form":
		return SourceForm, true
	case ".yaml", ".yml":
		return SourceYAML, true
	case ".json":
		return SourceJSON, true
	case ".env":
		return SourceEnv, true
	case ".sh":
		return SourceShell, true
	case ".js", ".mjs", ".cjs":
		return SourceJavaScript, true
	case ".ts", ".mts", ".cts":
		return SourceTypeScript, true
	case ".java":
		return SourceJava, true
	case ".go":
		return SourceGo, true
	case ".properties":
		return SourceProperties, true
	case ".txt":
		return SourceText, true
	default:
		return "", false
	}
}

func isBuiltInPath(relative string) bool {
	for _, component := range strings.Split(relative, "/") {
		if builtInDirectories[strings.ToLower(component)] {
			return true
		}
	}
	return false
}

func addIgnored(result *Result, path, reason string) {
	result.Stats.Ignored++
	result.Issues = append(result.Issues, Issue{
		Path: path, Kind: IssueIgnored, Reason: reason,
	})
}

func addError(result *Result, path string, kind IssueKind, action string, err error) {
	result.Stats.Errored++
	result.Complete = false
	message := action + ": " + safeError(err)
	result.Issues = append(result.Issues, Issue{
		Path: path, Kind: kind, Message: message, Err: err,
	})
}

func safeError(err error) string {
	var pathError *os.PathError
	if errors.As(err, &pathError) {
		return pathError.Op + ": " + pathError.Err.Error()
	}
	return err.Error()
}
