package lab

import (
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/config"
)

func TestFormatStatus(t *testing.T) {
	cfg := config.Config{
		Version:        "8.9",
		Profile:        "light",
		Resources:      "small",
		Host:           "localhost",
		ComposeProject: "camunda-lab",
	}

	raw := strings.Join([]string{
		`{"Name":"orchestration","Service":"orchestration","Image":"camunda/camunda:8.9.13","State":"running","Health":"healthy","Status":"Up 16 seconds (healthy)","RunningFor":"16 seconds ago","Publishers":[{"PublishedPort":8080,"TargetPort":8080,"Protocol":"tcp"},{"PublishedPort":26500,"TargetPort":26500,"Protocol":"tcp"},{"PublishedPort":9600,"TargetPort":9600,"Protocol":"tcp"}]}`,
		`{"Name":"connectors","Service":"connectors","Image":"camunda/connectors-bundle:8.9.6","State":"running","Health":"healthy","Status":"Up 6 seconds (healthy)","RunningFor":"6 seconds ago","Publishers":[{"PublishedPort":8086,"TargetPort":8080,"Protocol":"tcp"}]}`,
	}, "\n")

	out, err := formatStatus(cfg, "/tmp/camunda/8.9", raw)
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"Camunda Lab Status",
		"Version    8.9",
		"Services   2 total, 2 running, 2 healthy",
		"operate -> http://localhost:8080/operate",
		"tasklist -> http://localhost:8080/tasklist",
		"connectors -> http://localhost:8086/actuator/health",
		"- connectors",
		"ports   8086->8080/tcp",
		"- orchestration",
		"ports   26500->26500/tcp, 8080->8080/tcp, 9600->9600/tcp",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}

func TestParsePSJSONRejectsBadLine(t *testing.T) {
	_, err := parsePSJSON("{bad json}")
	if err == nil {
		t.Fatal("expected parse error")
	}
}
