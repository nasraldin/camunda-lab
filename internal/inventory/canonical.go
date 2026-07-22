package inventory

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// Canonicalize parses a resource strictly and returns its deterministic form.
func Canonicalize(kind Kind, raw []byte) ([]byte, error) {
	switch kind {
	case KindProcess:
		doc, err := bpmn.Parse(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		return json.Marshal(canonicalBPMNDocument(doc))
	case KindDecision:
		node, err := parseXML(raw)
		if err != nil {
			return nil, fmt.Errorf("canonicalize decision XML: %w", err)
		}
		if err := validateDMN(node); err != nil {
			return nil, fmt.Errorf("canonicalize decision XML: %w", err)
		}
		var out bytes.Buffer
		if err := encodeXMLNode(xml.NewEncoder(&out), node); err != nil {
			return nil, fmt.Errorf("canonicalize decision XML: %w", err)
		}
		return out.Bytes(), nil
	case KindForm:
		value, err := parseStrictJSON(raw)
		if err != nil {
			return nil, fmt.Errorf("canonicalize form JSON: %w", err)
		}
		if err := validateForm(value); err != nil {
			return nil, fmt.Errorf("canonicalize form JSON: %w", err)
		}
		return json.Marshal(value)
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedKind, kind)
	}
}

// ProcessSelectionError identifies an ambiguous or missing process identity in
// a BPMN document.
type ProcessSelectionError struct {
	ProcessID string
	Matches   int
}

func (e *ProcessSelectionError) Error() string {
	if e.Matches == 0 {
		return fmt.Sprintf("BPMN document does not contain process %q", e.ProcessID)
	}
	return fmt.Sprintf("BPMN document contains process %q %d times", e.ProcessID, e.Matches)
}

// CanonicalizeProcess selects exactly one process and includes document-level
// definitions/extensions needed by that process in the canonical form.
func CanonicalizeProcess(raw []byte, processID string) ([]byte, error) {
	doc, err := bpmn.Parse(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	processID = strings.TrimSpace(processID)
	matches := make([]bpmn.Process, 0, 1)
	for _, process := range doc.Processes {
		if process.ID == processID {
			matches = append(matches, process)
		}
	}
	if len(matches) != 1 {
		return nil, &ProcessSelectionError{ProcessID: processID, Matches: len(matches)}
	}
	return canonicalBPMNProcess(doc, matches[0])
}

// DigestCanonicalProcess hashes the shared local/remote per-process form.
func DigestCanonicalProcess(raw []byte, processID string) (string, error) {
	canonical, err := CanonicalizeProcess(raw, processID)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// DigestCanonical hashes the complete canonical representation.
func DigestCanonical(kind Kind, raw []byte) (string, error) {
	canonical, err := Canonicalize(kind, raw)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:]), nil
}

// ResourceIDs returns stable deployable identities contained in a resource.
func ResourceIDs(kind Kind, raw []byte) ([]string, error) {
	var ids []string
	switch kind {
	case KindProcess:
		doc, err := bpmn.Parse(bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		for _, process := range doc.Processes {
			ids = append(ids, strings.TrimSpace(process.ID))
		}
	case KindDecision:
		node, err := parseXML(raw)
		if err != nil {
			return nil, err
		}
		if err := validateDMN(node); err != nil {
			return nil, err
		}
		for _, child := range node.Children {
			if child.Name.Space == node.Name.Space && child.Name.Local == "decision" {
				ids = append(ids, xmlAttribute(child, "id"))
			}
		}
	case KindForm:
		value, err := parseStrictJSON(raw)
		if err != nil {
			return nil, err
		}
		if err := validateForm(value); err != nil {
			return nil, err
		}
		object, ok := value.(map[string]any)
		if !ok {
			return nil, errors.New("form must be a JSON object")
		}
		id, ok := object["id"].(string)
		if !ok {
			return nil, errors.New("form must contain a string id")
		}
		ids = append(ids, strings.TrimSpace(id))
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnsupportedKind, kind)
	}
	seen := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id == "" {
			return nil, fmt.Errorf("%s resource has an empty ID", kind)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%s resource contains duplicate ID %q", kind, id)
		}
		seen[id] = struct{}{}
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("%s resource contains no deployable IDs", kind)
	}
	sort.Strings(ids)
	return ids, nil
}

type canonicalDocument struct {
	Processes         []bpmn.Process
	Messages          []bpmn.Message
	Errors            []bpmn.Error
	UnknownExtensions []bpmn.Extension
}

func canonicalBPMNDocument(doc bpmn.Document) canonicalDocument {
	return canonicalDocument{
		Processes: doc.Processes, Messages: doc.Messages, Errors: doc.Errors,
		UnknownExtensions: doc.UnknownExtensions,
	}
}

func canonicalBPMNProcess(doc bpmn.Document, process bpmn.Process) ([]byte, error) {
	messageRefs := make(map[string]struct{})
	errorRefs := make(map[string]struct{})
	for _, element := range process.Elements {
		if element.MessageRef != "" {
			messageRefs[element.MessageRef] = struct{}{}
		}
		if element.ErrorRef != "" {
			errorRefs[element.ErrorRef] = struct{}{}
		}
	}
	messages := make([]bpmn.Message, 0, len(messageRefs))
	for _, message := range doc.Messages {
		if _, relevant := messageRefs[message.ID]; relevant {
			messages = append(messages, message)
		}
	}
	definitions := make([]bpmn.Error, 0, len(errorRefs))
	for _, definition := range doc.Errors {
		if _, relevant := errorRefs[definition.ID]; relevant {
			definitions = append(definitions, definition)
		}
	}
	process.Messages = append([]bpmn.Message(nil), messages...)
	return json.Marshal(canonicalDocument{
		Processes: []bpmn.Process{process}, Messages: messages, Errors: definitions,
		UnknownExtensions: documentRootExtensions(doc),
	})
}

func documentRootExtensions(doc bpmn.Document) []bpmn.Extension {
	owned := make(map[string]int)
	for _, process := range doc.Processes {
		for _, extension := range process.UnknownExtensions {
			owned[canonicalExtensionKey(extension)]++
		}
		for _, element := range process.Elements {
			for _, extension := range element.Extensions {
				owned[canonicalExtensionKey(extension)]++
			}
		}
	}
	var root []bpmn.Extension
	for _, extension := range doc.UnknownExtensions {
		key := canonicalExtensionKey(extension)
		if owned[key] > 0 {
			owned[key]--
			continue
		}
		root = append(root, extension)
	}
	return root
}

func canonicalExtensionKey(extension bpmn.Extension) string {
	data, _ := json.Marshal(extension)
	return string(data)
}

type xmlNode struct {
	Name     xml.Name
	Attrs    []xml.Attr
	Text     string
	Children []*xmlNode
}

func parseXML(raw []byte) (*xmlNode, error) {
	decoder := xml.NewDecoder(bytes.NewReader(raw))
	var root *xmlNode
	var stack []*xmlNode
	for {
		token, err := decoder.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		switch value := token.(type) {
		case xml.StartElement:
			node := &xmlNode{Name: value.Name, Attrs: append([]xml.Attr(nil), value.Attr...)}
			if len(stack) == 0 {
				if root != nil {
					return nil, errors.New("multiple root elements")
				}
				root = node
			} else {
				stack[len(stack)-1].Children = append(stack[len(stack)-1].Children, node)
			}
			stack = append(stack, node)
		case xml.EndElement:
			if len(stack) == 0 || stack[len(stack)-1].Name != value.Name {
				return nil, errors.New("mismatched XML element")
			}
			stack = stack[:len(stack)-1]
		case xml.CharData:
			text := strings.TrimSpace(string(value))
			if len(stack) == 0 {
				if text != "" {
					return nil, errors.New("unexpected content outside root element")
				}
			} else {
				if text != "" {
					stack[len(stack)-1].Text += text
				}
			}
		}
	}
	if root == nil || len(stack) != 0 {
		return nil, errors.New("XML must contain one complete root element")
	}
	return root, nil
}

func encodeXMLNode(encoder *xml.Encoder, node *xmlNode) error {
	attrs := append([]xml.Attr(nil), node.Attrs...)
	sort.SliceStable(attrs, func(i, j int) bool {
		return attrs[i].Name.Space+"\x00"+attrs[i].Name.Local+"\x00"+attrs[i].Value <
			attrs[j].Name.Space+"\x00"+attrs[j].Name.Local+"\x00"+attrs[j].Value
	})
	start := xml.StartElement{Name: node.Name, Attr: attrs}
	if err := encoder.EncodeToken(start); err != nil {
		return err
	}
	if node.Text != "" {
		if err := encoder.EncodeToken(xml.CharData(node.Text)); err != nil {
			return err
		}
	}
	for _, child := range node.Children {
		if err := encodeXMLNode(encoder, child); err != nil {
			return err
		}
	}
	if err := encoder.EncodeToken(start.End()); err != nil {
		return err
	}
	return encoder.Flush()
}

var supportedDMNNamespaces = map[string]struct{}{
	"https://www.omg.org/spec/DMN/20191111/MODEL/": {},
	"https://www.omg.org/spec/DMN/20180521/MODEL/": {},
	"http://www.omg.org/spec/DMN/20151101/dmn.xsd": {},
}

func validateDMN(root *xmlNode) error {
	if root.Name.Local != "definitions" {
		return errors.New("DMN root must be definitions")
	}
	if _, supported := supportedDMNNamespaces[root.Name.Space]; !supported {
		return fmt.Errorf("unsupported DMN namespace %q", root.Name.Space)
	}
	decisions := 0
	for _, child := range root.Children {
		if child.Name.Space != root.Name.Space || child.Name.Local != "decision" {
			continue
		}
		decisions++
		if strings.TrimSpace(xmlAttribute(child, "id")) == "" {
			return errors.New("DMN decision must have an id")
		}
		if !hasDecisionExpression(child, root.Name.Space) {
			return fmt.Errorf("DMN decision %q has no supported decision expression", xmlAttribute(child, "id"))
		}
	}
	if decisions == 0 {
		return errors.New("DMN definitions must contain at least one decision")
	}
	return nil
}

func hasDecisionExpression(decision *xmlNode, namespace string) bool {
	for _, child := range decision.Children {
		if child.Name.Space != namespace {
			continue
		}
		switch child.Name.Local {
		case "decisionTable", "literalExpression", "context", "relation", "invocation", "list", "functionDefinition":
			return true
		}
	}
	return false
}

func xmlAttribute(node *xmlNode, local string) string {
	for _, attribute := range node.Attrs {
		if attribute.Name.Local == local {
			return strings.TrimSpace(attribute.Value)
		}
	}
	return ""
}

func validateForm(value any) error {
	object, ok := value.(map[string]any)
	if !ok {
		return errors.New("Camunda Form must be a JSON object")
	}
	schemaVersion, ok := object["schemaVersion"].(json.Number)
	if !ok {
		return errors.New("Camunda Form schemaVersion must be an integer")
	}
	version, err := strconv.ParseInt(string(schemaVersion), 10, 64)
	if err != nil || version < 1 {
		return errors.New("Camunda Form schemaVersion must be a positive integer")
	}
	id, ok := object["id"].(string)
	if !ok || strings.TrimSpace(id) == "" {
		return errors.New("Camunda Form id must be a non-empty string")
	}
	if _, ok := object["components"].([]any); !ok {
		return errors.New("Camunda Form components must be an array")
	}
	formType, ok := object["type"].(string)
	if !ok || formType != "default" {
		return errors.New(`Camunda Form type must be "default"`)
	}
	return nil
}

func parseStrictJSON(raw []byte) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	value, err := decodeJSONValue(decoder)
	if err != nil {
		return nil, err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, errors.New("trailing JSON value")
	}
	return value, nil
}

func decodeJSONValue(decoder *json.Decoder) (any, error) {
	token, err := decoder.Token()
	if err != nil {
		return nil, err
	}
	delim, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return token, nil
	}
	switch delim {
	case '{':
		object := make(map[string]any)
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyToken.(string)
			if !ok {
				return nil, errors.New("JSON object key is not a string")
			}
			if _, duplicate := object[key]; duplicate {
				return nil, fmt.Errorf("duplicate JSON field %q", key)
			}
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			object[key] = value
		}
		if closeToken, err := decoder.Token(); err != nil || closeToken != json.Delim('}') {
			return nil, errors.New("JSON object is not closed")
		}
		return object, nil
	case '[':
		var array []any
		for decoder.More() {
			value, err := decodeJSONValue(decoder)
			if err != nil {
				return nil, err
			}
			array = append(array, value)
		}
		if closeToken, err := decoder.Token(); err != nil || closeToken != json.Delim(']') {
			return nil, errors.New("JSON array is not closed")
		}
		return array, nil
	default:
		return nil, errors.New("unexpected JSON delimiter")
	}
}
