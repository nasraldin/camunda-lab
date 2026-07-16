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

func TestBuildArgsDownVolumes(t *testing.T) {
	args := compose.BuildArgs("down", "camunda-lab", []string{"docker-compose-full.yaml"}, nil, "-v", "--remove-orphans")
	got := strings.Join(args, " ")
	if !strings.Contains(got, "down -v --remove-orphans") {
		t.Fatalf("%q", got)
	}
}
