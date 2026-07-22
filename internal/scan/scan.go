package scan

import (
	"encoding/json"
)

// Severity is the policy level assigned to a finding.
type Severity string

const (
	SeverityLow    Severity = "low"
	SeverityMedium Severity = "medium"
	SeverityHigh   Severity = "high"
)

// SourceKind identifies the documented source/configuration format inspected.
type SourceKind string

const (
	SourceBPMN       SourceKind = "bpmn"
	SourceDMN        SourceKind = "dmn"
	SourceForm       SourceKind = "form"
	SourceYAML       SourceKind = "yaml"
	SourceJSON       SourceKind = "json"
	SourceEnv        SourceKind = "env"
	SourceShell      SourceKind = "shell"
	SourceJavaScript SourceKind = "javascript"
	SourceTypeScript SourceKind = "typescript"
	SourceJava       SourceKind = "java"
	SourceGo         SourceKind = "go"
	SourceProperties SourceKind = "properties"
	SourceText       SourceKind = "text"
)

// Finding is a potential secret. Snippet is always masked.
type Finding struct {
	// Rule retains the original stable JSON field; it contains the rule ID.
	Rule       string     `json:"rule"`
	RuleID     string     `json:"-"`
	Severity   Severity   `json:"severity"`
	File       string     `json:"file"`
	Line       int        `json:"line"`
	Snippet    string     `json:"snippet"`
	SourceKind SourceKind `json:"sourceKind"`
}

// IssueKind describes why a candidate was not fully scanned.
type IssueKind string

const (
	IssueIgnored   IssueKind = "ignored"
	IssueError     IssueKind = "error"
	IssueTruncated IssueKind = "truncated"
)

// Options configures a scan.
type Options struct {
	Root         string
	FailOn       string
	Ignore       []string
	MaxFileSize  int64
	MaxLineBytes int
}

// Issue records an ignored or unsuccessfully inspected candidate.
type Issue struct {
	Path    string    `json:"path"`
	Kind    IssueKind `json:"kind"`
	Reason  string    `json:"reason,omitempty"`
	Message string    `json:"message,omitempty"`

	// Err preserves the original local error without destabilizing JSON output.
	Err error `json:"-"`
}

// Stats accounts for every discovered candidate exactly once.
type Stats struct {
	Discovered int `json:"discovered"`
	Scanned    int `json:"scanned"`
	Ignored    int `json:"ignored"`
	Errored    int `json:"errored"`
}

// Result includes findings and honest, stable scan accounting.
type Result struct {
	Findings []Finding `json:"findings"`
	Issues   []Issue   `json:"issues"`
	Complete bool      `json:"complete"`
	Stats    Stats     `json:"stats"`
}

// ShouldFail reports CI failure for findings at/above failOn.
func ShouldFail(fs []Finding, failOn string) bool {
	rank := map[string]int{"low": 1, "medium": 2, "high": 3}
	min := rank[failOn]
	if min == 0 {
		min = 2
	}
	for _, f := range fs {
		if rank[string(f.Severity)] >= min {
			return true
		}
	}
	return false
}

func (result Result) MarshalJSON() ([]byte, error) {
	type resultJSON Result
	if result.Findings == nil {
		result.Findings = []Finding{}
	}
	if result.Issues == nil {
		result.Issues = []Issue{}
	}
	return json.Marshal(resultJSON(result))
}
