package lint

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

func TestProcessStartEvent(t *testing.T) {
	m, err := bpmn.ParseFile(fixture(t, "bpmn/lint/broken.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	findings := Run(m, Options{File: "broken.bpmn"})
	if !hasRule(findings, "bpmn/process-start-event") {
		t.Fatalf("expected start event rule, got %#v", findings)
	}
	if !hasRule(findings, "bpmn/disconnected-element") {
		t.Fatal("expected disconnected element")
	}
	if !hasRule(findings, "bpmn/exclusive-gateway-condition") {
		t.Fatal("expected gateway condition")
	}
	if !ShouldFail(findings, "error") {
		t.Fatal("should fail on error")
	}
}

func TestCleanOrderPassesErrors(t *testing.T) {
	m, err := bpmn.ParseFile(fixture(t, "bpmn/order-v1.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	findings := Run(m, Options{})
	if ShouldFail(findings, "error") {
		t.Fatalf("unexpected errors: %s", FormatText(findings))
	}
}

func TestIgnore(t *testing.T) {
	m, err := bpmn.ParseFile(fixture(t, "bpmn/lint/broken.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	findings := Run(m, Options{Ignore: []string{"bpmn/process-start-event"}})
	if hasRule(findings, "bpmn/process-start-event") {
		t.Fatal("ignored rule still present")
	}
}

func hasRule(fs []Finding, id string) bool {
	for _, f := range fs {
		if f.Rule == id {
			return true
		}
	}
	return false
}
