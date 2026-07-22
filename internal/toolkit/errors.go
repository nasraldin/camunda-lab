package toolkit

import "fmt"

// ErrorKind categorizes service-boundary failures for presentation adapters.
type ErrorKind string

const (
	ErrorInvalidRequest ErrorKind = "invalid_request"
	ErrorInput          ErrorKind = "input"
	ErrorDiscovery      ErrorKind = "discovery"
	ErrorGit            ErrorKind = "git"
	ErrorAI             ErrorKind = "ai"
	ErrorArtifact       ErrorKind = "artifact"
	ErrorScan           ErrorKind = "scan"
)

// Error is a typed application error. Err retains the domain cause.
type Error struct {
	Kind      ErrorKind
	Operation Operation
	Path      string
	Err       error
}

func (e *Error) Error() string {
	if e.Path != "" {
		return fmt.Sprintf("%s %s %s: %v", e.Operation, e.Kind, e.Path, e.Err)
	}
	return fmt.Sprintf("%s %s: %v", e.Operation, e.Kind, e.Err)
}

func (e *Error) Unwrap() error { return e.Err }

func operationError(operation Operation, kind ErrorKind, path string, err error) error {
	return &Error{Kind: kind, Operation: operation, Path: path, Err: err}
}
