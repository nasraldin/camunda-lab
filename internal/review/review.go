package review

import (
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

// Result combines lint findings with an optional AI narrative.
type Result struct {
	Findings []lint.Finding
	AIText   string // empty when offline
}

// Options for review.
type Options struct {
	File        string
	FailOn      string
	Ignore      []string
	AI          bool
	AIRequired  bool
	AIClient    Client // optional; if nil and AI, returns error unless tests inject
}

// Client generates AI review text (injected for tests).
type Client interface {
	Review(model bpmn.Model, findings []lint.Finding) (string, error)
}

// Run always lints; optionally enriches with AI.
func Run(m bpmn.Model, opts Options) (Result, error) {
	findings := lint.Run(m, lint.Options{File: opts.File, Ignore: opts.Ignore})
	res := Result{Findings: findings}
	if !opts.AI {
		return res, nil
	}
	if opts.AIClient == nil {
		err := fmt.Errorf("AI review requested but no client configured (set secrets via camunda ai or inject client)")
		if opts.AIRequired {
			return res, err
		}
		res.AIText = "AI skipped: " + err.Error()
		return res, nil
	}
	text, err := opts.AIClient.Review(m, findings)
	if err != nil {
		if opts.AIRequired {
			return res, err
		}
		res.AIText = "AI failed: " + err.Error()
		return res, nil
	}
	res.AIText = text
	return res, nil
}

// FormatText prints lint + AI sections.
func FormatText(r Result) string {
	var b strings.Builder
	b.WriteString("Review (lint)\n")
	b.WriteString(lint.FormatText(r.Findings))
	if r.AIText != "" {
		b.WriteString("\nAI suggestions\n")
		b.WriteString(r.AIText)
		if !strings.HasSuffix(r.AIText, "\n") {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// StubClient returns fixed AI text (tests).
type StubClient struct {
	Text string
	Err  error
}

func (s StubClient) Review(model bpmn.Model, findings []lint.Finding) (string, error) {
	if s.Err != nil {
		return "", s.Err
	}
	if s.Text != "" {
		return s.Text, nil
	}
	var ids []string
	for _, f := range findings {
		ids = append(ids, f.Rule)
	}
	return "Stub review for process " + model.ProcessID + " rules=" + strings.Join(ids, ","), nil
}
