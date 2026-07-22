package lint

import "github.com/nasraldin/camunda-lab/internal/bpmn"

type processStartEventRule struct{}

func (processStartEventRule) ID() string { return "bpmn/process-start-event" }

func (rule processStartEventRule) Check(document bpmn.Document) []Finding {
	var findings []Finding
	for _, process := range document.Processes {
		hasStart := false
		for _, element := range process.Elements {
			if element.Type == "startEvent" && element.ParentID == "" {
				hasStart = true
				break
			}
		}
		if !hasStart {
			findings = append(findings, Finding{
				Rule: rule.ID(), Severity: SeverityError,
				Message: "process has no start event", Element: process.ID, ProcessID: process.ID,
			})
		}
	}
	return findings
}

type disconnectedElementRule struct{}

func (disconnectedElementRule) ID() string { return "bpmn/disconnected-element" }

func (rule disconnectedElementRule) Check(document bpmn.Document) []Finding {
	var findings []Finding
	for _, process := range document.Processes {
		elements := make(map[string]bpmn.Element, len(process.Elements))
		scopes := map[string]bool{}
		for _, element := range process.Elements {
			elements[element.ID] = element
			scopes[element.ParentID] = true
		}
		connected := make(map[string]bool, len(process.Flows)*2)
		for scope := range scopes {
			for id := range bpmn.NewGraphForScope(process, scope).ConnectedNodes() {
				connected[id] = true
			}
		}
		for _, element := range process.Elements {
			if connected[element.ID] || validBoundaryAttachment(element, elements) {
				continue
			}
			findings = append(findings, Finding{
				Rule: rule.ID(), Severity: SeverityError,
				Message: "element is not connected by sequence flows",
				Element: element.ID, ProcessID: process.ID,
			})
		}
	}
	return findings
}

func validBoundaryAttachment(element bpmn.Element, elements map[string]bpmn.Element) bool {
	if element.Type != "boundaryEvent" || element.AttachedTo == "" {
		return false
	}
	attached, exists := elements[element.AttachedTo]
	return exists && attached.ParentID == element.ParentID && isAttachableActivity(attached.Type)
}

func isAttachableActivity(elementType string) bool {
	switch elementType {
	case "task", "serviceTask", "userTask", "manualTask", "businessRuleTask",
		"scriptTask", "sendTask", "receiveTask", "callActivity", "subProcess",
		"transaction", "adHocSubProcess":
		return true
	default:
		return false
	}
}
