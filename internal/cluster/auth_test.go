package cluster

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
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
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"tok-abc"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tok, err := fetchClientCredentials(context.Background(), srv.URL+"/token", "connectors", "secret", "orchestration-api")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "tok-abc" {
		t.Fatalf("got %q", tok)
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
