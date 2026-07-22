package explain

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

// Result is a markdown explanation.
type Result struct {
	Business  string `json:"business"`
	Technical string `json:"technical"`
	Risks     string `json:"risks"`
	Missing   string `json:"missingPaths"`
	Optimize  string `json:"optimizationSuggestions"`
}

// Offline builds a deterministic explanation from IR (+ lint risks).
func Offline(m bpmn.Model) Result {
	var biz, tech, risks, missing, opt strings.Builder
	processes := m.Processes
	if len(processes) == 0 && (m.ProcessID != "" || len(m.Elements) > 0) {
		processes = []bpmn.Process{{
			ID: m.ProcessID, ProcessID: m.ProcessID, Name: m.Name,
			Elements: m.Elements, Flows: m.Flows, Messages: m.Messages,
		}}
	}
	writeDocumentDefinitions(&tech, m.Messages, m.Errors)
	for processIndex, process := range processes {
		if processIndex > 0 {
			biz.WriteByte('\n')
			tech.WriteByte('\n')
			missing.WriteByte('\n')
		}
		writeBusiness(&biz, process)
		writeTechnical(&tech, process)
		writeGraphDiagnostics(&missing, process)
	}

	findings := lint.Run(m, lint.Options{}).Findings
	if len(findings) == 0 {
		risks.WriteString("No lint risks detected.\n")
	} else {
		for _, f := range findings {
			fmt.Fprintf(&risks, "- [%s] %s (%s)\n", f.Severity, f.Message, f.Element)
		}
	}

	opt.WriteString("Validate alternate and cyclic paths with tests; define service-task retries and gateway defaults; verify message, timer, and error handling behavior.\n")

	return Result{
		Business:  biz.String(),
		Technical: tech.String(),
		Risks:     risks.String(),
		Missing:   missing.String(),
		Optimize:  opt.String(),
	}
}

func writeDocumentDefinitions(output *strings.Builder, messages []bpmn.Message, errors []bpmn.Error) {
	output.WriteString("### Document Definitions\n")
	orderedMessages := append([]bpmn.Message(nil), messages...)
	sort.SliceStable(orderedMessages, func(i, j int) bool {
		return orderedMessages[i].Name+"\x00"+orderedMessages[i].ID <
			orderedMessages[j].Name+"\x00"+orderedMessages[j].ID
	})
	for _, message := range orderedMessages {
		fmt.Fprintf(output, "- message %s name=%q\n", message.ID, displayName(message.Name, message.ID))
	}
	orderedErrors := append([]bpmn.Error(nil), errors...)
	sort.SliceStable(orderedErrors, func(i, j int) bool {
		left := orderedErrors[i].Name + "\x00" + orderedErrors[i].ErrorCode + "\x00" + orderedErrors[i].ID
		right := orderedErrors[j].Name + "\x00" + orderedErrors[j].ErrorCode + "\x00" + orderedErrors[j].ID
		return left < right
	})
	for _, definition := range orderedErrors {
		fmt.Fprintf(output, "- error %s name=%q errorCode=%q\n",
			definition.ID, displayName(definition.Name, definition.ID), definition.ErrorCode)
	}
	if len(orderedMessages) == 0 && len(orderedErrors) == 0 {
		output.WriteString("No document message or error definitions.\n")
	}
	output.WriteByte('\n')
}

func writeBusiness(output *strings.Builder, process bpmn.Process) {
	fmt.Fprintf(output, "### Process: %s (%s)\n", displayName(process.Name, process.ID), process.ID)
	for _, scope := range summarizeScopes(process) {
		prefix := ""
		happyLabel := "Happy path"
		if scope.id != "" {
			prefix = fmt.Sprintf("Scope %s (%s) ", displayName(scope.name, scope.id), scope.id)
			happyLabel = "happy path"
		}
		if len(scope.happy) == 0 {
			fmt.Fprintf(output, "%s%s: unavailable (no start event).\n", prefix, happyLabel)
		} else {
			fmt.Fprintf(output, "%s%s: %s\n", prefix, happyLabel, formatPath(process, scope.happy))
		}
		for index, path := range scope.alternates {
			fmt.Fprintf(output, "%sAlternate path %d: %s\n", prefix, index+1, formatPath(process, path))
		}
	}
}

func writeTechnical(output *strings.Builder, process bpmn.Process) {
	fmt.Fprintf(output, "### Process: %s (%s)\n", displayName(process.Name, process.ID), process.ID)
	fmt.Fprintf(output, "Elements: %d, sequence flows: %d\n", len(process.Elements), len(process.Flows))
	for _, element := range process.Elements {
		fmt.Fprintf(output, "- %s %s name=%q", element.Type, element.ID, displayName(element.Name, element.ID))
		if element.ParentID != "" {
			fmt.Fprintf(output, " scope=%q", element.ParentID)
		}
		switch element.Type {
		case "serviceTask", "scriptTask":
			fmt.Fprintf(output, " jobType=%q retries=%q", displayName(element.JobType, "(none)"), displayName(element.RetryCount, "(none)"))
		case "exclusiveGateway", "inclusiveGateway", "parallelGateway", "eventBasedGateway":
			fmt.Fprintf(output, " default=%q", displayName(element.DefaultFlow, "(none)"))
		}
		if len(element.EventDefs) > 0 {
			fmt.Fprintf(output, " events=%q", strings.Join(element.EventDefs, ","))
		}
		if element.Timer != "" || element.TimerKind != "" {
			fmt.Fprintf(output, " timerKind=%q timer=%q", displayName(element.TimerKind, "(none)"), displayName(element.Timer, "(none)"))
		}
		if element.MessageRef != "" {
			fmt.Fprintf(output, " messageRef=%q", element.MessageRef)
		}
		if element.ErrorRef != "" {
			fmt.Fprintf(output, " errorRef=%q", element.ErrorRef)
		}
		if element.CalledElement != "" {
			fmt.Fprintf(output, " calledElement=%q", element.CalledElement)
		}
		if element.AttachedTo != "" {
			fmt.Fprintf(output, " attachedTo=%q cancelActivity=%t", element.AttachedTo, element.CancelActivity)
		}
		output.WriteByte('\n')
	}
	for _, flow := range process.Flows {
		fmt.Fprintf(output, "- sequenceFlow %s %s → %s name=%q condition=%q\n",
			flow.ID, flow.Source, flow.Target, flow.Name, flow.Condition)
	}
}

func writeGraphDiagnostics(output *strings.Builder, process bpmn.Process) {
	fmt.Fprintf(output, "### Process: %s (%s)\n", displayName(process.Name, process.ID), process.ID)
	count := 0
	for _, scope := range summarizeScopes(process) {
		prefix := ""
		if scope.id != "" {
			prefix = fmt.Sprintf("Scope %s (%s): ", displayName(scope.name, scope.id), scope.id)
		}
		if len(scope.happy) == 0 {
			fmt.Fprintf(output, "- %sNo start event; path traversal cannot begin.\n", prefix)
			count++
		}
		for _, cycle := range scope.cycles {
			fmt.Fprintf(output, "- %sCycle: %s\n", prefix, formatPath(process, cycle))
			count++
		}
		for _, id := range scope.deadEnds {
			fmt.Fprintf(output, "- %sDead end: %s\n", prefix, formatPath(process, []string{id}))
			count++
		}
	}
	if count == 0 {
		output.WriteString("No obvious missing paths from graph analysis.\n")
	}
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

// FormatJSON renders deterministic JSON with a trailing newline.
func FormatJSON(result Result) ([]byte, error) {
	content, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}
