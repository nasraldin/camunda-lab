package doctor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/cluster"
)

type RemoteCredential struct {
	BearerToken string
}

type RemoteAuthenticator interface {
	Authenticate(context.Context, EnvironmentState) (RemoteCredential, error)
}

type RemoteReachability interface {
	Probe(context.Context, EnvironmentState, RemoteCredential) error
}

type httpRemoteAuthenticator struct {
	Client HTTPClient
}

// The doctor keeps separate auth/reachability checks for diagnostics, but this
// adapter delegates token acquisition and bearer scoping to cluster's shared
// production primitives rather than constructing credentials or auth headers.
func (a httpRemoteAuthenticator) Authenticate(ctx context.Context, state EnvironmentState) (RemoteCredential, error) {
	if state.accessToken != "" {
		return RemoteCredential{BearerToken: state.accessToken}, nil
	}
	clientID := state.clientID
	if state.clientIDEnv != "" {
		clientID = os.Getenv(state.clientIDEnv)
	}
	clientSecret := state.clientSecret
	if state.clientSecretEnv != "" {
		clientSecret = os.Getenv(state.clientSecretEnv)
	}
	tokenURL := state.tokenURL
	if state.tokenURLEnv != "" {
		tokenURL = strings.TrimSpace(os.Getenv(state.tokenURLEnv))
	}
	if clientID == "" || clientSecret == "" || tokenURL == "" {
		return RemoteCredential{}, fmt.Errorf("remote credential or token endpoint configuration is incomplete")
	}
	source := cluster.NewOIDCTokenSource(asHTTPClient(a.Client))
	token, err := source.Token(ctx, cluster.TokenRequest{
		TokenURL: tokenURL, ClientID: clientID, ClientSecret: clientSecret,
		Audience: state.audience, Scope: state.scope,
	})
	if err != nil {
		return RemoteCredential{}, fmt.Errorf("request remote token: %w", err)
	}
	return RemoteCredential{BearerToken: token}, nil
}

type httpRemoteReachability struct {
	Client HTTPClient
}

func (p httpRemoteReachability) Probe(ctx context.Context, state EnvironmentState, credential RemoteCredential) error {
	if credential.BearerToken == "" {
		return fmt.Errorf("authenticated remote probe requires a bearer token")
	}
	baseURL, err := cluster.NormalizeBaseURL(state.Endpoint)
	if err != nil {
		return fmt.Errorf("normalize protected cluster endpoint: %w", err)
	}
	endpoint := baseURL + "/process-definitions/search"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(`{"page":{"limit":1}}`))
	if err != nil {
		return fmt.Errorf("create protected cluster request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	authenticated, err := cluster.AuthenticatedHTTPClient(
		asHTTPClient(p.Client), baseURL, doctorToken(credential.BearerToken), cluster.TokenRequest{},
	)
	if err != nil {
		return fmt.Errorf("configure protected cluster request: %w", err)
	}
	resp, err := authenticated.Do(req)
	if err != nil {
		return fmt.Errorf("protected cluster request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("protected cluster endpoint returned HTTP %d", resp.StatusCode)
	}
	return nil
}

type httpClientTransport struct {
	client HTTPClient
}

func (t httpClientTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	return t.client.Do(request)
}

func asHTTPClient(client HTTPClient) *http.Client {
	if concrete, ok := client.(*http.Client); ok {
		return concrete
	}
	return &http.Client{Transport: httpClientTransport{client: client}}
}

type doctorToken string

func (t doctorToken) Token(context.Context, cluster.TokenRequest) (string, error) {
	return string(t), nil
}
