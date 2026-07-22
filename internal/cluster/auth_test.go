package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFetchClientCredentials(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" || r.Form.Get("client_id") != "connectors" {
			w.WriteHeader(400)
			return
		}
		if r.Form.Get("client_secret") != "secret" || r.Form.Get("audience") != "orchestration-api" {
			w.WriteHeader(400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"token_type":"Bearer","access_token":"tok-abc","expires_in":300}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
	tok, err := source.Token(context.Background(), TokenRequest{
		TokenURL: srv.URL + "/token", ClientID: "connectors", ClientSecret: "secret",
		Audience: "orchestration-api",
	})
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-abc" {
		t.Fatalf("got %q", tok)
	}
}

func TestTokenSourceUsesScopeWithoutAudience(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("scope"); got != "Zeebe" {
			t.Errorf("scope = %q", got)
		}
		if got := r.Form.Get("audience"); got != "" {
			t.Errorf("audience = %q", got)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = io.WriteString(w, `{"token_type":"bearer","access_token":"scoped","expires_in":60}`)
	}))
	defer srv.Close()
	source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
	got, err := source.Token(context.Background(), TokenRequest{
		TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret", Scope: "Zeebe",
	})
	if err != nil || got != "scoped" {
		t.Fatalf("Token() = %q, %v", got, err)
	}
}

func TestTokenSourceRejectsUnsafeURLAndResponses(t *testing.T) {
	source := newOIDCTokenSource(tokenSourceOptions{AllowHTTP: true})
	for _, raw := range []string{
		"http://user:pass@example.test/token",
		"http://example.test/token#fragment",
		"http://example.test/token?secret=value",
	} {
		if _, err := source.Token(context.Background(), TokenRequest{
			TokenURL: raw, ClientID: "id", ClientSecret: "secret",
		}); err == nil {
			t.Fatalf("Token(%q) succeeded", raw)
		}
	}

	tests := []struct {
		name        string
		status      int
		contentType string
		body        string
	}{
		{"http status", http.StatusUnauthorized, "application/json", `{"error":"secret-is-wrong"}`},
		{"content type", http.StatusOK, "text/plain", `{"token_type":"Bearer","access_token":"token","expires_in":60}`},
		{"malformed json", http.StatusOK, "application/json", `{`},
		{"missing token", http.StatusOK, "application/json", `{"token_type":"Bearer","expires_in":60}`},
		{"wrong token type", http.StatusOK, "application/json", `{"token_type":"MAC","access_token":"token","expires_in":60}`},
		{"missing expiry", http.StatusOK, "application/json", `{"token_type":"Bearer","access_token":"token"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tt.contentType)
				w.WriteHeader(tt.status)
				_, _ = io.WriteString(w, tt.body)
			}))
			defer srv.Close()
			source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
			_, err := source.Token(context.Background(), TokenRequest{
				TokenURL: srv.URL, ClientID: "id", ClientSecret: "do-not-leak",
			})
			if err == nil {
				t.Fatal("Token() succeeded")
			}
			message := err.Error()
			for _, forbidden := range []string{"do-not-leak", "secret-is-wrong", tt.body} {
				if forbidden != "" && strings.Contains(message, forbidden) {
					t.Fatalf("error leaked response or secret: %s", message)
				}
			}
		})
	}
}

func TestTokenSourceRejectsCrossOriginRedirectWithoutForwardingSecret(t *testing.T) {
	var forwarded atomic.Bool
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		data, _ := io.ReadAll(r.Body)
		if strings.Contains(string(data), "do-not-forward") {
			forwarded.Store(true)
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer target.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	source := newOIDCTokenSource(tokenSourceOptions{Client: origin.Client(), AllowHTTP: true})
	_, err := source.Token(context.Background(), TokenRequest{
		TokenURL: origin.URL, ClientID: "id", ClientSecret: "do-not-forward",
	})
	if err == nil {
		t.Fatal("redirect succeeded")
	}
	if forwarded.Load() {
		t.Fatal("client secret was forwarded cross-origin")
	}
}

func TestTokenSourceAllowsOnlyPreservingRedirects(t *testing.T) {
	for _, status := range []int{
		http.StatusMovedPermanently,
		http.StatusFound,
		http.StatusSeeOther,
		http.StatusTemporaryRedirect,
		http.StatusPermanentRedirect,
	} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			var targetCalls atomic.Int32
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/start" {
					http.Redirect(w, r, "/target", status)
					return
				}
				targetCalls.Add(1)
				if r.Method != http.MethodPost {
					t.Errorf("redirect method = %s", r.Method)
				}
				if err := r.ParseForm(); err != nil {
					t.Error(err)
				}
				if r.Form.Get("grant_type") != "client_credentials" ||
					r.Form.Get("client_secret") != "preserved-secret" {
					t.Errorf("redirect form = %v", r.Form)
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"token_type":"Bearer","access_token":"redirected","expires_in":60}`)
			}))
			defer srv.Close()
			source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
			token, err := source.Token(context.Background(), TokenRequest{
				TokenURL: srv.URL + "/start", ClientID: "id", ClientSecret: "preserved-secret",
			})
			allowed := status == http.StatusTemporaryRedirect || status == http.StatusPermanentRedirect
			if allowed {
				if err != nil || token != "redirected" || targetCalls.Load() != 1 {
					t.Fatalf("Token() = %q, %v; target calls=%d", token, err, targetCalls.Load())
				}
				return
			}
			if err == nil {
				t.Fatalf("Token() followed HTTP %d", status)
			}
			if targetCalls.Load() != 0 {
				t.Fatalf("HTTP %d target calls = %d", status, targetCalls.Load())
			}
		})
	}
}

func TestTokenSourceRejectsDowngradeRedirect(t *testing.T) {
	target := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("downgrade target was called")
	}))
	defer target.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	defer origin.Close()
	source := newOIDCTokenSource(tokenSourceOptions{Client: origin.Client()})
	if _, err := source.Token(context.Background(), TokenRequest{
		TokenURL: origin.URL, ClientID: "id", ClientSecret: "secret",
	}); err == nil {
		t.Fatal("HTTPS downgrade redirect succeeded")
	}
}

func TestTokenSourceBoundsPreservingRedirects(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		step := strings.TrimPrefix(r.URL.Path, "/")
		next := map[string]string{"0": "/1", "1": "/2", "2": "/3", "3": "/4"}[step]
		if next != "" {
			http.Redirect(w, r, next, http.StatusTemporaryRedirect)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"token_type":"Bearer","access_token":"too-far","expires_in":60}`)
	}))
	defer srv.Close()
	source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
	if _, err := source.Token(context.Background(), TokenRequest{
		TokenURL: srv.URL + "/0", ClientID: "id", ClientSecret: "secret",
	}); err == nil {
		t.Fatal("redirect chain exceeded limit")
	}
	if calls.Load() != 4 {
		t.Fatalf("redirect handler calls = %d, want 4 before rejection", calls.Load())
	}
}

func TestTokenSourceRejectsUnknownDuplicateAndTrailingJSON(t *testing.T) {
	tests := map[string]string{
		"unknown field":          `{"token_type":"Bearer","access_token":"token","expires_in":60,"refresh_token":"leak"}`,
		"duplicate token type":   `{"token_type":"Bearer","token_type":"Bearer","access_token":"token","expires_in":60}`,
		"duplicate access token": `{"token_type":"Bearer","access_token":"token","access_token":"other","expires_in":60}`,
		"duplicate expiry":       `{"token_type":"Bearer","access_token":"token","expires_in":60,"expires_in":60}`,
		"trailing value":         `{"token_type":"Bearer","access_token":"token","expires_in":60} {}`,
		"not object":             `["Bearer","token",60]`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			defer srv.Close()
			source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
			_, err := source.Token(context.Background(), TokenRequest{
				TokenURL: srv.URL, ClientID: "id", ClientSecret: "do-not-leak",
			})
			if err == nil {
				t.Fatal("Token() succeeded")
			}
			if strings.Contains(err.Error(), body) || strings.Contains(err.Error(), "do-not-leak") ||
				strings.Contains(err.Error(), "token") && strings.Contains(err.Error(), "other") {
				t.Fatalf("error leaked payload: %v", err)
			}
		})
	}
}

func TestTokenSourcePreservesSafeParserCauses(t *testing.T) {
	t.Run("MIME", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", `application/json; access_token="mime-secret`)
			_, _ = io.WriteString(w, `{}`)
		}))
		defer srv.Close()
		source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
		_, err := source.Token(context.Background(), TokenRequest{
			TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret",
		})
		var authErr *AuthError
		if !errors.As(err, &authErr) || authErr.Err == nil {
			t.Fatalf("parser cause not preserved: %v", err)
		}
		if strings.Contains(err.Error(), "mime-secret") {
			t.Fatalf("MIME parser error leaked access token: %v", err)
		}
	})
	t.Run("JSON", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"token_type":"Bearer","access_token":"json-secret","expires_in":}`)
		}))
		defer srv.Close()
		source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
		_, err := source.Token(context.Background(), TokenRequest{
			TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret",
		})
		var syntaxErr *json.SyntaxError
		if !errors.As(err, &syntaxErr) {
			t.Fatalf("JSON cause not preserved: %v", err)
		}
		if strings.Contains(err.Error(), `{"token_type"`) || strings.Contains(err.Error(), "json-secret") {
			t.Fatalf("error leaked response: %v", err)
		}
	})
}

func TestTokenSourceCoalescesRefreshAndDoesNotCacheFailures(t *testing.T) {
	var calls atomic.Int32
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		time.Sleep(20 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		if fail.Swap(false) {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = fmt.Fprintf(w, `{"token_type":"Bearer","access_token":"token-%d","expires_in":300}`, calls.Load())
	}))
	defer srv.Close()
	source := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
	request := TokenRequest{TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret"}
	var wg sync.WaitGroup
	results := make(chan string, 20)
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			token, err := source.Token(context.Background(), request)
			if err != nil {
				t.Errorf("Token: %v", err)
				return
			}
			results <- token
		}()
	}
	wg.Wait()
	close(results)
	if calls.Load() != 1 {
		t.Fatalf("token endpoint calls = %d, want 1", calls.Load())
	}
	for token := range results {
		if token != "token-1" {
			t.Fatalf("token = %q", token)
		}
	}

	failing := newOIDCTokenSource(tokenSourceOptions{Client: srv.Client(), AllowHTTP: true})
	fail.Store(true)
	if _, err := failing.Token(context.Background(), request); err == nil {
		t.Fatal("expected first request to fail")
	}
	if token, err := failing.Token(context.Background(), request); err != nil || token == "" {
		t.Fatalf("retry = %q, %v", token, err)
	}
}

func TestTokenSourceExpirySkewAndCancellation(t *testing.T) {
	now := time.Unix(1_000, 0)
	var calls atomic.Int32
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		call := calls.Add(1)
		if call == 2 {
			started <- struct{}{}
			<-release
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"token_type":"Bearer","access_token":"token-%d","expires_in":100}`, call)
	}))
	defer srv.Close()
	source := newOIDCTokenSource(tokenSourceOptions{
		Client: srv.Client(), AllowHTTP: true, Now: func() time.Time { return now },
	})
	request := TokenRequest{TokenURL: srv.URL, ClientID: "id", ClientSecret: "secret"}
	if _, err := source.Token(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	now = now.Add(91 * time.Second)
	leaderDone := make(chan error, 1)
	go func() {
		_, err := source.Token(context.Background(), request)
		leaderDone <- err
	}()
	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := source.Token(ctx, request); !errors.Is(err, context.Canceled) {
		t.Fatalf("waiting Token error = %v", err)
	}
	close(release)
	if err := <-leaderDone; err != nil {
		t.Fatal(err)
	}
}

func TestLoadComposeEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("CONNECTORS_CLIENT_ID=connectors\nCONNECTORS_CLIENT_SECRET=demo-connectors-secret\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	m := loadComposeEnv(path)
	if m["CONNECTORS_CLIENT_SECRET"] != "demo-connectors-secret" {
		t.Fatalf("%v", m)
	}
}

func TestApplyAuthBearerAndBasic(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	c := &Client{Token: "abc"}
	c.applyAuth(req)
	if got := req.Header.Get("Authorization"); got != "Bearer abc" {
		t.Fatalf("bearer: %q", got)
	}
	req2, _ := http.NewRequest(http.MethodGet, "http://x", nil)
	c2 := &Client{BasicUser: "demo", BasicPass: "demo"}
	c2.applyAuth(req2)
	u, p, ok := req2.BasicAuth()
	if !ok || u != "demo" || p != "demo" {
		t.Fatalf("basic auth missing")
	}
}
