package display_test

import (
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/display"
)

func TestReportWrite(t *testing.T) {
	var b strings.Builder
	display.Report{
		Title: "Camunda Lab Doctor",
		Fields: []display.Field{
			display.KV("Version", "8.9"),
			display.KV("Profile", "light"),
		},
		Sections: []display.Section{
			{Title: "Checks", Items: []string{display.Success("docker"), display.Info("cosign optional")}},
		},
		Footer: []string{"Lab looks healthy."},
	}.Write(&b)
	out := b.String()
	for _, want := range []string{
		"Camunda Lab Doctor",
		"Version  8.9",
		"Checks",
		"pass  docker",
		"Lab looks healthy.",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
}
