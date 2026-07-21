package api

import (
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSecurityAcceptsLiteralLoopbackHostsForReads(t *testing.T) {
	t.Parallel()
	for _, host := range []string{
		"localhost",
		"localhost:9090",
		"127.0.0.1",
		"127.0.0.1:9090",
		"[::1]",
		"[::1]:9090",
	} {
		host := host
		t.Run(host, func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				called = true
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			req.Host = host
			rec := httptest.NewRecorder()

			SecurityMiddleware("token", next).ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent || !called {
				t.Fatalf("host %q: status = %d, called = %v, body = %s", host, rec.Code, called, rec.Body.String())
			}
		})
	}
}

func TestSecurityRejectsNonLiteralOrMalformedHosts(t *testing.T) {
	t.Parallel()
	for _, host := range []string{
		"",
		"LOCALHOST",
		"localhost.",
		"localhost.example",
		"127.0.0.2",
		"127.0.0.1.example",
		"2130706433",
		"0177.0.0.1",
		"::1",
		"[::1].example",
		"localhost:http",
		"localhost:9090:extra",
		"user@localhost",
	} {
		host := host
		t.Run(strings.ReplaceAll(host, "/", "_"), func(t *testing.T) {
			called := false
			next := http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true })
			req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
			req.Host = host
			rec := httptest.NewRecorder()

			SecurityMiddleware("token", next).ServeHTTP(rec, req)

			assertSecurityError(t, rec, http.StatusMisdirectedRequest, "invalid_host")
			if called {
				t.Fatal("next handler was called")
			}
		})
	}
}

func TestSecurityAllowsReadOnlyMethodsWithoutCSRF(t *testing.T) {
	t.Parallel()
	for _, method := range []string{http.MethodGet, http.MethodHead, http.MethodOptions} {
		method := method
		t.Run(method, func(t *testing.T) {
			next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			})
			req := httptest.NewRequest(method, "http://example.test/", nil)
			req.Host = "localhost:9090"
			req.Header.Set("Origin", "http://attacker.example")
			rec := httptest.NewRecorder()

			SecurityMiddleware("token", next).ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
		})
	}
}

func TestSecurityRejectsMutationWithInvalidOrigin(t *testing.T) {
	t.Parallel()
	for name, origin := range map[string]string{
		"missing":       "",
		"foreign":       "http://attacker.example",
		"https":         "https://localhost:9090",
		"case mismatch": "http://LOCALHOST:9090",
	} {
		name, origin := name, origin
		t.Run(name, func(t *testing.T) {
			req := mutationRequest(http.MethodPost, "localhost:9090", origin, "token")
			rec := httptest.NewRecorder()

			SecurityMiddleware("token", http.HandlerFunc(successHandler)).ServeHTTP(rec, req)

			assertSecurityError(t, rec, http.StatusForbidden, "invalid_origin")
		})
	}
}

func TestSecurityRejectsMissingAndInvalidCSRFToken(t *testing.T) {
	t.Parallel()
	for name, token := range map[string]string{
		"missing": "",
		"invalid": "wrong-token",
	} {
		name, token := name, token
		t.Run(name, func(t *testing.T) {
			req := mutationRequest(http.MethodPost, "127.0.0.1:9090", "http://127.0.0.1:9090", token)
			rec := httptest.NewRecorder()

			SecurityMiddleware("correct-token", http.HandlerFunc(successHandler)).ServeHTTP(rec, req)

			code := "csrf_invalid"
			if token == "" {
				code = "csrf_missing"
			}
			assertSecurityError(t, rec, http.StatusForbidden, code)
		})
	}
}

func TestSecurityAllowsSameOriginMutations(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		method      string
		contentType string
	}{
		{name: "json", method: http.MethodPost, contentType: "application/json"},
		{name: "form", method: http.MethodPost, contentType: "application/x-www-form-urlencoded"},
		{name: "multipart", method: http.MethodPost, contentType: "multipart/form-data; boundary=test"},
		{name: "delete", method: http.MethodDelete, contentType: "application/json"},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			req := mutationRequest(tc.method, "[::1]:9090", "http://[::1]:9090", "correct-token")
			req.Header.Set("Content-Type", tc.contentType)
			rec := httptest.NewRecorder()

			SecurityMiddleware("correct-token", http.HandlerFunc(successHandler)).ServeHTTP(rec, req)

			if rec.Code != http.StatusNoContent {
				t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusNoContent, rec.Body.String())
			}
		})
	}
}

func TestNewCSRFTokenUsesRandom32Bytes(t *testing.T) {
	t.Parallel()
	first, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	second, err := NewCSRFToken()
	if err != nil {
		t.Fatal(err)
	}
	raw, err := hex.DecodeString(first)
	if err != nil {
		t.Fatalf("token is not hexadecimal: %v", err)
	}
	if len(raw) != 32 {
		t.Fatalf("decoded token length = %d, want 32", len(raw))
	}
	if first == second {
		t.Fatal("two generated tokens are equal")
	}
}

func TestSessionReturnsProcessCSRFToken(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	Register(mux, "test", "process-token")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	rec := httptest.NewRecorder()

	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body struct {
		CSRFToken string `json:"csrfToken"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.CSRFToken != "process-token" {
		t.Fatalf("csrfToken = %q, want process token", body.CSRFToken)
	}
}

func mutationRequest(method, host, origin, token string) *http.Request {
	req := httptest.NewRequest(method, "http://example.test/api", strings.NewReader("{}"))
	req.Host = host
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	if token != "" {
		req.Header.Set(CSRFHeader, token)
	}
	return req
}

func successHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

func assertSecurityError(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, status, rec.Body.String())
	}
	var body struct {
		Code string `json:"code"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatalf("decode error response: %v; body = %s", err, rec.Body.String())
	}
	if body.Code != code {
		t.Fatalf("code = %q, want %q", body.Code, code)
	}
}
