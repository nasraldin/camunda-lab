package sso

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

const (
	defaultUser = "demo"
	defaultPass = "demo"
	clientID    = "orchestration"
)

var (
	formActionRE = regexp.MustCompile(`(?i)<form[^>]+id="kc-form-login"[^>]+action="([^"]+)"`)
	actionAnyRE  = regexp.MustCompile(`(?i)<form[^>]+action="([^"]+)"`)
)

// collectingJar wraps a jar and records every Set-Cookie seen on responses
// (Go's cookiejar omits Secure cookies from Cookies() for http:// URLs).
type collectingJar struct {
	http.CookieJar
	mu   sync.Mutex
	seen map[string]*http.Cookie
}

func newCollectingJar() (*collectingJar, error) {
	inner, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	return &collectingJar{CookieJar: inner, seen: map[string]*http.Cookie{}}, nil
}

func (j *collectingJar) SetCookies(u *url.URL, cookies []*http.Cookie) {
	j.CookieJar.SetCookies(u, cookies)
	j.mu.Lock()
	defer j.mu.Unlock()
	for _, c := range cookies {
		cp := *c
		j.seen[c.Name] = &cp
	}
}

func (j *collectingJar) snapshot() []*http.Cookie {
	j.mu.Lock()
	defer j.mu.Unlock()
	out := make([]*http.Cookie, 0, len(j.seen))
	for _, c := range j.seen {
		out = append(out, c)
	}
	return out
}

// SessionCookies logs into the camunda-platform Keycloak realm and returns
// SSO cookies suitable for Set-Cookie on http://localhost.
func SessionCookies(ctx context.Context, keycloakAuthBase string) ([]*http.Cookie, error) {
	return SessionCookiesWithCreds(ctx, keycloakAuthBase, defaultUser, defaultPass)
}

// SessionCookiesWithCreds is like SessionCookies with explicit credentials.
func SessionCookiesWithCreds(ctx context.Context, keycloakAuthBase, username, password string) ([]*http.Cookie, error) {
	base := strings.TrimRight(strings.TrimSpace(keycloakAuthBase), "/")
	if base == "" {
		return nil, fmt.Errorf("keycloak auth base is empty")
	}
	if !strings.Contains(base, "/auth") {
		base += "/auth"
	}

	kcURL, err := url.Parse(base)
	if err != nil {
		return nil, fmt.Errorf("keycloak auth base: %w", err)
	}
	kcHost := strings.ToLower(kcURL.Hostname())
	kcPort := kcURL.Port()
	if kcPort == "" {
		if kcURL.Scheme == "https" {
			kcPort = "443"
		} else {
			kcPort = "80"
		}
	}

	jar, err := newCollectingJar()
	if err != nil {
		return nil, err
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 45 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) > 8 {
				return fmt.Errorf("too many redirects")
			}
			// Stop once Keycloak sends us to the app callback (leave Keycloak host/port).
			reqPort := req.URL.Port()
			if reqPort == "" {
				if req.URL.Scheme == "https" {
					reqPort = "443"
				} else {
					reqPort = "80"
				}
			}
			stillOnKC := strings.EqualFold(req.URL.Hostname(), kcHost) && reqPort == kcPort
			if !stillOnKC && !strings.Contains(req.URL.Path, "/auth/") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}

	verifier := randomB64(32)
	sum := sha256.Sum256([]byte(verifier))
	challenge := base64.RawURLEncoding.EncodeToString(sum[:])

	authURL := fmt.Sprintf(
		"%s/realms/camunda-platform/protocol/openid-connect/auth?response_type=code&client_id=%s&scope=%s&redirect_uri=%s&state=%s&nonce=%s&code_challenge=%s&code_challenge_method=S256",
		base,
		url.QueryEscape(clientID),
		url.QueryEscape("openid profile"),
		url.QueryEscape("http://localhost:8080/sso-callback"),
		url.QueryEscape("camunda-lab"),
		url.QueryEscape("camunda-lab"),
		url.QueryEscape(challenge),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, authURL, nil)
	if err != nil {
		return nil, err
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("keycloak auth page: %w", err)
	}
	body, _ := io.ReadAll(res.Body)
	_ = res.Body.Close()
	if res.StatusCode >= 400 {
		return nil, fmt.Errorf("keycloak auth page: HTTP %d", res.StatusCode)
	}

	action := formAction(string(body))
	if action == "" {
		return nil, fmt.Errorf("keycloak login form not found (is the full lab with Keycloak running?)")
	}

	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)
	form.Set("credentialId", "")

	preq, err := http.NewRequestWithContext(ctx, http.MethodPost, action, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	preq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	pres, err := client.Do(preq)
	if err != nil {
		return nil, fmt.Errorf("keycloak login: %w", err)
	}
	_, _ = io.Copy(io.Discard, pres.Body)
	_ = pres.Body.Close()

	want := map[string]bool{
		"KEYCLOAK_SESSION":         true,
		"KEYCLOAK_IDENTITY":        true,
		"KEYCLOAK_SESSION_LEGACY":  true,
		"KEYCLOAK_IDENTITY_LEGACY": true,
	}
	out := make([]*http.Cookie, 0, 4)
	for _, c := range jar.snapshot() {
		if want[c.Name] && c.Value != "" {
			out = append(out, cloneCookie(c))
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("keycloak login did not set SSO cookies (check demo/demo user)")
	}
	return out, nil
}

func formAction(html string) string {
	var raw string
	if m := formActionRE.FindStringSubmatch(html); len(m) > 1 {
		raw = m[1]
	} else if m := actionAnyRE.FindStringSubmatch(html); len(m) > 1 {
		raw = m[1]
	}
	return strings.ReplaceAll(raw, "&amp;", "&")
}

func cloneCookie(c *http.Cookie) *http.Cookie {
	cp := *c
	cp.Domain = ""
	cp.Secure = true
	cp.SameSite = http.SameSiteNoneMode
	if cp.Path == "" {
		cp.Path = "/auth/realms/camunda-platform/"
	}
	return &cp
}

func randomB64(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// WriteCookies adds cookies to the response for the browser.
func WriteCookies(w http.ResponseWriter, cookies []*http.Cookie) {
	for _, c := range cookies {
		http.SetCookie(w, c)
	}
}
