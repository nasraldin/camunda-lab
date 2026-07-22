package trace

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func FormatText(tl Timeline) string {
	var text strings.Builder
	fmt.Fprintf(
		&text, "Trace (environment=%s, status=%s)\nInstance: %s  State: %s\nComplete: %t  Partial: %t\n",
		tl.Environment.Profile.Name, tl.Status, tl.InstanceKey, tl.State, tl.Complete, tl.Partial,
	)
	if tl.OperateURL != "" {
		fmt.Fprintf(&text, "Operate: %s\n", tl.OperateURL)
	}
	text.WriteString("\n")
	text.WriteString(RenderASCII(tl))
	if len(tl.Events) > 0 {
		text.WriteString("\nEvents\n")
		for _, event := range tl.Events {
			fmt.Fprintf(
				&text, "  %s  %-9s  %-20s  %s",
				event.Timestamp, event.Kind, trim(event.Key, 20), event.Status,
			)
			if event.Name != "" {
				fmt.Fprintf(&text, "  %s", event.Name)
			}
			if event.Detail != "" {
				fmt.Fprintf(&text, "  (%s)", trim(event.Detail, 40))
			}
			text.WriteByte('\n')
		}
	}
	if len(tl.Warnings) > 0 {
		text.WriteString("\nWarnings\n")
		for _, warning := range tl.Warnings {
			fmt.Fprintf(&text, "  ! %s: %s\n", warning.Capability, warning.Message)
		}
	}
	return text.String()
}

func FormatJSON(tl Timeline) ([]byte, error) {
	stable := tl
	stable.Events = append([]Event{}, tl.Events...)
	stable.Steps = append([]Step{}, tl.Steps...)
	stable.Warnings = append([]inventory.Warning{}, tl.Warnings...)
	if stable.Events == nil {
		stable.Events = []Event{}
	}
	if stable.Steps == nil {
		stable.Steps = []Step{}
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

func trim(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit-1]) + "…"
}
