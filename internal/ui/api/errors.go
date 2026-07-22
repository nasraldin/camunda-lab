package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/laberrors"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

// errorEnvelope is the stable API failure shape for Lab UI clients.
type errorEnvelope struct {
	OK          bool   `json:"ok"`
	Code        string `json:"code"`
	Error       string `json:"error"`
	Hint        string `json:"hint,omitempty"`
	Recoverable bool   `json:"recoverable,omitempty"`
}

type codedAPIError struct {
	code    string
	message string
	hint    string
	status  int
}

func (e *codedAPIError) Error() string { return e.message }

func (e *codedAPIError) SafeCode() string { return e.code }

func (e *codedAPIError) SafeMessage() string { return e.message }

var errPayloadTooLarge = &codedAPIError{
	code:    "payload_too_large",
	message: "Uploaded file exceeds the 10MB limit.",
	hint:    "Split large uploads or use a smaller BPMN/archive file.",
	status:  http.StatusRequestEntityTooLarge,
}

func errPathForbidden(path string) error {
	_ = path // retained for call-site clarity; message stays opaque
	return &codedAPIError{
		code:    "path_forbidden",
		message: "The requested path is outside allowed roots.",
		hint:    "Use an absolute path under your home directory, lab home, or temporary directory.",
		status:  http.StatusForbidden,
	}
}

func writeMappedErr(w http.ResponseWriter, err error) {
	writeErr(w, httpStatusFor(err), err)
}

func writeErr(w http.ResponseWriter, status int, err error) {
	if status == http.StatusBadRequest {
		switch mapped := httpStatusFor(err); mapped {
		case http.StatusForbidden,
			http.StatusNotFound,
			http.StatusConflict,
			http.StatusRequestEntityTooLarge,
			http.StatusUnprocessableEntity,
			http.StatusBadGateway:
			status = mapped
		}
	}
	writeJSON(w, status, buildErrorEnvelope(err, status))
}

func httpStatusFor(err error) int {
	if err == nil {
		return http.StatusInternalServerError
	}
	var coded *codedAPIError
	if errors.As(err, &coded) && coded.status != 0 {
		return coded.status
	}
	var toolkitErr *toolkit.Error
	if errors.As(err, &toolkitErr) {
		switch toolkitErr.Kind {
		case toolkit.ErrorAI:
			return http.StatusBadGateway
		case toolkit.ErrorArtifact:
			return http.StatusConflict
		case toolkit.ErrorInvalidRequest:
			return http.StatusUnprocessableEntity
		default:
			return http.StatusBadRequest
		}
	}
	var envErr *env.Error
	if errors.As(err, &envErr) {
		switch envErr.Kind {
		case env.ErrorConflict:
			return http.StatusConflict
		case env.ErrorMissing:
			return http.StatusNotFound
		case env.ErrorInvalid, env.ErrorMigration:
			return http.StatusUnprocessableEntity
		}
	}
	var notFound *trace.NotFoundError
	if errors.As(err, &notFound) {
		return http.StatusNotFound
	}
	var apiErr *cluster.APIError
	if errors.As(err, &apiErr) {
		return http.StatusBadGateway
	}
	if isPayloadTooLarge(err) {
		return http.StatusRequestEntityTooLarge
	}
	if isPathForbidden(err) {
		return http.StatusForbidden
	}
	if isInternalFailure(err) {
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

func buildErrorEnvelope(err error, status int) errorEnvelope {
	envelope := errorEnvelope{OK: false, Code: "internal_error", Error: "An unexpected error occurred.", Hint: "Retry the action. If it keeps failing, check Lab doctor output."}
	if err == nil {
		return envelope
	}

	if coded, ok := laberrors.AsSafeCoded(err); ok {
		envelope.Code = coded.SafeCode()
		envelope.Error = coded.SafeMessage()
		if status == 0 || status == http.StatusInternalServerError {
			status = http.StatusBadRequest
		}
		envelope.Hint = hintForCode(envelope.Code, status)
		if envelope.Hint == "" {
			envelope.Hint = "Correct the configuration and retry."
		}
		return sanitizeEnvelope(envelope)
	}

	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		envelope.Code = codeForStatus(status)
		envelope.Error = "A temporary file operation failed."
		envelope.Hint = hintForCode(envelope.Code, status)
		return sanitizeEnvelope(envelope)
	}

	var coded *codedAPIError
	if errors.As(err, &coded) {
		envelope.Code = coded.code
		envelope.Error = coded.message
		envelope.Hint = coded.hint
		return sanitizeEnvelope(envelope)
	}

	err = laberrors.Wrap(err)
	if u, ok := laberrors.AsUser(err); ok {
		envelope.Error = u.Message
		envelope.Hint = u.Hint
		envelope.Recoverable = u.Recoverable
		if u.Code != "" {
			envelope.Code = u.Code
		} else {
			envelope.Code = codeForStatus(status)
		}
		return sanitizeEnvelope(envelope)
	}

	var toolkitErr *toolkit.Error
	if errors.As(err, &toolkitErr) {
		envelope.Code = string(toolkitErr.Kind)
		envelope.Error = toolkitErr.Error()
		envelope.Hint = hintForCode(envelope.Code, status)
		return sanitizeEnvelope(envelope)
	}

	var envErr *env.Error
	if errors.As(err, &envErr) {
		envelope.Code = string(envErr.Kind)
		envelope.Error = envErr.Error()
		envelope.Hint = hintForCode(envelope.Code, status)
		return sanitizeEnvelope(envelope)
	}

	var notFound *trace.NotFoundError
	if errors.As(err, &notFound) {
		envelope.Code = "not_found"
		envelope.Error = notFound.Error()
		envelope.Hint = "Confirm the process instance key and active environment."
		return sanitizeEnvelope(envelope)
	}

	var apiErr *cluster.APIError
	if errors.As(err, &apiErr) {
		envelope.Code = "upstream"
		envelope.Error = "The Camunda cluster request failed."
		envelope.Hint = "Check environment credentials and cluster connectivity, then retry."
		return sanitizeEnvelope(envelope)
	}

	if isPayloadTooLarge(err) {
		envelope.Code = errPayloadTooLarge.code
		envelope.Error = errPayloadTooLarge.message
		envelope.Hint = errPayloadTooLarge.hint
		return sanitizeEnvelope(envelope)
	}
	if isPathForbidden(err) {
		envelope.Code = "path_forbidden"
		envelope.Error = "The requested path is outside allowed roots."
		envelope.Hint = "Use an absolute path under your home directory, lab home, or temporary directory."
		return sanitizeEnvelope(envelope)
	}
	if isInvalidRequest(err) {
		envelope.Code = "invalid_request"
		envelope.Error = sanitizeErrorMessage(err.Error())
		envelope.Hint = "Check required fields and remove unknown JSON properties."
		return sanitizeEnvelope(envelope)
	}

	envelope.Code = codeForStatus(status)
	envelope.Error = sanitizeErrorMessage(err.Error())
	if envelope.Code == "internal_error" {
		envelope.Error = "An unexpected error occurred."
		envelope.Hint = "Retry the action. If it keeps failing, check Lab doctor output."
	} else if envelope.Hint == "" {
		envelope.Hint = hintForCode(envelope.Code, status)
	}
	return sanitizeEnvelope(envelope)
}

func sanitizeEnvelope(envelope errorEnvelope) errorEnvelope {
	envelope.Error = sanitizeErrorMessage(envelope.Error)
	envelope.Hint = sanitizeErrorMessage(envelope.Hint)
	return envelope
}

func sanitizeErrorMessage(message string) string {
	message = strings.TrimSpace(message)
	if message == "" {
		return message
	}
	temp := os.TempDir()
	if temp != "" && strings.Contains(message, temp) {
		return "A temporary file operation failed."
	}
	for _, marker := range []string{"/var/folders/", "/tmp/", "/private/var/folders/", "camunda-lab-"} {
		if strings.Contains(message, marker) && (strings.Contains(message, "/") || strings.Contains(message, string(os.PathSeparator))) {
			if strings.Contains(strings.ToLower(message), "rename") ||
				strings.Contains(strings.ToLower(message), "create") ||
				strings.Contains(strings.ToLower(message), "open") ||
				strings.Contains(strings.ToLower(message), "remove") {
				return "A temporary file operation failed."
			}
		}
	}
	var pathErr *os.PathError
	if errors.As(fmt.Errorf("%s", message), &pathErr) {
		return "A file operation failed."
	}
	return message
}

func codeForStatus(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "invalid_request"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusRequestEntityTooLarge:
		return "payload_too_large"
	case http.StatusUnprocessableEntity:
		return "unprocessable"
	case http.StatusBadGateway:
		return "upstream"
	default:
		return "internal_error"
	}
}

func hintForCode(code string, status int) string {
	switch code {
	case "invalid_request":
		return "Check required fields and remove unknown JSON properties."
	case "path_forbidden":
		return "Use an absolute path under your home directory, lab home, or temporary directory."
	case "not_found", "missing":
		return "Confirm the resource identity and active environment."
	case "conflict":
		return "Resolve the conflicting environment or artifact state, then retry."
	case "artifact":
		return "Choose a different output path or pass force when overwriting is intentional."
	case "payload_too_large":
		return "Split large uploads or use a smaller BPMN/archive file."
	case "invalid", "unprocessable":
		return "Correct the invalid field values and retry."
	case "ai", "upstream":
		return "Check provider/cluster connectivity and credentials, then retry."
	case "csrf_missing", "csrf_invalid":
		return "Reload the Lab UI to refresh the session CSRF token."
	case "invalid_origin":
		return "Lab UI mutations must come from the same loopback origin."
	case "invalid_host":
		return "Open the Lab UI via localhost, 127.0.0.1, or [::1]."
	default:
		if status == http.StatusInternalServerError {
			return "Retry the action. If it keeps failing, check Lab doctor output."
		}
		return ""
	}
}

func isPayloadTooLarge(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, errPayloadTooLarge) {
		return true
	}
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "file too large") ||
		strings.Contains(lower, "request body too large") ||
		strings.Contains(lower, "http: request body too large") ||
		strings.Contains(lower, "multipart body exceeds limits")
}

func isPathForbidden(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "outside allowed roots")
}

func isInvalidRequest(err error) bool {
	if err == nil {
		return false
	}
	var syntax *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	if errors.As(err, &syntax) || errors.As(err, &typeErr) {
		return true
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	return strings.Contains(msg, "unknown field") ||
		strings.Contains(lower, "invalid ") ||
		strings.Contains(lower, "required") ||
		strings.Contains(lower, "must be") ||
		strings.Contains(lower, "must appear") ||
		strings.Contains(lower, "exactly one") ||
		strings.Contains(lower, "too many") ||
		strings.Contains(lower, "refusing") ||
		strings.Contains(lower, "not supported") ||
		strings.HasPrefix(lower, "json:")
}

func isInternalFailure(err error) bool {
	if err == nil {
		return false
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return true
	}
	lower := strings.ToLower(err.Error())
	return strings.Contains(lower, "unexpected boom") ||
		strings.Contains(lower, "streaming unsupported") ||
		strings.Contains(lower, "could not stage")
}
