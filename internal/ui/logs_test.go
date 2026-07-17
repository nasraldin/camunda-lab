package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestPrintLogsMissingFile(t *testing.T) {
	dir := t.TempDir()
	paths.Reset()
	t.Setenv("CAMUNDA_LAB_HOME", dir)

	err := PrintLogs(10, false)
	if err == nil || !strings.Contains(err.Error(), "no UI log") {
		t.Fatalf("expected missing log error, got %v", err)
	}
}

func TestPrintLogsLastLines(t *testing.T) {
	dir := t.TempDir()
	paths.Reset()
	t.Setenv("CAMUNDA_LAB_HOME", dir)
	if err := os.MkdirAll(paths.LogsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(paths.LogsDir(), "ui.log")
	body := "line1\nline2\nline3\n"
	if err := os.WriteFile(logPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	t.Cleanup(func() { os.Stdout = old })

	if err := PrintLogs(2, false); err != nil {
		t.Fatal(err)
	}
	_ = w.Close()
	var out bytes.Buffer
	_, _ = out.ReadFrom(r)
	got := strings.TrimSpace(out.String())
	want := "line2\nline3"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}
