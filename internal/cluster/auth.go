package cluster

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/config"
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

// AttachLabAuth is a compatibility adapter. New production code must obtain
// clients through Factory.Client so project-local environment selection is
// preserved.
func AttachLabAuth(ctx context.Context, c *Client, labHome string, cfg config.Config) error {
	if c == nil {
		return nil
	}
	built, _, err := NewFactory(labHome, cfg).Client(ctx, "", "")
	if err != nil {
		return err
	}
	c.HTTPClient = built.HTTPClient
	c.Kind = built.Kind
	c.Token = built.Token
	c.BasicUser = built.BasicUser
	c.BasicPass = built.BasicPass
	return nil
}

// FetchLabOIDCToken is retained for callers that need a raw local-lab token.
// Environment-aware cluster operations must use Factory.Client instead.
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
	source := newOIDCTokenSource(tokenSourceOptions{AllowHTTP: isLoopbackHTTP(tokenURL)})
	return source.Token(ctx, TokenRequest{
		TokenURL: tokenURL, ClientID: clientID, ClientSecret: clientSecret, Audience: audience,
	})
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
