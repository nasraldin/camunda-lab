package incidents

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func FormatText(result Result) string {
	var text strings.Builder
	fmt.Fprintf(
		&text, "Incidents (environment=%s, status=%s)\nComplete: %t  Partial: %t\n",
		result.Environment.Profile.Name, result.Status, result.Complete, result.Partial,
	)
	text.WriteString(FormatTable(result.Incidents))
	if len(result.Warnings) > 0 {
		text.WriteString("\nWarnings\n")
		for _, warning := range result.Warnings {
			fmt.Fprintf(&text, "  ! %s: %s\n", warning.Capability, warning.Message)
		}
	}
	fmt.Fprintf(&text, "\nPolicy: %s (exit %d)\n", result.Policy.Outcome, result.Policy.ExitCode)
	return text.String()
}

func FormatIncidentText(incident Incident) string {
	var text strings.Builder
	fmt.Fprintf(&text, "Incident %s\n", incident.Key)
	fmt.Fprintf(&text, "State: %s\nCreated: %s\n", incident.State, incident.CreationTime)
	fmt.Fprintf(&text, "Process: %s (%s)\n", incident.ProcessDefinitionID, incident.ProcessInstanceKey)
	fmt.Fprintf(&text, "Element: %s (%s)\n", incident.ElementID, incident.ElementInstanceKey)
	fmt.Fprintf(&text, "Error: %s: %s\n", incident.ErrorType, incident.ErrorMessage)
	if incident.OperateURL != "" {
		fmt.Fprintf(&text, "Operate: %s\n", incident.OperateURL)
	}
	if len(incident.Warnings) > 0 {
		text.WriteString("Warnings:\n")
		for _, warning := range incident.Warnings {
			fmt.Fprintf(&text, "  ! %s: %s\n", warning.Capability, warning.Message)
		}
	}
	return text.String()
}

// FormatTable remains a compact adapter for current CLI/UI callers.
func FormatTable(items []Incident) string {
	if len(items) == 0 {
		return "No incidents.\n"
	}
	ordered := append([]Incident{}, items...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Key < ordered[j].Key })
	var text strings.Builder
	fmt.Fprintf(&text, "%-20s %-20s %-20s %-20s %s\n", "KEY", "CREATED", "PROCESS", "ELEMENT", "ERROR")
	for _, item := range ordered {
		fmt.Fprintf(
			&text, "%-20s %-20s %-20s %-20s %s: %s\n",
			trim(item.Key, 20), trim(item.CreationTime, 20),
			trim(item.ProcessDefinitionID, 20), trim(item.ElementID, 20),
			trim(item.ErrorType, 24), trim(item.ErrorMessage, 40),
		)
	}
	return text.String()
}

func FormatJSON(result Result) ([]byte, error) {
	stable := result
	stable.Incidents = append([]Incident{}, result.Incidents...)
	stable.Warnings = append([]inventory.Warning{}, result.Warnings...)
	sort.SliceStable(stable.Incidents, func(i, j int) bool {
		return stable.Incidents[i].Key < stable.Incidents[j].Key
	})
	if stable.Incidents == nil {
		stable.Incidents = []Incident{}
	}
	if stable.Warnings == nil {
		stable.Warnings = []inventory.Warning{}
	}
	data, err := json.MarshalIndent(stable, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func FormatAPIItem(incident Incident) map[string]any {
	message := cluster.RedactAPIMessage(incident.ErrorMessage)
	return map[string]any{
		"key":                 incident.Key,
		"id":                  incident.Key,
		"errorMessage":        message,
		"error":               message,
		"processDefinitionId": incident.ProcessDefinitionID,
		"process":             incident.ProcessDefinitionID,
		"elementId":           incident.ElementID,
		"state":               incident.State,
		"creationTime":        incident.CreationTime,
		"processInstanceKey":  incident.ProcessInstanceKey,
		"errorType":           incident.ErrorType,
	}
}

func FormatAPIItems(items []Incident) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, FormatAPIItem(item))
	}
	return out
}

func trim(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
