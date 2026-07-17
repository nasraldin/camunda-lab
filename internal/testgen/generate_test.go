package testgen

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func TestGenerateJava(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	bpmnPath := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "order-v1.bpmn")
	m, err := bpmn.ParseFile(bpmnPath)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	paths, err := Generate(m, Options{OutDir: out, Lang: "java"})
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths %v", paths)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "orderProcess") {
		t.Fatalf("missing process id in %s", data)
	}
	if !strings.Contains(string(data), "validate-customer") {
		t.Fatal("missing job type")
	}
	_, err = Generate(m, Options{OutDir: out, Lang: "java"})
	if err == nil {
		t.Fatal("expected overwrite error")
	}
	if _, err := Generate(m, Options{OutDir: out, Lang: "java", Force: true}); err != nil {
		t.Fatal(err)
	}
}

func TestGenerateJS(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	bpmnPath := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "order-v1.bpmn")
	m, err := bpmn.ParseFile(bpmnPath)
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	paths, err := Generate(m, Options{OutDir: out, Lang: "js"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(paths[0])
	if !strings.Contains(string(data), "it.todo") {
		t.Fatalf("%s", data)
	}
}
