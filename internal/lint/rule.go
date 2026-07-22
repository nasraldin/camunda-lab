package lint

import "github.com/nasraldin/camunda-lab/internal/bpmn"

// Severity is the policy level assigned to a deterministic finding.
type Severity string

const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Finding is a single lint result. ProcessID is empty for document-scoped
// findings.
type Finding struct {
	Rule      string   `json:"rule"`
	Severity  Severity `json:"severity"`
	Message   string   `json:"message"`
	Element   string   `json:"element,omitempty"`
	ProcessID string   `json:"processId,omitempty"`
	File      string   `json:"file,omitempty"`
}

// Rule is a deterministic check over a normalized BPMN document.
type Rule interface {
	ID() string
	Check(bpmn.Document) []Finding
}

// Options controls suppression, attribution, and the policy threshold.
type Options struct {
	FailOn string
	Ignore []string
	File   string
}

// Result contains sorted findings and the threshold decision.
type Result struct {
	Failed   bool      `json:"failed"`
	Findings []Finding `json:"findings"`
}
