package api

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"strings"
)

const CSRFHeader = "X-Camunda-Lab-CSRF"

// NewCSRFToken returns a token backed by 32 bytes from the system CSPRNG.
func NewCSRFToken() (string, error) {
	var token [32]byte
	if _, err := rand.Read(token[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(token[:]), nil
}

// SecurityMiddleware restricts the UI server to literal loopback hosts and
// requires same-origin, token-authenticated requests for every mutation.
func SecurityMiddleware(csrfToken string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !isAllowedHost(r.Host) {
			writeSecurityError(w, http.StatusMisdirectedRequest, "invalid_host")
			return
		}
		if isReadOnlyMethod(r.Method) {
			next.ServeHTTP(w, r)
			return
		}
		if r.Header.Get("Origin") != "http://"+r.Host {
			writeSecurityError(w, http.StatusForbidden, "invalid_origin")
			return
		}
		presented := r.Header.Get(CSRFHeader)
		if presented == "" {
			writeSecurityError(w, http.StatusForbidden, "csrf_missing")
			return
		}
		expectedHash := sha256.Sum256([]byte(csrfToken))
		presentedHash := sha256.Sum256([]byte(presented))
		if subtle.ConstantTimeCompare(expectedHash[:], presentedHash[:]) != 1 {
			writeSecurityError(w, http.StatusForbidden, "csrf_invalid")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func isAllowedHost(host string) bool {
	for _, loopback := range []string{"localhost", "127.0.0.1", "[::1]"} {
		if host == loopback {
			return true
		}
		if strings.HasPrefix(host, loopback+":") && isNumericPort(strings.TrimPrefix(host, loopback+":")) {
			return true
		}
	}
	return false
}

func isNumericPort(port string) bool {
	if port == "" {
		return false
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func isReadOnlyMethod(method string) bool {
	return method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions
}

func writeSecurityError(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, errorEnvelope{
		OK:    false,
		Code:  code,
		Error: code,
		Hint:  hintForCode(code, status),
	})
}
