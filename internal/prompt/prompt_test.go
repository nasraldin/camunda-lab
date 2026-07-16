package prompt_test

import (
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/prompt"
)

func TestChooseDefault(t *testing.T) {
	r := strings.NewReader("\n")
	var w strings.Builder
	got, err := prompt.Choose(r, &w, "Pick:", []string{"a", "b", "c"}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if got != "b" {
		t.Fatalf("%q", got)
	}
}

func TestChooseNumber(t *testing.T) {
	r := strings.NewReader("3\n")
	var w strings.Builder
	got, err := prompt.Choose(r, &w, "Pick:", []string{"a", "b", "c"}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got != "c" {
		t.Fatalf("%q", got)
	}
}
