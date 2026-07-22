package explain

import (
	"context"
	"errors"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// EnrichOptimization optionally replaces only the advisory optimization section.
// The graph-derived sections remain the source of truth.
func EnrichOptimization(
	ctx context.Context,
	document bpmn.Document,
	offline Result,
	client ai.ChatClient,
) (Result, error) {
	if client == nil {
		return offline, errors.New("AI client is not configured")
	}
	prompt := "Suggest concise validation and optimization tests for this deterministic BPMN explanation.\n" +
		"Do not rewrite factual sections, propose autofixes, or claim formal model checking.\n\n" +
		offline.Markdown()
	response, err := client.Complete(ctx, ai.ChatRequest{
		Purpose: "explain-optimization", Prompt: prompt, Document: document,
	})
	if err != nil {
		return offline, err
	}
	if strings.TrimSpace(response.Content) == "" {
		return offline, errors.New("AI provider returned an empty explanation enrichment")
	}
	enriched := offline
	enriched.Optimize = response.Content
	return enriched, nil
}
