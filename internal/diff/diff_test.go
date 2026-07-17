package diff

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func fixture(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", rel)
}

func TestCompareOrderVersions(t *testing.T) {
	a, err := bpmn.ParseFile(fixture(t, "bpmn/order-v1.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := bpmn.ParseFile(fixture(t, "bpmn/order-v2.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	changes := Compare(a, b)
	if len(changes) == 0 {
		t.Fatal("expected changes")
	}
	var addedPayment, timerChanged bool
	for _, c := range changes {
		if c.Kind == ElementAdded && c.ID == "Payment" {
			addedPayment = true
		}
		if c.Kind == AttrChanged && c.ID == "Wait" {
			timerChanged = true
		}
	}
	if !addedPayment {
		t.Fatalf("expected Payment added: %#v", changes)
	}
	if !timerChanged {
		t.Fatalf("expected timer change: %#v", changes)
	}
}

func TestCompareIdentical(t *testing.T) {
	a, err := bpmn.ParseFile(fixture(t, "bpmn/order-v1.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	if len(Compare(a, a)) != 0 {
		t.Fatal("expected no changes")
	}
}
