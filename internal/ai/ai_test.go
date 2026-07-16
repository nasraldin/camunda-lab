package ai_test

import (
	"os"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestSecretsConfigured(t *testing.T) {
	if (ai.Secrets{}).Configured() {
		t.Fatal("empty")
	}
	if !(ai.Secrets{OpenAIKey: "sk"}).Configured() {
		t.Fatal("openai")
	}
	if !(ai.Secrets{OpenAIBaseURL: "http://localhost:11434/v1"}).Configured() {
		t.Fatal("base url")
	}
}

func TestWriteAndLoadSecrets(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()

	in := ai.Secrets{OpenAIKey: "sk-test", AnthropicKey: "ant-test", OpenAIBaseURL: "http://host.docker.internal:11434/v1"}
	if err := ai.WriteSecrets(in); err != nil {
		t.Fatal(err)
	}
	got, err := ai.LoadSecrets()
	if err != nil {
		t.Fatal(err)
	}
	if got != in {
		t.Fatalf("%+v != %+v", got, in)
	}
	fi, err := os.Stat(paths.AIEnvFile())
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o077 != 0 {
		t.Fatalf("ai.env should not be group/world readable, mode=%v", fi.Mode())
	}
}

func TestMCPClientConfigLight(t *testing.T) {
	cfg := config.Config{Version: "8.9", Profile: "light", Host: "localhost", AI: config.AIConfig{Enabled: true}}
	out, err := ai.MCPClientConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "http://localhost:8080/mcp/cluster") {
		t.Fatalf("%s", out)
	}
	if strings.Contains(out, "mcp-proxy") {
		t.Fatal("light should not use mcp-proxy")
	}
}

func TestMCPClientConfigFull(t *testing.T) {
	cfg := config.Config{Version: "8.9", Profile: "full", Host: "myhost", AI: config.AIConfig{Enabled: true}}
	out, err := ai.MCPClientConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "mcp-proxy") {
		t.Fatalf("%s", out)
	}
	if !strings.Contains(out, "http://myhost:18080/auth/realms/camunda-platform/protocol/openid-connect/token") {
		t.Fatalf("oauth host: %s", out)
	}
	if strings.Contains(out, "camunda-processes") {
		t.Fatal("8.9 full should not list processes MCP")
	}
}

func TestMCPClientConfigFull810IncludesProcesses(t *testing.T) {
	cfg := config.Config{Version: "8.10", Profile: "full", Host: "localhost", AI: config.AIConfig{Enabled: true}}
	out, err := ai.MCPClientConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "camunda-processes") {
		t.Fatalf("%s", out)
	}
}

func TestMCPClientConfig810IncludesProcesses(t *testing.T) {
	cfg := config.Config{Version: "8.10", Profile: "light", Host: "localhost", AI: config.AIConfig{Enabled: true}}
	out, err := ai.MCPClientConfig(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "/mcp/processes") {
		t.Fatalf("%s", out)
	}
}
