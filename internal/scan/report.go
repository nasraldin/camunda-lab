package scan

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatText renders either a complete Result or the legacy findings slice.
func FormatText(value any) string {
	switch typed := value.(type) {
	case Result:
		return formatResultText(typed)
	case []Finding:
		return formatLegacyText(typed)
	default:
		return "Scan report unavailable.\n"
	}
}

func formatResultText(result Result) string {
	var builder strings.Builder
	for _, finding := range result.Findings {
		fmt.Fprintf(
			&builder,
			"%s %-6s %s:%d [%s]  %s\n",
			findingRuleID(finding),
			finding.Severity,
			finding.File,
			finding.Line,
			finding.SourceKind,
			finding.Snippet,
		)
	}
	for _, issue := range result.Issues {
		switch issue.Kind {
		case IssueIgnored:
			fmt.Fprintf(&builder, "ignored %s: %s\n", issue.Path, issue.Reason)
		default:
			fmt.Fprintf(&builder, "%s %s: %s\n", issue.Kind, issue.Path, issue.Message)
		}
	}
	if len(result.Findings) == 0 {
		if result.Complete {
			builder.WriteString("No secrets found.\n")
		} else {
			builder.WriteString("Scan incomplete; no clean result.\n")
		}
	}
	fmt.Fprintf(
		&builder,
		"Scanned %d, ignored %d, errored %d (discovered %d); complete=%t\n",
		result.Stats.Scanned,
		result.Stats.Ignored,
		result.Stats.Errored,
		result.Stats.Discovered,
		result.Complete,
	)
	return builder.String()
}

func formatLegacyText(findings []Finding) string {
	if len(findings) == 0 {
		return "No secrets found.\n"
	}
	var builder strings.Builder
	for _, finding := range findings {
		fmt.Fprintf(
			&builder,
			"%s %-6s %s:%d  %s\n",
			findingRuleID(finding),
			finding.Severity,
			finding.File,
			finding.Line,
			finding.Snippet,
		)
	}
	return builder.String()
}

// FormatJSON renders the stable Result schema with non-null arrays.
func FormatJSON(result Result) ([]byte, error) {
	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func findingRuleID(finding Finding) string {
	if finding.RuleID != "" {
		return finding.RuleID
	}
	return finding.Rule
}
