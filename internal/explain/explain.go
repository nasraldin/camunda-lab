package explain

import (
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

// Result is a markdown explanation.
type Result struct {
	Business  string
	Technical string
	Risks     string
	Missing   string
	Optimize  string
}

// Offline builds a deterministic explanation from IR (+ lint risks).
func Offline(m bpmn.Model) Result {
	var biz, tech, risks, missing, opt strings.Builder

	fmt.Fprintf(&biz, "Process %q (%s).\n", emptyName(m.Name, m.ProcessID), m.ProcessID)
	fmt.Fprintf(&biz, "Happy-path elements: ")
	var names []string
	for _, e := range m.Elements {
		if e.Type == "startEvent" || e.Type == "endEvent" || e.Type == "serviceTask" || e.Type == "userTask" {
			names = append(names, emptyName(e.Name, e.ID))
		}
	}
	biz.WriteString(strings.Join(names, " → "))
	biz.WriteByte('\n')

	fmt.Fprintf(&tech, "Elements: %d, sequence flows: %d, messages: %d\n", len(m.Elements), len(m.Flows), len(m.Messages))
	for _, e := range m.ServiceTasks() {
		fmt.Fprintf(&tech, "- serviceTask %s jobType=%s retries=%s\n", e.ID, emptyName(e.JobType, "(none)"), emptyName(e.RetryCount, "(none)"))
	}
	for _, e := range m.Elements {
		if e.Type == "exclusiveGateway" {
			fmt.Fprintf(&tech, "- exclusiveGateway %s default=%s\n", e.ID, emptyName(e.DefaultFlow, "(none)"))
		}
		if e.Timer != "" {
			fmt.Fprintf(&tech, "- timer %s = %s\n", e.ID, e.Timer)
		}
	}

	findings := lint.Run(m, lint.Options{})
	if len(findings) == 0 {
		risks.WriteString("No lint risks detected.\n")
	} else {
		for _, f := range findings {
			fmt.Fprintf(&risks, "- [%s] %s (%s)\n", f.Severity, f.Message, f.Element)
		}
	}

	reachable := map[string]bool{}
	// reuse lint reachability via disconnected findings
	for _, e := range m.Elements {
		if e.Type == "endEvent" {
			continue
		}
	}
	_ = reachable
	missing.WriteString(missingPaths(m))

	opt.WriteString("Ensure service tasks define retries; exclusive gateways declare defaults; cover message correlation paths in tests.\n")

	return Result{
		Business:  biz.String(),
		Technical: tech.String(),
		Risks:     risks.String(),
		Missing:   missing.String(),
		Optimize:  opt.String(),
	}
}

func missingPaths(m bpmn.Model) string {
	var b strings.Builder
	hasStart := false
	for _, e := range m.Elements {
		if e.Type == "startEvent" {
			hasStart = true
		}
	}
	if !hasStart {
		b.WriteString("- No start event — process cannot begin.\n")
	}
	outdegree := map[string]int{}
	indegree := map[string]int{}
	for _, f := range m.Flows {
		outdegree[f.Source]++
		indegree[f.Target]++
	}
	for _, e := range m.Elements {
		if e.Type == "endEvent" || e.Type == "boundaryEvent" {
			continue
		}
		if outdegree[e.ID] == 0 && e.Type != "endEvent" {
			fmt.Fprintf(&b, "- Dead end at %s (%s)\n", e.ID, e.Type)
		}
	}
	if b.Len() == 0 {
		return "No obvious missing paths from static analysis.\n"
	}
	return b.String()
}

// Markdown renders fixed sections.
func (r Result) Markdown() string {
	var b strings.Builder
	b.WriteString("## Business Summary\n\n")
	b.WriteString(r.Business)
	b.WriteString("\n## Technical Summary\n\n")
	b.WriteString(r.Technical)
	b.WriteString("\n## Risks\n\n")
	b.WriteString(r.Risks)
	b.WriteString("\n## Missing Paths\n\n")
	b.WriteString(r.Missing)
	b.WriteString("\n## Optimization Suggestions\n\n")
	b.WriteString(r.Optimize)
	return b.String()
}

func emptyName(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
