package cluster

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	pathpkg "path"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

// ClientKind distinguishes an implicit local lab from a named remote cluster.
type ClientKind string

const (
	ClientLocal  ClientKind = "local"
	ClientRemote ClientKind = "remote"
)

// Factory is the only production entry point for environment-aware cluster
// clients.
type Factory interface {
	Client(context.Context, string, string) (*Client, env.Resolved, error)
}

type factoryOptions struct {
	LabHome         string
	Config          config.Config
	HTTPClient      *http.Client
	TokenSource     TokenSource
	AllowRemoteHTTP bool
	LocalBaseURL    string
}

type factory struct {
	options      factoryOptions
	remoteSource TokenSource
}

// NewFactory creates an active-environment cluster client factory.
func NewFactory(labHome string, cfg config.Config) Factory {
	return newFactory(factoryOptions{LabHome: labHome, Config: cfg})
}

func newFactory(options factoryOptions) Factory {
	if options.LabHome == "" {
		options.LabHome = paths.Home()
	}
	instance := &factory{options: options, remoteSource: options.TokenSource}
	if instance.remoteSource == nil {
		instance.remoteSource = newOIDCTokenSource(tokenSourceOptions{
			Client: instance.baseHTTPClient(), AllowHTTP: options.AllowRemoteHTTP,
		})
	}
	return instance
}

func (f *factory) Client(ctx context.Context, envName, projectRoot string) (*Client, env.Resolved, error) {
	resolved, err := env.NewService(f.options.LabHome).Resolve(env.ResolveRequest{
		Name: envName, ProjectRoot: projectRoot,
	})
	if err != nil {
		return nil, env.Resolved{}, fmt.Errorf("resolve cluster environment: %w", err)
	}
	if resolved.Profile.Kind == "lab" {
		client, err := f.localClient(ctx)
		if err != nil {
			return nil, env.Resolved{}, fmt.Errorf("create local cluster client: %w", err)
		}
		return client, resolved, nil
	}
	client, err := f.remoteClient(resolved.Profile)
	if err != nil {
		return nil, env.Resolved{}, fmt.Errorf("create remote cluster client for %q: %w", resolved.Profile.Name, err)
	}
	return client, resolved, nil
}

func (f *factory) remoteClient(profile env.Profile) (*Client, error) {
	baseURL, err := NormalizeBaseURL(profile.Endpoints["orchestration"])
	if err != nil {
		return nil, fmt.Errorf("orchestration endpoint: %w", err)
	}
	parsedBase, _ := url.Parse(baseURL)
	if parsedBase.Scheme != "https" && !f.options.AllowRemoteHTTP {
		return nil, errors.New("remote orchestration endpoint must use HTTPS")
	}
	baseClient := f.baseHTTPClient()
	if override := strings.TrimSpace(os.Getenv("CAMUNDA_ACCESS_TOKEN")); override != "" {
		authenticated, err := authenticatedHTTPClient(baseClient, baseURL, staticBearerToken(override), TokenRequest{})
		if err != nil {
			return nil, err
		}
		return &Client{BaseURL: baseURL, HTTPClient: authenticated, Kind: ClientRemote}, nil
	}

	missing := make([]string, 0, 3)
	_ = valueFromNamedEnv(profile.Auth.ClientIDEnv, &missing)
	_ = valueFromNamedEnv(profile.Auth.ClientSecretEnv, &missing)
	tokenURL := strings.TrimSpace(profile.Auth.TokenURL)
	if profile.Auth.TokenURLEnv != "" {
		tokenURL = valueFromNamedEnv(profile.Auth.TokenURLEnv, &missing)
	}
	if tokenURL == "" && profile.Auth.TokenURLEnv == "" {
		missing = append(missing, "auth.tokenUrl or auth.tokenUrlEnv")
	}
	if len(missing) != 0 {
		return nil, fmt.Errorf("missing remote credential configuration: set %s", strings.Join(missing, ", "))
	}
	audience := strings.TrimSpace(profile.Auth.Audience)
	scope := strings.TrimSpace(profile.Auth.Scope)
	if audience == "" && scope == "" {
		audience = "orchestration-api"
	}
	if _, err := validateTokenURL(tokenURL, f.options.AllowRemoteHTTP); err != nil {
		return nil, err
	}
	tokenRequest := TokenRequest{
		TokenURL: tokenURL, Audience: audience, Scope: scope,
	}
	// Resolve referenced credentials for every refresh. The returned Client
	// retains only profile references, never the remote client secret.
	authenticated, err := authenticatedHTTPClientWithProvider(baseClient, baseURL, f.remoteSource, func() (TokenRequest, error) {
		request := tokenRequest
		request.ClientID = os.Getenv(profile.Auth.ClientIDEnv)
		request.ClientSecret = os.Getenv(profile.Auth.ClientSecretEnv)
		if profile.Auth.TokenURLEnv != "" {
			request.TokenURL = strings.TrimSpace(os.Getenv(profile.Auth.TokenURLEnv))
		}
		if request.ClientID == "" || request.ClientSecret == "" || request.TokenURL == "" {
			return TokenRequest{}, errors.New("remote credential environment variable is no longer set")
		}
		return request, nil
	})
	if err != nil {
		return nil, err
	}
	return &Client{BaseURL: baseURL, HTTPClient: authenticated, Kind: ClientRemote}, nil
}

func (f *factory) localClient(context.Context) (*Client, error) {
	baseURL := f.options.LocalBaseURL
	if baseURL == "" {
		var err error
		baseURL, err = localBaseURL(f.options.Config)
		if err != nil {
			return nil, err
		}
	} else {
		var err error
		baseURL, err = NormalizeBaseURL(baseURL)
		if err != nil {
			return nil, err
		}
	}
	client := &Client{BaseURL: baseURL, HTTPClient: f.baseHTTPClient(), Kind: ClientLocal}
	if override := strings.TrimSpace(os.Getenv("CAMUNDA_ACCESS_TOKEN")); override != "" {
		authenticated, err := authenticatedHTTPClient(client.HTTPClient, baseURL, staticBearerToken(override), TokenRequest{})
		if err != nil {
			return nil, err
		}
		client.HTTPClient = authenticated
		return client, nil
	}
	if f.options.Config.Profile != "full" {
		if user := os.Getenv("CAMUNDA_BASIC_USER"); user != "" {
			client.BasicUser = user
			client.BasicPass = os.Getenv("CAMUNDA_BASIC_PASSWORD")
			if client.BasicPass == "" {
				client.BasicPass = "demo"
			}
		}
		return client, nil
	}

	values := loadComposeEnv(paths.VersionDir(f.options.Config.Version) + "/.env")
	clientID := firstNonEmpty(os.Getenv("CAMUNDA_CLIENT_ID"), values["CONNECTORS_CLIENT_ID"], "connectors")
	clientSecret := firstNonEmpty(
		os.Getenv("CAMUNDA_CLIENT_SECRET"),
		values["CONNECTORS_CLIENT_SECRET"],
		values["ORCHESTRATION_CLIENT_SECRET"],
	)
	if clientSecret == "" {
		return nil, errors.New("no client secret (set CAMUNDA_CLIENT_SECRET or install a full lab)")
	}
	host := f.options.Config.Host
	if host == "" {
		host = "localhost"
	}
	tokenURL := firstNonEmpty(
		os.Getenv("CAMUNDA_OAUTH_URL"),
		fmt.Sprintf("http://%s:18080/auth/realms/camunda-platform/protocol/openid-connect/token", host),
	)
	source := f.options.TokenSource
	if source == nil {
		source = newOIDCTokenSource(tokenSourceOptions{Client: f.baseHTTPClient(), AllowHTTP: isLoopbackHTTP(tokenURL)})
	}
	authenticated, err := authenticatedHTTPClient(client.HTTPClient, baseURL, source, TokenRequest{
		TokenURL: tokenURL, ClientID: clientID, ClientSecret: clientSecret,
		Audience: firstNonEmpty(os.Getenv("CAMUNDA_TOKEN_AUDIENCE"), "orchestration-api"),
	})
	if err != nil {
		return nil, err
	}
	client.HTTPClient = authenticated
	return client, nil
}

func (f *factory) baseHTTPClient() *http.Client {
	if f.options.HTTPClient != nil {
		return f.options.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

func localBaseURL(cfg config.Config) (string, error) {
	for _, entry := range urls.List(cfg) {
		if entry.Name == "rest" && strings.HasPrefix(entry.URL, "http") {
			return NormalizeBaseURL(entry.URL)
		}
	}
	port := 8080
	if cfg.Version == "8.8" || cfg.Version == "8.7" || cfg.Version == "8.6" || cfg.Version == "8.5" {
		port = 8088
	}
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	return NormalizeBaseURL(fmt.Sprintf("http://%s:%d", host, port))
}

func valueFromNamedEnv(name string, missing *[]string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		*missing = append(*missing, "environment variable reference")
		return ""
	}
	value := os.Getenv(name)
	if value == "" {
		*missing = append(*missing, name)
	}
	return value
}

type staticBearerToken string

func (s staticBearerToken) Token(context.Context, TokenRequest) (string, error) {
	return string(s), nil
}

type bearerTransport struct {
	base        http.RoundTripper
	origin      *url.URL
	allowedPath string
	source      TokenSource
	request     func() (TokenRequest, error)
}

func authenticatedHTTPClient(base *http.Client, canonicalBase string, source TokenSource, request TokenRequest) (*http.Client, error) {
	return authenticatedHTTPClientWithProvider(base, canonicalBase, source, func() (TokenRequest, error) {
		return request, nil
	})
}

func authenticatedHTTPClientWithProvider(base *http.Client, canonicalBase string, source TokenSource, request func() (TokenRequest, error)) (*http.Client, error) {
	allowed, err := url.Parse(canonicalBase)
	if err != nil || allowed.Host == "" {
		return nil, errors.New("invalid authenticated cluster base URL")
	}
	if base == nil {
		base = &http.Client{Timeout: 15 * time.Second}
	}
	clone := *base
	transport := clone.Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	clone.Transport = &bearerTransport{
		base: transport, origin: allowed, allowedPath: pathpkg.Clean(allowed.Path),
		source: source, request: request,
	}
	clone.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("cluster API redirects are not allowed")
	}
	return &clone, nil
}

// AuthenticatedHTTPClient is a thin adapter for diagnostics that must use the
// same origin/path-constrained bearer transport as production cluster clients.
func AuthenticatedHTTPClient(base *http.Client, canonicalBase string, source TokenSource, request TokenRequest) (*http.Client, error) {
	return authenticatedHTTPClient(base, canonicalBase, source, request)
}

func (t *bearerTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.Header = request.Header.Clone()
	stripAuthorization(clone.Header)
	path := pathpkg.Clean(clone.URL.Path)
	allowed := clone.Response == nil && sameOrigin(clone.URL, t.origin) &&
		(path == t.allowedPath || strings.HasPrefix(path, t.allowedPath+"/"))
	if allowed {
		tokenRequest, err := t.request()
		if err != nil {
			return nil, fmt.Errorf("resolve cluster credential: %w", err)
		}
		token, err := t.source.Token(clone.Context(), tokenRequest)
		if err != nil {
			return nil, fmt.Errorf("authorize cluster request: %w", err)
		}
		clone.Header.Set("Authorization", "Bearer "+token)
	}
	return t.base.RoundTrip(clone)
}

func stripAuthorization(header http.Header) {
	for key := range header {
		if strings.EqualFold(key, "Authorization") {
			delete(header, key)
		}
	}
}

func isLoopbackHTTP(raw string) bool {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "http" {
		return false
	}
	host := strings.ToLower(parsed.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1"
}
