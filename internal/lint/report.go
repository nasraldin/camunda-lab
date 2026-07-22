package lint

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatText renders stable, line-oriented CLI output.
func FormatText(result Result) string {
	if len(result.Findings) == 0 {
		return "No lint findings.\n"
	}
	var output strings.Builder
	for _, finding := range result.Findings {
		location := finding.Element
		if finding.File != "" {
			location = finding.File + ":" + finding.Element
		}
		fmt.Fprintf(
			&output, "%s %-7s %s %s\n",
			finding.Rule, finding.Severity, location, finding.Message,
		)
	}
	return output.String()
}

// FormatJSON renders stable JSON with a trailing newline.
func FormatJSON(result Result) ([]byte, error) {
	content, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}
