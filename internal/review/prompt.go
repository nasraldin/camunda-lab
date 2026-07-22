package review

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

const defaultPromptLimit = 128 * 1024

// BuildPrompt renders normalized review semantics without environment or credential data.
func BuildPrompt(document bpmn.Document, findings []lint.Finding, maxBytes int) (string, bool, error) {
	semantics := promptDocument{
		Processes:  make([]promptProcess, 0, len(document.Processes)),
		Extensions: promptExtensions(document.UnknownExtensions),
	}
	for _, message := range document.Messages {
		semantics.Messages = append(semantics.Messages, promptMessage{ID: message.ID, Name: message.Name})
	}
	for _, definition := range document.Errors {
		semantics.Errors = append(semantics.Errors, promptError{
			ID: definition.ID, Name: definition.Name, ErrorCode: definition.ErrorCode,
		})
	}
	processes := document.Processes
	if len(processes) == 0 && (document.ProcessID != "" || len(document.Elements) > 0) {
		processes = []bpmn.Process{{
			ID: document.ProcessID, Name: document.Name,
			Elements: document.Elements, Flows: document.Flows,
		}}
	}
	for _, process := range processes {
		elements := make([]promptElement, 0, len(process.Elements))
		for _, element := range process.Elements {
			elements = append(elements, promptElement{
				ID: element.ID, Type: element.Type, Name: element.Name, ParentID: element.ParentID,
				CalledElement: element.CalledElement, DefaultFlow: element.DefaultFlow,
				Timer: element.Timer, TimerKind: element.TimerKind, CancelActivity: element.CancelActivity,
				RetryCount: element.RetryCount, ErrorRef: element.ErrorRef, MessageRef: element.MessageRef,
				JobType: element.JobType, AttachedTo: element.AttachedTo, EventDefs: element.EventDefs,
				Extensions: promptExtensions(element.Extensions),
			})
		}
		flows := make([]promptFlow, 0, len(process.Flows))
		for _, flow := range process.Flows {
			flows = append(flows, promptFlow{
				ID: flow.ID, Name: flow.Name, Source: flow.Source, Target: flow.Target, Condition: flow.Condition,
			})
		}
		semantics.Processes = append(semantics.Processes, promptProcess{
			ID: process.ID, Name: process.Name, Elements: elements, Flows: flows,
			Extensions: promptExtensions(process.UnknownExtensions),
		})
	}
	if maxBytes <= 0 {
		maxBytes = defaultPromptLimit
	}
	envelope := promptEnvelope{
		Instructions: reviewInstructions(),
		Model:        promptModel{Semantics: &semantics},
		Findings:     append([]lint.Finding(nil), findings...),
	}
	full, err := marshalPromptJSON(envelope)
	if err != nil {
		return "", false, fmt.Errorf("encode review prompt: %w", err)
	}
	if len(full) <= maxBytes {
		return string(full), false, nil
	}
	semanticJSON, err := marshalPromptJSON(semantics)
	if err != nil {
		return "", false, fmt.Errorf("encode normalized BPMN semantics: %w", err)
	}
	digest := sha256.Sum256(semanticJSON)
	envelope.Model = promptModel{Omission: &promptOmission{
		Count: semanticRecordCount(semantics), SHA256: fmt.Sprintf("%x", digest),
	}}
	compacted, err := marshalPromptJSON(envelope)
	if err != nil {
		return "", false, fmt.Errorf("encode compacted review prompt: %w", err)
	}
	if len(compacted) > maxBytes {
		return "", false, fmt.Errorf(
			"deterministic findings and prompt instructions require %d bytes, exceeding the %d-byte prompt limit; increase the limit",
			len(compacted), maxBytes,
		)
	}
	return string(compacted), true, nil
}

func reviewInstructions() []string {
	return []string{
		"Review normalized BPMN semantics and deterministic lint findings.",
		"Identify risks involving infinite loops, missing compensation, unreachable paths, duplicate messages, retries, timers, errors, and message correlation.",
		"Return concise Markdown sections: Additional Risks, Operational Concerns, Suggested Tests.",
		"Do not propose autofixes, mutate BPMN, claim formal model checking, or invent absent semantics.",
	}
}

type promptEnvelope struct {
	Instructions []string       `json:"instructions"`
	Model        promptModel    `json:"model"`
	Findings     []lint.Finding `json:"findings"`
}

type promptModel struct {
	Semantics *promptDocument `json:"semantics,omitempty"`
	Omission  *promptOmission `json:"omission,omitempty"`
}

type promptOmission struct {
	Count  int    `json:"count"`
	SHA256 string `json:"sha256"`
}

func marshalPromptJSON(value any) ([]byte, error) {
	var output bytes.Buffer
	encoder := json.NewEncoder(&output)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(output.Bytes(), []byte("\n")), nil
}

type promptDocument struct {
	Processes  []promptProcess   `json:"processes"`
	Messages   []promptMessage   `json:"messages"`
	Errors     []promptError     `json:"errors"`
	Extensions []promptExtension `json:"extensions,omitempty"`
}

type promptMessage struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type promptError struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	ErrorCode string `json:"errorCode,omitempty"`
}

type promptProcess struct {
	ID         string            `json:"id"`
	Name       string            `json:"name,omitempty"`
	Elements   []promptElement   `json:"elements"`
	Flows      []promptFlow      `json:"flows"`
	Extensions []promptExtension `json:"extensions,omitempty"`
}

type promptElement struct {
	ID             string            `json:"id"`
	Type           string            `json:"type"`
	Name           string            `json:"name,omitempty"`
	ParentID       string            `json:"parentId,omitempty"`
	CalledElement  string            `json:"calledElement,omitempty"`
	DefaultFlow    string            `json:"defaultFlow,omitempty"`
	Timer          string            `json:"timer,omitempty"`
	TimerKind      string            `json:"timerKind,omitempty"`
	CancelActivity bool              `json:"cancelActivity,omitempty"`
	RetryCount     string            `json:"retryCount,omitempty"`
	ErrorRef       string            `json:"errorRef,omitempty"`
	MessageRef     string            `json:"messageRef,omitempty"`
	JobType        string            `json:"jobType,omitempty"`
	AttachedTo     string            `json:"attachedTo,omitempty"`
	EventDefs      []string          `json:"eventDefinitions,omitempty"`
	Extensions     []promptExtension `json:"extensions,omitempty"`
}

type promptFlow struct {
	ID        string `json:"id"`
	Name      string `json:"name,omitempty"`
	Source    string `json:"source"`
	Target    string `json:"target"`
	Condition string `json:"condition,omitempty"`
}

type promptExtension struct {
	Namespace  string            `json:"namespace,omitempty"`
	Local      string            `json:"local"`
	Attributes []promptAttribute `json:"attributes,omitempty"`
	Content    []promptXMLNode   `json:"content,omitempty"`
}

type promptAttribute struct {
	Namespace string `json:"namespace,omitempty"`
	Local     string `json:"local"`
	Value     string `json:"value"`
}

type promptXMLNode struct {
	Namespace  string            `json:"namespace,omitempty"`
	Local      string            `json:"local"`
	Attributes []promptAttribute `json:"attributes,omitempty"`
	Text       string            `json:"text,omitempty"`
	Children   []promptXMLNode   `json:"children,omitempty"`
}

func promptExtensions(extensions []bpmn.Extension) []promptExtension {
	result := make([]promptExtension, 0, len(extensions))
	for _, extension := range extensions {
		designated := false
		for _, attribute := range extension.Attributes {
			local := strings.ToLower(attribute.QName.Local)
			if (local == "key" || local == "name") && sensitiveName(attribute.Value) {
				designated = true
			}
		}
		rootSensitive := sensitiveName(extension.QName.Local) || designated
		item := promptExtension{
			Namespace: extension.QName.Space, Local: extension.QName.Local,
			Content: parseExtensionXML(extension.InnerXML, rootSensitive),
		}
		for _, attribute := range extension.Attributes {
			local := strings.ToLower(attribute.QName.Local)
			value := attribute.Value
			if sensitiveName(local) || (rootSensitive && !structuralIdentifierName(local)) {
				value = "[redacted]"
			}
			item.Attributes = append(item.Attributes, promptAttribute{
				Namespace: attribute.QName.Space, Local: attribute.QName.Local,
				Value: value,
			})
		}
		result = append(result, item)
	}
	return result
}

func sensitiveName(name string) bool {
	normalized := strings.ToLower(strings.NewReplacer("-", "", "_", "", ":", "").Replace(name))
	for _, sensitive := range []string{"apikey", "secret", "password", "token", "authorization", "credential"} {
		if strings.Contains(normalized, sensitive) {
			return true
		}
	}
	return false
}

func parseExtensionXML(fragment string, inheritedSensitive bool) []promptXMLNode {
	if strings.TrimSpace(fragment) == "" {
		return nil
	}
	decoder := xml.NewDecoder(strings.NewReader("<root>" + fragment + "</root>"))
	decoder.Strict = false
	if _, err := decoder.Token(); err != nil {
		return redactedMalformedXML()
	}
	var nodes []promptXMLNode
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return redactedMalformedXML()
		}
		switch typed := token.(type) {
		case xml.StartElement:
			node, err := parseXMLNode(decoder, typed, inheritedSensitive)
			if err != nil {
				return redactedMalformedXML()
			}
			nodes = append(nodes, node)
		case xml.EndElement:
			return nodes
		case xml.CharData:
			if text := strings.TrimSpace(string(typed)); text != "" {
				if inheritedSensitive {
					text = "[redacted]"
				}
				nodes = append(nodes, promptXMLNode{Local: "#text", Text: text})
			}
		case xml.Comment, xml.Directive, xml.ProcInst:
			// Comments and directives are not BPMN semantics and may contain credentials.
		}
	}
	return nodes
}

func parseXMLNode(decoder *xml.Decoder, start xml.StartElement, inheritedSensitive bool) (promptXMLNode, error) {
	designated := false
	for _, attribute := range start.Attr {
		local := strings.ToLower(attribute.Name.Local)
		if (local == "key" || local == "name") && sensitiveName(attribute.Value) {
			designated = true
		}
	}
	sensitiveElement := inheritedSensitive || sensitiveName(start.Name.Local)
	sensitive := sensitiveElement || designated
	node := promptXMLNode{Namespace: start.Name.Space, Local: start.Name.Local}
	for _, attribute := range start.Attr {
		local := strings.ToLower(attribute.Name.Local)
		value := attribute.Value
		if sensitiveName(local) || (sensitive && !structuralIdentifierName(local)) {
			value = "[redacted]"
		}
		node.Attributes = append(node.Attributes, promptAttribute{
			Namespace: attribute.Name.Space, Local: attribute.Name.Local, Value: value,
		})
	}
	var text strings.Builder
	for {
		token, err := decoder.Token()
		if err != nil {
			return promptXMLNode{}, err
		}
		switch typed := token.(type) {
		case xml.StartElement:
			child, err := parseXMLNode(decoder, typed, sensitive)
			if err != nil {
				return promptXMLNode{}, err
			}
			node.Children = append(node.Children, child)
		case xml.EndElement:
			value := strings.TrimSpace(text.String())
			if sensitive && value != "" {
				value = "[redacted]"
			}
			node.Text = value
			return node, nil
		case xml.CharData:
			text.Write([]byte(typed))
		case xml.Comment, xml.Directive, xml.ProcInst:
			// Omit non-semantic and potentially sensitive content.
		}
	}
}

func structuralIdentifierName(name string) bool {
	switch strings.ToLower(name) {
	case "id", "key", "name", "type", "kind", "ref":
		return true
	default:
		return false
	}
}

func redactedMalformedXML() []promptXMLNode {
	return []promptXMLNode{{Local: "#redacted", Text: "malformed extension XML"}}
}

func semanticRecordCount(document promptDocument) int {
	count := len(document.Messages) + len(document.Errors) + len(document.Extensions)
	for _, process := range document.Processes {
		count++ // process
		count += len(process.Elements) + len(process.Flows) + len(process.Extensions)
		for _, element := range process.Elements {
			count += len(element.Extensions)
		}
	}
	return count
}
