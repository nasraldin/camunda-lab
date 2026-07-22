package testgen

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func renderPython(process bpmn.Process) (Artifact, error) {
	path := "python/test_" + strings.ToLower(sanitizeIdent(process.ID)) + ".py"
	var source strings.Builder
	source.WriteString("\"\"\"Generated pytest scaffold; it performs no live-cluster operations.\"\"\"\n\n")
	source.WriteString("import pytest\n\n")
	fmt.Fprintf(&source, "PROCESS_ID = %s\n\n", strconv.Quote(process.ID))
	source.WriteString("@pytest.mark.skip(reason=\"TODO: provide an authenticated process-test fixture\")\n")
	source.WriteString("def test_completes_happy_path():\n")
	source.WriteString("    \"\"\"TODO: deploy BPMN, start PROCESS_ID, and assert the completed path.\n\n")
	source.WriteString("    Limitation: generated tests never connect to or execute against a live cluster.\n")
	source.WriteString("    \"\"\"\n")
	source.WriteString("    assert PROCESS_ID\n")
	for _, jobType := range uniqueJobTypes(process) {
		fmt.Fprintf(&source, "\n\n@pytest.mark.skip(reason=%s)\n", strconv.Quote("TODO: implement worker fixture for "+jobType))
		fmt.Fprintf(&source, "def test_handles_%s():\n", pythonIdent(jobType))
		source.WriteString("    \"\"\"Complete the activated job and assert variables and the next BPMN state.\"\"\"\n")
		source.WriteString("    assert PROCESS_ID\n")
	}
	source.WriteByte('\n')
	return Artifact{Path: path, MediaType: "text/x-python", Content: []byte(source.String())}, nil
}

func pythonIdent(value string) string {
	return strings.ToLower(sanitizeIdent(value))
}
