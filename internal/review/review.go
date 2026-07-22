package review

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/ai"
	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

// AIStatus reports enrichment independently from deterministic lint.
type AIStatus string

const (
	AIStatusDisabled  AIStatus = "disabled"
	AIStatusSkipped   AIStatus = "skipped"
	AIStatusSucceeded AIStatus = "succeeded"
	AIStatusFailed    AIStatus = "failed"
)

// Warning records a recoverable AI degradation.
type Warning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Result combines lint findings with an optional AI narrative.
type Result struct {
	Findings []lint.Finding `json:"findings"`
	AIText   string         `json:"aiText,omitempty"`
	AIStatus AIStatus       `json:"aiStatus"`
	Warnings []Warning      `json:"warnings,omitempty"`
}

// Options for review.
type Options struct {
	File        string
	FailOn      string
	Ignore      []string
	AI          bool
	AIRequired  bool
	AIClient    ai.ChatClient
	PromptLimit int
	// PromptDocument can retain document-level semantics while m is scoped for lint.
	PromptDocument *bpmn.Document
	// PromptFindings adds precomputed document-level findings without re-running lint.
	PromptFindings []lint.Finding
}

// AIError is returned when required enrichment cannot complete.
type AIError struct {
	Stage   string
	Code    string
	Message string
	Err     error
}

func (e *AIError) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("required AI review %s failed", e.Stage)
	}
	return fmt.Sprintf("required AI review %s failed: %s", e.Stage, e.Message)
}
func (e *AIError) Unwrap() error { return e.Err }

// Run always lints and uses a background context for compatibility.
func Run(m bpmn.Model, opts Options) (Result, error) {
	return RunContext(context.Background(), m, opts)
}

// RunContext always completes deterministic lint and optionally enriches it with AI.
func RunContext(ctx context.Context, m bpmn.Model, opts Options) (Result, error) {
	findings := lint.Run(m, lint.Options{File: opts.File, Ignore: opts.Ignore, FailOn: opts.FailOn}).Findings
	res := Result{Findings: findings, AIStatus: AIStatusDisabled}
	aiRequested := opts.AI || opts.AIRequired
	if !aiRequested {
		return res, nil
	}
	if opts.AIClient == nil {
		err := errors.New("AI client is not configured; choose a provider, model, and credentials")
		code, message := "ai_unavailable", err.Error()
		if opts.AIRequired {
			res.AIStatus = AIStatusFailed
			return res, newAIError("configuration", code, message, err)
		}
		res.AIStatus = AIStatusSkipped
		res.Warnings = append(res.Warnings, Warning{Code: code, Message: message})
		return res, nil
	}
	if err := ctx.Err(); err != nil {
		code, message := publicAIDetail("request", err)
		if opts.AIRequired {
			res.AIStatus = AIStatusFailed
			return res, newAIError("request", code, message, err)
		}
		res.AIStatus = AIStatusFailed
		res.Warnings = append(res.Warnings, Warning{Code: code, Message: message})
		return res, nil
	}
	promptDocument := m
	if opts.PromptDocument != nil {
		promptDocument = *opts.PromptDocument
	}
	promptFindings := mergePromptFindings(findings, opts.PromptFindings)
	prompt, compacted, err := BuildPrompt(promptDocument, promptFindings, opts.PromptLimit)
	if err != nil {
		code, message := publicAIDetail("prompt", err)
		if opts.AIRequired {
			res.AIStatus = AIStatusFailed
			return res, newAIError("prompt", code, message, err)
		}
		res.AIStatus = AIStatusFailed
		res.Warnings = append(res.Warnings, Warning{Code: code, Message: message})
		return res, nil
	}
	if compacted {
		res.Warnings = append(res.Warnings, Warning{
			Code: "ai_prompt_compacted", Message: "AI prompt model semantics were compacted with omission metadata",
		})
	}
	response, err := opts.AIClient.Complete(ctx, ai.ChatRequest{
		Purpose: "review", Prompt: prompt, Document: promptDocument, Findings: promptFindings,
	})
	if err != nil {
		code, message := publicAIDetail("provider", err)
		if opts.AIRequired {
			res.AIStatus = AIStatusFailed
			return res, newAIError("provider", code, message, err)
		}
		res.AIStatus = AIStatusFailed
		res.Warnings = append(res.Warnings, Warning{Code: code, Message: message})
		return res, nil
	}
	if strings.TrimSpace(response.Content) == "" {
		err = errors.New("AI provider returned an empty completion")
		code, message := "ai_empty_response",
			"AI provider returned an empty completion; verify provider model and endpoint compatibility"
		if opts.AIRequired {
			res.AIStatus = AIStatusFailed
			return res, newAIError("provider", code, message, err)
		}
		res.AIStatus = AIStatusFailed
		res.Warnings = append(res.Warnings, Warning{Code: code, Message: message})
		return res, nil
	}
	res.AIStatus = AIStatusSucceeded
	res.AIText = response.Content
	return res, nil
}

func newAIError(stage, code, message string, err error) *AIError {
	return &AIError{Stage: stage, Code: code, Message: message, Err: err}
}

func publicAIDetail(stage string, err error) (string, string) {
	var configErr *ai.ConfigError
	if errors.As(err, &configErr) {
		return configErr.SafeCode(), configErr.SafeMessage()
	}
	var providerErr *ai.ProviderError
	if errors.As(err, &providerErr) {
		return providerErr.SafeCode(), providerErr.SafeMessage()
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "ai_timeout", "AI provider request timed out; retry or increase the configured timeout"
	}
	if errors.Is(err, context.Canceled) {
		return "ai_canceled", "AI review was canceled"
	}
	switch stage {
	case "prompt":
		return "ai_prompt_invalid", "AI prompt construction failed; increase the prompt limit or reduce model semantics"
	case "configuration":
		return "ai_configuration_invalid", "AI configuration is invalid; verify provider, model, endpoint, and credentials"
	default:
		return "ai_provider_failed", "AI provider request failed; verify provider endpoint, model, credentials, and service availability"
	}
}

func mergePromptFindings(processFindings, documentFindings []lint.Finding) []lint.Finding {
	result := make([]lint.Finding, 0, len(processFindings)+len(documentFindings))
	seen := map[string]bool{}
	for _, group := range [][]lint.Finding{processFindings, documentFindings} {
		for _, finding := range group {
			key := finding.File + "\x00" + finding.ProcessID + "\x00" + finding.Rule + "\x00" +
				finding.Element + "\x00" + string(finding.Severity) + "\x00" + finding.Message
			if seen[key] {
				continue
			}
			seen[key] = true
			result = append(result, finding)
		}
	}
	return result
}

// FormatText prints lint + AI sections.
func FormatText(r Result) string {
	var b strings.Builder
	b.WriteString("Review (lint)\n")
	b.WriteString(lint.FormatText(lint.Result{Findings: r.Findings}))
	if r.AIText != "" {
		b.WriteString("\nAI suggestions\n")
		b.WriteString(r.AIText)
		if !strings.HasSuffix(r.AIText, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}
