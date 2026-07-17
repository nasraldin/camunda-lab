package explain

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

func TestOfflineOrder(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	path := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "bpmn", "order-v1.bpmn")
	m, err := bpmn.ParseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	r := Offline(m)
	md := r.Markdown()
	for _, section := range []string{"## Business Summary", "## Technical Summary", "## Risks", "## Missing Paths", "## Optimization Suggestions"} {
		if !strings.Contains(md, section) {
			t.Fatalf("missing section %s", section)
		}
	}
	if !strings.Contains(r.Technical, "validate-customer") {
		t.Fatalf("tech: %s", r.Technical)
	}
	if !strings.Contains(r.Business, "OrderCreated") {
		t.Fatalf("biz: %s", r.Business)
	}
}
