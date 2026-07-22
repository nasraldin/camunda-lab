package bpmn

import (
	"crypto/sha256"
	"sort"
	"strconv"
	"strings"
)

type referenceIndexes struct {
	processes map[string]string
	messages  map[string]string
	errors    map[string]string
}

func normalizeDocument(doc *Document) {
	sort.SliceStable(doc.Messages, func(i, j int) bool {
		return messageKey(doc.Messages[i]) < messageKey(doc.Messages[j])
	})
	sort.SliceStable(doc.Errors, func(i, j int) bool {
		return errorKey(doc.Errors[i]) < errorKey(doc.Errors[j])
	})
	sortExtensions(doc.UnknownExtensions)
	for i := range doc.Processes {
		normalizeRetainedCollections(&doc.Processes[i])
	}
	references := buildReferenceIndexes(*doc)
	for i := range doc.Processes {
		normalizeProcessWithReferences(&doc.Processes[i], references)
		doc.Processes[i].Messages = append([]Message(nil), doc.Messages...)
	}
	sort.SliceStable(doc.Processes, func(i, j int) bool {
		return processKey(doc.Processes[i], references) < processKey(doc.Processes[j], references)
	})
}

func normalizeProcess(p *Process) {
	normalizeProcessWithReferences(p, referenceIndexes{})
}

func normalizeProcessWithReferences(p *Process, references referenceIndexes) {
	normalizeRetainedCollections(p)
	index := make(map[string]Element, len(p.Elements))
	for _, element := range p.Elements {
		index[element.ID] = element
	}
	flows := make(map[string]Flow, len(p.Flows))
	for _, flow := range p.Flows {
		flows[flow.ID] = flow
	}
	sort.SliceStable(p.Elements, func(i, j int) bool {
		return normalizedElementKey(p.Elements[i], index, flows, references, true) <
			normalizedElementKey(p.Elements[j], index, flows, references, true)
	})
	sort.SliceStable(p.Flows, func(i, j int) bool {
		return normalizedFlowKey(p.Flows[i], index, references, true) <
			normalizedFlowKey(p.Flows[j], index, references, true)
	})
}

func normalizeRetainedCollections(p *Process) {
	for i := range p.Elements {
		sort.Strings(p.Elements[i].EventDefs)
		sortExtensions(p.Elements[i].Extensions)
	}
	sortExtensions(p.UnknownExtensions)
}

func applyCompatibilityView(doc *Document) {
	if len(doc.Processes) == 0 {
		return
	}
	first := doc.Processes[0]
	doc.ProcessID = first.ID
	doc.Name = first.Name
	doc.Elements = append([]Element(nil), first.Elements...)
	doc.Flows = append([]Flow(nil), first.Flows...)
}

func processKey(p Process, references referenceIndexes) string {
	return processSemanticKey(p, references) + "\x00" + p.ID
}

func processSemanticKey(p Process, references referenceIndexes) string {
	return strings.Join([]string{p.Name, processShape(p, references)}, "\x00")
}

func processShape(p Process, references referenceIndexes) string {
	var elementParts, flowParts, extensionParts []string
	index := make(map[string]Element, len(p.Elements))
	for _, element := range p.Elements {
		index[element.ID] = element
	}
	flows := make(map[string]Flow, len(p.Flows))
	for _, flow := range p.Flows {
		flows[flow.ID] = flow
	}
	for _, element := range p.Elements {
		elementParts = append(elementParts, normalizedElementKey(element, index, flows, references, false))
	}
	for _, flow := range p.Flows {
		flowParts = append(flowParts, normalizedFlowKey(flow, index, references, false))
	}
	for _, extension := range p.UnknownExtensions {
		extensionParts = append(extensionParts, extensionKey(extension))
	}
	sort.Strings(elementParts)
	sort.Strings(flowParts)
	sort.Strings(extensionParts)
	return strings.Join([]string{
		strings.Join(elementParts, "|"),
		strings.Join(flowParts, "|"),
		strings.Join(extensionParts, "|"),
	}, "\x03")
}

func normalizedElementKey(
	e Element,
	elements map[string]Element,
	flows map[string]Flow,
	references referenceIndexes,
	includeID bool,
) string {
	parent := resolvedElementCoreKey(elements[e.ParentID], references)
	attached := resolvedElementCoreKey(elements[e.AttachedTo], references)
	defaultFlow := ""
	if flow, ok := flows[e.DefaultFlow]; ok {
		defaultFlow = normalizedFlowKey(flow, elements, references, false)
	}
	key := strings.Join([]string{
		parent, resolvedElementCoreKey(e, references), defaultFlow, attached,
	}, "\x00")
	if includeID {
		return key + "\x00" + e.ID
	}
	return key
}

func resolvedElementCoreKey(e Element, references referenceIndexes) string {
	var extensions []string
	for _, extension := range e.Extensions {
		extensions = append(extensions, extensionKey(extension))
	}
	return strings.Join([]string{
		e.Type, e.Name, referenceSemantic(references.processes, e.CalledElement, "process"),
		e.JobType, e.RetryCount,
		strconv.FormatBool(e.CancelActivity), strings.Join(e.EventDefs, ","),
		e.TimerKind, e.Timer,
		referenceSemantic(references.messages, e.MessageRef, "message"),
		referenceSemantic(references.errors, e.ErrorRef, "error"),
		strings.Join(extensions, "\x02"),
	}, "\x00")
}

func normalizedFlowKey(
	f Flow,
	elements map[string]Element,
	references referenceIndexes,
	includeID bool,
) string {
	key := strings.Join([]string{
		resolvedElementCoreKey(elements[f.Source], references),
		resolvedElementCoreKey(elements[f.Target], references),
		f.Condition, f.Name,
	}, "\x00")
	if includeID {
		return key + "\x00" + f.ID
	}
	return key
}

func buildReferenceIndexes(doc Document) referenceIndexes {
	references := referenceIndexes{
		processes: map[string]string{},
		messages:  map[string]string{},
		errors:    map[string]string{},
	}
	for _, message := range doc.Messages {
		references.messages[message.ID] = message.Name
	}
	for _, definition := range doc.Errors {
		references.errors[definition.ID] = strings.Join([]string{definition.Name, definition.ErrorCode}, "\x00")
	}

	// Seed process references with names, then refine them through referenced
	// process shapes. Bounded refinement handles nested and cyclic call graphs
	// deterministically without allowing raw process IDs into resolved keys.
	for _, process := range doc.Processes {
		references.processes[process.ID] = process.Name
	}
	for round := 0; round <= len(doc.Processes); round++ {
		enriched := make(map[string]string, len(doc.Processes))
		unchanged := true
		for _, process := range doc.Processes {
			sum := sha256.Sum256([]byte(processSemanticKey(process, references)))
			semantic := string(sum[:])
			enriched[process.ID] = semantic
			if semantic != references.processes[process.ID] {
				unchanged = false
			}
		}
		references.processes = enriched
		if unchanged {
			break
		}
	}
	return references
}

func referenceSemantic(index map[string]string, id, kind string) string {
	if id == "" {
		return ""
	}
	if semantic, ok := index[id]; ok {
		return "resolved:" + kind + ":" + semantic
	}
	return "unresolved:" + kind + ":" + id
}

func messageKey(m Message) string {
	return strings.Join([]string{m.Name, m.ID}, "\x00")
}

func errorKey(e Error) string {
	return strings.Join([]string{e.Name, e.ErrorCode, e.ID}, "\x00")
}

func sortExtensions(extensions []Extension) {
	for i := range extensions {
		sort.SliceStable(extensions[i].Attributes, func(a, b int) bool {
			left := extensions[i].Attributes[a]
			right := extensions[i].Attributes[b]
			return left.QName.Space+"\x00"+left.QName.Local+"\x00"+left.Value <
				right.QName.Space+"\x00"+right.QName.Local+"\x00"+right.Value
		})
	}
	sort.SliceStable(extensions, func(i, j int) bool {
		return extensionKey(extensions[i]) < extensionKey(extensions[j])
	})
}

func extensionKey(extension Extension) string {
	attributes := append([]Attribute(nil), extension.Attributes...)
	sort.SliceStable(attributes, func(i, j int) bool {
		return attributes[i].QName.Space+"\x00"+attributes[i].QName.Local+"\x00"+attributes[i].Value <
			attributes[j].QName.Space+"\x00"+attributes[j].QName.Local+"\x00"+attributes[j].Value
	})
	var parts []string
	for _, attribute := range attributes {
		parts = append(parts, attribute.QName.Space, attribute.QName.Local, attribute.Value)
	}
	return strings.Join([]string{
		extension.QName.Space, extension.QName.Local,
		strings.Join(parts, "\x01"), extension.InnerXML,
	}, "\x00")
}
