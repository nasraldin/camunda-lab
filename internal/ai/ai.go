package ai

import (
	"fmt"
	"os"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

// Secrets holds connector SECRET_* values for AI Agent providers.
type Secrets struct {
	OpenAIKey     string
	AnthropicKey  string
	OpenAIBaseURL string
}

func (s Secrets) Configured() bool {
	return s.OpenAIKey != "" || s.AnthropicKey != "" || s.OpenAIBaseURL != ""
}

func ValidateForEnable(minor, profile string, s Secrets) error {
	if err := versions.SupportsAIFeature(minor, profile); err != nil {
		return err
	}
	if !s.Configured() {
		return fmt.Errorf("set at least one of SECRET_OPENAI_API_KEY, SECRET_ANTHROPIC_API_KEY, or SECRET_OPENAI_BASE_URL (flags: --openai-key, --anthropic-key, --openai-base-url)")
	}
	return nil
}

func WriteSecrets(s Secrets) error {
	if err := os.MkdirAll(paths.Home(), 0o755); err != nil {
		return err
	}
	var b strings.Builder
	b.WriteString("# camunda-lab AI connector secrets — do not commit\n")
	if s.OpenAIKey != "" {
		fmt.Fprintf(&b, "SECRET_OPENAI_API_KEY=%s\n", s.OpenAIKey)
	}
	if s.AnthropicKey != "" {
		fmt.Fprintf(&b, "SECRET_ANTHROPIC_API_KEY=%s\n", s.AnthropicKey)
	}
	if s.OpenAIBaseURL != "" {
		fmt.Fprintf(&b, "SECRET_OPENAI_BASE_URL=%s\n", s.OpenAIBaseURL)
	}
	return os.WriteFile(paths.AIEnvFile(), []byte(b.String()), 0o600)
}

func LoadSecrets() (Secrets, error) {
	data, err := os.ReadFile(paths.AIEnvFile())
	if err != nil {
		if os.IsNotExist(err) {
			return Secrets{}, nil
		}
		return Secrets{}, err
	}
	var s Secrets
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "SECRET_OPENAI_API_KEY":
			s.OpenAIKey = v
		case "SECRET_ANTHROPIC_API_KEY":
			s.AnthropicKey = v
		case "SECRET_OPENAI_BASE_URL":
			s.OpenAIBaseURL = v
		}
	}
	return s, nil
}

func DeleteSecretsFile() error {
	err := os.Remove(paths.AIEnvFile())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func Mask(v string) string {
	if v == "" {
		return "(not set)"
	}
	if len(v) <= 4 {
		return "****"
	}
	return v[:2] + "…" + v[len(v)-2:]
}

// MCPClientConfig returns JSON for Cursor/Claude MCP client settings.
func MCPClientConfig(cfg config.Config) (string, error) {
	if err := versions.SupportsAIFeature(cfg.Version, cfg.Profile); err != nil {
		return "", err
	}
	host := cfg.Host
	if host == "" {
		host = "localhost"
	}
	port := 8080
	if cfg.Version == "8.8" {
		port = 8088
	}
	base := fmt.Sprintf("http://%s:%d", host, port)
	oauth := fmt.Sprintf("http://%s:18080/auth/realms/camunda-platform/protocol/openid-connect/token", host)
	cluster := base + "/mcp/cluster"
	processes := base + "/mcp/processes"

	if cfg.Profile == "full" {
		servers := fmt.Sprintf(`    "camunda-cluster": {
      "command": "npx",
      "args": ["-y", "@camunda8/cli", "mcp-proxy"],
      "env": {
        "CAMUNDA_BASE_URL": %q,
        "CAMUNDA_CLIENT_ID": "<client-id>",
        "CAMUNDA_CLIENT_SECRET": "<client-secret>",
        "CAMUNDA_OAUTH_URL": %q,
        "CAMUNDA_TOKEN_AUDIENCE": "orchestration-api"
      }
    }`, base, oauth)
		if versions.SupportsProcessesMCP(cfg.Version) {
			// Processes MCP shares the same OAuth proxy; clients that support path selection
			// can target /mcp/processes. Listed so 8.10 full matches light discoverability.
			servers += fmt.Sprintf(`,
    "camunda-processes": {
      "command": "npx",
      "args": ["-y", "@camunda8/cli", "mcp-proxy"],
      "env": {
        "CAMUNDA_BASE_URL": %q,
        "CAMUNDA_CLIENT_ID": "<client-id>",
        "CAMUNDA_CLIENT_SECRET": "<client-secret>",
        "CAMUNDA_OAUTH_URL": %q,
        "CAMUNDA_TOKEN_AUDIENCE": "orchestration-api",
        "CAMUNDA_MCP_PATH": "/mcp/processes"
      }
    }`, base, oauth)
		}
		out := fmt.Sprintf("{\n  \"mcpServers\": {\n%s\n  }\n}\n", servers)
		if versions.SupportsProcessesMCP(cfg.Version) {
			out += fmt.Sprintf("# Processes MCP endpoint: %s\n", processes)
		}
		return out, nil
	}

	servers := fmt.Sprintf(`    "camunda-cluster": {
      "type": "http",
      "url": %q
    }`, cluster)
	if versions.SupportsProcessesMCP(cfg.Version) {
		servers += fmt.Sprintf(`,
    "camunda-processes": {
      "type": "http",
      "url": %q
    }`, processes)
	}
	return fmt.Sprintf("{\n  \"mcpServers\": {\n%s\n  }\n}\n", servers), nil
}
