package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"mime"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxTokenResponseBytes = 1 << 20
	maxTokenRedirects     = 3
)

// TokenRequest contains a client-credentials request. ClientSecret is runtime
// only and is never included in errors or cache keys.
type TokenRequest struct {
	TokenURL     string
	ClientID     string
	ClientSecret string
	Audience     string
	Scope        string
}

// TokenSource returns a valid bearer token.
type TokenSource interface {
	Token(context.Context, TokenRequest) (string, error)
}

// AuthErrorKind identifies a safe, actionable authentication failure.
type AuthErrorKind string

const (
	AuthErrorConfiguration AuthErrorKind = "configuration"
	AuthErrorNetwork       AuthErrorKind = "network"
	AuthErrorResponse      AuthErrorKind = "response"
)

// AuthError preserves an underlying cause without exposing credentials or
// token endpoint response bodies.
type AuthError struct {
	Kind      AuthErrorKind
	Operation string
	Err       error
}

func (e *AuthError) Error() string {
	return fmt.Sprintf("%s token: %v", e.Operation, e.Err)
}

func (e *AuthError) Unwrap() error { return e.Err }

type tokenSourceOptions struct {
	Client    *http.Client
	Now       func() time.Time
	AllowHTTP bool
}

type oidcTokenSource struct {
	client    *http.Client
	now       func() time.Time
	allowHTTP bool

	mu      sync.Mutex
	entries map[tokenCacheKey]*tokenCacheEntry
}

type tokenCacheKey struct {
	tokenURL string
	clientID string
	audience string
	scope    string
}

type tokenCacheEntry struct {
	token      string
	validUntil time.Time
	refreshing bool
	ready      chan struct{}
}

// NewOIDCTokenSource creates a strict HTTPS client-credentials token source.
func NewOIDCTokenSource(client *http.Client) TokenSource {
	return newOIDCTokenSource(tokenSourceOptions{Client: client})
}

func newOIDCTokenSource(options tokenSourceOptions) *oidcTokenSource {
	client := options.Client
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	now := options.Now
	if now == nil {
		now = time.Now
	}
	return &oidcTokenSource{
		client: client, now: now, allowHTTP: options.AllowHTTP,
		entries: make(map[tokenCacheKey]*tokenCacheEntry),
	}
}

func (s *oidcTokenSource) Token(ctx context.Context, request TokenRequest) (string, error) {
	normalized, err := s.validateRequest(request)
	if err != nil {
		return "", err
	}
	key := tokenCacheKey{
		tokenURL: normalized.TokenURL, clientID: normalized.ClientID,
		audience: normalized.Audience, scope: normalized.Scope,
	}
	for {
		now := s.now()
		s.mu.Lock()
		entry := s.entries[key]
		if entry != nil && entry.token != "" && now.Before(entry.validUntil) {
			token := entry.token
			s.mu.Unlock()
			return token, nil
		}
		if entry != nil && entry.refreshing {
			ready := entry.ready
			s.mu.Unlock()
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-ready:
				continue
			}
		}
		entry = &tokenCacheEntry{refreshing: true, ready: make(chan struct{})}
		s.entries[key] = entry
		s.mu.Unlock()

		token, expiresAt, fetchErr := s.fetch(ctx, normalized)
		s.mu.Lock()
		if fetchErr != nil {
			delete(s.entries, key)
		} else {
			lifetime := expiresAt.Sub(s.now())
			skew := 30 * time.Second
			if tenth := lifetime / 10; tenth < skew {
				skew = tenth
			}
			entry.token = token
			entry.validUntil = expiresAt.Add(-skew)
			entry.refreshing = false
		}
		close(entry.ready)
		s.mu.Unlock()
		if fetchErr != nil {
			return "", fetchErr
		}
		return token, nil
	}
}

func (s *oidcTokenSource) validateRequest(request TokenRequest) (TokenRequest, error) {
	request.TokenURL = strings.TrimSpace(request.TokenURL)
	request.ClientID = strings.TrimSpace(request.ClientID)
	request.Audience = strings.TrimSpace(request.Audience)
	request.Scope = strings.TrimSpace(request.Scope)
	if request.TokenURL == "" || request.ClientID == "" || request.ClientSecret == "" {
		return TokenRequest{}, authError(AuthErrorConfiguration, "configure",
			errors.New("token URL, client ID, and client secret are required"))
	}
	if request.Audience != "" && request.Scope != "" {
		return TokenRequest{}, authError(AuthErrorConfiguration, "configure",
			errors.New("configure audience or scope, not both"))
	}
	normalizedURL, err := validateTokenURL(request.TokenURL, s.allowHTTP)
	if err != nil {
		return TokenRequest{}, err
	}
	request.TokenURL = normalizedURL
	return request, nil
}

func validateTokenURL(raw string, allowHTTP bool) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", authError(AuthErrorConfiguration, "parse URL", err)
	}
	if parsed.Scheme != "https" && !(allowHTTP && parsed.Scheme == "http") {
		return "", authError(AuthErrorConfiguration, "validate URL",
			errors.New("token URL must use HTTPS"))
	}
	if parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", authError(AuthErrorConfiguration, "validate URL",
			errors.New("token URL must be absolute and contain no userinfo, query, or fragment"))
	}
	if parsed.Path == "" {
		parsed.Path = "/"
	}
	return parsed.String(), nil
}

func (s *oidcTokenSource) fetch(ctx context.Context, request TokenRequest) (string, time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", request.ClientID)
	form.Set("client_secret", request.ClientSecret)
	if request.Audience != "" {
		form.Set("audience", request.Audience)
	}
	if request.Scope != "" {
		form.Set("scope", request.Scope)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, request.TokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, authError(AuthErrorConfiguration, "create request", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := *s.client
	priorRedirect := client.CheckRedirect
	origin := req.URL
	client.CheckRedirect = func(next *http.Request, via []*http.Request) error {
		if len(via) > maxTokenRedirects {
			return errors.New("token endpoint redirect limit exceeded")
		}
		if next.Response == nil ||
			(next.Response.StatusCode != http.StatusTemporaryRedirect &&
				next.Response.StatusCode != http.StatusPermanentRedirect) {
			return errors.New("token endpoint redirect must use HTTP 307 or 308")
		}
		if next.URL.Scheme != origin.Scheme || !sameOrigin(next.URL, origin) {
			return errors.New("token endpoint redirect must remain on the same origin and scheme")
		}
		if next.Method != http.MethodPost || next.Body == nil {
			return errors.New("token endpoint redirect did not preserve POST body")
		}
		if priorRedirect != nil {
			return priorRedirect(next, via)
		}
		return nil
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", time.Time{}, authError(AuthErrorNetwork, "request", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", time.Time{}, authError(AuthErrorResponse, "request",
			fmt.Errorf("token endpoint returned HTTP %d", resp.StatusCode))
	}
	mediaType, _, err := mime.ParseMediaType(resp.Header.Get("Content-Type"))
	if err != nil {
		return "", time.Time{}, authError(AuthErrorResponse, "parse response content type",
			fmt.Errorf("invalid token response content type: %w", err))
	}
	if mediaType != "application/json" {
		return "", time.Time{}, authError(AuthErrorResponse, "validate response",
			errors.New("token endpoint must return application/json"))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxTokenResponseBytes+1))
	if err != nil {
		return "", time.Time{}, authError(AuthErrorNetwork, "read response", err)
	}
	if len(data) > maxTokenResponseBytes {
		return "", time.Time{}, authError(AuthErrorResponse, "validate response",
			errors.New("token response exceeds size limit"))
	}
	payload, err := parseTokenResponse(data)
	if err != nil {
		return "", time.Time{}, authError(AuthErrorResponse, "decode response",
			fmt.Errorf("invalid token response JSON: %w", err))
	}
	if !strings.EqualFold(payload.TokenType, "Bearer") {
		return "", time.Time{}, authError(AuthErrorResponse, "validate response",
			errors.New("token response token_type must be Bearer"))
	}
	if strings.TrimSpace(payload.AccessToken) == "" {
		return "", time.Time{}, authError(AuthErrorResponse, "validate response",
			errors.New("token response missing access_token"))
	}
	seconds, err := strconv.ParseInt(string(payload.ExpiresIn), 10, 64)
	if err != nil || seconds <= 0 || seconds > int64(math.MaxInt64)/int64(time.Second) {
		return "", time.Time{}, authError(AuthErrorResponse, "validate response",
			errors.New("token response expires_in must be a positive integer"))
	}
	return payload.AccessToken, s.now().Add(time.Duration(seconds) * time.Second), nil
}

type tokenResponse struct {
	TokenType   string
	AccessToken string
	ExpiresIn   json.Number
}

func parseTokenResponse(data []byte) (tokenResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	open, err := decoder.Token()
	if err != nil {
		return tokenResponse{}, err
	}
	if delimiter, ok := open.(json.Delim); !ok || delimiter != '{' {
		return tokenResponse{}, errors.New("token response must be a JSON object")
	}
	var payload tokenResponse
	seen := make(map[string]struct{}, 3)
	for decoder.More() {
		rawKey, err := decoder.Token()
		if err != nil {
			return tokenResponse{}, err
		}
		key, ok := rawKey.(string)
		if !ok {
			return tokenResponse{}, errors.New("token response field name must be a string")
		}
		if _, duplicate := seen[key]; duplicate {
			return tokenResponse{}, errors.New("token response contains a duplicate field")
		}
		seen[key] = struct{}{}
		switch key {
		case "token_type":
			if err := decoder.Decode(&payload.TokenType); err != nil {
				return tokenResponse{}, fmt.Errorf("decode token_type: %w", err)
			}
		case "access_token":
			if err := decoder.Decode(&payload.AccessToken); err != nil {
				return tokenResponse{}, fmt.Errorf("decode access_token: %w", err)
			}
		case "expires_in":
			if err := decoder.Decode(&payload.ExpiresIn); err != nil {
				return tokenResponse{}, fmt.Errorf("decode expires_in: %w", err)
			}
		default:
			return tokenResponse{}, errors.New("token response contains an unknown field")
		}
	}
	closeToken, err := decoder.Token()
	if err != nil {
		return tokenResponse{}, err
	}
	if delimiter, ok := closeToken.(json.Delim); !ok || delimiter != '}' {
		return tokenResponse{}, errors.New("token response object is not closed")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return tokenResponse{}, err
		}
		return tokenResponse{}, errors.New("token response contains a trailing JSON value")
	}
	return payload, nil
}

func authError(kind AuthErrorKind, operation string, err error) error {
	return &AuthError{Kind: kind, Operation: operation, Err: err}
}

func sameOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		effectivePort(left) == effectivePort(right)
}

func effectivePort(value *url.URL) string {
	if value.Port() != "" {
		return value.Port()
	}
	if value.Scheme == "https" {
		return "443"
	}
	if value.Scheme == "http" {
		return "80"
	}
	return ""
}
