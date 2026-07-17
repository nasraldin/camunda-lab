package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildCreateUpdateDelete(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "bpmn"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bpmn", "order.bpmn"), []byte("v2"), 0o644); err != nil {
		t.Fatal(err)
	}
	local, err := LocalInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	remote := []Resource{
		{Key: "order.bpmn", Digest: "olddigest", Version: "3"},
		{Key: "gone.bpmn", Digest: "abc", Version: "1"},
	}
	p := Build("lab", local, remote)
	kinds := map[string]string{}
	for _, a := range p.Actions {
		kinds[a.Key] = a.Kind
	}
	if kinds["order.bpmn"] != "update" {
		t.Fatalf("%+v", p.Actions)
	}
	if kinds["gone.bpmn"] != "delete" {
		t.Fatal(kinds)
	}
	text := FormatText(p)
	if !strings.Contains(text, "Preview only") {
		t.Fatal(text)
	}
}

func TestLocalCreate(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "bpmn"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "bpmn", "new.bpmn"), []byte("x"), 0o644)
	local, _ := LocalInventory(root)
	p := Build("lab", local, nil)
	if len(p.Actions) != 1 || p.Actions[0].Kind != "create" {
		t.Fatalf("%+v", p.Actions)
	}
}

func TestLocalInventoryProcessesDir(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "processes"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "processes", "order-v1.bpmn"), []byte("xml"), 0o644)
	local, err := LocalInventory(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(local) != 1 || local[0].Key != "order-v1.bpmn" {
		t.Fatalf("%+v", local)
	}
}
