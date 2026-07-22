package bpmn

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

const modelNamespace = "http://www.omg.org/spec/BPMN/20100524/MODEL"
const zeebeNamespace = "http://camunda.org/schema/zeebe/1.0"

// ParseFile reads and parses a BPMN XML file.
func ParseFile(path string) (Document, error) {
	f, err := os.Open(path)
	if err != nil {
		return Document{}, err
	}
	defer f.Close()
	return Parse(f)
}

// Parse parses and validates a BPMN XML document.
func Parse(r io.Reader) (Document, error) {
	dec := xml.NewDecoder(r)
	var root rawElement
	if err := dec.Decode(&root); err != nil {
		return Document{}, &ParseError{
			Kind: ErrorMalformedXML, Detail: err.Error(),
			Action: "fix the XML syntax and try again", Err: err,
		}
	}
	for {
		token, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return Document{}, &ParseError{
				Kind: ErrorMalformedXML, Detail: err.Error(),
				Action: "remove malformed content after the BPMN definitions element", Err: err,
			}
		}
		if data, ok := token.(xml.CharData); ok && strings.TrimSpace(string(data)) == "" {
			continue
		}
		switch token.(type) {
		case xml.Comment, xml.Directive, xml.ProcInst:
			continue
		}
		return Document{}, &ParseError{
			Kind: ErrorMalformedXML, Detail: "unexpected content after the definitions element",
			Action: "keep exactly one BPMN definitions root element",
		}
	}
	if root.Name.Local != "definitions" {
		return Document{}, &ParseError{
			Kind: ErrorInvalidRoot, Detail: "root element is " + root.Name.Local,
			Action: "provide a BPMN definitions document",
		}
	}
	if root.Name.Space != modelNamespace {
		return Document{}, &ParseError{
			Kind: ErrorInvalidNamespace, Detail: fmt.Sprintf("definitions namespace is %q", root.Name.Space),
			Action: "use the BPMN 2.0 model namespace " + modelNamespace,
		}
	}
	doc := documentFromRaw(root)
	if len(doc.Processes) == 0 {
		return Document{}, &ParseError{
			Kind: ErrorNoProcess, Action: "add at least one BPMN process",
		}
	}
	usable := 0
	for _, p := range doc.Processes {
		usable += len(p.Elements)
	}
	if usable == 0 {
		return Document{}, &ParseError{
			Kind: ErrorNoFlowNodes, Action: "add a task, gateway, event, subprocess, or call activity",
		}
	}
	normalizeDocument(&doc)
	applyCompatibilityView(&doc)
	return doc, nil
}

type rawElement struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Children []rawElement
	Text     string
	Content  []rawContent
}

type rawContent struct {
	Kind       string
	Text       string
	Child      *rawElement
	Comment    string
	Directive  string
	ProcTarget string
	ProcInst   string
}

func (e *rawElement) UnmarshalXML(dec *xml.Decoder, start xml.StartElement) error {
	e.Name, e.Attrs = start.Name, append([]xml.Attr(nil), start.Attr...)
	for {
		token, err := dec.Token()
		if err != nil {
			return err
		}
		switch value := token.(type) {
		case xml.StartElement:
			var child rawElement
			if err := dec.DecodeElement(&child, &value); err != nil {
				return err
			}
			e.Children = append(e.Children, child)
			e.Content = append(e.Content, rawContent{Kind: "child", Child: &e.Children[len(e.Children)-1]})
		case xml.CharData:
			text := string(value)
			e.Text += text
			e.Content = append(e.Content, rawContent{Kind: "text", Text: text})
		case xml.Comment:
			e.Content = append(e.Content, rawContent{Kind: "comment", Comment: string(value)})
		case xml.Directive:
			e.Content = append(e.Content, rawContent{Kind: "directive", Directive: string(value)})
		case xml.ProcInst:
			e.Content = append(e.Content, rawContent{Kind: "procinst", ProcTarget: value.Target, ProcInst: string(value.Inst)})
		case xml.EndElement:
			if value.Name == start.Name {
				return nil
			}
		}
	}
}

func documentFromRaw(root rawElement) Document {
	var doc Document
	for _, child := range root.Children {
		if child.Name.Space != modelNamespace {
			continue
		}
		switch child.Name.Local {
		case "extensionElements":
			for _, extension := range child.Children {
				doc.UnknownExtensions = append(doc.UnknownExtensions, retainExtension(extension))
			}
		case "message":
			doc.Messages = append(doc.Messages, Message{ID: attr(child, "id"), Name: attr(child, "name")})
		case "error":
			doc.Errors = append(doc.Errors, Error{
				ID: attr(child, "id"), Name: attr(child, "name"), ErrorCode: attr(child, "errorCode"),
			})
		case "process":
			doc.Processes = append(doc.Processes, processFromRaw(child, doc.Messages, &doc.UnknownExtensions))
		}
	}
	return doc
}

func processFromRaw(raw rawElement, messages []Message, documentExtensions *[]Extension) Process {
	p := Process{ID: attr(raw, "id"), Name: attr(raw, "name"), Messages: append([]Message(nil), messages...)}
	p.ProcessID = p.ID
	parseContainer(raw, "", &p, documentExtensions)
	return p
}

func parseContainer(container rawElement, parentID string, p *Process, documentExtensions *[]Extension) {
	for _, child := range container.Children {
		if child.Name.Space != modelNamespace {
			continue
		}
		if child.Name.Local == "extensionElements" {
			for _, extension := range child.Children {
				retained := retainExtension(extension)
				p.UnknownExtensions = append(p.UnknownExtensions, retained)
				*documentExtensions = append(*documentExtensions, retained)
			}
			continue
		}
		if child.Name.Local == "sequenceFlow" {
			p.Flows = append(p.Flows, Flow{
				ID: attr(child, "id"), Name: attr(child, "name"),
				Source: attr(child, "sourceRef"), Target: attr(child, "targetRef"),
				Condition: strings.TrimSpace(childText(child, "conditionExpression")),
			})
			continue
		}
		if !isFlowNode(child.Name.Local) {
			continue
		}
		element := elementFromRaw(child, parentID, documentExtensions)
		p.Elements = append(p.Elements, element)
		if child.Name.Local == "subProcess" || child.Name.Local == "transaction" || child.Name.Local == "adHocSubProcess" {
			parseContainer(child, element.ID, p, documentExtensions)
		}
	}
}

func elementFromRaw(raw rawElement, parentID string, documentExtensions *[]Extension) Element {
	e := Element{
		ID: attr(raw, "id"), Type: raw.Name.Local, Name: attr(raw, "name"),
		ParentID: parentID, CalledElement: attr(raw, "calledElement"),
		DefaultFlow: attr(raw, "default"), AttachedTo: attr(raw, "attachedToRef"),
	}
	if raw.Name.Local == "boundaryEvent" {
		cancelActivity := attr(raw, "cancelActivity")
		e.CancelActivity = cancelActivity != "false" && cancelActivity != "0"
	}
	for _, child := range raw.Children {
		switch child.Name.Local {
		case "timerEventDefinition":
			for _, timer := range child.Children {
				if timer.Name.Space != modelNamespace {
					continue
				}
				switch timer.Name.Local {
				case "timeDate":
					e.TimerKind, e.Timer = "date", strings.TrimSpace(timer.Text)
				case "timeDuration":
					e.TimerKind, e.Timer = "duration", strings.TrimSpace(timer.Text)
				case "timeCycle":
					e.TimerKind, e.Timer = "cycle", strings.TrimSpace(timer.Text)
				}
				if e.TimerKind != "" {
					break
				}
			}
			e.EventDefs = append(e.EventDefs, "timer")
		case "errorEventDefinition":
			e.ErrorRef = attr(child, "errorRef")
			e.EventDefs = append(e.EventDefs, "error")
		case "messageEventDefinition":
			e.MessageRef = attr(child, "messageRef")
			e.EventDefs = append(e.EventDefs, "message")
		case "extensionElements":
			for _, extension := range child.Children {
				if extension.Name.Space == zeebeNamespace && extension.Name.Local == "taskDefinition" {
					e.JobType = attr(extension, "type")
					e.RetryCount = attr(extension, "retries")
					continue
				}
				retained := retainExtension(extension)
				e.Extensions = append(e.Extensions, retained)
				*documentExtensions = append(*documentExtensions, retained)
			}
		}
	}
	return e
}

func isFlowNode(local string) bool {
	switch local {
	case "task", "serviceTask", "userTask", "manualTask", "businessRuleTask", "scriptTask",
		"sendTask", "receiveTask", "callActivity", "subProcess", "transaction", "adHocSubProcess",
		"exclusiveGateway", "parallelGateway", "inclusiveGateway", "complexGateway", "eventBasedGateway",
		"startEvent", "endEvent", "intermediateCatchEvent", "intermediateThrowEvent", "boundaryEvent":
		return true
	default:
		return false
	}
}

func retainExtension(raw rawElement) Extension {
	extension := Extension{QName: raw.Name}
	for _, a := range raw.Attrs {
		extension.Attributes = append(extension.Attributes, Attribute{QName: a.Name, Value: a.Value})
	}
	var b bytes.Buffer
	enc := xml.NewEncoder(&b)
	for _, content := range raw.Content {
		_ = encodeRawContent(enc, content)
	}
	_ = enc.Flush()
	extension.InnerXML = b.String()
	return extension
}

func encodeRaw(enc *xml.Encoder, raw rawElement) error {
	attrs := append([]xml.Attr(nil), raw.Attrs...)
	sort.SliceStable(attrs, func(i, j int) bool {
		return attrs[i].Name.Space+"\x00"+attrs[i].Name.Local+"\x00"+attrs[i].Value <
			attrs[j].Name.Space+"\x00"+attrs[j].Name.Local+"\x00"+attrs[j].Value
	})
	start := xml.StartElement{Name: raw.Name, Attr: attrs}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, content := range raw.Content {
		if err := encodeRawContent(enc, content); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

func encodeRawContent(enc *xml.Encoder, content rawContent) error {
	switch content.Kind {
	case "child":
		return encodeRaw(enc, *content.Child)
	case "text":
		return enc.EncodeToken(xml.CharData(content.Text))
	case "comment":
		return enc.EncodeToken(xml.Comment(content.Comment))
	case "directive":
		return enc.EncodeToken(xml.Directive(content.Directive))
	case "procinst":
		return enc.EncodeToken(xml.ProcInst{Target: content.ProcTarget, Inst: []byte(content.ProcInst)})
	default:
		return nil
	}
}

func attr(raw rawElement, local string) string {
	for _, a := range raw.Attrs {
		if a.Name.Local == local {
			return strings.TrimSpace(a.Value)
		}
	}
	return ""
}

func childText(raw rawElement, local string) string {
	for _, child := range raw.Children {
		if child.Name.Local == local {
			return strings.TrimSpace(child.Text)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
