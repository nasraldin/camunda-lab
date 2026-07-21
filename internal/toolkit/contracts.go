package toolkit

import (
	"github.com/nasraldin/camunda-lab/internal/bpmn"
	bpmndiff "github.com/nasraldin/camunda-lab/internal/diff"
	"github.com/nasraldin/camunda-lab/internal/explain"
	"github.com/nasraldin/camunda-lab/internal/lint"
	"github.com/nasraldin/camunda-lab/internal/review"
	"github.com/nasraldin/camunda-lab/internal/scan"
)

// Operation identifies a toolkit use case.
type Operation string

const (
	OperationLint     Operation = "lint"
	OperationDiff     Operation = "diff"
	OperationExplain  Operation = "explain"
	OperationReview   Operation = "review"
	OperationGenerate Operation = "generate"
	OperationScan     Operation = "scan"
)

// Status describes both execution and policy-threshold outcomes.
type Status string

const (
	StatusCompleted Status = "completed"
	StatusPartial   Status = "partial"
	StatusFailed    Status = "failed"
	StatusSkipped   Status = "skipped"
)

// Warning records a recoverable problem without presentation formatting.
type Warning struct {
	Code    string
	Message string
	Path    string
}

// BPMNInput is either in-memory BPMN or a file reference. Content wins when set.
type BPMNInput struct {
	Name    string
	Path    string
	Content []byte
}

// GitInput identifies file content at a Git revision.
type GitInput struct {
	Ref  string
	Path string
}

// AIOptions controls optional enrichment.
type AIOptions struct {
	Enabled  bool
	Required bool
}

// AIStatus describes enrichment independently of the operation status.
type AIStatus string

const (
	AIStatusDisabled  AIStatus = "disabled"
	AIStatusSkipped   AIStatus = "skipped"
	AIStatusSucceeded AIStatus = "succeeded"
	AIStatusFailed    AIStatus = "failed"
)

type LintThreshold string

const (
	LintThresholdError   LintThreshold = "error"
	LintThresholdWarning LintThreshold = "warning"
)

type ScanThreshold string

const (
	ScanThresholdLow    ScanThreshold = "low"
	ScanThresholdMedium ScanThreshold = "medium"
	ScanThresholdHigh   ScanThreshold = "high"
)

type GenerateLanguage string

const (
	GenerateLanguageJava       GenerateLanguage = "java"
	GenerateLanguageJavaScript GenerateLanguage = "js"
)

type LintRequest struct {
	Inputs     []BPMNInput
	ProjectDir string
	FailOn     LintThreshold
	Ignore     []string
}

type LintResult struct {
	Status    Status
	Complete  bool
	Warnings  []Warning
	Inputs    []string
	Documents []bpmn.Document
	Findings  []LintFinding
}

// LintFinding attributes process-scoped findings while leaving document-scoped
// findings with an empty ProcessID.
type LintFinding struct {
	ProcessID string
	Finding   lint.Finding
}

type DiffRequest struct {
	Before     BPMNInput
	After      BPMNInput
	BeforeGit  *GitInput
	ProjectDir string
}

type DiffResult struct {
	Status   Status
	Complete bool
	Warnings []Warning
	Before   bpmn.Document
	After    bpmn.Document
	Changes  []ProcessChange
}

type ProcessChangeKind string

const (
	ProcessModified ProcessChangeKind = "process_modified"
	ProcessAdded    ProcessChangeKind = "process_added"
	ProcessRemoved  ProcessChangeKind = "process_removed"
	DocumentChanged ProcessChangeKind = "document_changed"
)

// ProcessChange retains process identity around the current diff domain record.
// Change is nil for explicit process additions/removals.
type ProcessChange struct {
	Kind            ProcessChangeKind
	BeforeProcessID string
	AfterProcessID  string
	Change          *bpmndiff.Change
}

type ExplainRequest struct {
	Input      BPMNInput
	ProjectDir string
}

type ProcessExplanation struct {
	ProcessID   string
	Explanation explain.Result
}

type ExplainResult struct {
	Status    Status
	Complete  bool
	Warnings  []Warning
	Document  bpmn.Document
	Processes []ProcessExplanation
}

type ReviewRequest struct {
	Inputs     []BPMNInput
	ProjectDir string
	FailOn     LintThreshold
	Ignore     []string
	AI         AIOptions
}

type ProcessReview struct {
	ProcessID string
	Review    review.Result
}

type ReviewResult struct {
	Status    Status
	Complete  bool
	Warnings  []Warning
	Inputs    []string
	Documents []bpmn.Document
	Processes []ProcessReview
	Findings  []LintFinding
	AIStatus  AIStatus
}

type GenerateRequest struct {
	Input      BPMNInput
	ProjectDir string
	OutDir     string
	Lang       GenerateLanguage
	Force      bool
}

// Artifact is generated output plus the written path used by the current domain writer.
type Artifact struct {
	Path      string
	MediaType string
	Content   []byte
}

type GenerateResult struct {
	Status    Status
	Complete  bool
	Warnings  []Warning
	Document  bpmn.Document
	Artifacts []Artifact
}

type ScanRequest struct {
	Roots      []string
	ProjectDir string
	FailOn     ScanThreshold
}

type ScanResult struct {
	Status       Status
	Complete     bool
	Warnings     []Warning
	ScannedRoots []string
	FailedRoots  []string
	Findings     []scan.Finding
}
