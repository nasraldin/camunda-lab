package diff

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
)

const (
	maxGitErrorBytes = 4096
	maxGitFileBytes  = 16 << 20
)

// GitError is a bounded, user-safe Git input or command failure.
type GitError struct {
	Operation string
	Detail    string
	Err       error
}

func (e *GitError) Error() string {
	detail := strings.TrimSpace(e.Detail)
	if len(detail) > maxGitErrorBytes {
		detail = detail[:maxGitErrorBytes] + "…"
	}
	if detail == "" {
		return e.Operation + " failed"
	}
	return e.Operation + " failed: " + detail
}

func (e *GitError) Unwrap() error { return e.Err }

// Reader reads BPMN content from revisions relative to a project directory.
type Reader struct {
	projectDir string
}

// NewGitReader creates a document reader rooted at projectDir.
func NewGitReader(projectDir string) Reader {
	return Reader{projectDir: projectDir}
}

// Read implements the toolkit GitReader contract.
func (reader Reader) Read(ctx context.Context, ref, path string) ([]byte, error) {
	if err := validateGitRef(ref); err != nil {
		return nil, err
	}
	cleanPath, err := validateProjectBPMNPath(path)
	if err != nil {
		return nil, err
	}
	projectRoot, err := filepath.Abs(reader.projectDir)
	if err != nil {
		return nil, gitInputError("resolve project directory", err)
	}
	projectRoot, err = filepath.EvalSymlinks(projectRoot)
	if err != nil {
		return nil, gitInputError("resolve project directory", err)
	}
	repositoryRoot, err := gitRepositoryRoot(ctx, projectRoot)
	if err != nil {
		return nil, err
	}
	absolutePath := filepath.Join(projectRoot, filepath.FromSlash(cleanPath))
	repositoryPath, err := filepath.Rel(repositoryRoot, absolutePath)
	if err != nil || escapesRoot(repositoryPath) {
		if err == nil {
			err = errors.New("project path is outside the Git repository")
		}
		return nil, gitInputError("resolve project-relative Git path", err)
	}
	repositoryPath = filepath.ToSlash(repositoryPath)

	var stdout, stderr limitedBuffer
	stdout.limit = maxGitFileBytes
	stderr.limit = maxGitErrorBytes
	command := exec.CommandContext(ctx, "git", "-C", repositoryRoot, "show", "--end-of-options", ref+":"+repositoryPath)
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		detail := stderr.String()
		if errors.Is(stdout.err, errLimitReached) {
			detail = "BPMN content exceeds size limit"
		}
		return nil, &GitError{Operation: "git show", Detail: detail, Err: err}
	}
	if stdout.err != nil {
		return nil, &GitError{Operation: "git show", Detail: "BPMN content exceeds size limit", Err: stdout.err}
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func gitRepositoryRoot(ctx context.Context, projectRoot string) (string, error) {
	var stdout, stderr limitedBuffer
	stdout.limit, stderr.limit = maxGitErrorBytes, maxGitErrorBytes
	command := exec.CommandContext(ctx, "git", "-C", projectRoot, "rev-parse", "--show-toplevel")
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return "", &GitError{Operation: "locate Git repository", Detail: stderr.String(), Err: err}
	}
	root := strings.TrimSpace(stdout.String())
	if root == "" {
		return "", &GitError{Operation: "locate Git repository", Detail: "Git returned an empty repository root"}
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", gitInputError("resolve Git repository", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return "", gitInputError("resolve Git repository", err)
	}
	return root, nil
}

func validateGitRef(ref string) error {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return gitInputError("validate Git ref", errors.New("Git ref is required"))
	}
	if ref != trimmed || strings.HasPrefix(ref, "-") || strings.Contains(ref, ":") || strings.ContainsAny(ref, "\x00\r\n") {
		return gitInputError("validate Git ref", errors.New("Git ref contains unsafe characters"))
	}
	return nil
}

func validateProjectBPMNPath(path string) (string, error) {
	if path == "" || filepath.IsAbs(path) || strings.ContainsAny(path, "\x00\r\n") {
		return "", gitInputError("validate project path", errors.New("path must be a project-relative BPMN file"))
	}
	clean := filepath.Clean(path)
	if clean == "." || escapesRoot(clean) {
		return "", gitInputError("validate project path", errors.New("path escapes the project"))
	}
	if !strings.EqualFold(filepath.Ext(clean), ".bpmn") {
		return "", gitInputError("validate project path", errors.New("only .bpmn files are supported"))
	}
	return filepath.ToSlash(clean), nil
}

func escapesRoot(path string) bool {
	return path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator))
}

func gitInputError(operation string, err error) error {
	return &GitError{Operation: operation, Detail: err.Error(), Err: err}
}

var errLimitReached = errors.New("output limit reached")

type limitedBuffer struct {
	bytes.Buffer
	limit int
	err   error
}

func (buffer *limitedBuffer) Write(content []byte) (int, error) {
	if buffer.limit <= buffer.Len() {
		buffer.err = errLimitReached
		return len(content), nil
	}
	remaining := buffer.limit - buffer.Len()
	if len(content) > remaining {
		_, _ = buffer.Buffer.Write(content[:remaining])
		buffer.err = errLimitReached
		return len(content), nil
	}
	return buffer.Buffer.Write(content)
}

var _ io.Writer = (*limitedBuffer)(nil)
var _ interface {
	Read(context.Context, string, string) ([]byte, error)
} = Reader{}
