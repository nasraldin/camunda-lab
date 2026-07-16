package urls_test

import (
	"testing"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

func mustURL(t *testing.T, cfg config.Config, name, want string) {
	t.Helper()
	e, err := urls.Find(cfg, name)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	if e.URL != want {
		t.Fatalf("%s: got %q want %q", name, e.URL, want)
	}
}

func TestLight87Operate(t *testing.T) {
	cfg := config.Config{Version: "8.7", Profile: "light", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8081")
	mustURL(t, cfg, "tasklist", "http://localhost:8082")
	mustURL(t, cfg, "connectors", "http://localhost:8085")
	mustURL(t, cfg, "rest", "http://localhost:8088")
}

func TestFull87NoConsole(t *testing.T) {
	cfg := config.Config{Version: "8.7", Profile: "full", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8081")
	mustURL(t, cfg, "optimize", "http://localhost:8083")
	mustURL(t, cfg, "web-modeler", "http://localhost:8070")
	if _, err := urls.Find(cfg, "console"); err == nil {
		t.Fatal("8.7 full should not list console")
	}
}

func TestLight88Uses8088(t *testing.T) {
	cfg := config.Config{Version: "8.8", Profile: "light", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8088/operate")
	mustURL(t, cfg, "tasklist", "http://localhost:8088/tasklist")
	mustURL(t, cfg, "admin", "http://localhost:8088/admin")
	mustURL(t, cfg, "rest", "http://localhost:8088/v2")
	mustURL(t, cfg, "connectors", "http://localhost:8086")
	if got := urls.ModelerRESTBase(cfg); got != "http://localhost:8088" {
		t.Fatalf("modeler rest base: %s", got)
	}
}

func TestFull88ConsoleAnd8088(t *testing.T) {
	cfg := config.Config{Version: "8.8", Profile: "full", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8088/operate")
	mustURL(t, cfg, "console", "http://localhost:8087")
	mustURL(t, cfg, "keycloak", "http://localhost:18080/auth/")
}

func TestLight89Uses8080(t *testing.T) {
	cfg := config.Config{Version: "8.9", Profile: "light", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8080/operate")
	mustURL(t, cfg, "rest", "http://localhost:8080/v2")
	if got := urls.ModelerRESTBase(cfg); got != "http://localhost:8080" {
		t.Fatalf("modeler rest base: %s", got)
	}
}

func TestLight810NoBundledES(t *testing.T) {
	cfg := config.Config{Version: "8.10", Profile: "light", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8080/operate")
	if _, err := urls.Find(cfg, "elasticsearch"); err == nil {
		t.Fatal("8.10 light should not list bundled ES")
	}
}

func TestFull810NoConsoleHasES(t *testing.T) {
	cfg := config.Config{Version: "8.10", Profile: "full", Host: "localhost"}
	mustURL(t, cfg, "operate", "http://localhost:8080/operate")
	mustURL(t, cfg, "web-modeler", "http://localhost:8070")
	mustURL(t, cfg, "elasticsearch", "http://localhost:9200")
	if _, err := urls.Find(cfg, "console"); err == nil {
		t.Fatal("8.10 full should not list console")
	}
}

func TestElasticvueWhenHostES(t *testing.T) {
	mustURL(t, config.Config{Version: "8.8", Profile: "light", Host: "localhost"},
		"elasticvue", "http://localhost:9800")
	mustURL(t, config.Config{Version: "8.9", Profile: "full", Host: "localhost"},
		"elasticvue", "http://localhost:9800")
	mustURL(t, config.Config{Version: "8.10", Profile: "full", Host: "localhost"},
		"elasticvue", "http://localhost:9800")
	if _, err := urls.Find(config.Config{Version: "8.9", Profile: "light", Host: "localhost"}, "elasticvue"); err == nil {
		t.Fatal("8.9 light should not list elasticvue (no host ES)")
	}
	if _, err := urls.Find(config.Config{Version: "8.9", Profile: "light", Host: "localhost"}, "elasticsearch"); err == nil {
		t.Fatal("8.9 light should not list elasticsearch")
	}
	if _, err := urls.Find(config.Config{Version: "8.10", Profile: "light", Host: "localhost"}, "elasticvue"); err == nil {
		t.Fatal("8.10 light should not list elasticvue")
	}
	if _, err := urls.Find(config.Config{Version: "8.9", Profile: "modeler", Host: "localhost"}, "elasticvue"); err == nil {
		t.Fatal("modeler should not list elasticvue")
	}
}

func TestMCPURLsWhenAIEnabled(t *testing.T) {
	cfg := config.Config{Version: "8.9", Profile: "light", Host: "localhost", AI: config.AIConfig{Enabled: true}}
	mustURL(t, cfg, "mcp-cluster", "http://localhost:8080/mcp/cluster")
	if _, err := urls.Find(cfg, "mcp-processes"); err == nil {
		t.Fatal("8.9 should not list processes MCP")
	}
}

func TestMCPProcessesURL810(t *testing.T) {
	cfg := config.Config{Version: "8.10", Profile: "light", Host: "localhost", AI: config.AIConfig{Enabled: true}}
	mustURL(t, cfg, "mcp-processes", "http://localhost:8080/mcp/processes")
}

func TestMCPURLsHiddenWhenAIDisabled(t *testing.T) {
	cfg := config.Config{Version: "8.9", Profile: "light", Host: "localhost"}
	if _, err := urls.Find(cfg, "mcp-cluster"); err == nil {
		t.Fatal("expected hidden")
	}
}
