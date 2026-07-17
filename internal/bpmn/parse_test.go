package bpmn

import (
	"path/filepath"
	"runtime"
	"testing"
)

func testdata(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("caller")
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", rel)
}

func TestParseOrderV1(t *testing.T) {
	m, err := ParseFile(testdata(t, "bpmn/order-v1.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	if m.ProcessID != "orderProcess" {
		t.Fatalf("process %q", m.ProcessID)
	}
	start := m.ElementByID("Start")
	if start == nil || start.Type != "startEvent" {
		t.Fatal("start event")
	}
	validate := m.ElementByID("Validate")
	if validate == nil || validate.JobType != "validate-customer" {
		t.Fatalf("validate job: %+v", validate)
	}
	if validate.RetryCount != "3" {
		t.Fatalf("retries %q", validate.RetryCount)
	}
	wait := m.ElementByID("Wait")
	if wait == nil || wait.Timer != "PT5M" {
		t.Fatalf("timer %+v", wait)
	}
	gw := m.ElementByID("Gateway")
	if gw == nil || gw.DefaultFlow != "FlowDefault" {
		t.Fatalf("gateway %+v", gw)
	}
	var cond string
	for _, f := range m.Flows {
		if f.ID == "FlowOK" {
			cond = f.Condition
		}
	}
	if cond == "" {
		t.Fatal("expected condition on FlowOK")
	}
	if len(m.Messages) != 1 || m.Messages[0].Name != "order-received" {
		t.Fatalf("messages %+v", m.Messages)
	}
}
