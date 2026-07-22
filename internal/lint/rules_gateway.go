package lint

import (
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

type exclusiveGatewayDefaultRule struct{}

func (exclusiveGatewayDefaultRule) ID() string { return "bpmn/exclusive-gateway-default" }

func (rule exclusiveGatewayDefaultRule) Check(document bpmn.Document) []Finding {
	var findings []Finding
	for _, process := range document.Processes {
		for _, element := range process.Elements {
			if element.Type != "exclusiveGateway" || element.DefaultFlow != "" {
				continue
			}
			findings = append(findings, Finding{
				Rule: rule.ID(), Severity: SeverityWarning,
				Message: "exclusive gateway has no default flow",
				Element: element.ID, ProcessID: process.ID,
			})
		}
	}
	return findings
}

type exclusiveGatewayConditionRule struct{}

func (exclusiveGatewayConditionRule) ID() string {
	return "bpmn/exclusive-gateway-condition"
}

func (rule exclusiveGatewayConditionRule) Check(document bpmn.Document) []Finding {
	var findings []Finding
	for _, process := range document.Processes {
		gateways := make(map[string]string)
		for _, element := range process.Elements {
			if element.Type == "exclusiveGateway" {
				gateways[element.ID] = element.DefaultFlow
			}
		}
		for _, flow := range process.Flows {
			defaultFlow, ok := gateways[flow.Source]
			if !ok || flow.ID == defaultFlow || strings.TrimSpace(flow.Condition) != "" {
				continue
			}
			findings = append(findings, Finding{
				Rule: rule.ID(), Severity: SeverityError,
				Message: "non-default outgoing flow missing condition",
				Element: flow.ID, ProcessID: process.ID,
			})
		}
	}
	return findings
}
