package drift

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultMaxGitOutput = 16 << 20

var (
	safeGitRefPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	commitPattern     = regexp.MustCompile(`^[0-9a-fA-F]{40}([0-9a-fA-F]{24})?$`)
)

type GitRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type GitInputRunner interface {
	GitRunner
	RunInput(context.Context, string, []byte, ...string) ([]byte, error)
}

type ExecGitRunner struct {
	MaxOutputBytes int
}

func (r ExecGitRunner) Run(ctx context.Context, repository string, args ...string) ([]byte, error) {
	return r.run(ctx, repository, nil, args...)
}

func (r ExecGitRunner) RunInput(ctx context.Context, repository string, input []byte, args ...string) ([]byte, error) {
	return r.run(ctx, repository, input, args...)
}

func (r ExecGitRunner) run(ctx context.Context, repository string, input []byte, args ...string) ([]byte, error) {
	limit := r.MaxOutputBytes
	if limit == 0 {
		limit = defaultMaxGitOutput
	}
	if limit < 1 {
		return nil, errors.New("Git output limit must be positive")
	}
	if len(input) > limit {
		return nil, errors.New("Git input exceeded the configured size limit")
	}
	command := exec.CommandContext(ctx, "git", append([]string{"-C", repository}, args...)...)
	command.Env = appendWithoutGitOptionalLocks(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	if input != nil {
		command.Stdin = bytes.NewReader(input)
	}
	var stdout, stderr boundedBuffer
	stdout.limit, stderr.limit = limit, limit
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}
		if stdout.exceeded || stderr.exceeded {
			return nil, errors.New("Git output exceeded the configured size limit")
		}
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = "Git command failed"
		}
		return nil, fmt.Errorf("%s: %w", message, err)
	}
	if stdout.exceeded {
		return nil, errors.New("Git output exceeded the configured size limit")
	}
	return stdout.Bytes(), nil
}

func runSafeGit(ctx context.Context, runner GitRunner, repository string, args ...string) ([]byte, error) {
	safeArgs := make([]string, 0, len(args)+2)
	safeArgs = append(safeArgs, "--no-optional-locks", "--literal-pathspecs")
	safeArgs = append(safeArgs, args...)
	return runner.Run(ctx, repository, safeArgs...)
}

func runSafeGitInput(
	ctx context.Context,
	runner GitRunner,
	repository string,
	input []byte,
	args ...string,
) ([]byte, error) {
	inputRunner, ok := runner.(GitInputRunner)
	if !ok {
		return nil, errors.New("Git runner does not support bounded standard input")
	}
	safeArgs := make([]string, 0, len(args)+2)
	safeArgs = append(safeArgs, "--no-optional-locks", "--literal-pathspecs")
	safeArgs = append(safeArgs, args...)
	return inputRunner.RunInput(ctx, repository, input, safeArgs...)
}

func appendWithoutGitOptionalLocks(environment []string, value string) []string {
	result := make([]string, 0, len(environment)+1)
	for _, item := range environment {
		if !strings.HasPrefix(item, "GIT_OPTIONAL_LOCKS=") {
			result = append(result, item)
		}
	}
	return append(result, value)
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(value []byte) (int, error) {
	if b.Buffer.Len()+len(value) > b.limit {
		remaining := b.limit - b.Buffer.Len()
		if remaining > 0 {
			_, _ = b.Buffer.Write(value[:remaining])
		}
		b.exceeded = true
		return len(value), errors.New("output limit exceeded")
	}
	return b.Buffer.Write(value)
}

func validateGitRef(ref string) error {
	if ref == "" {
		return errors.New("Git baseline ref is required")
	}
	if ref != strings.TrimSpace(ref) || !safeGitRefPattern.MatchString(ref) ||
		strings.Contains(ref, "..") || strings.Contains(ref, "//") ||
		strings.Contains(ref, "@{") || strings.HasSuffix(ref, "/") ||
		filepath.IsAbs(ref) {
		return fmt.Errorf("unsafe Git baseline ref %q", ref)
	}
	return nil
}

func resolveRepository(ctx context.Context, runner GitRunner, projectRoot string) (string, error) {
	output, err := runSafeGit(ctx, runner, projectRoot, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", fmt.Errorf("resolve Git repository: %w", err)
	}
	repository := strings.TrimSpace(string(output))
	if repository == "" || !filepath.IsAbs(repository) {
		return "", errors.New("Git returned an invalid repository root")
	}
	repository, err = filepath.EvalSymlinks(filepath.Clean(repository))
	if err != nil {
		return "", fmt.Errorf("canonicalize Git repository root: %w", err)
	}
	project, err := filepath.EvalSymlinks(filepath.Clean(projectRoot))
	if err != nil {
		return "", fmt.Errorf("canonicalize project root: %w", err)
	}
	relative, err := filepath.Rel(repository, project)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("project root escapes the resolved Git repository")
	}
	return repository, nil
}

func resolveCommit(ctx context.Context, runner GitRunner, repository, ref string) (string, error) {
	output, err := runSafeGit(ctx, runner, repository, "rev-parse", "--verify", "--end-of-options", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("resolve Git baseline ref %q: %w", ref, err)
	}
	commit := strings.TrimSpace(string(output))
	if !commitPattern.MatchString(commit) {
		return "", errors.New("Git baseline did not resolve to a full commit ID")
	}
	return strings.ToLower(commit), nil
}

func splitNUL(output []byte) ([]string, error) {
	if len(output) == 0 {
		return []string{}, nil
	}
	if output[len(output)-1] != 0 {
		return nil, errors.New("Git returned an unterminated path record")
	}
	parts := bytes.Split(output[:len(output)-1], []byte{0})
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if bytes.IndexByte(part, '\n') >= 0 || bytes.IndexByte(part, '\r') >= 0 {
			return nil, errors.New("Git returned an unsafe path record")
		}
		result = append(result, string(part))
	}
	return result, nil
}

type treeEntry struct {
	Mode   string
	Type   string
	Object string
	Path   string
}

func parseTreeEntries(output []byte) ([]treeEntry, error) {
	records, err := splitNUL(output)
	if err != nil {
		return nil, err
	}
	entries := make([]treeEntry, 0, len(records))
	for _, record := range records {
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			return nil, errors.New("Git tree record is missing its path separator")
		}
		fields := strings.Fields(record[:tab])
		if len(fields) != 3 || !validTreeMode(fields[0]) ||
			(fields[1] != "blob" && fields[1] != "tree" && fields[1] != "commit") ||
			!validTreeModeType(fields[0], fields[1]) || !commitPattern.MatchString(fields[2]) {
			return nil, errors.New("Git returned an invalid tree record")
		}
		path := record[tab+1:]
		if path == "" || strings.HasPrefix(path, "/") || strings.Contains(path, `\`) ||
			strings.Contains(path, "\n") || strings.Contains(path, "\r") {
			return nil, errors.New("Git returned an unsafe tree path")
		}
		entries = append(entries, treeEntry{
			Mode: fields[0], Type: fields[1], Object: strings.ToLower(fields[2]), Path: path,
		})
	}
	return entries, nil
}

func validTreeModeType(mode, objectType string) bool {
	switch mode {
	case "040000":
		return objectType == "tree"
	case "100644", "100755", "120000":
		return objectType == "blob"
	case "160000":
		return objectType == "commit"
	default:
		return false
	}
}

func validTreeMode(value string) bool {
	if len(value) != 6 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '7' {
			return false
		}
	}
	return true
}
