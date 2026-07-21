package ui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
)

func TestAssertLoopback(t *testing.T) {
	for _, host := range []string{"127.0.0.1", "localhost", "::1"} {
		if err := assertLoopback(host); err != nil {
			t.Fatalf("%s: %v", host, err)
		}
	}
	if err := assertLoopback("0.0.0.0"); err == nil {
		t.Fatal("expected error for 0.0.0.0")
	}
	if err := assertLoopback("192.168.1.10"); err == nil {
		t.Fatal("expected error for LAN IP")
	}
}

func TestServerHandlerWrapsAPIAndStaticUIWithSecurity(t *testing.T) {
	static := http.FS(fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("ui")},
	})
	handler := serverHandler(static, "test", "process-token")

	t.Run("session", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
		req.Host = "localhost:9090"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK || rec.Body.String() != "{\"csrfToken\":\"process-token\"}\n" {
			t.Fatalf("status = %d, body = %q", rec.Code, rec.Body.String())
		}
	})

	t.Run("static UI", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "127.0.0.1:9090"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		body, err := io.ReadAll(rec.Result().Body)
		if err != nil {
			t.Fatal(err)
		}
		if rec.Code != http.StatusOK || string(body) != "ui" {
			t.Fatalf("status = %d, body = %q", rec.Code, body)
		}
	})

	t.Run("invalid host cannot reach static UI", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.Host = "attacker.example"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMisdirectedRequest {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusMisdirectedRequest, rec.Body.String())
		}
	})

	t.Run("mutation guard precedes API routing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/not-found", nil)
		req.Host = "localhost:9090"
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusForbidden {
			t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusForbidden, rec.Body.String())
		}
	})
}
