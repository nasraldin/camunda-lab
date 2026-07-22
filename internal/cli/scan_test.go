package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/scan"
)

func TestScanCommandEmitsResultSchemaAndPartialErrors(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken.env")); err != nil {
		t.Fatal(err)
	}
	command := newScanCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{root, "--json"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected incomplete scan to return an error")
	}
	var result scan.Result
	if err := json.Unmarshal(output.Bytes(), &result); err != nil {
		t.Fatalf("JSON output = %q: %v", output.String(), err)
	}
	if result.Complete || len(result.Issues) != 1 || result.Findings == nil || result.Issues == nil {
		t.Fatalf("result = %+v", result)
	}
}

func TestScanCommandNeverClaimsPartialScanIsClean(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing"), filepath.Join(root, "broken.env")); err != nil {
		t.Fatal(err)
	}
	command := newScanCmd()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetErr(&output)
	command.SetArgs([]string{root})
	_ = command.Execute()
	if strings.Contains(output.String(), "No secrets found") ||
		!strings.Contains(output.String(), "Scan incomplete") {
		t.Fatalf("output = %q", output.String())
	}
}
