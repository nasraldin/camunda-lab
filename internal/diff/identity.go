package diff

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func elementStructuralIdentities(process bpmn.Process, resolver resolver, rounds int) map[string]string {
	if rounds < 1 {
		rounds = 1
	}
	base := make(map[string]string, len(process.Elements))
	colors := make(map[string]string, len(process.Elements))
	for _, element := range process.Elements {
		base[element.ID] = elementCore(element, resolver) + "\x00" + extensionsFingerprint(element.Extensions)
		colors[element.ID] = fingerprint(base[element.ID])
	}
	for round := 0; round < rounds; round++ {
		next := make(map[string]string, len(colors))
		for _, element := range process.Elements {
			var relations []string
			for _, flow := range process.Flows {
				switch {
				case flow.Source == element.ID:
					relations = append(relations, strings.Join([]string{"out", flow.Name, flow.Condition, color(colors, flow.Target)}, "\x00"))
				case flow.Target == element.ID:
					relations = append(relations, strings.Join([]string{"in", flow.Name, flow.Condition, color(colors, flow.Source)}, "\x00"))
				}
				if element.DefaultFlow != "" && flow.ID == element.DefaultFlow {
					relations = append(relations, strings.Join([]string{"default", flow.Name, flow.Condition, color(colors, flow.Target)}, "\x00"))
				}
			}
			if element.ParentID != "" {
				relations = append(relations, "parent\x00"+color(colors, element.ParentID))
			}
			if element.AttachedTo != "" {
				relations = append(relations, "attached\x00"+color(colors, element.AttachedTo))
			}
			for _, candidate := range process.Elements {
				if candidate.ParentID == element.ID {
					relations = append(relations, "child\x00"+color(colors, candidate.ID))
				}
				if candidate.AttachedTo == element.ID {
					relations = append(relations, "boundary\x00"+color(colors, candidate.ID))
				}
			}
			sort.Strings(relations)
			next[element.ID] = fingerprint(base[element.ID] + "\x01" + strings.Join(relations, "\x02"))
		}
		colors = next
	}
	return colors
}

func flowStructuralIdentity(flow bpmn.Flow, identities map[string]string) string {
	return fingerprint(strings.Join([]string{
		flow.Name, flow.Condition, color(identities, flow.Source), color(identities, flow.Target),
	}, "\x00"))
}

func extensionsFingerprint(extensions []bpmn.Extension) string {
	if len(extensions) == 0 {
		return ""
	}
	parts := make([]string, 0, len(extensions))
	for _, extension := range extensions {
		parts = append(parts, canonicalExtension(extension))
	}
	sort.Strings(parts)
	return fingerprint(strings.Join(parts, "\x03"))
}

func canonicalExtension(extension bpmn.Extension) string {
	attributes := make([]bpmn.Attribute, 0, len(extension.Attributes))
	for _, attribute := range extension.Attributes {
		if !isNamespaceDeclaration(attribute.QName) {
			attributes = append(attributes, attribute)
		}
	}
	sort.Slice(attributes, func(i, j int) bool {
		left, right := attributes[i], attributes[j]
		return qName(left.QName)+"\x00"+left.Value < qName(right.QName)+"\x00"+right.Value
	})
	var attributeParts []string
	for _, attribute := range attributes {
		attributeParts = append(attributeParts, qName(attribute.QName), attribute.Value)
	}
	return strings.Join([]string{
		qName(extension.QName),
		strings.Join(attributeParts, "\x01"),
		canonicalExtensionContent(extension.InnerXML),
	}, "\x00")
}

func canonicalExtensionContent(content string) string {
	decoder := xml.NewDecoder(strings.NewReader("<root>" + content + "</root>"))
	var parts []string
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			return strings.Join(parts, "\x01")
		}
		if err != nil {
			return "raw:" + strings.TrimSpace(content)
		}
		switch value := token.(type) {
		case xml.StartElement:
			depth++
			if depth == 1 {
				continue
			}
			var attributes []string
			for _, attribute := range value.Attr {
				if isNamespaceDeclaration(attribute.Name) {
					continue
				}
				attributes = append(attributes, qName(attribute.Name)+"\x00"+attribute.Value)
			}
			sort.Strings(attributes)
			parts = append(parts, "start:"+qName(value.Name)+":"+strings.Join(attributes, "\x02"))
		case xml.EndElement:
			if depth > 1 {
				parts = append(parts, "end:"+qName(value.Name))
			}
			depth--
		case xml.CharData:
			if text := strings.TrimSpace(string(value)); text != "" {
				parts = append(parts, "text:"+text)
			}
		case xml.Comment:
			parts = append(parts, "comment:"+strings.TrimSpace(string(value)))
		case xml.Directive:
			parts = append(parts, "directive:"+strings.TrimSpace(string(value)))
		case xml.ProcInst:
			parts = append(parts, "proc:"+value.Target+":"+strings.TrimSpace(string(value.Inst)))
		}
	}
}

func qName(name xml.Name) string {
	return name.Space + "\x00" + name.Local
}

func isNamespaceDeclaration(name xml.Name) bool {
	return name.Space == "xmlns" || (name.Space == "" && name.Local == "xmlns")
}

func processScopeExtensions(process bpmn.Process) []bpmn.Extension {
	var elementExtensions []bpmn.Extension
	for _, element := range process.Elements {
		elementExtensions = append(elementExtensions, element.Extensions...)
	}
	return subtractExtensions(process.UnknownExtensions, elementExtensions)
}

func documentScopeExtensions(document bpmn.Document) []bpmn.Extension {
	var nested []bpmn.Extension
	for _, process := range document.Processes {
		nested = append(nested, processScopeExtensions(process)...)
		for _, element := range process.Elements {
			nested = append(nested, element.Extensions...)
		}
	}
	return subtractExtensions(document.UnknownExtensions, nested)
}

func subtractExtensions(all, removed []bpmn.Extension) []bpmn.Extension {
	counts := make(map[string]int, len(removed))
	for _, extension := range removed {
		counts[canonicalExtension(extension)]++
	}
	var result []bpmn.Extension
	for _, extension := range all {
		key := canonicalExtension(extension)
		if counts[key] > 0 {
			counts[key]--
			continue
		}
		result = append(result, extension)
	}
	return result
}

func color(values map[string]string, id string) string {
	if id == "" {
		return ""
	}
	if value, ok := values[id]; ok {
		return value
	}
	return "unresolved:" + id
}

func fingerprint(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
