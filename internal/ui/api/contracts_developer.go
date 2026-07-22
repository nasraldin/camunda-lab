package api

import (
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/scan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
)

type developerInputRequest struct {
	Path       string   `json:"path,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	ProjectDir string   `json:"projectDir,omitempty"`
	FailOn     string   `json:"failOn,omitempty"`
	Ignore     []string `json:"ignore,omitempty"`
	Format     string   `json:"format,omitempty"`
}

type developerDiffRequest struct {
	Paths      []string `json:"paths,omitempty"`
	From       string   `json:"from,omitempty"`
	To         string   `json:"to,omitempty"`
	Path       string   `json:"path,omitempty"`
	Against    string   `json:"against,omitempty"`
	Base       string   `json:"base,omitempty"`
	ProjectDir string   `json:"projectDir,omitempty"`
}

type developerReviewRequest struct {
	Path       string   `json:"path,omitempty"`
	Paths      []string `json:"paths,omitempty"`
	ProjectDir string   `json:"projectDir,omitempty"`
	FailOn     string   `json:"failOn,omitempty"`
	Ignore     []string `json:"ignore,omitempty"`
	AI         bool     `json:"ai,omitempty"`
	AIRequired bool     `json:"aiRequired,omitempty"`
	Provider   *string  `json:"provider,omitempty"`
	Model      *string  `json:"model,omitempty"`
}

type developerGenerateRequest struct {
	Path       string `json:"path"`
	ProjectDir string `json:"projectDir,omitempty"`
	Lang       string `json:"lang,omitempty"`
	Write      bool   `json:"write,omitempty"`
	Output     string `json:"output,omitempty"`
	Force      bool   `json:"force,omitempty"`
}

type developerScanRequest struct {
	Dir        string   `json:"dir,omitempty"`
	Roots      []string `json:"roots,omitempty"`
	ProjectDir string   `json:"projectDir,omitempty"`
	FailOn     string   `json:"failOn,omitempty"`
	Ignore     []string `json:"ignore,omitempty"`
}

type developerErrorResponse struct {
	OK    bool   `json:"ok"`
	Code  string `json:"code"`
	Error string `json:"error"`
	Hint  string `json:"hint,omitempty"`
}

type lintResponse struct {
	OK       bool                  `json:"ok"`
	Status   toolkit.Status        `json:"status"`
	Complete bool                  `json:"complete"`
	Warnings []toolkit.Warning     `json:"warnings"`
	Findings []toolkit.LintFinding `json:"findings"`
	Inputs   []string              `json:"inputs"`
	CLI      string                `json:"cli"`
}

type diffResponse struct {
	OK       bool                    `json:"ok"`
	Status   toolkit.Status          `json:"status"`
	Complete bool                    `json:"complete"`
	Warnings []toolkit.Warning       `json:"warnings"`
	Changes  []toolkit.ProcessChange `json:"changes"`
	CLI      string                  `json:"cli"`
}

type explainProcessDTO struct {
	ProcessID string `json:"processId"`
	Markdown  string `json:"markdown"`
}

type explainResponse struct {
	OK        bool                `json:"ok"`
	Status    toolkit.Status      `json:"status"`
	Complete  bool                `json:"complete"`
	Warnings  []toolkit.Warning   `json:"warnings"`
	Processes []explainProcessDTO `json:"processes"`
	Output    string              `json:"output"`
	CLI       string              `json:"cli"`
}

type reviewResponse struct {
	OK       bool                    `json:"ok"`
	Status   toolkit.Status          `json:"status"`
	Complete bool                    `json:"complete"`
	Warnings []toolkit.Warning       `json:"warnings"`
	Findings []toolkit.LintFinding   `json:"findings"`
	AIStatus toolkit.AIStatus        `json:"aiStatus"`
	Reviews  []toolkit.ProcessReview `json:"reviews"`
	CLI      string                  `json:"cli"`
}

type artifactDTO struct {
	Path      string `json:"path"`
	MediaType string `json:"mediaType"`
	Content   string `json:"content,omitempty"`
}

type generateResponse struct {
	OK        bool              `json:"ok"`
	Status    toolkit.Status    `json:"status"`
	Complete  bool              `json:"complete"`
	Warnings  []toolkit.Warning `json:"warnings"`
	Mode      string            `json:"mode"`
	Artifacts []artifactDTO     `json:"artifacts"`
	Paths     []string          `json:"paths"`
	Contents  map[string]string `json:"contents,omitempty"`
	CLI       string            `json:"cli"`
}

type scanResponse struct {
	OK           bool              `json:"ok"`
	Status       toolkit.Status    `json:"status"`
	Complete     bool              `json:"complete"`
	Warnings     []toolkit.Warning `json:"warnings"`
	ScannedRoots []string          `json:"scannedRoots"`
	FailedRoots  []string          `json:"failedRoots"`
	Findings     []scan.Finding    `json:"findings"`
	Issues       []scan.Issue      `json:"issues"`
	Stats        scan.Stats        `json:"stats"`
	CLI          string            `json:"cli"`
}

type doctorDeepResponse struct {
	OK     bool           `json:"ok"`
	Status string         `json:"status"`
	Checks []doctor.Check `json:"checks"`
	Report string         `json:"report"`
	CLI    string         `json:"cli"`
}
