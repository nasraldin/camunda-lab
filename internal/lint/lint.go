package lint

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// Finding is a single lint result.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"` // error|warning
	Message  string `json:"message"`
	Element  string `json:"element,omitempty"`
	File     string `json:"file,omitempty"`
}

// Options controls which findings fail the run.
type Options struct {
	FailOn string   // error|warning
	Ignore []string // rule IDs to skip
	File   string
}

// Run evaluates all MVP rules against a model.
func Run(m bpmn.Model, opts Options) []Finding {
	ignore := map[string]bool{}
	for _, id := range opts.Ignore {
		ignore[id] = true
	}
	var out []Finding
	add := func(f Finding) {
		if ignore[f.Rule] {
			return
		}
		f.File = opts.File
		out = append(out, f)
	}

	hasStart := false
	for _, e := range m.Elements {
		if e.Type == "startEvent" {
			hasStart = true
			break
		}
	}
	if !hasStart && m.ProcessID != "" {
		add(Finding{Rule: "bpmn/process-start-event", Severity: "error", Message: "process has no start event", Element: m.ProcessID})
	}

	connected := map[string]bool{}
	for _, f := range m.Flows {
		connected[f.Source] = true
		connected[f.Target] = true
	}
	for _, e := range m.Elements {
		if e.Type == "boundaryEvent" {
			continue
		}
		if !connected[e.ID] {
			add(Finding{Rule: "bpmn/disconnected-element", Severity: "error", Message: "element is not connected by sequence flows", Element: e.ID})
		}
	}

	for _, e := range m.Elements {
		if e.Type != "exclusiveGateway" {
			continue
		}
		if e.DefaultFlow == "" {
			add(Finding{Rule: "bpmn/exclusive-gateway-default", Severity: "warning", Message: "exclusive gateway has no default flow", Element: e.ID})
		}
		for _, f := range m.Flows {
			if f.Source != e.ID {
				continue
			}
			if f.ID == e.DefaultFlow {
				continue
			}
			if strings.TrimSpace(f.Condition) == "" {
				add(Finding{Rule: "bpmn/exclusive-gateway-condition", Severity: "error", Message: "non-default outgoing flow missing condition", Element: f.ID})
			}
		}
	}

	seenMsg := map[string]string{}
	for _, msg := range m.Messages {
		key := strings.ToLower(strings.TrimSpace(msg.Name))
		if key == "" {
			continue
		}
		if prev, ok := seenMsg[key]; ok {
			add(Finding{Rule: "bpmn/duplicate-message-name", Severity: "error", Message: fmt.Sprintf("duplicate message name %q (also %s)", msg.Name, prev), Element: msg.ID})
		} else {
			seenMsg[key] = msg.ID
		}
	}

	for _, e := range m.Elements {
		if e.Type != "serviceTask" {
			continue
		}
		if e.RetryCount == "" {
			add(Finding{Rule: "bpmn/service-task-retry", Severity: "warning", Message: "service task missing retry extension", Element: e.ID})
		}
	}

	reachable := reachableFromStart(m)
	for _, e := range m.Elements {
		if e.Timer == "" {
			continue
		}
		if !reachable[e.ID] {
			add(Finding{Rule: "bpmn/timer-reachable", Severity: "warning", Message: "timer event is not reachable from a start event", Element: e.ID})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Rule != out[j].Rule {
			return out[i].Rule < out[j].Rule
		}
		return out[i].Element < out[j].Element
	})
	return out
}

func reachableFromStart(m bpmn.Model) map[string]bool {
	adj := map[string][]string{}
	for _, f := range m.Flows {
		adj[f.Source] = append(adj[f.Source], f.Target)
	}
	seen := map[string]bool{}
	var q []string
	for _, e := range m.Elements {
		if e.Type == "startEvent" {
			q = append(q, e.ID)
			seen[e.ID] = true
		}
	}
	for len(q) > 0 {
		cur := q[0]
		q = q[1:]
		for _, n := range adj[cur] {
			if seen[n] {
				continue
			}
			seen[n] = true
			q = append(q, n)
		}
	}
	return seen
}

// ShouldFail reports whether findings fail given FailOn level.
func ShouldFail(findings []Finding, failOn string) bool {
	if failOn == "" {
		failOn = "error"
	}
	for _, f := range findings {
		if f.Severity == "error" {
			return true
		}
		if failOn == "warning" && f.Severity == "warning" {
			return true
		}
	}
	return false
}

// FormatText renders findings for the CLI.
func FormatText(findings []Finding) string {
	if len(findings) == 0 {
		return "No lint findings.\n"
	}
	var b strings.Builder
	for _, f := range findings {
		loc := f.Element
		if f.File != "" {
			loc = f.File + ":" + f.Element
		}
		fmt.Fprintf(&b, "%s %-7s %s  %s\n", f.Rule, f.Severity, loc, f.Message)
	}
	return b.String()
}
