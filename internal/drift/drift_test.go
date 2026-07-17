package drift

import (
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/plan"
)

func TestCompareDrift(t *testing.T) {
	local := []plan.Resource{{Key: "order.bpmn", Digest: "aaa"}}
	remote := []plan.Resource{{Key: "order.bpmn", Digest: "bbb", Version: "12"}}
	r := Compare("prod", local, remote)
	if !HasDrift(r) {
		t.Fatal("expected drift")
	}
	if r.Entries[0].Status != "DRIFT" {
		t.Fatalf("%+v", r.Entries)
	}
	text := FormatText(r)
	if !strings.Contains(text, "DRIFT") {
		t.Fatal(text)
	}
}

func TestInSync(t *testing.T) {
	local := []plan.Resource{{Key: "a.bpmn", Digest: "x"}}
	remote := []plan.Resource{{Key: "a.bpmn", Digest: "x"}}
	r := Compare("lab", local, remote)
	if HasDrift(r) {
		t.Fatal(r)
	}
}
