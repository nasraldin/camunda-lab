package laberrors

import (
	"errors"
	"fmt"
	"strings"
)

// UserError is a plain-language error for CLI and UI consumers.
type UserError struct {
	Message     string
	Hint        string
	Code        string
	Recoverable bool
}

func (e *UserError) Error() string { return e.Message }

// SafeCodedError exposes a stable machine code and a detail-free message that
// may be returned by API boundaries. Implementations must not include secrets
// or internal diagnostics in SafeMessage.
type SafeCodedError interface {
	error
	SafeCode() string
	SafeMessage() string
}

// AsSafeCoded returns a safe coded error through arbitrary wrapping.
func AsSafeCoded(err error) (SafeCodedError, bool) {
	var coded SafeCodedError
	if errors.As(err, &coded) {
		return coded, true
	}
	return nil, false
}

// AsUser returns a UserError if err is or wraps one.
func AsUser(err error) (*UserError, bool) {
	var u *UserError
	if errors.As(err, &u) {
		return u, true
	}
	return nil, false
}

// ContainerConflict builds a recoverable error for Docker name collisions.
func ContainerConflict(names []string) error {
	unique := dedupe(names)
	label := strings.Join(unique, ", ")
	return &UserError{
		Message: fmt.Sprintf(
			"A previous Camunda setup left containers behind (%s), so Docker could not start the lab.",
			label,
		),
		Hint:        "Use “Clean up and try again” — Camunda Lab will remove leftover containers from an incomplete install, then retry.",
		Code:        "container_conflict",
		Recoverable: true,
	}
}

// ForeignContainerConflict is when another app owns the conflicting container.
func ForeignContainerConflict(names []string) error {
	return &UserError{
		Message: fmt.Sprintf(
			"Another application on your computer is already using the Docker name “%s”.",
			strings.Join(dedupe(names), ", "),
		),
		Hint:        "Stop that other app or Docker stack, then try again. If unsure, restart Docker Desktop and retry.",
		Code:        "container_conflict_foreign",
		Recoverable: false,
	}
}

// Wrap converts common Docker errors into user-friendly messages.
func Wrap(err error) error {
	if err == nil {
		return nil
	}
	if _, ok := AsUser(err); ok {
		return err
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "port is already allocated"),
		strings.Contains(lower, "bind: address already in use"):
		return &UserError{
			Message: "A port this lab needs is already in use on your computer.",
			Hint:    "Stop other apps using Camunda ports (often 8080, 9200, or 18080), go to Home → Stop lab, then try again.",
			Code:    "port_conflict",
		}
	case strings.Contains(lower, "cannot connect to the docker daemon"),
		strings.Contains(lower, "docker daemon"):
		return &UserError{
			Message: "Docker is not running or not reachable.",
			Hint:    "Start Docker Desktop (or your Docker engine), wait until it is ready, then try again.",
			Code:    "docker_unavailable",
		}
	default:
		return err
	}
}

func dedupe(names []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, n := range names {
		n = strings.TrimSpace(n)
		if n == "" || seen[n] {
			continue
		}
		seen[n] = true
		out = append(out, n)
	}
	return out
}
