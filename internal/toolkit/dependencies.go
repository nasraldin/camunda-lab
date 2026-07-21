package toolkit

import (
	"context"

	"github.com/nasraldin/camunda-lab/internal/ai"
)

// GitReader reads a repository-relative path at a revision.
type GitReader interface {
	Read(context.Context, string, string) ([]byte, error)
}

// Service coordinates toolkit domain packages.
type Service struct {
	Git GitReader
	AI  ai.ChatClient
}
