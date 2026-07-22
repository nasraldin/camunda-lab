package bpmn

import (
	"encoding/xml"
	"errors"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
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
	doc, err := ParseFile(testdata(t, "bpmn/order-v1.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	m := doc.Processes[0]
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

func TestParseEveryProcessAndNestedSubprocess(t *testing.T) {
	doc, err := ParseFile(testdata(t, "bpmn/multi-process.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Processes) != 2 {
		t.Fatalf("processes = %d", len(doc.Processes))
	}
	if got := doc.Processes[0].ID; got != "fulfillment" {
		t.Fatalf("first normalized process = %q", got)
	}
	p := doc.Processes[0]
	sub := p.ElementByID("pack")
	if sub == nil || sub.Type != "subProcess" {
		t.Fatalf("subprocess = %+v", sub)
	}
	child := p.ElementByID("box")
	if child == nil || child.ParentID != "pack" {
		t.Fatalf("nested task = %+v", child)
	}
	call := p.ElementByID("charge")
	if call == nil || call.CalledElement != "paymentProcess" {
		t.Fatalf("call activity = %+v", call)
	}
}

func TestParseBoundaryDefinitionsConditionsAndExtensions(t *testing.T) {
	doc, err := ParseFile(testdata(t, "bpmn/boundary-extensions.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Processes[0]
	task := p.ElementByID("work")
	if task == nil || task.JobType != "ship-order" || task.RetryCount != "5" {
		t.Fatalf("task semantics = %+v", task)
	}
	for id, want := range map[string][2]string{
		"timerBoundary":   {"timer", "PT10M"},
		"messageBoundary": {"message", "deliveryMessage"},
		"errorBoundary":   {"error", "deliveryError"},
	} {
		wantDef, wantRef := want[0], want[1]
		event := p.ElementByID(id)
		if event == nil || event.AttachedTo != "work" || !reflect.DeepEqual(event.EventDefs, []string{wantDef}) {
			t.Fatalf("%s = %+v", id, event)
		}
		gotRef := event.Timer
		if wantDef == "message" {
			gotRef = event.MessageRef
		}
		if wantDef == "error" {
			gotRef = event.ErrorRef
		}
		if gotRef != wantRef {
			t.Fatalf("%s ref = %q", id, gotRef)
		}
	}
	var condition string
	for _, flow := range p.Flows {
		if flow.ID == "conditional" {
			condition = flow.Condition
		}
	}
	if condition != "= approved" {
		t.Fatalf("condition = %q", condition)
	}
	if len(doc.UnknownExtensions) != 2 || len(p.UnknownExtensions) != 1 {
		t.Fatalf("extensions = %+v", doc.UnknownExtensions)
	}
	var ext Extension
	for _, candidate := range doc.UnknownExtensions {
		if candidate.QName.Local == "policy" {
			ext = candidate
		}
	}
	if ext.QName.Space != "urn:vendor:test" || ext.QName.Local != "policy" ||
		len(ext.Attributes) != 1 || ext.Attributes[0].Value != "strict" ||
		!strings.Contains(ext.InnerXML, "threshold") {
		t.Fatalf("extension not retained: %+v", ext)
	}
}

func TestParseRejectsInvalidDocumentsWithTypedErrors(t *testing.T) {
	tests := []struct {
		name string
		xml  string
		kind ErrorKind
	}{
		{"malformed", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process>`, ErrorMalformedXML},
		{"malformed trailing content", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="s"/></process></definitions><`, ErrorMalformedXML},
		{"wrong root", `<foo xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"/>`, ErrorInvalidRoot},
		{"wrong namespace", `<definitions xmlns="urn:not-bpmn"><process id="p"><startEvent id="s"/></process></definitions>`, ErrorInvalidNamespace},
		{"no process", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"/>`, ErrorNoProcess},
		{"no nodes", `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"/></definitions>`, ErrorNoFlowNodes},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse(strings.NewReader(tt.xml))
			var parseErr *ParseError
			if !errors.As(err, &parseErr) || parseErr.Kind != tt.kind {
				t.Fatalf("error = %#v, want kind %s", err, tt.kind)
			}
			if parseErr.Action == "" {
				t.Fatalf("error is not actionable: %#v", parseErr)
			}
		})
	}
}

func TestNormalizationIgnoresXMLAndElementIDOrder(t *testing.T) {
	a, err := ParseFile(testdata(t, "bpmn/order-independent-a.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	b, err := ParseFile(testdata(t, "bpmn/order-independent-b.bpmn"))
	if err != nil {
		t.Fatal(err)
	}
	shape := func(doc Document) []string {
		var got []string
		for _, e := range doc.Processes[0].Elements {
			got = append(got, e.Type+":"+e.Name)
		}
		return got
	}
	if !reflect.DeepEqual(shape(a), shape(b)) {
		t.Fatalf("normalized shapes differ:\n%v\n%v", shape(a), shape(b))
	}
}

func TestVendorTaskDefinitionCollisionRemainsOpaque(t *testing.T) {
	const input = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:vendor="urn:vendor">
  <process id="p"><serviceTask id="work"><extensionElements>
    <vendor:taskDefinition type="must-not-be-a-job" retries="99">opaque</vendor:taskDefinition>
  </extensionElements></serviceTask></process>
</definitions>`
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	task := doc.Processes[0].ElementByID("work")
	if task.JobType != "" || task.RetryCount != "" {
		t.Fatalf("vendor collision parsed as Zeebe semantics: %+v", task)
	}
	if len(task.Extensions) != 1 || task.Extensions[0].QName.Space != "urn:vendor" ||
		task.Extensions[0].QName.Local != "taskDefinition" {
		t.Fatalf("vendor extension not retained: %+v", task.Extensions)
	}
}

func TestUnknownExtensionPreservesMixedContentOrder(t *testing.T) {
	const input = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:v="urn:vendor">
  <process id="p"><task id="work"><extensionElements>
    <v:mixed>before<v:first/>middle<v:second/>after</v:mixed>
  </extensionElements></task></process>
</definitions>`
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	inner := doc.Processes[0].ElementByID("work").Extensions[0].InnerXML
	before := strings.Index(inner, "before")
	first := strings.Index(inner, "first")
	middle := strings.Index(inner, "middle")
	second := strings.Index(inner, "second")
	after := strings.Index(inner, "after")
	if !(before < first && first < middle && middle < second && second < after) {
		t.Fatalf("mixed content reordered: %q", inner)
	}
}

func TestCanonicalElementKeyIncludesEveryRetainedSemanticField(t *testing.T) {
	base := Element{Type: "boundaryEvent", Name: "same", CancelActivity: true}
	key := func(element Element) string {
		elements := map[string]Element{
			"parent": {Type: "subProcess", Name: "Parent"},
			"task":   {Type: "task", Name: "Attached task"},
			"target": {Type: "task", Name: "Default target"},
		}
		flows := map[string]Flow{
			"flow": {ID: "flow", Source: element.ID, Target: "target"},
		}
		references := referenceIndexes{
			processes: map[string]string{"called": "Called process"},
			messages:  map[string]string{"message": "Message"},
			errors:    map[string]string{"error": "Error"},
		}
		return normalizedElementKey(element, elements, flows, references, false)
	}
	tests := []struct {
		name   string
		change func(*Element)
	}{
		{"parent", func(e *Element) { e.ParentID = "parent" }},
		{"called process", func(e *Element) { e.CalledElement = "called" }},
		{"default flow", func(e *Element) { e.DefaultFlow = "flow" }},
		{"retry count", func(e *Element) { e.RetryCount = "3" }},
		{"attachment", func(e *Element) { e.AttachedTo = "task" }},
		{"event type", func(e *Element) { e.EventDefs = []string{"message"} }},
		{"event reference", func(e *Element) { e.MessageRef = "message" }},
		{"error reference", func(e *Element) { e.ErrorRef = "error" }},
		{"timer kind", func(e *Element) { e.TimerKind = "duration"; e.Timer = "PT1M" }},
		{"interruption", func(e *Element) { e.CancelActivity = false }},
		{"extension qname", func(e *Element) { e.Extensions = []Extension{{QName: xml.Name{Space: "urn:v", Local: "x"}}} }},
		{"extension attributes", func(e *Element) {
			e.Extensions = []Extension{{QName: xml.Name{Space: "urn:v", Local: "x"}, Attributes: []Attribute{{QName: xml.Name{Local: "mode"}, Value: "strict"}}}}
		}},
		{"extension content", func(e *Element) {
			e.Extensions = []Extension{{QName: xml.Name{Space: "urn:v", Local: "x"}, InnerXML: "value"}}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed := base
			tt.change(&changed)
			if key(base) == key(changed) {
				t.Fatalf("%s omitted from canonical key", tt.name)
			}
		})
	}
}

func TestNormalizationUsesReferencedSemanticsNotIDLexicalOrder(t *testing.T) {
	makeProcess := func(approveGateway, rejectGateway, approveFlow, rejectFlow string) Process {
		p := Process{
			Elements: []Element{
				{ID: approveGateway, Type: "exclusiveGateway", Name: "Decision", DefaultFlow: approveFlow},
				{ID: rejectGateway, Type: "exclusiveGateway", Name: "Decision", DefaultFlow: rejectFlow},
				{ID: "approve", Type: "task", Name: "Approve"},
				{ID: "reject", Type: "task", Name: "Reject"},
			},
			Flows: []Flow{
				{ID: approveFlow, Source: approveGateway, Target: "approve"},
				{ID: rejectFlow, Source: rejectGateway, Target: "reject"},
			},
		}
		normalizeProcess(&p)
		return p
	}
	a := makeProcess("ga", "gb", "z-flow", "a-flow")
	b := makeProcess("yb", "ya", "a2-flow", "z2-flow")
	targets := func(p Process) []string {
		byFlow := map[string]string{}
		for _, flow := range p.Flows {
			byFlow[flow.ID] = p.ElementByID(flow.Target).Name
		}
		var result []string
		for _, element := range p.Elements {
			if element.Type == "exclusiveGateway" {
				result = append(result, byFlow[element.DefaultFlow])
			}
		}
		return result
	}
	if !reflect.DeepEqual(targets(a), targets(b)) {
		t.Fatalf("normalization depends on IDs: %v != %v", targets(a), targets(b))
	}
}

func TestExtensionContentCanonicalizesNestedAttributeOrder(t *testing.T) {
	parseInner := func(attributes string) string {
		t.Helper()
		input := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:v="urn:v"><process id="p"><task id="t"><extensionElements><v:outer><v:inner ` +
			attributes + `/></v:outer></extensionElements></task></process></definitions>`
		doc, err := Parse(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		return doc.Processes[0].ElementByID("t").Extensions[0].InnerXML
	}
	a := parseInner(`z="last" a="first"`)
	b := parseInner(`a="first" z="last"`)
	if a != b {
		t.Fatalf("extension canonical content depends on attribute order:\n%s\n%s", a, b)
	}
}

func TestParseBoundaryInterruptionAndTimerKinds(t *testing.T) {
	const input = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <task id="work"/>
  <boundaryEvent id="interrupting" attachedToRef="work"><timerEventDefinition><timeDuration>PT1M</timeDuration></timerEventDefinition></boundaryEvent>
  <boundaryEvent id="nonInterrupting" attachedToRef="work" cancelActivity="false"><timerEventDefinition><timeCycle>R/PT1H</timeCycle></timerEventDefinition></boundaryEvent>
  <intermediateCatchEvent id="date"><timerEventDefinition><timeDate>2030-01-01T00:00:00Z</timeDate></timerEventDefinition></intermediateCatchEvent>
</process></definitions>`
	doc, err := Parse(strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	p := doc.Processes[0]
	assertTimer := func(id, kind, value string, cancel bool) {
		t.Helper()
		event := p.ElementByID(id)
		if event == nil || event.TimerKind != kind || event.Timer != value || event.CancelActivity != cancel {
			t.Fatalf("%s = %+v", id, event)
		}
	}
	assertTimer("interrupting", "duration", "PT1M", true)
	assertTimer("nonInterrupting", "cycle", "R/PT1H", false)
	assertTimer("date", "date", "2030-01-01T00:00:00Z", false)
}

func TestNormalizationResolvesCalledProcessSemanticsAcrossIDRenames(t *testing.T) {
	parse := func(alphaTargetID, betaTargetID string) Document {
		t.Helper()
		input := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <process id="` + alphaTargetID + `" name="Z Alpha target"><task id="alphaTask" name="Alpha"/></process>
  <process id="` + betaTargetID + `" name="Z Beta target"><task id="betaTask" name="Beta"/></process>
  <process id="wrapperOne" name="A Wrapper"><callActivity id="callOne" name="Call" calledElement="` + alphaTargetID + `"/></process>
  <process id="wrapperTwo" name="A Wrapper"><callActivity id="callTwo" name="Call" calledElement="` + betaTargetID + `"/></process>
</definitions>`
		doc, err := Parse(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	a := parse("z-target", "a-target")
	b := parse("a2-target", "z2-target")
	normalizedTargets := func(doc Document) []string {
		processNames := map[string]string{}
		for _, process := range doc.Processes {
			processNames[process.ID] = process.Name
		}
		var targets []string
		for _, process := range doc.Processes {
			if process.Name == "A Wrapper" {
				targets = append(targets, processNames[process.Elements[0].CalledElement])
			}
		}
		return targets
	}
	if gotA, gotB := normalizedTargets(a), normalizedTargets(b); !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("process normalization changed across ID rename: %v != %v", gotA, gotB)
	}
	compatibilityTarget := func(doc Document) string {
		t.Helper()
		processNames := map[string]string{}
		for _, process := range doc.Processes {
			processNames[process.ID] = process.Name
		}
		if len(doc.Elements) != 1 {
			t.Fatalf("compatibility elements = %+v", doc.Elements)
		}
		return processNames[doc.Elements[0].CalledElement]
	}
	if gotA, gotB := compatibilityTarget(a), compatibilityTarget(b); gotA != gotB {
		t.Fatalf("compatibility process changed across ID rename: %q != %q", gotA, gotB)
	}
	if a.ProcessID != b.ProcessID || a.Name != b.Name {
		t.Fatalf("compatibility identity changed: (%q, %q) != (%q, %q)", a.ProcessID, a.Name, b.ProcessID, b.Name)
	}
}

func TestNormalizationResolvesMessageAndErrorSemanticsAcrossIDRenames(t *testing.T) {
	parse := func(alphaMessageID, betaMessageID, alphaErrorID, betaErrorID string) Document {
		t.Helper()
		input := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
  <message id="` + alphaMessageID + `" name="Alpha message"/>
  <message id="` + betaMessageID + `" name="Beta message"/>
  <error id="` + alphaErrorID + `" name="Alpha error" errorCode="A"/>
  <error id="` + betaErrorID + `" name="Beta error" errorCode="B"/>
  <process id="events" name="Events">
    <intermediateCatchEvent id="messageAlpha" name="Message"><messageEventDefinition messageRef="` + alphaMessageID + `"/></intermediateCatchEvent>
    <intermediateCatchEvent id="messageBeta" name="Message"><messageEventDefinition messageRef="` + betaMessageID + `"/></intermediateCatchEvent>
    <boundaryEvent id="errorAlpha" name="Error" attachedToRef="work"><errorEventDefinition errorRef="` + alphaErrorID + `"/></boundaryEvent>
    <boundaryEvent id="errorBeta" name="Error" attachedToRef="work"><errorEventDefinition errorRef="` + betaErrorID + `"/></boundaryEvent>
    <task id="work"/>
  </process>
</definitions>`
		doc, err := Parse(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		return doc
	}
	a := parse("z-message", "a-message", "z-error", "a-error")
	b := parse("a2-message", "z2-message", "a2-error", "z2-error")
	referenceNames := func(doc Document, elements []Element) []string {
		messageNames := map[string]string{}
		for _, message := range doc.Messages {
			messageNames[message.ID] = message.Name
		}
		errorNames := map[string]string{}
		for _, definition := range doc.Errors {
			errorNames[definition.ID] = definition.Name
		}
		var result []string
		for _, element := range elements {
			if element.MessageRef != "" {
				result = append(result, messageNames[element.MessageRef])
			}
			if element.ErrorRef != "" {
				result = append(result, errorNames[element.ErrorRef])
			}
		}
		return result
	}
	if gotA, gotB := referenceNames(a, a.Processes[0].Elements), referenceNames(b, b.Processes[0].Elements); !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("event normalization changed across ID rename: %v != %v", gotA, gotB)
	}
	if gotA, gotB := referenceNames(a, a.Elements), referenceNames(b, b.Elements); !reflect.DeepEqual(gotA, gotB) {
		t.Fatalf("compatibility event order changed across ID rename: %v != %v", gotA, gotB)
	}
}

func TestUnresolvedReferencesRemainDistinctAndDeterministic(t *testing.T) {
	const input = `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p">
  <callActivity id="b" name="Call" calledElement="missing-b"/>
  <callActivity id="a" name="Call" calledElement="missing-a"/>
</process></definitions>`
	for attempt := 0; attempt < 2; attempt++ {
		doc, err := Parse(strings.NewReader(input))
		if err != nil {
			t.Fatal(err)
		}
		got := []string{doc.Processes[0].Elements[0].CalledElement, doc.Processes[0].Elements[1].CalledElement}
		if want := []string{"missing-a", "missing-b"}; !reflect.DeepEqual(got, want) {
			t.Fatalf("unresolved references were equated or unstable: %v", got)
		}
	}
	for _, kind := range []string{"process", "message", "error"} {
		left := referenceSemantic(nil, "missing-a", kind)
		right := referenceSemantic(nil, "missing-b", kind)
		if left == right || left != referenceSemantic(nil, "missing-a", kind) {
			t.Fatalf("%s unresolved references are equated or unstable: %q, %q", kind, left, right)
		}
	}
}
