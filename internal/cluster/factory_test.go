package cluster

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/env"
)

func TestFactoryRemoteProfileUsesConfiguredOIDCAndMetadata(t *testing.T) {
	var tokenCalls atomic.Int32
	var clusterAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		tokenCalls.Add(1)
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("audience") != "orchestration-api" {
			t.Errorf("audience = %q", r.Form.Get("audience"))
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"token_type":"Bearer","access_token":"remote-token","expires_in":300}`)
	})
	mux.HandleFunc("/camunda/v2/process-definitions/search", func(w http.ResponseWriter, r *http.Request) {
		clusterAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"page":{"totalItems":0,"endCursor":null}}`)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	labHome := t.TempDir()
	t.Setenv("P2_CLIENT_ID", "client")
	t.Setenv("P2_CLIENT_SECRET", "top-secret")
	t.Setenv("P2_TOKEN_URL", srv.URL+"/oauth/token")
	service := env.NewService(labHome)
	if err := service.SaveGlobal(env.Profile{
		Name: "prod", Kind: "remote",
		Endpoints: map[string]string{"orchestration": srv.URL + "/camunda"},
		Auth: env.AuthRefs{
			ClientIDEnv: "P2_CLIENT_ID", ClientSecretEnv: "P2_CLIENT_SECRET",
			TokenURLEnv: "P2_TOKEN_URL", Audience: "orchestration-api",
		},
	}); err != nil {
		t.Fatal(err)
	}

	factory := newFactory(factoryOptions{
		LabHome: labHome, Config: config.Defaults(),
		HTTPClient: srv.Client(), AllowRemoteHTTP: true,
	})
	client, resolved, err := factory.Client(context.Background(), "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Profile.Name != "prod" || resolved.Source != env.ProfileSourceGlobal {
		t.Fatalf("resolved = %+v", resolved)
	}
	if client.Kind != ClientRemote || client.BaseURL != srv.URL+"/camunda/v2" {
		t.Fatalf("client = %+v", client)
	}
	if client.Token != "" {
		t.Fatal("factory exposed a remote access token on Client")
	}
	if _, err := client.SearchProcessDefinitions(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if clusterAuth != "Bearer remote-token" || tokenCalls.Load() != 1 {
		t.Fatalf("auth = %q, token calls = %d", clusterAuth, tokenCalls.Load())
	}
}

func TestFactoryRemoteProfileNeverFallsBackToLocalhost(t *testing.T) {
	labHome := t.TempDir()
	service := env.NewService(labHome)
	if err := service.SaveGlobal(env.Profile{
		Name: "prod", Kind: "remote",
		Endpoints: map[string]string{"orchestration": "https://cluster.example"},
		Auth: env.AuthRefs{
			ClientIDEnv: "P2_ID", ClientSecretEnv: "P2_SECRET", TokenURLEnv: "P2_TOKEN_URL",
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("P2_ID", "id")
	t.Setenv("P2_SECRET", "secret")
	t.Setenv("P2_TOKEN_URL", "")
	factory := NewFactory(labHome, config.Defaults())
	_, _, err := factory.Client(context.Background(), "prod", "")
	if err == nil || !strings.Contains(err.Error(), "P2_TOKEN_URL") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(strings.ToLower(err.Error()), "localhost") {
		t.Fatalf("remote error contains localhost fallback: %v", err)
	}
}

func TestFactoryMissingNamedEnvironmentVariablesAreActionableAndRedacted(t *testing.T) {
	labHome := t.TempDir()
	if err := env.NewService(labHome).SaveGlobal(env.Profile{
		Name: "prod", Kind: "remote",
		Endpoints: map[string]string{"orchestration": "https://cluster.example"},
		Auth: env.AuthRefs{
			ClientIDEnv: "P2_MISSING_ID", ClientSecretEnv: "P2_MISSING_SECRET",
			TokenURL: "https://login.example/token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	_, _, err := NewFactory(labHome, config.Defaults()).Client(context.Background(), "prod", "")
	if err == nil || !strings.Contains(err.Error(), "P2_MISSING_ID") || !strings.Contains(err.Error(), "P2_MISSING_SECRET") {
		t.Fatalf("error = %v", err)
	}
}

func TestFactoryAccessTokenOverrideSkipsOIDC(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"items":[],"page":{"totalItems":0,"endCursor":null}}`)
	}))
	defer srv.Close()
	labHome := t.TempDir()
	if err := env.NewService(labHome).SaveGlobal(env.Profile{
		Name: "prod", Kind: "remote",
		Endpoints: map[string]string{"orchestration": srv.URL},
		Auth: env.AuthRefs{
			ClientIDEnv: "P2_UNUSED_ID", ClientSecretEnv: "P2_UNUSED_SECRET",
			TokenURL: "https://unused.example/token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CAMUNDA_ACCESS_TOKEN", "override-token")
	factory := newFactory(factoryOptions{
		LabHome: labHome, Config: config.Defaults(), HTTPClient: srv.Client(), AllowRemoteHTTP: true,
	})
	client, _, err := factory.Client(context.Background(), "prod", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SearchProcessDefinitions(context.Background(), 1); err != nil {
		t.Fatal(err)
	}
	if auth != "Bearer override-token" {
		t.Fatalf("Authorization = %q", auth)
	}
}

func TestFactoryProjectLocalProfileSelection(t *testing.T) {
	labHome := t.TempDir()
	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".camunda.yaml"), []byte("environment: staging\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := env.NewService(labHome).SaveProject(project, env.Profile{
		Name: "staging", Kind: "remote",
		Endpoints: map[string]string{"orchestration": "https://project.example"},
		Auth: env.AuthRefs{
			ClientIDEnv: "P2_PROJECT_ID", ClientSecretEnv: "P2_PROJECT_SECRET",
			TokenURL: "https://login.example/token",
		},
	}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("P2_PROJECT_ID", "id")
	t.Setenv("P2_PROJECT_SECRET", "secret")
	client, resolved, err := NewFactory(labHome, config.Defaults()).Client(context.Background(), "", project)
	if err != nil {
		t.Fatal(err)
	}
	if resolved.Source != env.ProfileSourceProject || client.BaseURL != "https://project.example/v2" {
		t.Fatalf("client = %+v, resolved = %+v", client, resolved)
	}
}

func TestFactoryLocalLightAndFullLabOIDC(t *testing.T) {
	t.Run("light has no OIDC", func(t *testing.T) {
		client, resolved, err := NewFactory(t.TempDir(), config.Defaults()).Client(context.Background(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Profile.Kind != "lab" || client.Kind != ClientLocal || client.Token != "" {
			t.Fatalf("client = %+v, resolved = %+v", client, resolved)
		}
	})

	t.Run("full uses local OIDC", func(t *testing.T) {
		var clusterAuth string
		mux := http.NewServeMux()
		mux.HandleFunc("/auth/realms/camunda-platform/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"token_type":"Bearer","access_token":"lab-token","expires_in":300}`)
		})
		mux.HandleFunc("/v2/process-definitions/search", func(w http.ResponseWriter, r *http.Request) {
			clusterAuth = r.Header.Get("Authorization")
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"items":[],"page":{"totalItems":0,"endCursor":null}}`)
		})
		srv := httptest.NewServer(mux)
		defer srv.Close()
		t.Setenv("CAMUNDA_CLIENT_ID", "connectors")
		t.Setenv("CAMUNDA_CLIENT_SECRET", "secret")
		t.Setenv("CAMUNDA_OAUTH_URL", srv.URL+"/auth/realms/camunda-platform/protocol/openid-connect/token")
		cfg := config.Defaults()
		cfg.Profile = "full"
		cfg.Host = "localhost"
		factory := newFactory(factoryOptions{
			LabHome: t.TempDir(), Config: cfg, HTTPClient: srv.Client(),
			LocalBaseURL: srv.URL,
		})
		client, _, err := factory.Client(context.Background(), "", "")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := client.SearchProcessDefinitions(context.Background(), 1); err != nil {
			t.Fatal(err)
		}
		if clusterAuth != "Bearer lab-token" {
			t.Fatalf("Authorization = %q", clusterAuth)
		}
	})
}

func TestAuthTransportStripsAllAuthorizationAndNeverAuthenticatesRedirectRequests(t *testing.T) {
	capture := &captureTransport{}
	baseClient := &http.Client{Transport: capture}
	client, err := authenticatedHTTPClient(baseClient, "https://cluster.example/prefix/v2", staticTokenSource("safe-token"), TokenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	transport := client.Transport

	direct, _ := http.NewRequest(http.MethodGet, "https://cluster.example/prefix/v2/processes", nil)
	direct.Header["authorization"] = []string{"Bearer lowercase-attacker"}
	direct.Header["AUTHORIZATION"] = []string{"Bearer uppercase-attacker"}
	if _, err := transport.RoundTrip(direct); err != nil {
		t.Fatal(err)
	}
	if got := authorizationValues(capture.request.Header); len(got) != 1 || got[0] != "Bearer safe-token" {
		t.Fatalf("direct authorization headers = %v", got)
	}

	redirected, _ := http.NewRequest(http.MethodGet, "https://cluster.example/prefix/v2/processes", nil)
	redirected.Response = &http.Response{StatusCode: http.StatusTemporaryRedirect}
	redirected.Header["authorization"] = []string{"Bearer lowercase-attacker"}
	if _, err := transport.RoundTrip(redirected); err != nil {
		t.Fatal(err)
	}
	if got := authorizationValues(capture.request.Header); len(got) != 0 {
		t.Fatalf("redirect authorization headers = %v", got)
	}
}

func TestAuthenticatedClusterClientRejectsAllRedirects(t *testing.T) {
	var destinationCalls atomic.Int32
	other := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		destinationCalls.Add(1)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer other.Close()

	var origin *httptest.Server
	origin = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/prefix/v2/same-in":
			http.Redirect(w, r, origin.URL+"/prefix/v2/destination", http.StatusTemporaryRedirect)
		case "/prefix/v2/same-out":
			http.Redirect(w, r, origin.URL+"/outside", http.StatusTemporaryRedirect)
		case "/prefix/v2/cross-in":
			http.Redirect(w, r, other.URL+"/prefix/v2/destination", http.StatusTemporaryRedirect)
		case "/prefix/v2/cross-out":
			http.Redirect(w, r, other.URL+"/outside", http.StatusTemporaryRedirect)
		default:
			destinationCalls.Add(1)
			w.WriteHeader(http.StatusNoContent)
		}
	}))
	defer origin.Close()
	client, err := authenticatedHTTPClient(origin.Client(), origin.URL+"/prefix/v2", staticTokenSource("safe-token"), TokenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"same-in", "same-out", "cross-in", "cross-out"} {
		t.Run(path, func(t *testing.T) {
			req, _ := http.NewRequest(http.MethodGet, origin.URL+"/prefix/v2/"+path, nil)
			req.Header["authorization"] = []string{"Bearer attacker"}
			resp, err := client.Do(req)
			if err == nil {
				resp.Body.Close()
				t.Fatal("redirect succeeded")
			}
		})
	}
	if destinationCalls.Load() != 0 {
		t.Fatalf("redirect destinations called = %d", destinationCalls.Load())
	}
}

type staticTokenSource string

func (s staticTokenSource) Token(context.Context, TokenRequest) (string, error) {
	return string(s), nil
}

type captureTransport struct {
	request *http.Request
}

func (t *captureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.request = request
	return &http.Response{
		StatusCode: http.StatusNoContent,
		Header:     make(http.Header),
		Body:       http.NoBody,
		Request:    request,
	}, nil
}

func authorizationValues(header http.Header) []string {
	var values []string
	for key, current := range header {
		if strings.EqualFold(key, "Authorization") {
			values = append(values, current...)
		}
	}
	return values
}
