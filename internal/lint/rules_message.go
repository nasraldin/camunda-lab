package lint

import (
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

type duplicateMessageNameRule struct{}

func (duplicateMessageNameRule) ID() string { return "bpmn/duplicate-message-name" }

func (rule duplicateMessageNameRule) Check(document bpmn.Document) []Finding {
	seen := make(map[string]string, len(document.Messages))
	var findings []Finding
	for _, message := range document.Messages {
		key := strings.ToLower(strings.TrimSpace(message.Name))
		if key == "" {
			continue
		}
		previous, duplicate := seen[key]
		if !duplicate {
			seen[key] = message.ID
			continue
		}
		findings = append(findings, Finding{
			Rule: rule.ID(), Severity: SeverityError,
			Message: fmt.Sprintf("duplicate message name %q (also %s)", message.Name, previous),
			Element: message.ID,
		})
	}
	return findings
}
