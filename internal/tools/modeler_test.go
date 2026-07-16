package tools_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/tools"
)

func TestWriteModelerProfile(t *testing.T) {
	// Use a temp HOME so we don't touch real Modeler config.
	home := t.TempDir()
	t.Setenv("HOME", home)
	path, err := tools.WriteModelerProfile("camunda-lab", "http://localhost:8080", "localhost:26500")
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if _, ok := m["camunda-lab"]; !ok {
		t.Fatalf("%s", data)
	}
	if filepath.Base(path) != "profiles.json" {
		t.Fatalf("%s", path)
	}
}
