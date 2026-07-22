package compose_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/compose"
)

func TestRemoveByNameIgnoresMissingInError(t *testing.T) {
	r := &compose.Runner{
		Exec: func(_ string, args []string) (string, error) {
			if len(args) >= 4 && args[2] == "-f" && args[3] == "gone" {
				// Mimic defaultExec: stderr text lives on the error, stdout empty.
				return "", errors.New("Error response from daemon: No such container: gone")
			}
			return "ok", nil
		},
	}
	if err := r.RemoveByName("gone", "present"); err != nil {
		t.Fatalf("expected missing container to be ignored: %v", err)
	}
}

func TestRemoveByNamePropagatesRealErrors(t *testing.T) {
	r := &compose.Runner{
		Exec: func(_ string, args []string) (string, error) {
			return "", errors.New("permission denied")
		},
	}
	if err := r.RemoveByName("stuck"); err == nil {
		t.Fatal("expected real docker error to propagate")
	}
}

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
