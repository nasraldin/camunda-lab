package diff

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func TestChangeKindUsesPublicStringContract(t *testing.T) {
	var kind string = (Change{}).Kind
	if kind != "" {
		t.Fatalf("zero-value kind = %q", kind)
	}
}

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
		if c.Kind == ElementAdded && c.ElementID == "Payment" {
			addedPayment = true
		}
		if c.Kind == FieldChanged && c.ElementID == "Wait" && c.Field == "event_value" {
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

func TestCompareReportsDocumentSemantics(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0">
  <message id="oldMessage" name="Order received"/>
  <process id="removed" name="Removed process"><startEvent id="removedStart"/></process>
  <process id="main">
    <startEvent id="start"><messageEventDefinition messageRef="oldMessage"/></startEvent>
    <serviceTask id="work" name="Work"><extensionElements><zeebe:taskDefinition type="old-worker" retries="3"/></extensionElements></serviceTask>
    <exclusiveGateway id="choice" default="fallback"/>
    <callActivity id="call" calledElement="removed"/>
    <boundaryEvent id="timeout" attachedToRef="work" cancelActivity="false"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
    <endEvent id="end"/>
    <sequenceFlow id="route" name="old route" sourceRef="start" targetRef="work"><conditionExpression>= old</conditionExpression></sequenceFlow>
    <sequenceFlow id="fallback" sourceRef="choice" targetRef="end"/>
  </process>
</definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:zeebe="http://camunda.org/schema/zeebe/1.0">
  <message id="oldMessage" name="Order changed"/>
  <message id="newMessage" name="New message"/>
  <process id="added" name="Added process"><startEvent id="addedStart"/></process>
  <process id="main">
    <startEvent id="start"><timerEventDefinition><timeCycle>R/PT5M</timeCycle></timerEventDefinition></startEvent>
    <userTask id="work" name="Changed work"><extensionElements><zeebe:taskDefinition type="new-worker" retries="5"/></extensionElements></userTask>
    <exclusiveGateway id="choice"/>
    <callActivity id="call" calledElement="added"/>
    <boundaryEvent id="timeout" attachedToRef="end"><timerEventDefinition><timeDuration>PT2M</timeDuration></timerEventDefinition></boundaryEvent>
    <endEvent id="end"/>
    <sequenceFlow id="route" name="new route" sourceRef="choice" targetRef="end"><conditionExpression>= new</conditionExpression></sequenceFlow>
  </process>
</definitions>`)

	changes := Compare(before, after)
	want := map[string]bool{
		"process_added:added":                false,
		"process_removed:removed":            false,
		"message_added:newMessage":           false,
		"field_changed:oldMessage:name":      false,
		"field_changed:start:event_kind":     false,
		"field_changed:start:event_ref":      false,
		"field_changed:start:event_value":    false,
		"field_changed:work:type":            false,
		"field_changed:work:name":            false,
		"field_changed:work:job_type":        false,
		"field_changed:work:retry_count":     false,
		"field_changed:choice:default_flow":  false,
		"field_changed:call:call_target":     false,
		"field_changed:timeout:attached_to":  false,
		"field_changed:timeout:interrupting": false,
		"field_changed:timeout:event_value":  false,
		"field_changed:route:source":         false,
		"field_changed:route:target":         false,
		"field_changed:route:name":           false,
		"field_changed:route:condition":      false,
	}
	for _, change := range changes {
		key := string(change.Kind) + ":" + change.ElementID
		if change.ProcessID != "" && change.ElementID == "" {
			key = string(change.Kind) + ":" + change.ProcessID
		}
		if change.Field != "" {
			key += ":" + change.Field
		}
		if _, ok := want[key]; ok {
			want[key] = true
		}
	}
	for key, found := range want {
		if !found {
			t.Errorf("missing %s in %#v", key, changes)
		}
	}
	for _, change := range changes {
		if change.ElementID == "route" && change.Field == "source" &&
			(change.Before != "start" || change.After != "choice") {
			t.Fatalf("flow source values are not user-facing: %#v", change)
		}
		if change.ElementID == "call" && change.Field == "call_target" &&
			(change.Before != "removed" || change.After != "added") {
			t.Fatalf("call target values are not user-facing: %#v", change)
		}
	}
}

func TestCompareReportsElementAndFlowAddRemove(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="start"/><task id="old"/><sequenceFlow id="oldFlow" sourceRef="start" targetRef="old"/>
</process></definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="start"/><task id="new" name="new"/><sequenceFlow id="newFlow" sourceRef="start" targetRef="new"/>
</process></definitions>`)
	changes := Compare(before, after)
	assertChange(t, changes, ElementRemoved, "p", "old", "")
	assertChange(t, changes, ElementAdded, "p", "new", "")
	assertChange(t, changes, FlowRemoved, "p", "oldFlow", "")
	assertChange(t, changes, FlowAdded, "p", "newFlow", "")
}

func TestCompareReportsProcessNameChange(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" name="Before"><startEvent id="start"/></process>
</definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="p" name="After"><startEvent id="start"/></process>
</definitions>`)

	changes := Compare(before, after)
	assertChange(t, changes, FieldChanged, "p", "", "name")
}

func TestCompareReportsErrorDefinitionSemantics(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <error id="changed" name="Old error" errorCode="OLD"/>
  <error id="removed" name="Removed error" errorCode="REMOVED"/>
  <process id="p">
    <startEvent id="start"/>
    <boundaryEvent id="failure"><errorEventDefinition errorRef="changed"/></boundaryEvent>
  </process>
</definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <error id="changed" name="New error" errorCode="NEW"/>
  <error id="added" name="Added error" errorCode="ADDED"/>
  <process id="p">
    <startEvent id="start"/>
    <boundaryEvent id="failure"><errorEventDefinition errorRef="changed"/></boundaryEvent>
  </process>
</definitions>`)

	changes := Compare(before, after)
	assertChange(t, changes, ErrorAdded, "", "added", "")
	assertChange(t, changes, ErrorRemoved, "", "removed", "")
	assertChange(t, changes, FieldChanged, "", "changed", "name")
	assertChange(t, changes, FieldChanged, "", "changed", "error_code")
	assertChange(t, changes, FieldChanged, "p", "failure", "event_ref")
}

func TestCompareReportsSemanticScopeMoves(t *testing.T) {
	tests := []struct {
		name   string
		before string
		after  string
	}{
		{
			name: "top-level to subprocess",
			before: `<process id="p"><subProcess id="left" name="Left"/>
  <task id="work" name="Work"/></process>`,
			after: `<process id="p"><subProcess id="left" name="Left">
    <task id="work" name="Work"/>
  </subProcess></process>`,
		},
		{
			name: "between subprocesses",
			before: `<process id="p"><subProcess id="left" name="Left">
    <task id="work" name="Work"/>
  </subProcess><subProcess id="right" name="Right"/></process>`,
			after: `<process id="p"><subProcess id="left" name="Left"/>
  <subProcess id="right" name="Right"><task id="work" name="Work"/></subProcess></process>`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`+test.before+`</definitions>`)
			after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`+test.after+`</definitions>`)
			changes := Compare(before, after)
			assertChange(t, changes, FieldChanged, "p", "work", "parent")
			if len(changes) != 1 {
				t.Fatalf("scope move changes = %#v", changes)
			}
		})
	}
}

func TestCompareIgnoresEquivalentRenamedScopeIDs(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="old-process" name="Process"><subProcess id="old-scope" name="Scope">
    <task id="old-work" name="Work"/>
  </subProcess></process>
</definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="new-process" name="Process"><subProcess id="new-scope" name="Scope">
    <task id="new-work" name="Work"/>
  </subProcess></process>
</definitions>`)
	if changes := Compare(before, after); len(changes) != 0 {
		t.Fatalf("equivalent renamed scopes = %#v", changes)
	}
}

func TestCompareReportsUnknownExtensionChangesAtEveryScope(t *testing.T) {
	type expectedScope struct {
		processID string
		elementID string
	}
	scopes := map[string]expectedScope{
		"document": {},
		"process":  {processID: "p"},
		"element":  {processID: "p", elementID: "work"},
	}
	operations := map[string]struct {
		before string
		after  string
	}{
		"add":    {after: `<v:policy mode="strict"><v:rule>allow</v:rule></v:policy>`},
		"remove": {before: `<v:policy mode="strict"><v:rule>allow</v:rule></v:policy>`},
		"change": {
			before: `<v:policy mode="strict"><v:rule>allow</v:rule></v:policy>`,
			after:  `<v:policy mode="strict"><v:rule>deny</v:rule></v:policy>`,
		},
	}
	for scope, want := range scopes {
		for operation, extension := range operations {
			t.Run(scope+"/"+operation, func(t *testing.T) {
				before := parseDocument(t, extensionFixture(scope, extension.before))
				after := parseDocument(t, extensionFixture(scope, extension.after))
				changes := Compare(before, after)
				assertChange(t, changes, FieldChanged, want.processID, want.elementID, "extensions")
				if len(changes) != 1 {
					t.Fatalf("%s %s extension changes = %#v", scope, operation, changes)
				}
			})
		}
	}
}

func TestCompareUnknownExtensionQNameAttributesAndContentOrderAtEveryScope(t *testing.T) {
	changes := map[string]struct {
		before string
		after  string
	}{
		"qname": {
			before: `<v:policy mode="strict"/>`,
			after:  `<v:guard mode="strict"/>`,
		},
		"attribute": {
			before: `<v:policy mode="strict"/>`,
			after:  `<v:policy mode="relaxed"/>`,
		},
		"ordered content": {
			before: `<v:policy><v:first/><v:second/></v:policy>`,
			after:  `<v:policy><v:second/><v:first/></v:policy>`,
		},
	}
	for _, scope := range []string{"document", "process", "element"} {
		for name, extension := range changes {
			t.Run(scope+"/"+name, func(t *testing.T) {
				before := parseDocument(t, extensionFixture(scope, extension.before))
				after := parseDocument(t, extensionFixture(scope, extension.after))
				if got := Compare(before, after); len(got) != 1 || got[0].Field != "extensions" {
					t.Fatalf("%s %s extension semantics = %#v", scope, name, got)
				}
			})
		}
	}
}

func TestCompareIgnoresCosmeticUnknownExtensionFormattingAtEveryScope(t *testing.T) {
	beforeExtension := `<v:policy mode="strict" level="high">
  <v:rule z="last" a="first"> allow </v:rule>
</v:policy>`
	afterExtension := `<vendor:policy level="high" mode="strict"><vendor:rule a="first" z="last">allow</vendor:rule></vendor:policy>`
	for _, scope := range []string{"document", "process", "element"} {
		t.Run(scope, func(t *testing.T) {
			before := parseDocument(t, extensionFixture(scope, beforeExtension))
			after := parseDocument(t, strings.Replace(extensionFixture(scope, afterExtension), `xmlns:v="urn:vendor"`, `xmlns:vendor="urn:vendor"`, 1))
			if changes := Compare(before, after); len(changes) != 0 {
				t.Fatalf("cosmetic %s extension formatting = %#v", scope, changes)
			}
		})
	}
}

func TestCompareIgnoresExtensionLocalNamespaceDeclarationsAtEveryScope(t *testing.T) {
	equivalent := map[string]struct {
		before string
		after  string
	}{
		"prefixed to prefixed": {
			before: `<first:policy xmlns:first="urn:local"><first:rule/></first:policy>`,
			after:  `<second:policy xmlns:second="urn:local"><second:rule/></second:policy>`,
		},
		"prefixed to default": {
			before: `<local:policy xmlns:local="urn:local"><local:rule/></local:policy>`,
			after:  `<policy xmlns="urn:local"><rule/></policy>`,
		},
	}
	for _, scope := range []string{"document", "process", "element"} {
		for spelling, extension := range equivalent {
			t.Run(scope+"/"+spelling, func(t *testing.T) {
				before := parseDocument(t, extensionFixture(scope, extension.before))
				after := parseDocument(t, extensionFixture(scope, extension.after))
				if changes := Compare(before, after); len(changes) != 0 {
					t.Fatalf("%s local namespace spelling %s = %#v", scope, spelling, changes)
				}
			})
		}
	}
}

func TestCompareRetainsOrdinaryNamespacedExtensionAttributesAtEveryScope(t *testing.T) {
	beforeExtension := `<v:policy xmlns:meta="urn:meta" meta:mode="strict"/>`
	afterExtension := `<v:policy xmlns:renamed="urn:meta" renamed:mode="relaxed"/>`
	for _, scope := range []string{"document", "process", "element"} {
		t.Run(scope, func(t *testing.T) {
			before := parseDocument(t, extensionFixture(scope, beforeExtension))
			after := parseDocument(t, extensionFixture(scope, afterExtension))
			if changes := Compare(before, after); len(changes) != 1 || changes[0].Field != "extensions" {
				t.Fatalf("%s namespaced attribute semantics = %#v", scope, changes)
			}
		})
	}
}

func TestCompareTreatsFormattingAttributesAndEquivalentIDsAsEqual(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="message-old" name="Notice"/>
  <process id="target-old" name="Target"><task id="target-task-old" name="Target task"/></process>
  <process id="main-old" name="Main">
    <startEvent id="start-old"><messageEventDefinition messageRef="message-old"/></startEvent>
    <task id="work-old" name="Work"/>
    <callActivity id="call-old" calledElement="target-old"/>
    <boundaryEvent id="boundary-old" attachedToRef="work-old" cancelActivity="false"><timerEventDefinition><timeDuration> PT1M </timeDuration></timerEventDefinition></boundaryEvent>
    <endEvent id="end-old"/>
    <sequenceFlow id="flow-old" name="go" sourceRef="start-old" targetRef="work-old"><conditionExpression> = approved </conditionExpression></sequenceFlow>
  </process>
</definitions>`)
	after := parseDocument(t, `<definitions exporter="ignored" xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process name="Main" id="main-new"><endEvent id="end-new"/>
    <boundaryEvent cancelActivity="0" id="boundary-new" attachedToRef="work-new"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
    <callActivity calledElement="target-new" id="call-new"/><task name="Work" id="work-new"/>
    <startEvent id="start-new"><messageEventDefinition messageRef="message-new"/></startEvent>
    <sequenceFlow targetRef="work-new" sourceRef="start-new" name="go" id="flow-new"><conditionExpression>= approved</conditionExpression></sequenceFlow>
  </process>
  <process name="Target" id="target-new"><task name="Target task" id="target-task-new"/></process>
  <message name="Notice" id="message-new"/>
</definitions>`)
	if changes := Compare(before, after); len(changes) != 0 {
		t.Fatalf("ID-only/formatting changes = %#v", changes)
	}
}

func TestCompareMatchesSemanticIdentityBeforePermutedIDs(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="start"/><task id="a" name="Alpha"/><task id="b" name="Beta"/><endEvent id="end"/>
  <sequenceFlow id="to-a" sourceRef="start" targetRef="a"/><sequenceFlow id="a-end" sourceRef="a" targetRef="end"/>
  <sequenceFlow id="to-b" sourceRef="start" targetRef="b"/><sequenceFlow id="b-end" sourceRef="b" targetRef="end"/>
</process></definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="start"/><task id="b" name="Alpha"/><task id="a" name="Beta"/><endEvent id="end"/>
  <sequenceFlow id="to-b" sourceRef="start" targetRef="b"/><sequenceFlow id="b-end" sourceRef="b" targetRef="end"/>
  <sequenceFlow id="to-a" sourceRef="start" targetRef="a"/><sequenceFlow id="a-end" sourceRef="a" targetRef="end"/>
</process></definitions>`)
	if changes := Compare(before, after); len(changes) != 0 {
		t.Fatalf("ID permutation produced changes: %#v", changes)
	}
}

func TestCompareDuplicateSemanticIdentityPreservesSameIDChange(t *testing.T) {
	tests := []struct {
		name      string
		before    string
		after     string
		processID string
		elementID string
		field     string
		forbidden map[ChangeKind]bool
	}{
		{
			name: "process",
			before: `<process id="p1" name="Same"><task id="t1"/></process>
  <process id="p2" name="Same"><task id="t2"/></process>`,
			after: `<process id="p1" name="Changed"><task id="t1"/></process>
  <process id="p2" name="Same"><task id="t2"/></process>`,
			processID: "p1", field: "name",
			forbidden: map[ChangeKind]bool{ProcessAdded: true, ProcessRemoved: true},
		},
		{
			name:      "element",
			before:    `<process id="p"><task id="a" name="Same"/><task id="b" name="Same"/></process>`,
			after:     `<process id="p"><task id="a" name="Changed"/><task id="b" name="Same"/></process>`,
			processID: "p", elementID: "a", field: "name",
			forbidden: map[ChangeKind]bool{ElementAdded: true, ElementRemoved: true},
		},
		{
			name: "flow",
			before: `<process id="p"><startEvent id="start"/><endEvent id="end"/>
  <sequenceFlow id="a" sourceRef="start" targetRef="end"/><sequenceFlow id="b" sourceRef="start" targetRef="end"/></process>`,
			after: `<process id="p"><startEvent id="start"/><endEvent id="end"/>
  <sequenceFlow id="a" sourceRef="start" targetRef="end"><conditionExpression>= changed</conditionExpression></sequenceFlow>
  <sequenceFlow id="b" sourceRef="start" targetRef="end"/></process>`,
			processID: "p", elementID: "a", field: "condition",
			forbidden: map[ChangeKind]bool{FlowAdded: true, FlowRemoved: true},
		},
		{
			name: "message",
			before: `<message id="a" name="Same"/><message id="b" name="Same"/>
  <process id="p"><startEvent id="start"/></process>`,
			after: `<message id="a" name="Changed"/><message id="b" name="Same"/>
  <process id="p"><startEvent id="start"/></process>`,
			elementID: "a", field: "name",
			forbidden: map[ChangeKind]bool{MessageAdded: true, MessageRemoved: true},
		},
		{
			name: "error",
			before: `<error id="a" name="Same" errorCode="S"/><error id="b" name="Same" errorCode="S"/>
  <process id="p"><startEvent id="start"/></process>`,
			after: `<error id="a" name="Changed" errorCode="S"/><error id="b" name="Same" errorCode="S"/>
  <process id="p"><startEvent id="start"/></process>`,
			elementID: "a", field: "name",
			forbidden: map[ChangeKind]bool{ErrorAdded: true, ErrorRemoved: true},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`+test.before+`</definitions>`)
			after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">`+test.after+`</definitions>`)
			changes := Compare(before, after)
			assertChange(t, changes, FieldChanged, test.processID, test.elementID, test.field)
			for _, change := range changes {
				if test.forbidden[change.Kind] {
					t.Fatalf("same-ID change became add/remove: %#v", changes)
				}
			}
		})
	}
}

func TestCompareUsesNeighborhoodToDetectEndpointAndAttachmentChanges(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="leftStart" name="Left"/><startEvent id="rightStart" name="Right"/>
  <task id="left"/><task id="right"/><endEvent id="end"/>
  <boundaryEvent id="boundary" attachedToRef="left"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
  <sequenceFlow id="leftIn" sourceRef="leftStart" targetRef="left"/><sequenceFlow id="rightIn" sourceRef="rightStart" targetRef="right"/>
  <sequenceFlow id="route" sourceRef="left" targetRef="end"/>
</process></definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="leftStart" name="Left"/><startEvent id="rightStart" name="Right"/>
  <task id="left"/><task id="right"/><endEvent id="end"/>
  <boundaryEvent id="boundary" attachedToRef="right"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
  <sequenceFlow id="leftIn" sourceRef="leftStart" targetRef="left"/><sequenceFlow id="rightIn" sourceRef="rightStart" targetRef="right"/>
  <sequenceFlow id="route" sourceRef="right" targetRef="end"/>
</process></definitions>`)
	changes := Compare(before, after)
	assertChange(t, changes, FieldChanged, "p", "boundary", "attached_to")
	assertChange(t, changes, FieldChanged, "p", "route", "source")
}

func TestCompareDoesNotReportStableReferencesWhenTargetFieldsChange(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="start"/><task id="work" name="Before"/><exclusiveGateway id="choice" default="fallback"/>
  <boundaryEvent id="boundary" attachedToRef="work"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
  <sequenceFlow id="route" sourceRef="start" targetRef="work"/><sequenceFlow id="fallback" sourceRef="choice" targetRef="work"/>
</process></definitions>`)
	after := parseDocument(t, strings.Replace(
		`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <startEvent id="start"/><task id="work" name="Before"/><exclusiveGateway id="choice" default="fallback"/>
  <boundaryEvent id="boundary" attachedToRef="work"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
  <sequenceFlow id="route" sourceRef="start" targetRef="work"/><sequenceFlow id="fallback" sourceRef="choice" targetRef="work"/>
</process></definitions>`, `name="Before"`, `name="After"`, 1))
	changes := Compare(before, after)
	assertChange(t, changes, FieldChanged, "p", "work", "name")
	for _, change := range changes {
		if change.Field == "source" || change.Field == "target" || change.Field == "attached_to" || change.Field == "default_flow" {
			t.Fatalf("stable reference reported as changed: %#v", change)
		}
	}
}

func TestCompareReportsRetainedExtensionChanges(t *testing.T) {
	before := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:v="urn:vendor"><process id="p">
  <extensionElements><v:policy mode="strict">old</v:policy></extensionElements>
  <task id="work"><extensionElements><v:config level="1"/></extensionElements></task>
</process></definitions>`)
	after := parseDocument(t, `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:v="urn:vendor"><process id="p">
  <extensionElements><v:policy mode="strict">new</v:policy></extensionElements>
  <task id="work"><extensionElements><v:config level="2"/></extensionElements></task>
</process></definitions>`)
	changes := Compare(before, after)
	assertChange(t, changes, FieldChanged, "p", "", "extensions")
	assertChange(t, changes, FieldChanged, "p", "work", "extensions")
}

func TestFormatIsDeterministicTextAndJSON(t *testing.T) {
	changes := []Change{
		{Kind: FieldChanged, ProcessID: "p", ElementID: "b", ElementType: "task", Field: "name", Before: "old", After: "new", Summary: "Changed task b name"},
		{Kind: ElementAdded, ProcessID: "p", ElementID: "a", ElementType: "task", Summary: "Added task a"},
	}
	text := FormatText(changes)
	if text != "✓ Added task a\n✓ Changed task b name\n" {
		t.Fatalf("text = %q", text)
	}
	first, err := FormatJSON(changes)
	if err != nil {
		t.Fatal(err)
	}
	second, err := FormatJSON([]Change{changes[1], changes[0]})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) || !json.Valid(first) || !strings.HasSuffix(string(first), "\n") {
		t.Fatalf("unstable JSON:\n%s\n%s", first, second)
	}
	empty, err := FormatJSON(nil)
	if err != nil {
		t.Fatal(err)
	}
	if string(empty) != "[]\n" {
		t.Fatalf("empty JSON = %q", empty)
	}
}

func extensionFixture(scope, extension string) string {
	wrap := func(value string) string {
		if value == "" {
			return ""
		}
		return `<extensionElements>` + value + `</extensionElements>`
	}
	documentExtension, processExtension, elementExtension := "", "", ""
	switch scope {
	case "document":
		documentExtension = wrap(extension)
	case "process":
		processExtension = wrap(extension)
	case "element":
		elementExtension = wrap(extension)
	}
	return `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:v="urn:vendor">` +
		documentExtension + `<process id="p">` + processExtension +
		`<task id="work">` + elementExtension + `</task></process></definitions>`
}

func parseDocument(t *testing.T, source string) bpmn.Document {
	t.Helper()
	document, err := bpmn.Parse(strings.NewReader(source))
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func assertChange(t *testing.T, changes []Change, kind ChangeKind, processID, elementID, field string) {
	t.Helper()
	for _, change := range changes {
		if change.Kind == kind && change.ProcessID == processID && change.ElementID == elementID && change.Field == field {
			return
		}
	}
	t.Fatalf("missing %s/%s/%s/%s in %#v", kind, processID, elementID, field, changes)
}
