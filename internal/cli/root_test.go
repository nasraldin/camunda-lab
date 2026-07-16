package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cli"
)

func TestRootHelp(t *testing.T) {
	cli.SetVersion("1.2.3")
	cmd := cli.NewRoot()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"camunda", "install", "doctor", "switch"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}

func TestAboutCard(t *testing.T) {
	cli.SetVersion("1.2.3")
	cmd := cli.NewRoot()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"about"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{
		"Camunda Lab",
		"Author      Nasr Aldin",
		"Website     https://nasraldin.com",
		"Version     1.2.3",
		"Tagline     Local Camunda 8 platform lab (Docker Compose)",
		"Lab home",
		"Features",
		"Commands",
		"Not affiliated with Camunda GmbH",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("about missing %q:\n%s", want, out)
		}
	}
}
