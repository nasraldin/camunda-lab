package lint

import "github.com/nasraldin/camunda-lab/internal/bpmn"

type serviceTaskRetryRule struct{}

func (serviceTaskRetryRule) ID() string { return "bpmn/service-task-retry" }

func (rule serviceTaskRetryRule) Check(document bpmn.Document) []Finding {
	var findings []Finding
	for _, process := range document.Processes {
		for _, element := range process.Elements {
			if element.Type != "serviceTask" || element.RetryCount != "" {
				continue
			}
			findings = append(findings, Finding{
				Rule: rule.ID(), Severity: SeverityWarning,
				Message: "service task missing retry extension",
				Element: element.ID, ProcessID: process.ID,
			})
		}
	}
	return findings
}
