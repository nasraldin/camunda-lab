package inventory_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/inventory"
)

const bpmnHeader = `<?xml version="1.0" encoding="UTF-8"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`

func TestCanonicalSemanticEquivalentBPMN(t *testing.T) {
	left := bpmnHeader + `<process id="order" name="Order">
		<startEvent id="start" name="Start"/><endEvent id="end" name="End"/>
		<sequenceFlow id="flow" sourceRef="start" targetRef="end"/>
	</process></definitions>`
	right := bpmnHeader + `<process name="Order" id="order">
		<endEvent name="End" id="end"/>
		<sequenceFlow targetRef="end" sourceRef="start" id="flow"/>
		<startEvent name="Start" id="start"/>
	</process></definitions>`

	a, err := inventory.DigestCanonical(inventory.KindProcess, []byte(left))
	if err != nil {
		t.Fatal(err)
	}
	b, err := inventory.DigestCanonical(inventory.KindProcess, []byte(right))
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a != b {
		t.Fatalf("digests = %q and %q", a, b)
	}
}

func TestResourceIDsRejectDuplicateAndMissingProcessIDs(t *testing.T) {
	for _, body := range []string{
		bpmnHeader + `<process id="same"><startEvent id="a"/></process><process id="same"><startEvent id="b"/></process></definitions>`,
		bpmnHeader + `<process><startEvent id="a"/></process></definitions>`,
	} {
		if _, err := inventory.ResourceIDs(inventory.KindProcess, []byte(body)); err == nil {
			t.Fatalf("ResourceIDs accepted %s", body)
		}
	}
}

func TestBuildLocalUsesConfiguredRecursivePathsAndMultiProcessIdentity(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".camunda.yaml"), `name: orders
camundaVersion: "8.9"
paths:
  bpmn: workflows
  dmn: decisions
  forms: ui/forms
  tests: tests
`)
	write(t, filepath.Join(root, "workflows", "nested", "orders.bpmn"), bpmnHeader+
		`<process id="order"><startEvent id="a"/></process>`+
		`<process id="refund"><startEvent id="b"/></process></definitions>`)
	write(t, filepath.Join(root, "decisions", "risk.dmn"), `<?xml version="1.0"?><definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"><decision id="risk" name="Risk"><decisionTable id="risk-table"/></decision></definitions>`)
	write(t, filepath.Join(root, "ui", "forms", "approval.form"), `{"schemaVersion":18,"id":"approval","components":[],"type":"default"}`)

	got, err := inventory.BuildLocal(inventory.LocalRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Resources) != 4 {
		t.Fatalf("resources = %+v", got.Resources)
	}
	want := []string{"decision:risk", "form:approval", "process:order", "process:refund"}
	for i, resource := range got.Resources {
		if resource.Kind.String()+":"+resource.ID != want[i] {
			t.Fatalf("resource[%d] = %+v, want %s", i, resource, want[i])
		}
		if resource.Digest == "" || filepath.IsAbs(resource.Path) {
			t.Fatalf("resource not canonical/project-relative: %+v", resource)
		}
	}
	if err := got.ValidateComparable(); err != nil {
		t.Fatal(err)
	}
}

func TestBuildLocalRejectsDuplicateIDsAcrossFiles(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, ".camunda.yaml"), `name: orders
paths: {bpmn: bpmn, dmn: dmn, forms: forms, tests: tests}
`)
	for _, name := range []string{"a.bpmn", "b.bpmn"} {
		write(t, filepath.Join(root, "bpmn", name), bpmnHeader+
			`<process id="duplicate"><startEvent id="start-`+name+`"/></process></definitions>`)
	}
	_, err := inventory.BuildLocal(inventory.LocalRequest{Root: root})
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("error = %v", err)
	}
}

func TestInventoryRejectsEmptyDigest(t *testing.T) {
	value := inventory.Inventory{Resources: []inventory.Resource{{
		Kind: inventory.KindProcess, ID: "orders",
	}}}
	if err := value.ValidateComparable(); err == nil {
		t.Fatal("empty digest inventory is comparable")
	}
}

func TestUnsupportedKindIsNotComparable(t *testing.T) {
	value := inventory.Inventory{Unsupported: []inventory.Unsupported{{
		Kind: inventory.KindForm, Reason: "cluster endpoint unavailable", Required: true,
	}}}
	if err := value.ValidateComparable(); err == nil {
		t.Fatal("required unsupported kind is comparable")
	}
}

func TestCanonicalJSONRejectsDuplicateAndTrailingValues(t *testing.T) {
	for _, raw := range []string{
		`{"id":"one","id":"two"}`,
		`{"id":"one"} {"id":"two"}`,
	} {
		if _, err := inventory.Canonicalize(inventory.KindForm, []byte(raw)); err == nil {
			t.Fatalf("Canonicalize accepted %q", raw)
		}
	}
}

func TestCanonicalDecisionRejectsTrailingContent(t *testing.T) {
	raw := `<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"><decision id="risk"/></definitions>junk`
	if _, err := inventory.Canonicalize(inventory.KindDecision, []byte(raw)); err == nil {
		t.Fatal("decision canonicalization accepted trailing content")
	}
}

func TestCanonicalizeRejectsUnsupportedKind(t *testing.T) {
	_, err := inventory.Canonicalize(inventory.Kind("job"), []byte(`{}`))
	if err == nil || !errors.Is(err, inventory.ErrUnsupportedKind) {
		t.Fatalf("error = %v", err)
	}
}

func write(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
