package toolkit

import (
	"context"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/scan"
)

// GitReader reads a repository-relative path at a revision.
type GitReader interface {
	Read(context.Context, string, string) ([]byte, error)
}

// BPMNService is the injectable developer-toolkit surface shared by CLI and API.
type BPMNService interface {
	Lint(context.Context, LintRequest) (LintResult, error)
	Diff(context.Context, DiffRequest) (DiffResult, error)
	Explain(context.Context, ExplainRequest) (ExplainResult, error)
	Review(context.Context, ReviewRequest) (ReviewResult, error)
	Generate(context.Context, GenerateRequest) (GenerateResult, error)
	Scan(context.Context, ScanRequest) (ScanResult, error)
}

// Service coordinates toolkit domain packages.
type Service struct {
	Git  GitReader
	AI   ai.ChatClient
	scan func(context.Context, scan.Options) (scan.Result, error)
}

var _ BPMNService = Service{}
