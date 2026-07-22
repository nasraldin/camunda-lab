package inventory_test

import (
	"reflect"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func TestDecisionCanonicalizationValidatesDMNSchemaShape(t *testing.T) {
	valid := `<?xml version="1.0"?><definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"><decision id="risk" name="Risk"><decisionTable id="table"/></decision></definitions>`
	if _, err := inventory.Canonicalize(inventory.KindDecision, []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`<process xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"><decision id="risk"><decisionTable/></decision></process>`,
		`<definitions xmlns="urn:not-dmn"><decision id="risk"><decisionTable/></decision></definitions>`,
		`<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"></definitions>`,
		`<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/"><decision id="risk"/></definitions>`,
	} {
		if _, err := inventory.Canonicalize(inventory.KindDecision, []byte(raw)); err == nil {
			t.Fatalf("accepted invalid DMN: %s", raw)
		}
	}
}

func TestFormCanonicalizationValidatesCamundaSchemaShape(t *testing.T) {
	valid := `{"schemaVersion":18,"id":"approval","components":[],"type":"default"}`
	if _, err := inventory.Canonicalize(inventory.KindForm, []byte(valid)); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{
		`{"id":"approval","components":[],"type":"default"}`,
		`{"schemaVersion":18,"components":[],"type":"default"}`,
		`{"schemaVersion":18,"id":"approval","type":"default"}`,
		`{"schemaVersion":"18","id":"approval","components":[],"type":"default"}`,
		`{"schemaVersion":18,"id":"approval","components":{},"type":"default"}`,
		`{"schemaVersion":18,"id":"approval","components":[],"type":7}`,
		`{"schemaVersion":18,"id":"approval","components":[],"type":"unsupported"}`,
	} {
		if _, err := inventory.Canonicalize(inventory.KindForm, []byte(raw)); err == nil {
			t.Fatalf("accepted invalid form: %s", raw)
		}
	}
}

func TestCanonicalProcessIncludesOnlyRelevantDocumentDefinitions(t *testing.T) {
	left := `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
	  <message id="order-message" name="Order"/><message id="refund-message" name="Refund A"/>
	  <process id="orders"><startEvent id="start"><messageEventDefinition messageRef="order-message"/></startEvent></process>
	  <process id="refunds"><startEvent id="refund"><messageEventDefinition messageRef="refund-message"/></startEvent></process>
	</definitions>`
	right := strings.Replace(left, `name="Refund A"`, `name="Refund B"`, 1)
	a, err := inventory.DigestCanonicalProcess([]byte(left), "orders")
	if err != nil {
		t.Fatal(err)
	}
	b, err := inventory.DigestCanonicalProcess([]byte(right), "orders")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Fatalf("unrelated definition changed digest: %s != %s", a, b)
	}
}

func TestDecisionResourceIDsOnlyUseDirectDMNDecisions(t *testing.T) {
	raw := `<?xml version="1.0"?>
	<definitions xmlns="https://www.omg.org/spec/DMN/20191111/MODEL/" xmlns:ext="urn:extension">
	  <decision id="z-direct"><decisionTable id="z-table"/></decision>
	  <extensionElements>
	    <decision id="z-direct"><decisionTable id="nested-table"/></decision>
	    <ext:decision id="foreign"/>
	  </extensionElements>
	  <ext:decision id="foreign-direct"/>
	  <decision id="a-direct"><literalExpression id="a-expression"><text>true</text></literalExpression></decision>
	</definitions>`
	got, err := inventory.ResourceIDs(inventory.KindDecision, []byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a-direct", "z-direct"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("decision IDs = %v, want %v", got, want)
	}
}
