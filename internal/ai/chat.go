package ai

import (
	"context"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

// ChatClient is the provider-neutral AI boundary used by application services.
// Provider adapters are added separately; callers can always omit the client.
type ChatClient interface {
	Complete(context.Context, ChatRequest) (ChatResponse, error)
}

// ChatRequest carries domain data without coupling callers to a provider.
type ChatRequest struct {
	Purpose  string
	Document bpmn.Document
	Findings []lint.Finding
}

// ChatResponse is provider-neutral generated content.
type ChatResponse struct {
	Content string
}
