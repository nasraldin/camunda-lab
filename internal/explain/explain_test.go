package explain

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func TestOfflineOrder(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "order-v1.bpmn")
	m, err := bpmn.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := Offline(m)
	md := r.Markdown()
	for _, section := range []string{"## Business Summary", "## Technical Summary", "## Risks", "## Missing Paths", "## Optimization Suggestions"} {
		if !strings.Contains(md, section) {
			t.Fatalf("missing section %s", section)
		}
	}
	if !strings.Contains(r.Technical, "validate-customer") {
		t.Fatalf("tech: %s", r.Technical)
	}
	if !strings.Contains(r.Business, "OrderCreated") {
		t.Fatalf("biz: %s", r.Business)
	}
}

func TestOfflineUsesScopeAwareGraphsForEveryProcess(t *testing.T) {
	document := bpmn.Document{
		Processes: []bpmn.Process{
			{
				ID: "fulfillment", Name: "Fulfillment",
				Elements: []bpmn.Element{
					{ID: "end", Type: "endEvent", Name: "Done"},
					{ID: "reject", Type: "userTask", Name: "Reject order"},
					{ID: "approve", Type: "serviceTask", Name: "Approve order", JobType: "approve", RetryCount: "3"},
					{ID: "decision", Type: "exclusiveGateway", Name: "Approved?", DefaultFlow: "yes"},
					{ID: "start", Type: "startEvent", Name: "Order received", MessageRef: "order-message", EventDefs: []string{"message"}},
					{ID: "sub", Type: "subProcess", Name: "Notify"},
					{ID: "nested-end", Type: "endEvent", ParentID: "sub", Name: "Notified"},
					{ID: "nested-start", Type: "startEvent", ParentID: "sub", Name: "Begin notification"},
					{ID: "nested-task", Type: "serviceTask", ParentID: "sub", Name: "Send email", JobType: "email"},
				},
				Flows: []bpmn.Flow{
					{ID: "finish-reject", Source: "reject", Target: "end"},
					{ID: "no", Source: "decision", Target: "reject", Condition: "= not approved"},
					{ID: "yes", Source: "decision", Target: "approve"},
					{ID: "to-end", Source: "approve", Target: "end"},
					{ID: "begin", Source: "start", Target: "decision"},
					{ID: "nested-1", Source: "nested-start", Target: "nested-task"},
					{ID: "nested-2", Source: "nested-task", Target: "nested-end"},
				},
			},
			{
				ID: "audit", Name: "Audit",
				Elements: []bpmn.Element{
					{ID: "audit-start", Type: "startEvent", Name: "Start audit"},
					{ID: "loop-a", Type: "task", Name: "Check"},
					{ID: "loop-b", Type: "task", Name: "Retry"},
					{ID: "orphan", Type: "task", Name: "Orphan"},
				},
				Flows: []bpmn.Flow{
					{ID: "a", Source: "audit-start", Target: "loop-a"},
					{ID: "b", Source: "loop-a", Target: "loop-b"},
					{ID: "c", Source: "loop-b", Target: "loop-a"},
				},
			},
		},
		Messages: []bpmn.Message{
			{ID: "order-message", Name: "Order received message"},
			{ID: "unused-message", Name: "Unreferenced message"},
		},
		Errors: []bpmn.Error{
			{ID: "payment-error", Name: "Payment failed", ErrorCode: "PAYMENT_FAILED"},
			{ID: "unused-error", Name: "Unreferenced error", ErrorCode: "UNUSED"},
		},
	}

	result := Offline(document)
	markdown := result.Markdown()
	for _, want := range []string{
		"### Process: Fulfillment (fulfillment)",
		"Happy path: Order received [start:startEvent] → Approved? [decision:exclusiveGateway] → Approve order [approve:serviceTask] → Done [end:endEvent]",
		"Alternate path 1: Order received [start:startEvent] → Approved? [decision:exclusiveGateway] → Reject order [reject:userTask] → Done [end:endEvent]",
		"Scope Notify (sub) happy path: Begin notification [nested-start:startEvent] → Send email [nested-task:serviceTask] → Notified [nested-end:endEvent]",
		"### Process: Audit (audit)",
		"Cycle: Check [loop-a:task] → Retry [loop-b:task] → Check [loop-a:task]",
		"Dead end: Orphan [orphan:task]",
		"serviceTask approve name=\"Approve order\" jobType=\"approve\" retries=\"3\"",
		"exclusiveGateway decision name=\"Approved?\" default=\"yes\"",
		"startEvent start name=\"Order received\" events=\"message\" messageRef=\"order-message\"",
		"### Document Definitions",
		"- message order-message name=\"Order received message\"",
		"- message unused-message name=\"Unreferenced message\"",
		"- error payment-error name=\"Payment failed\" errorCode=\"PAYMENT_FAILED\"",
		"- error unused-error name=\"Unreferenced error\" errorCode=\"UNUSED\"",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("missing %q in:\n%s", want, markdown)
		}
	}
	if strings.Index(markdown, "Fulfillment") > strings.Index(markdown, "Audit") {
		t.Fatalf("process order is not normalized:\n%s", markdown)
	}
	for _, definition := range []string{
		"Order received message", "Unreferenced message", "Payment failed", "Unreferenced error",
	} {
		if strings.Count(markdown, definition) != 1 {
			t.Fatalf("definition %q rendered %d times:\n%s", definition, strings.Count(markdown, definition), markdown)
		}
	}
	jsonOutput, err := FormatJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range []string{"Order received message", "Unreferenced message", "Payment failed", "Unreferenced error"} {
		if strings.Count(string(jsonOutput), definition) != 1 {
			t.Fatalf("JSON definition %q rendered %d times:\n%s", definition, strings.Count(string(jsonOutput), definition), jsonOutput)
		}
	}
}

func TestOfflineMarkdownAndJSONAreStable(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "stable",
		Elements: []bpmn.Element{
			{ID: "z", Type: "endEvent", Name: "End"},
			{ID: "a", Type: "startEvent", Name: "Start"},
		},
		Flows: []bpmn.Flow{{ID: "f", Source: "a", Target: "z"}},
	}}}
	first := Offline(document)
	second := Offline(document)
	if first.Markdown() != second.Markdown() {
		t.Fatal("markdown changed between runs")
	}
	firstJSON, err := FormatJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := FormatJSON(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstJSON, secondJSON) || !bytes.HasSuffix(firstJSON, []byte("\n")) {
		t.Fatalf("unstable JSON:\n%s\n%s", firstJSON, secondJSON)
	}
}

func TestEnrichOptimizationPreservesOfflineSections(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "one", Elements: []bpmn.Element{{ID: "s", Type: "startEvent"}},
	}}}
	offline := Offline(document)
	client := &explainClient{response: ai.ChatResponse{Content: "Add a boundary-error test."}}
	enriched, err := EnrichOptimization(context.Background(), document, offline, client)
	if err != nil {
		t.Fatal(err)
	}
	if enriched.Business != offline.Business || enriched.Technical != offline.Technical ||
		enriched.Risks != offline.Risks || enriched.Missing != offline.Missing {
		t.Fatalf("AI changed deterministic sections: offline=%+v enriched=%+v", offline, enriched)
	}
	if enriched.Optimize != "Add a boundary-error test." || len(client.requests) != 1 ||
		client.requests[0].Purpose != "explain-optimization" ||
		!strings.Contains(client.requests[0].Prompt, offline.Markdown()) {
		t.Fatalf("enriched=%+v requests=%+v", enriched, client.requests)
	}

	client.err = context.Canceled
	failed, err := EnrichOptimization(context.Background(), document, offline, client)
	if !errors.Is(err, context.Canceled) || failed != offline {
		t.Fatalf("failed=%+v err=%v", failed, err)
	}
}

type explainClient struct {
	requests []ai.ChatRequest
	response ai.ChatResponse
	err      error
}

func (c *explainClient) Complete(_ context.Context, request ai.ChatRequest) (ai.ChatResponse, error) {
	c.requests = append(c.requests, request)
	return c.response, c.err
}
