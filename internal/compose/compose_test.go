package compose_test

import (
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/compose"
)

func TestBuildArgsUp(t *testing.T) {
	args := compose.BuildArgs("up", "camunda-lab", []string{"docker-compose.yaml"}, []string{"resources.env"}, "-d")
	got := strings.Join(args, " ")
	want := "compose -p camunda-lab -f docker-compose.yaml --env-file resources.env up -d"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestBuildArgsMultipleEnvFiles(t *testing.T) {
	args := compose.BuildArgs("up", "camunda-lab",
		[]string{"docker-compose.yaml"},
		[]string{".env", "resources.env"},
		"-d",
	)
	got := strings.Join(args, " ")
	want := "compose -p camunda-lab -f docker-compose.yaml --env-file .env --env-file resources.env up -d"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
