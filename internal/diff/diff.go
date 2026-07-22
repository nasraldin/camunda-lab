package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// Compare returns deterministic semantic changes from before to after.
func Compare(before, after bpmn.Document) []Change {
	beforeResolver := newResolver(before)
	afterResolver := newResolver(after)
	var changes []Change
	if left, right := extensionsFingerprint(documentScopeExtensions(before)), extensionsFingerprint(documentScopeExtensions(after)); left != right {
		changes = append(changes, newChange(
			FieldChanged, "", "", "document", "extensions", left, right,
			"Changed document retained extensions",
		))
	}

	beforeProcesses, afterProcesses := pairProcesses(before, after, beforeResolver, afterResolver)
	for _, pair := range append(beforeProcesses, afterProcesses...) {
		switch {
		case pair.before == nil:
			changes = append(changes, newChange(ProcessAdded, pair.after.ID, "", "", "", "", "",
				fmt.Sprintf("Added process %s", label(pair.after.ID, pair.after.Name))))
		case pair.after == nil:
			changes = append(changes, newChange(ProcessRemoved, pair.before.ID, "", "", "", "", "",
				fmt.Sprintf("Removed process %s", label(pair.before.ID, pair.before.Name))))
		default:
			changes = append(changes, compareProcess(*pair.before, *pair.after, beforeResolver, afterResolver)...)
		}
	}
	changes = append(changes, compareMessages(before.Messages, after.Messages)...)
	changes = append(changes, compareErrors(before.Errors, after.Errors)...)
	sortChanges(changes)
	return changes
}

type processPair struct {
	before *bpmn.Process
	after  *bpmn.Process
}

func pairProcesses(before, after bpmn.Document, br, ar resolver) ([]processPair, []processPair) {
	afterByID := processIndex(after.Processes)
	var paired, unmatched []processPair
	usedBefore := map[string]bool{}
	usedAfter := map[string]bool{}

	// Pass 1: preserve exact semantic entities that also retain their ID.
	for _, process := range sortedProcesses(before.Processes) {
		candidate, ok := afterByID[process.ID]
		if ok && processSignature(process, br) == processSignature(candidate, ar) {
			left, right := process, candidate
			paired = append(paired, processPair{before: &left, after: &right})
			usedBefore[process.ID], usedAfter[candidate.ID] = true, true
		}
	}
	// Pass 2: pair remaining exact semantics across changed IDs.
	for _, process := range sortedProcesses(before.Processes) {
		if usedBefore[process.ID] {
			continue
		}
		signature := processSignature(process, br)
		for _, possible := range sortedProcesses(after.Processes) {
			if process.ID != possible.ID && !usedAfter[possible.ID] && processSignature(possible, ar) == signature {
				left, right := process, possible
				paired = append(paired, processPair{before: &left, after: &right})
				usedBefore[process.ID], usedAfter[possible.ID] = true, true
				break
			}
		}
	}
	// Pass 3: pair remaining same IDs so field changes are reported.
	for _, process := range sortedProcesses(before.Processes) {
		if usedBefore[process.ID] {
			continue
		}
		if candidate, ok := afterByID[process.ID]; ok && !usedAfter[candidate.ID] {
			left, right := process, candidate
			paired = append(paired, processPair{before: &left, after: &right})
			usedBefore[process.ID], usedAfter[candidate.ID] = true, true
			continue
		}
		left := process
		unmatched = append(unmatched, processPair{before: &left})
	}
	for _, process := range sortedProcesses(after.Processes) {
		if usedAfter[process.ID] {
			continue
		}
		right := process
		unmatched = append(unmatched, processPair{after: &right})
	}
	return paired, unmatched
}

func compareProcess(before, after bpmn.Process, br, ar resolver) []Change {
	processID := before.ID
	if processID == "" {
		processID = after.ID
	}
	var changes []Change
	if before.Name != after.Name {
		changes = append(changes, newChange(
			FieldChanged, processID, "", "process", "name", before.Name, after.Name,
			fmt.Sprintf("Changed process %s name: %s -> %s", processID, shown(before.Name), shown(after.Name)),
		))
	}
	if left, right := extensionsFingerprint(processScopeExtensions(before)), extensionsFingerprint(processScopeExtensions(after)); left != right {
		changes = append(changes, newChange(
			FieldChanged, processID, "", "process", "extensions", left, right,
			fmt.Sprintf("Changed process %s retained extensions", processID),
		))
	}
	rounds := max(len(before.Elements), len(after.Elements)) + 1
	beforeIdentities := elementStructuralIdentities(before, br, rounds)
	afterIdentities := elementStructuralIdentities(after, ar, rounds)
	elementPairs, elementUnmatched := pairElements(before, after, beforeIdentities, afterIdentities)
	for _, pair := range append(elementPairs, elementUnmatched...) {
		switch {
		case pair.before == nil:
			element := *pair.after
			changes = append(changes, newChange(ElementAdded, processID, element.ID, element.Type, "", "", "",
				fmt.Sprintf("Added %s %s", element.Type, label(element.ID, element.Name))))
		case pair.after == nil:
			element := *pair.before
			changes = append(changes, newChange(ElementRemoved, processID, element.ID, element.Type, "", "", "",
				fmt.Sprintf("Removed %s %s", element.Type, label(element.ID, element.Name))))
		default:
			changes = append(changes, compareElement(
				processID, *pair.before, *pair.after, before, after, br, ar, beforeIdentities, afterIdentities,
			)...)
		}
	}
	changes = append(changes, compareFlows(processID, before, after, beforeIdentities, afterIdentities)...)
	return changes
}

type elementPair struct {
	before *bpmn.Element
	after  *bpmn.Element
}

func pairElements(before, after bpmn.Process, beforeIdentities, afterIdentities map[string]string) ([]elementPair, []elementPair) {
	afterByID := elementIndex(after.Elements)
	var paired, unmatched []elementPair
	usedBefore := map[string]bool{}
	usedAfter := map[string]bool{}
	// Pass 1: exact semantic identity with the same ID.
	for _, element := range sortedElements(before.Elements) {
		candidate, ok := afterByID[element.ID]
		if ok && beforeIdentities[element.ID] == afterIdentities[candidate.ID] {
			left, right := element, candidate
			paired = append(paired, elementPair{before: &left, after: &right})
			usedBefore[element.ID], usedAfter[candidate.ID] = true, true
		}
	}
	// Pass 2: remaining exact semantics across different IDs.
	for _, element := range sortedElements(before.Elements) {
		if usedBefore[element.ID] {
			continue
		}
		for _, possible := range sortedElements(after.Elements) {
			if element.ID != possible.ID && !usedAfter[possible.ID] &&
				beforeIdentities[element.ID] == afterIdentities[possible.ID] {
				left, right := element, possible
				paired = append(paired, elementPair{before: &left, after: &right})
				usedBefore[element.ID], usedAfter[possible.ID] = true, true
				break
			}
		}
	}
	// Pass 3: remaining same IDs are field changes.
	for _, element := range sortedElements(before.Elements) {
		if usedBefore[element.ID] {
			continue
		}
		if candidate, ok := afterByID[element.ID]; ok && !usedAfter[candidate.ID] {
			left, right := element, candidate
			paired = append(paired, elementPair{before: &left, after: &right})
			usedBefore[element.ID], usedAfter[candidate.ID] = true, true
			continue
		}
		left := element
		unmatched = append(unmatched, elementPair{before: &left})
	}
	for _, element := range sortedElements(after.Elements) {
		if usedAfter[element.ID] {
			continue
		}
		right := element
		unmatched = append(unmatched, elementPair{after: &right})
	}
	return paired, unmatched
}

func compareElement(
	processID string,
	before, after bpmn.Element,
	bp, ap bpmn.Process,
	br, ar resolver,
	beforeIdentities, afterIdentities map[string]string,
) []Change {
	id := before.ID
	elementType := after.Type
	var changes []Change
	addSemantic := func(field, leftSemantic, rightSemantic, left, right string) {
		if leftSemantic == rightSemantic {
			return
		}
		changes = append(changes, newChange(FieldChanged, processID, id, elementType, field, left, right,
			fmt.Sprintf("Changed %s %s %s: %s -> %s", elementType, id, field, shown(left), shown(right))))
	}
	add := func(field, left, right string) {
		addSemantic(field, left, right, left, right)
	}
	add("type", before.Type, after.Type)
	add("name", before.Name, after.Name)
	add("job_type", before.JobType, after.JobType)
	add("retry_count", before.RetryCount, after.RetryCount)
	addSemantic("parent",
		referenceComparison(before.ParentID, after.ParentID, color(beforeIdentities, before.ParentID), color(afterIdentities, after.ParentID)),
		"equal",
		before.ParentID, after.ParentID)
	addSemantic("default_flow",
		referenceComparison(
			before.DefaultFlow, after.DefaultFlow,
			flowReferenceIdentity(before.DefaultFlow, bp, beforeIdentities),
			flowReferenceIdentity(after.DefaultFlow, ap, afterIdentities),
		),
		"equal",
		before.DefaultFlow, after.DefaultFlow)
	add("event_kind", eventKind(before), eventKind(after))
	addSemantic("event_ref", br.eventReference(before), ar.eventReference(after),
		rawEventReference(before), rawEventReference(after))
	add("event_value", before.Timer, after.Timer)
	addSemantic("attached_to",
		referenceComparison(before.AttachedTo, after.AttachedTo, color(beforeIdentities, before.AttachedTo), color(afterIdentities, after.AttachedTo)),
		"equal",
		before.AttachedTo, after.AttachedTo)
	if before.Type == "boundaryEvent" || after.Type == "boundaryEvent" {
		add("interrupting", strconv.FormatBool(before.CancelActivity), strconv.FormatBool(after.CancelActivity))
	}
	addSemantic("call_target", br.processReference(before.CalledElement), ar.processReference(after.CalledElement),
		before.CalledElement, after.CalledElement)
	add("extensions", extensionsFingerprint(before.Extensions), extensionsFingerprint(after.Extensions))
	return changes
}

func compareFlows(
	processID string,
	before, after bpmn.Process,
	beforeIdentities, afterIdentities map[string]string,
) []Change {
	afterByID := flowIndex(after.Flows)
	beforeFlowIdentities := make(map[string]string, len(before.Flows))
	afterFlowIdentities := make(map[string]string, len(after.Flows))
	for _, flow := range before.Flows {
		beforeFlowIdentities[flow.ID] = flowStructuralIdentity(flow, beforeIdentities)
	}
	for _, flow := range after.Flows {
		afterFlowIdentities[flow.ID] = flowStructuralIdentity(flow, afterIdentities)
	}
	usedBefore := map[string]bool{}
	usedAfter := map[string]bool{}
	var changes []Change
	// Pass 1: exact semantic identity with the same ID.
	for _, flow := range sortedFlows(before.Flows) {
		candidate, ok := afterByID[flow.ID]
		if ok && beforeFlowIdentities[flow.ID] == afterFlowIdentities[candidate.ID] {
			usedBefore[flow.ID], usedAfter[candidate.ID] = true, true
		}
	}
	// Pass 2: remaining exact semantics across different IDs.
	for _, flow := range sortedFlows(before.Flows) {
		if usedBefore[flow.ID] {
			continue
		}
		for _, candidate := range sortedFlows(after.Flows) {
			if flow.ID != candidate.ID && !usedAfter[candidate.ID] &&
				beforeFlowIdentities[flow.ID] == afterFlowIdentities[candidate.ID] {
				usedBefore[flow.ID], usedAfter[candidate.ID] = true, true
				break
			}
		}
	}
	// Pass 3: remaining same IDs are field changes.
	for _, flow := range sortedFlows(before.Flows) {
		if usedBefore[flow.ID] {
			continue
		}
		if candidate, ok := afterByID[flow.ID]; ok && !usedAfter[candidate.ID] {
			usedBefore[flow.ID], usedAfter[candidate.ID] = true, true
			changes = append(changes, compareFlow(
				processID, flow, candidate, beforeIdentities, afterIdentities,
			)...)
			continue
		}
		changes = append(changes, newChange(FlowRemoved, processID, flow.ID, "sequenceFlow", "", "", "",
			fmt.Sprintf("Removed sequence flow %s", flow.ID)))
	}
	for _, flow := range sortedFlows(after.Flows) {
		if usedAfter[flow.ID] {
			continue
		}
		changes = append(changes, newChange(FlowAdded, processID, flow.ID, "sequenceFlow", "", "", "",
			fmt.Sprintf("Added sequence flow %s", flow.ID)))
	}
	return changes
}

func compareFlow(
	processID string,
	before, after bpmn.Flow,
	beforeIdentities, afterIdentities map[string]string,
) []Change {
	var changes []Change
	addSemantic := func(field, leftSemantic, rightSemantic, left, right string) {
		if leftSemantic == rightSemantic {
			return
		}
		changes = append(changes, newChange(FieldChanged, processID, before.ID, "sequenceFlow", field, left, right,
			fmt.Sprintf("Changed sequence flow %s %s: %s -> %s", before.ID, field, shown(left), shown(right))))
	}
	addSemantic("source",
		referenceComparison(before.Source, after.Source, color(beforeIdentities, before.Source), color(afterIdentities, after.Source)),
		"equal",
		before.Source, after.Source)
	addSemantic("target",
		referenceComparison(before.Target, after.Target, color(beforeIdentities, before.Target), color(afterIdentities, after.Target)),
		"equal",
		before.Target, after.Target)
	addSemantic("name", before.Name, after.Name, before.Name, after.Name)
	addSemantic("condition", before.Condition, after.Condition, before.Condition, after.Condition)
	return changes
}

func compareMessages(before, after []bpmn.Message) []Change {
	afterByID := messageIndex(after)
	usedBefore := map[string]bool{}
	usedAfter := map[string]bool{}
	var changes []Change
	// Pass 1: exact semantic identity with the same ID.
	for _, message := range sortedMessages(before) {
		candidate, ok := afterByID[message.ID]
		if ok && candidate.Name == message.Name {
			usedBefore[message.ID], usedAfter[candidate.ID] = true, true
		}
	}
	// Pass 2: remaining exact semantics across different IDs.
	for _, message := range sortedMessages(before) {
		if usedBefore[message.ID] {
			continue
		}
		for _, candidate := range sortedMessages(after) {
			if message.ID != candidate.ID && !usedAfter[candidate.ID] && candidate.Name == message.Name {
				usedBefore[message.ID], usedAfter[candidate.ID] = true, true
				break
			}
		}
	}
	// Pass 3: remaining same IDs are field changes.
	for _, message := range sortedMessages(before) {
		if usedBefore[message.ID] {
			continue
		}
		if candidate, ok := afterByID[message.ID]; ok && !usedAfter[candidate.ID] {
			usedBefore[message.ID], usedAfter[candidate.ID] = true, true
			changes = append(changes, newChange(FieldChanged, "", message.ID, "message", "name", message.Name, candidate.Name,
				fmt.Sprintf("Changed message %s name: %s -> %s", message.ID, shown(message.Name), shown(candidate.Name))))
			continue
		}
		changes = append(changes, newChange(MessageRemoved, "", message.ID, "message", "", "", "",
			fmt.Sprintf("Removed message %s", label(message.ID, message.Name))))
	}
	for _, message := range sortedMessages(after) {
		if usedAfter[message.ID] {
			continue
		}
		changes = append(changes, newChange(MessageAdded, "", message.ID, "message", "", "", "",
			fmt.Sprintf("Added message %s", label(message.ID, message.Name))))
	}
	return changes
}

func compareErrors(before, after []bpmn.Error) []Change {
	afterByID := errorIndex(after)
	usedBefore := map[string]bool{}
	usedAfter := map[string]bool{}
	var changes []Change
	// Pass 1: exact semantic identity with the same ID.
	for _, definition := range sortedErrors(before) {
		candidate, ok := afterByID[definition.ID]
		if ok && candidate.Name == definition.Name && candidate.ErrorCode == definition.ErrorCode {
			usedBefore[definition.ID], usedAfter[candidate.ID] = true, true
		}
	}
	// Pass 2: remaining exact semantics across different IDs.
	for _, definition := range sortedErrors(before) {
		if usedBefore[definition.ID] {
			continue
		}
		for _, candidate := range sortedErrors(after) {
			if definition.ID != candidate.ID && !usedAfter[candidate.ID] &&
				candidate.Name == definition.Name && candidate.ErrorCode == definition.ErrorCode {
				usedBefore[definition.ID], usedAfter[candidate.ID] = true, true
				break
			}
		}
	}
	// Pass 3: remaining same IDs are field changes.
	for _, definition := range sortedErrors(before) {
		if usedBefore[definition.ID] {
			continue
		}
		if candidate, ok := afterByID[definition.ID]; ok && !usedAfter[candidate.ID] {
			usedBefore[definition.ID], usedAfter[candidate.ID] = true, true
			if definition.Name != candidate.Name {
				changes = append(changes, newChange(
					FieldChanged, "", definition.ID, "error", "name", definition.Name, candidate.Name,
					fmt.Sprintf("Changed error %s name: %s -> %s", definition.ID, shown(definition.Name), shown(candidate.Name)),
				))
			}
			if definition.ErrorCode != candidate.ErrorCode {
				changes = append(changes, newChange(
					FieldChanged, "", definition.ID, "error", "error_code", definition.ErrorCode, candidate.ErrorCode,
					fmt.Sprintf("Changed error %s error_code: %s -> %s", definition.ID, shown(definition.ErrorCode), shown(candidate.ErrorCode)),
				))
			}
			continue
		}
		changes = append(changes, newChange(
			ErrorRemoved, "", definition.ID, "error", "", "", "",
			fmt.Sprintf("Removed error %s", label(definition.ID, definition.Name)),
		))
	}
	for _, definition := range sortedErrors(after) {
		if usedAfter[definition.ID] {
			continue
		}
		changes = append(changes, newChange(
			ErrorAdded, "", definition.ID, "error", "", "", "",
			fmt.Sprintf("Added error %s", label(definition.ID, definition.Name)),
		))
	}
	return changes
}

type resolver struct {
	processes map[string]string
	messages  map[string]string
	errors    map[string]string
}

func newResolver(document bpmn.Document) resolver {
	r := resolver{processes: map[string]string{}, messages: map[string]string{}, errors: map[string]string{}}
	for _, message := range document.Messages {
		r.messages[message.ID] = "message:" + message.Name
	}
	for _, definition := range document.Errors {
		r.errors[definition.ID] = "error:" + definition.Name + "\x00" + definition.ErrorCode
	}
	for _, process := range document.Processes {
		r.processes[process.ID] = "process:" + process.Name
	}
	for round := 0; round <= len(document.Processes); round++ {
		next := map[string]string{}
		for _, process := range document.Processes {
			sum := sha256.Sum256([]byte(processSignature(process, r)))
			next[process.ID] = "process:" + process.Name + ":" + hex.EncodeToString(sum[:])
		}
		r.processes = next
	}
	return r
}

func (r resolver) processReference(id string) string { return reference(r.processes, "process", id) }
func (r resolver) eventReference(element bpmn.Element) string {
	switch {
	case element.MessageRef != "":
		return reference(r.messages, "message", element.MessageRef)
	case element.ErrorRef != "":
		return reference(r.errors, "error", element.ErrorRef)
	default:
		return ""
	}
}

func processSignature(process bpmn.Process, r resolver) string {
	identities := elementStructuralIdentities(process, r, len(process.Elements)+1)
	var elements, flows []string
	for _, element := range process.Elements {
		elements = append(elements, identities[element.ID])
	}
	for _, flow := range process.Flows {
		flows = append(flows, flowStructuralIdentity(flow, identities))
	}
	sort.Strings(elements)
	sort.Strings(flows)
	return strings.Join([]string{
		process.Name,
		strings.Join(elements, "\x02"),
		strings.Join(flows, "\x02"),
		extensionsFingerprint(processScopeExtensions(process)),
	}, "\x03")
}

func elementCore(element bpmn.Element, r resolver) string {
	return strings.Join([]string{
		element.Type, element.Name, r.processReference(element.CalledElement), element.JobType, element.RetryCount,
		strconv.FormatBool(element.CancelActivity), strings.Join(sortedCopy(element.EventDefs), ","),
		element.TimerKind, element.Timer, r.eventReference(element),
	}, "\x00")
}

func flowReferenceIdentity(id string, process bpmn.Process, identities map[string]string) string {
	if id == "" {
		return ""
	}
	if flow, ok := flowIndex(process.Flows)[id]; ok {
		return flowStructuralIdentity(flow, identities)
	}
	return "unresolved:flow:" + id
}

func reference(index map[string]string, kind, id string) string {
	if id == "" {
		return ""
	}
	if value, ok := index[id]; ok {
		return value
	}
	return "unresolved:" + kind + ":" + id
}

func referenceComparison(leftID, rightID, leftSemantic, rightSemantic string) string {
	if leftID == rightID || leftSemantic == rightSemantic {
		return "equal"
	}
	return leftSemantic + "\x00" + rightSemantic
}

func eventKind(element bpmn.Element) string {
	kinds := sortedCopy(element.EventDefs)
	for index, kind := range kinds {
		if kind == "timer" && element.TimerKind != "" {
			kinds[index] += "/" + element.TimerKind
		}
	}
	return strings.Join(kinds, ",")
}

func rawEventReference(element bpmn.Element) string {
	if element.MessageRef != "" {
		return element.MessageRef
	}
	return element.ErrorRef
}

func newChange(kind ChangeKind, processID, elementID, elementType, field, before, after, summary string) Change {
	return Change{
		Kind: kind, ProcessID: processID, ElementID: elementID, ElementType: elementType,
		Field: field, Before: before, After: after, Summary: summary, ID: elementID,
	}
}

func sortChanges(changes []Change) {
	sort.SliceStable(changes, func(i, j int) bool {
		left, right := changes[i], changes[j]
		return strings.Join([]string{string(left.Kind), left.ProcessID, left.ElementID, left.Field, left.Before, left.After}, "\x00") <
			strings.Join([]string{string(right.Kind), right.ProcessID, right.ElementID, right.Field, right.Before, right.After}, "\x00")
	})
}

func processIndex(values []bpmn.Process) map[string]bpmn.Process {
	result := make(map[string]bpmn.Process, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func elementIndex(values []bpmn.Element) map[string]bpmn.Element {
	result := make(map[string]bpmn.Element, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func flowIndex(values []bpmn.Flow) map[string]bpmn.Flow {
	result := make(map[string]bpmn.Flow, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func messageIndex(values []bpmn.Message) map[string]bpmn.Message {
	result := make(map[string]bpmn.Message, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func errorIndex(values []bpmn.Error) map[string]bpmn.Error {
	result := make(map[string]bpmn.Error, len(values))
	for _, value := range values {
		result[value.ID] = value
	}
	return result
}

func sortedProcesses(values []bpmn.Process) []bpmn.Process {
	result := append([]bpmn.Process(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedElements(values []bpmn.Element) []bpmn.Element {
	result := append([]bpmn.Element(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedFlows(values []bpmn.Flow) []bpmn.Flow {
	result := append([]bpmn.Flow(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedMessages(values []bpmn.Message) []bpmn.Message {
	result := append([]bpmn.Message(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedErrors(values []bpmn.Error) []bpmn.Error {
	result := append([]bpmn.Error(nil), values...)
	sort.Slice(result, func(i, j int) bool { return result[i].ID < result[j].ID })
	return result
}

func sortedCopy(values []string) []string {
	result := append([]string(nil), values...)
	sort.Strings(result)
	return result
}

func label(id, name string) string {
	if name == "" {
		return id
	}
	return fmt.Sprintf("%s (%q)", id, name)
}

func shown(value string) string {
	if value == "" {
		return "(none)"
	}
	return strconv.Quote(value)
}
