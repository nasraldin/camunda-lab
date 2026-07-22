package diff

import (
	"encoding/json"
	"fmt"
	"strings"
)

// FormatText renders changes in deterministic order.
func FormatText(changes []Change) string {
	if len(changes) == 0 {
		return "No semantic changes.\n"
	}
	ordered := append([]Change(nil), changes...)
	sortChanges(ordered)
	var output strings.Builder
	for _, change := range ordered {
		fmt.Fprintf(&output, "✓ %s\n", change.Summary)
	}
	return output.String()
}

// FormatJSON renders a stable, indented JSON array followed by a newline.
func FormatJSON(changes []Change) ([]byte, error) {
	ordered := make([]Change, len(changes))
	copy(ordered, changes)
	sortChanges(ordered)
	content, err := json.MarshalIndent(ordered, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(content, '\n'), nil
}
