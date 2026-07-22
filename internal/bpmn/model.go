package bpmn

import (
	"encoding/xml"
	"fmt"
)

// Document is a normalized BPMN definitions document.
type Document struct {
	Processes         []Process
	Messages          []Message
	Errors            []Error
	UnknownExtensions []Extension

	// Deprecated compatibility view of the first normalized process.
	ProcessID string
	Name      string
	Elements  []Element
	Flows     []Flow
}

// Model is retained as a compatibility alias for downstream D1 tools.
type Model = Document

// Process is one executable or reusable process in a BPMN document.
type Process struct {
	ID                string
	ProcessID         string // compatibility name for ID
	Name              string
	Elements          []Element
	Flows             []Flow
	Messages          []Message // document messages, for compatibility adapters
	UnknownExtensions []Extension
}

// Element is a flow node (task, event, gateway, …).
type Element struct {
	ID             string
	Type           string // startEvent, endEvent, serviceTask, userTask, scriptTask, exclusiveGateway, parallelGateway, inclusiveGateway, intermediateCatchEvent, boundaryEvent, …
	Name           string
	ParentID       string // containing subprocess, if any
	CalledElement  string
	DefaultFlow    string // exclusive gateway default
	Timer          string // compatibility timer value
	TimerKind      string // date, duration, or cycle
	CancelActivity bool   // boundary-event interruption state
	RetryCount     string // zeebe:taskDefinition retries or similar
	ErrorRef       string
	MessageRef     string
	JobType        string // zeebe task type
	AttachedTo     string // boundary event
	EventDefs      []string
	Extensions     []Extension
}

// Flow is a sequence flow.
type Flow struct {
	ID        string
	Name      string
	Source    string
	Target    string
	Condition string
}

// Message is a BPMN message definition.
type Message struct {
	ID   string
	Name string
}

// Error is a BPMN error definition.
type Error struct {
	ID        string
	Name      string
	ErrorCode string
}

// Attribute retains an extension attribute's expanded QName and value.
type Attribute struct {
	QName xml.Name
	Value string
}

// Extension retains an extension element without assigning semantics to it.
type Extension struct {
	QName      xml.Name
	Attributes []Attribute
	InnerXML   string
}

// ErrorKind categorizes a parse failure for callers.
type ErrorKind string

const (
	ErrorMalformedXML     ErrorKind = "malformed_xml"
	ErrorInvalidRoot      ErrorKind = "invalid_root"
	ErrorInvalidNamespace ErrorKind = "invalid_namespace"
	ErrorNoProcess        ErrorKind = "no_process"
	ErrorNoFlowNodes      ErrorKind = "no_flow_nodes"
)

// ParseError is a typed, actionable BPMN input error.
type ParseError struct {
	Kind   ErrorKind
	Detail string
	Action string
	Err    error
}

func (e *ParseError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("parse BPMN: %s; %s", e.Kind, e.Action)
	}
	return fmt.Sprintf("parse BPMN: %s: %s; %s", e.Kind, e.Detail, e.Action)
}

func (e *ParseError) Unwrap() error { return e.Err }

// ElementByID returns an element or nil.
func (p Process) ElementByID(id string) *Element {
	for i := range p.Elements {
		if p.Elements[i].ID == id {
			return &p.Elements[i]
		}
	}
	return nil
}

// ServiceTasks returns service/script-style external tasks.
func (p Process) ServiceTasks() []Element {
	var out []Element
	for _, e := range p.Elements {
		if e.Type == "serviceTask" || e.Type == "scriptTask" {
			out = append(out, e)
		}
	}
	return out
}

// ElementByID uses the first process compatibility view.
func (d Document) ElementByID(id string) *Element {
	for i := range d.Elements {
		if d.Elements[i].ID == id {
			return &d.Elements[i]
		}
	}
	return nil
}

// ServiceTasks uses the first process compatibility view.
func (d Document) ServiceTasks() []Element {
	return Process{Elements: d.Elements}.ServiceTasks()
}
