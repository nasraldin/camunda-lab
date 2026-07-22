package lint

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func fixture(t *testing.T, rel string) string {
	t.Helper()
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata", rel)
}

func TestEveryRuleHasPositiveAndNegativeFixture(t *testing.T) {
	tests := []struct {
		rule     string
		positive string
		negative string
	}{
		{"bpmn/process-start-event", "process-start-event-valid.bpmn", "process-start-event-invalid.bpmn"},
		{"bpmn/disconnected-element", "disconnected-element-valid.bpmn", "disconnected-element-invalid.bpmn"},
		{"bpmn/exclusive-gateway-default", "exclusive-gateway-default-valid.bpmn", "exclusive-gateway-default-invalid.bpmn"},
		{"bpmn/exclusive-gateway-condition", "exclusive-gateway-condition-valid.bpmn", "exclusive-gateway-condition-invalid.bpmn"},
		{"bpmn/duplicate-message-name", "duplicate-message-name-valid.bpmn", "duplicate-message-name-invalid.bpmn"},
		{"bpmn/service-task-retry", "service-task-retry-valid.bpmn", "service-task-retry-invalid.bpmn"},
		{"bpmn/timer-reachable", "timer-reachable-valid.bpmn", "timer-reachable-invalid.bpmn"},
	}
	for _, test := range tests {
		t.Run(test.rule, func(t *testing.T) {
			valid := runFixture(t, test.positive, Options{})
			if hasRule(valid.Findings, test.rule) {
				t.Fatalf("valid fixture triggered %s: %+v", test.rule, valid.Findings)
			}
			invalid := runFixture(t, test.negative, Options{})
			if !hasRule(invalid.Findings, test.rule) {
				t.Fatalf("invalid fixture did not trigger %s: %+v", test.rule, invalid.Findings)
			}
		})
	}
}

func TestRunAttributesMultipleProcessesAndDocumentRuleOnce(t *testing.T) {
	result := runFixture(t, "multi-process.bpmn", Options{})
	count := 0
	processes := map[string]bool{}
	for _, finding := range result.Findings {
		if finding.Rule == "bpmn/duplicate-message-name" {
			count++
			if finding.ProcessID != "" {
				t.Fatalf("document finding has process attribution: %+v", finding)
			}
		}
		if finding.Rule == "bpmn/process-start-event" {
			processes[finding.ProcessID] = true
		}
	}
	if count != 1 {
		t.Fatalf("duplicate message findings = %d, want 1: %+v", count, result.Findings)
	}
	if !reflect.DeepEqual(processes, map[string]bool{"alpha": true, "beta": true}) {
		t.Fatalf("process attribution = %v", processes)
	}
}

func TestIgnoreStableOrderingAndThreshold(t *testing.T) {
	result := runFixture(t, "ordering.bpmn", Options{
		File: "models/order.bpmn", Ignore: []string{"bpmn/process-start-event"},
	})
	if hasRule(result.Findings, "bpmn/process-start-event") {
		t.Fatal("ignored finding remains")
	}
	for i := 1; i < len(result.Findings); i++ {
		if findingKey(result.Findings[i]) < findingKey(result.Findings[i-1]) {
			t.Fatalf("findings are unstable: %+v", result.Findings)
		}
	}
	if result.Failed {
		t.Fatalf("default error threshold failed warning-only findings: %+v", result.Findings)
	}
	warnings := runFixture(t, "ordering.bpmn", Options{
		FailOn: "warning", Ignore: []string{"bpmn/process-start-event"},
	})
	if !warnings.Failed {
		t.Fatal("warning threshold did not fail")
	}
}

func TestTextAndJSONReportsAreStable(t *testing.T) {
	result := runFixture(t, "service-task-retry-invalid.bpmn", Options{File: "retry.bpmn"})
	wantText := "bpmn/service-task-retry warning retry.bpmn:task service task missing retry extension\n"
	if got := FormatText(result); got != wantText {
		t.Fatalf("text:\n%q\nwant:\n%q", got, wantText)
	}
	first, err := FormatJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FormatJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	if string(first) != string(second) || !json.Valid(first) || !strings.HasSuffix(string(first), "\n") {
		t.Fatalf("unstable JSON: %q / %q", first, second)
	}
	const want = `{"failed":false,"findings":[{"rule":"bpmn/service-task-retry","severity":"warning","message":"service task missing retry extension","element":"task","processId":"retry","file":"retry.bpmn"}]}` + "\n"
	if string(first) != want {
		t.Fatalf("JSON = %q, want %q", first, want)
	}
}

func TestCleanJSONReportUsesFindingsArray(t *testing.T) {
	result := runFixture(t, "process-start-event-valid.bpmn", Options{})
	content, err := FormatJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	const want = `{"failed":false,"findings":[]}` + "\n"
	if string(content) != want {
		t.Fatalf("JSON = %q, want %q", content, want)
	}
}

func TestRegistryContainsSevenUniqueRules(t *testing.T) {
	rules := Rules()
	if len(rules) != 7 {
		t.Fatalf("rules = %d, want 7", len(rules))
	}
	seen := map[string]bool{}
	for _, rule := range rules {
		if seen[rule.ID()] {
			t.Fatalf("duplicate rule ID %q", rule.ID())
		}
		seen[rule.ID()] = true
	}
}

func TestDisconnectedBoundaryEventRequiresValidSameScopeAttachment(t *testing.T) {
	tests := []struct {
		name           string
		attachedTo     string
		attachmentType string
		parentID       string
		want           bool
	}{
		{name: "task in same scope", attachedTo: "task", attachmentType: "serviceTask", want: false},
		{name: "call activity in same scope", attachedTo: "task", attachmentType: "callActivity", want: false},
		{name: "subprocess in same scope", attachedTo: "task", attachmentType: "subProcess", want: false},
		{name: "missing attachment", want: true},
		{name: "dangling attachment", attachedTo: "missing", want: true},
		{name: "cross scope", attachedTo: "nested-task", want: true},
		{name: "boundary scope differs", attachedTo: "task", parentID: "sub", want: true},
		{name: "start event target", attachedTo: "task", attachmentType: "startEvent", want: true},
		{name: "gateway target", attachedTo: "task", attachmentType: "exclusiveGateway", want: true},
		{name: "event target", attachedTo: "task", attachmentType: "intermediateCatchEvent", want: true},
		{name: "boundary target", attachedTo: "task", attachmentType: "boundaryEvent", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			attachmentType := test.attachmentType
			if attachmentType == "" {
				attachmentType = "serviceTask"
			}
			document := bpmn.Document{Processes: []bpmn.Process{{
				ID: "boundary",
				Elements: []bpmn.Element{
					{ID: "task", Type: attachmentType},
					{ID: "sub", Type: "subProcess"},
					{ID: "nested-task", Type: "serviceTask", ParentID: "sub"},
					{
						ID: "boundary-event", Type: "boundaryEvent",
						AttachedTo: test.attachedTo, ParentID: test.parentID,
					},
				},
				Flows: []bpmn.Flow{
					{ID: "top", Source: "task", Target: "sub"},
					{ID: "nested", Source: "nested-task", Target: "nested-task"},
				},
			}}}
			findings := disconnectedElementRule{}.Check(document)
			got := hasElement(findings, "boundary-event")
			if got != test.want {
				t.Fatalf("boundary finding = %v, want %v: %+v", got, test.want, findings)
			}
		})
	}
}

func TestDisconnectedElementIgnoresDanglingAndCrossScopeFlows(t *testing.T) {
	document := bpmn.Document{Processes: []bpmn.Process{{
		ID: "invalid-flows",
		Elements: []bpmn.Element{
			{ID: "start", Type: "startEvent"},
			{ID: "valid", Type: "task"},
			{ID: "sub", Type: "subProcess"},
			{ID: "end", Type: "endEvent"},
			{ID: "dangling-source", Type: "task"},
			{ID: "dangling-target", Type: "task"},
			{ID: "cross-top", Type: "task"},
			{ID: "nested-start", Type: "startEvent", ParentID: "sub"},
			{ID: "nested-valid", Type: "task", ParentID: "sub"},
			{ID: "nested-end", Type: "endEvent", ParentID: "sub"},
			{ID: "cross-nested", Type: "task", ParentID: "sub"},
		},
		Flows: []bpmn.Flow{
			{ID: "top-one", Source: "start", Target: "valid"},
			{ID: "top-two", Source: "valid", Target: "sub"},
			{ID: "top-three", Source: "sub", Target: "end"},
			{ID: "nested-one", Source: "nested-start", Target: "nested-valid"},
			{ID: "nested-two", Source: "nested-valid", Target: "nested-end"},
			{ID: "missing-target", Source: "dangling-source", Target: "missing"},
			{ID: "missing-source", Source: "missing", Target: "dangling-target"},
			{ID: "cross-scope", Source: "cross-top", Target: "cross-nested"},
		},
	}}}

	findings := disconnectedElementRule{}.Check(document)
	for _, element := range []string{"dangling-source", "dangling-target", "cross-top", "cross-nested"} {
		if !hasElement(findings, element) {
			t.Errorf("missing disconnected finding for %s: %+v", element, findings)
		}
	}
	for _, element := range []string{"start", "valid", "sub", "end", "nested-start", "nested-valid", "nested-end"} {
		if hasElement(findings, element) {
			t.Errorf("valid same-scope flow marked %s disconnected: %+v", element, findings)
		}
	}
}

func TestTimerReachabilityRespectsSubprocessScope(t *testing.T) {
	tests := []struct {
		name      string
		timerFlow bool
		want      bool
	}{
		{name: "reachable nested timer", timerFlow: true, want: false},
		{name: "unreachable nested timer", timerFlow: false, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flows := []bpmn.Flow{
				{ID: "top-one", Source: "top-start", Target: "sub"},
				{ID: "top-two", Source: "sub", Target: "top-end"},
				{ID: "nested-end-flow", Source: "nested-start", Target: "nested-end"},
			}
			if test.timerFlow {
				flows[2] = bpmn.Flow{ID: "nested-one", Source: "nested-start", Target: "timer"}
				flows = append(flows, bpmn.Flow{ID: "nested-two", Source: "timer", Target: "nested-end"})
			}
			document := bpmn.Document{Processes: []bpmn.Process{{
				ID: "scoped",
				Elements: []bpmn.Element{
					{ID: "top-start", Type: "startEvent"},
					{ID: "sub", Type: "subProcess"},
					{ID: "top-end", Type: "endEvent"},
					{ID: "nested-start", Type: "startEvent", ParentID: "sub"},
					{ID: "timer", Type: "intermediateCatchEvent", ParentID: "sub", Timer: "PT1M"},
					{ID: "nested-end", Type: "endEvent", ParentID: "sub"},
				},
				Flows: flows,
			}}}
			findings := timerReachableRule{}.Check(document)
			got := hasElement(findings, "timer")
			if got != test.want {
				t.Fatalf("timer finding = %v, want %v: %+v", got, test.want, findings)
			}
		})
	}
}

func TestNestedTimerRequiresEveryAncestorScopeReachable(t *testing.T) {
	tests := []struct {
		name           string
		outerReachable bool
		innerReachable bool
		want           bool
	}{
		{name: "all ancestors reachable", outerReachable: true, innerReachable: true, want: false},
		{name: "outer ancestor unreachable", innerReachable: true, want: true},
		{name: "inner ancestor unreachable", outerReachable: true, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			flows := []bpmn.Flow{
				{ID: "top-direct", Source: "top-start", Target: "top-end"},
				{ID: "outer-direct", Source: "outer-start", Target: "outer-end"},
				{ID: "inner-one", Source: "inner-start", Target: "timer"},
				{ID: "inner-two", Source: "timer", Target: "inner-end"},
			}
			if test.outerReachable {
				flows[0] = bpmn.Flow{ID: "top-one", Source: "top-start", Target: "outer"}
				flows = append(flows, bpmn.Flow{ID: "top-two", Source: "outer", Target: "top-end"})
			}
			if test.innerReachable {
				flows[1] = bpmn.Flow{ID: "outer-one", Source: "outer-start", Target: "inner"}
				flows = append(flows, bpmn.Flow{ID: "outer-two", Source: "inner", Target: "outer-end"})
			}
			document := bpmn.Document{Processes: []bpmn.Process{{
				ID: "nested-ancestors",
				Elements: []bpmn.Element{
					{ID: "top-start", Type: "startEvent"},
					{ID: "outer", Type: "subProcess"},
					{ID: "top-end", Type: "endEvent"},
					{ID: "outer-start", Type: "startEvent", ParentID: "outer"},
					{ID: "inner", Type: "subProcess", ParentID: "outer"},
					{ID: "outer-end", Type: "endEvent", ParentID: "outer"},
					{ID: "inner-start", Type: "startEvent", ParentID: "inner"},
					{ID: "timer", Type: "intermediateCatchEvent", ParentID: "inner", Timer: "PT1M"},
					{ID: "inner-end", Type: "endEvent", ParentID: "inner"},
				},
				Flows: flows,
			}}}

			findings := timerReachableRule{}.Check(document)
			if got := hasElement(findings, "timer"); got != test.want {
				t.Fatalf("timer finding = %v, want %v: %+v", got, test.want, findings)
			}
		})
	}
}

func TestTimerReachabilityFollowsBoundaryAttachment(t *testing.T) {
	tests := []struct {
		name          string
		attachToTask  string
		connectTarget string
		want          bool
	}{
		{name: "reachable boundary timer", attachToTask: "reachable", connectTarget: "reachable", want: false},
		{name: "unreachable boundary timer", attachToTask: "unreachable", connectTarget: "end", want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			document := bpmn.Document{Processes: []bpmn.Process{{
				ID: "boundary-timer",
				Elements: []bpmn.Element{
					{ID: "start", Type: "startEvent"},
					{ID: "reachable", Type: "serviceTask"},
					{ID: "unreachable", Type: "serviceTask"},
					{ID: "end", Type: "endEvent"},
					{
						ID: "timer", Type: "boundaryEvent", Timer: "PT1M",
						AttachedTo: test.attachToTask,
					},
				},
				Flows: []bpmn.Flow{
					{ID: "one", Source: "start", Target: test.connectTarget},
					{ID: "two", Source: "reachable", Target: "end"},
				},
			}}}
			findings := timerReachableRule{}.Check(document)
			got := hasElement(findings, "timer")
			if got != test.want {
				t.Fatalf("timer finding = %v, want %v: %+v", got, test.want, findings)
			}
		})
	}
}

func runFixture(t *testing.T, name string, options Options) Result {
	t.Helper()
	document, err := bpmn.ParseFile(fixture(t, "bpmn/lint/"+name))
	if err != nil {
		t.Fatal(err)
	}
	return Run(document, options)
}

func hasRule(fs []Finding, id string) bool {
	for _, f := range fs {
		if f.Rule == id {
			return true
		}
	}
	return false
}

func hasElement(findings []Finding, element string) bool {
	for _, finding := range findings {
		if finding.Element == element {
			return true
		}
	}
	return false
}
