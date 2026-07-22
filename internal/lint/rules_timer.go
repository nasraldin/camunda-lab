package lint

import "github.com/nasraldin/camunda-lab/internal/bpmn"

type timerReachableRule struct{}

func (timerReachableRule) ID() string { return "bpmn/timer-reachable" }

func (rule timerReachableRule) Check(document bpmn.Document) []Finding {
	var findings []Finding
	for _, process := range document.Processes {
		scopes := map[string]bool{}
		elements := make(map[string]bpmn.Element, len(process.Elements))
		for _, element := range process.Elements {
			elements[element.ID] = element
			scopes[element.ParentID] = true
		}
		reachableByScope := make(map[string]map[string]bool, len(scopes))
		for scope := range scopes {
			var starts []string
			for _, element := range process.Elements {
				if element.ParentID == scope && element.Type == "startEvent" {
					starts = append(starts, element.ID)
				}
			}
			reachableByScope[scope] = bpmn.NewGraphForScope(process, scope).ReachableFrom(starts...)
		}
		activeScopes := map[string]bool{"": true}
		resolvedScopes := map[string]bool{"": true}
		var scopeReachable func(string, map[string]bool) bool
		scopeReachable = func(scope string, visiting map[string]bool) bool {
			if resolvedScopes[scope] {
				return activeScopes[scope]
			}
			if visiting[scope] {
				return false
			}
			visiting[scope] = true
			container, exists := elements[scope]
			active := exists &&
				isContainingSubprocess(container.Type) &&
				scopeReachable(container.ParentID, visiting) &&
				reachableByScope[container.ParentID][scope]
			delete(visiting, scope)
			resolvedScopes[scope] = true
			activeScopes[scope] = active
			return active
		}
		for _, element := range process.Elements {
			if element.Timer == "" {
				continue
			}
			reachable := reachableByScope[element.ParentID]
			locallyReachable := reachable[element.ID] ||
				(element.Type == "boundaryEvent" && reachable[element.AttachedTo])
			if locallyReachable && scopeReachable(element.ParentID, map[string]bool{}) {
				continue
			}
			findings = append(findings, Finding{
				Rule: rule.ID(), Severity: SeverityWarning,
				Message: "timer event is not reachable from a start event",
				Element: element.ID, ProcessID: process.ID,
			})
		}
	}
	return findings
}

func isContainingSubprocess(elementType string) bool {
	switch elementType {
	case "subProcess", "transaction", "adHocSubProcess":
		return true
	default:
		return false
	}
}
