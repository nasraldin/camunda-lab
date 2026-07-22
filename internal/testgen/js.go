package testgen

import (
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func renderJS(process bpmn.Process) (Artifact, error) {
	path := "js/" + sanitizeIdent(process.ID) + ".spec.js"
	var source strings.Builder
	source.WriteString("'use strict';\n\n")
	source.WriteString("const { test, expect } = require('@playwright/test');\n\n")
	fmt.Fprintf(&source, "const PROCESS_ID = %s;\n\n", jsString(process.ID))
	fmt.Fprintf(&source, "test.describe(%s, () => {\n", jsString("BPMN process "+process.ID))
	source.WriteString("  test.skip('completes the happy path', async ({ request }) => {\n")
	source.WriteString("    // TODO: provide an authenticated test fixture, deploy the BPMN, and start PROCESS_ID.\n")
	source.WriteString("    // Limitation: generated tests never connect to or execute against a live cluster.\n")
	source.WriteString("    expect(PROCESS_ID).toBeTruthy();\n")
	source.WriteString("    void request;\n")
	source.WriteString("  });\n")
	for _, jobType := range uniqueJobTypes(process) {
		fmt.Fprintf(&source, "\n  test.skip(%s, async () => {\n", jsString("handles job type "+jobType))
		source.WriteString("    // TODO: complete the activated job and assert process variables and the next BPMN state.\n")
		source.WriteString("  });\n")
	}
	source.WriteString("});\n")
	return Artifact{Path: path, MediaType: "text/javascript", Content: []byte(source.String())}, nil
}

func jsString(value string) string {
	replacer := strings.NewReplacer(
		`\`, `\\`,
		`'`, `\'`,
		"\r", `\r`,
		"\n", `\n`,
		"\u2028", `\u2028`,
		"\u2029", `\u2029`,
	)
	return "'" + replacer.Replace(value) + "'"
}
