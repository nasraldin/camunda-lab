package cluster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

// applyAuth sets Authorization on the request.
func (c *Client) applyAuth(req *http.Request) {
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
		return
	}
	if c.BasicUser != "" {
		req.SetBasicAuth(c.BasicUser, c.BasicPass)
	}
}

// AttachLabAuth fills Token (or basic) for the active lab / remote env.
// Full Compose labs use Keycloak client credentials (connectors client from version .env).
// Light labs leave auth empty (unprotected API by default).
func AttachLabAuth(ctx context.Context, c *Client, labHome string, cfg config.Config) error {
	if c == nil {
		return nil
	}
	if t := strings.TrimSpace(os.Getenv("CAMUNDA_ACCESS_TOKEN")); t != "" {
		c.Token = t
		return nil
	}
	active := env.GetActive(labHome)
	if active != "" && active != "lab" {
		tok, err := tokenFromRemoteProfile(ctx, labHome, active)
		if err != nil {
			return err
		}
		c.Token = tok
		return nil
	}
	if cfg.Profile != "full" {
		if os.Getenv("CAMUNDA_BASIC_USER") != "" {
			c.BasicUser = os.Getenv("CAMUNDA_BASIC_USER")
			c.BasicPass = os.Getenv("CAMUNDA_BASIC_PASSWORD")
			if c.BasicPass == "" {
				c.BasicPass = "demo"
			}
		}
		return nil
	}
	tok, err := FetchLabOIDCToken(ctx, cfg)
	if err != nil {
		return fmt.Errorf("OIDC token for full lab: %w", err)
	}
	c.Token = tok
	return nil
}

// FetchLabOIDCToken uses the official Compose connectors client against Keycloak.
func FetchLabOIDCToken(ctx context.Context, cfg config.Config) (string, error) {
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	vals := loadComposeEnv(filepath.Join(paths.VersionDir(cfg.Version), ".env"))
	clientID := firstNonEmpty(
		os.Getenv("CAMUNDA_CLIENT_ID"),
		vals["CONNECTORS_CLIENT_ID"],
		"connectors",
	)
	clientSecret := firstNonEmpty(
		os.Getenv("CAMUNDA_CLIENT_SECRET"),
		vals["CONNECTORS_CLIENT_SECRET"],
		vals["ORCHESTRATION_CLIENT_SECRET"],
	)
	if clientSecret == "" {
		return "", fmt.Errorf("no client secret (set CAMUNDA_CLIENT_SECRET or install a full lab)")
	}
	tokenURL := firstNonEmpty(
		os.Getenv("CAMUNDA_OAUTH_URL"),
		fmt.Sprintf("http://%s:18080/auth/realms/camunda-platform/protocol/openid-connect/token", host),
	)
	audience := firstNonEmpty(os.Getenv("CAMUNDA_TOKEN_AUDIENCE"), "orchestration-api")
	return fetchClientCredentials(ctx, tokenURL, clientID, clientSecret, audience)
}

func tokenFromRemoteProfile(ctx context.Context, labHome, name string) (string, error) {
	ps, err := env.ListProfiles(filepath.Join(labHome, "envs"))
	if err != nil {
		return "", err
	}
	var p env.Profile
	found := false
	for _, x := range ps {
		if x.Name == name {
			p = x
			found = true
			break
		}
	}
	if !found {
		return "", fmt.Errorf("env profile %q not found", name)
	}
	id := os.Getenv(p.Auth.ClientIDEnv)
	secret := os.Getenv(p.Auth.ClientSecretEnv)
	if id == "" || secret == "" {
		return "", fmt.Errorf("set %s and %s for remote env %q", p.Auth.ClientIDEnv, p.Auth.ClientSecretEnv, name)
	}
	tokenURL := "http://localhost:18080/auth/realms/camunda-platform/protocol/openid-connect/token"
	if p.Auth.TokenURLEnv != "" {
		if v := os.Getenv(p.Auth.TokenURLEnv); v != "" {
			tokenURL = v
		}
	}
	audience := firstNonEmpty(os.Getenv("CAMUNDA_TOKEN_AUDIENCE"), "orchestration-api")
	return fetchClientCredentials(ctx, tokenURL, id, secret, audience)
}

func fetchClientCredentials(ctx context.Context, tokenURL, clientID, clientSecret, audience string) (string, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", clientID)
	form.Set("client_secret", clientSecret)
	if audience != "" {
		form.Set("audience", audience)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("token endpoint HTTP %d: %s", resp.StatusCode, truncate(string(data), 160))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(data, &out); err != nil {
		return "", err
	}
	if out.AccessToken == "" {
		return "", fmt.Errorf("token response missing access_token")
	}
	return out.AccessToken, nil
}

func loadComposeEnv(path string) map[string]string {
	out := map[string]string{}
	data, err := os.ReadFile(path)
	if err != nil {
		return out
	}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	return out
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}
