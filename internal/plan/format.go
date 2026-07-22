package plan

import (
	"encoding/json"
	"fmt"
	"strings"
)

func FormatText(result Result) string {
	var text strings.Builder
	environment := result.Environment.Profile.Name
	if environment == "" {
		environment = "unresolved"
	}
	fmt.Fprintf(&text, "Plan (environment=%s, status=%s)\n", environment, result.Status)
	fmt.Fprintf(&text, "Complete: %t  Comparable: %t\n\n", result.Complete, result.Comparable)
	if !result.Comparable {
		text.WriteString("Comparison refused: canonical inventory state is not trustworthy.\n")
	} else {
		for _, action := range result.Actions {
			fmt.Fprintf(&text, "%-11s %s/%s  %s\n", action.Type, action.Resource.Kind, action.Resource.ID, action.Detail)
		}
		if len(result.Actions) == 0 {
			text.WriteString("No deployable resources observed.\n")
		}
		if result.Counts.Unknown > 0 {
			fmt.Fprintf(&text, "unknown     %d local resources excluded because remote content is uninventoried\n", result.Counts.Unknown)
		}
	}
	if len(result.Warnings) > 0 {
		text.WriteString("\nWarnings\n")
		for _, warning := range result.Warnings {
			fmt.Fprintf(&text, "  ! %s: %s\n", warning.Capability, warning.Message)
		}
	}
	fmt.Fprintf(&text, "\nPolicy: %s (exit %d)\n", result.Policy.Outcome, result.Policy.ExitCode)
	text.WriteString("Read-only preview — no deployment, undeploy, delete, Git, or mutation is performed.\n")
	return text.String()
}

func FormatJSON(result Result) ([]byte, error) {
	return json.MarshalIndent(result, "", "  ")
}
