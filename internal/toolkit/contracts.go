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
	Code    string `json:"code"`
	Message string `json:"message"`
	Path    string `json:"path,omitempty"`
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
	GenerateLanguagePython     GenerateLanguage = "python"
)

type LintRequest struct {
	Inputs     []BPMNInput
	ProjectDir string
	FailOn     LintThreshold
	Ignore     []string
}

type LintResult struct {
	Status    Status          `json:"status"`
	Complete  bool            `json:"complete"`
	Warnings  []Warning       `json:"warnings"`
	Inputs    []string        `json:"inputs"`
	Documents []bpmn.Document `json:"documents,omitempty"`
	Findings  []LintFinding   `json:"findings"`
}

// LintFinding attributes process-scoped findings while leaving document-scoped
// findings with an empty ProcessID.
type LintFinding struct {
	ProcessID string       `json:"processId,omitempty"`
	Finding   lint.Finding `json:"finding"`
}

type DiffRequest struct {
	Before     BPMNInput
	After      BPMNInput
	BeforeGit  *GitInput
	ProjectDir string
}

type DiffResult struct {
	Status   Status          `json:"status"`
	Complete bool            `json:"complete"`
	Warnings []Warning       `json:"warnings"`
	Before   bpmn.Document   `json:"before,omitempty"`
	After    bpmn.Document   `json:"after,omitempty"`
	Changes  []ProcessChange `json:"changes"`
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
	Kind            ProcessChangeKind `json:"kind"`
	BeforeProcessID string            `json:"beforeProcessId,omitempty"`
	AfterProcessID  string            `json:"afterProcessId,omitempty"`
	Change          *bpmndiff.Change  `json:"change,omitempty"`
}

type ExplainRequest struct {
	Input      BPMNInput
	ProjectDir string
}

type ProcessExplanation struct {
	ProcessID   string         `json:"processId"`
	Explanation explain.Result `json:"explanation"`
}

type ExplainResult struct {
	Status    Status               `json:"status"`
	Complete  bool                 `json:"complete"`
	Warnings  []Warning            `json:"warnings"`
	Document  bpmn.Document        `json:"document,omitempty"`
	Processes []ProcessExplanation `json:"processes"`
}

type ReviewRequest struct {
	Inputs     []BPMNInput
	ProjectDir string
	FailOn     LintThreshold
	Ignore     []string
	AI         AIOptions
}

type ProcessReview struct {
	ProcessID string        `json:"processId"`
	Review    review.Result `json:"review"`
}

type ReviewResult struct {
	Status    Status          `json:"status"`
	Complete  bool            `json:"complete"`
	Warnings  []Warning       `json:"warnings"`
	Inputs    []string        `json:"inputs"`
	Documents []bpmn.Document `json:"documents,omitempty"`
	Processes []ProcessReview `json:"processes"`
	Findings  []LintFinding   `json:"findings"`
	AIStatus  AIStatus        `json:"aiStatus"`
}

type GenerateRequest struct {
	Input      BPMNInput
	ProjectDir string
	OutDir     string
	Lang       GenerateLanguage
	Force      bool
}

// Artifact is generated output. Path is relative unless the caller requested publication.
type Artifact struct {
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
	Content   []byte `json:"content,omitempty"`
}

type GenerateResult struct {
	Status    Status        `json:"status"`
	Complete  bool          `json:"complete"`
	Warnings  []Warning     `json:"warnings"`
	Document  bpmn.Document `json:"document,omitempty"`
	Artifacts []Artifact    `json:"artifacts"`
}

type ScanRequest struct {
	Roots      []string
	ProjectDir string
	FailOn     ScanThreshold
	Ignore     []string
}

type ScanResult struct {
	Status       Status         `json:"status"`
	Complete     bool           `json:"complete"`
	Warnings     []Warning      `json:"warnings"`
	ScannedRoots []string       `json:"scannedRoots"`
	FailedRoots  []string       `json:"failedRoots"`
	Findings     []scan.Finding `json:"findings"`
	Issues       []scan.Issue   `json:"issues"`
	Stats        scan.Stats     `json:"stats"`
}
