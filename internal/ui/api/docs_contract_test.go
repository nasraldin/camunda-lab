package api

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", ".."))
}

func readAPIDocs(t *testing.T) string {
	t.Helper()
	path := filepath.Join(repoRoot(t), "docs", "api-reference.md")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(content)
}

func TestToolkitRoutesDocumentedInAPIReference(t *testing.T) {
	doc := readAPIDocs(t)
	routes := []struct {
		method string
		path   string
	}{
		{method: "POST", path: "/api/v1/bpmn/lint"},
		{method: "POST", path: "/api/v1/bpmn/diff"},
		{method: "POST", path: "/api/v1/bpmn/explain"},
		{method: "POST", path: "/api/v1/bpmn/review"},
		{method: "POST", path: "/api/v1/bpmn/test-generate"},
		{method: "POST", path: "/api/v1/bpmn/test-generate/download"},
		{method: "POST", path: "/api/v1/bpmn/scan"},
		{method: "GET", path: "/api/v1/doctor/deep"},
		{method: "GET", path: "/api/v1/env"},
		{method: "POST", path: "/api/v1/env"},
		{method: "POST", path: "/api/v1/env/use"},
		{method: "DELETE", path: "/api/v1/env/{name}"},
		{method: "POST", path: "/api/v1/plan"},
		{method: "POST", path: "/api/v1/drift"},
		{method: "GET", path: "/api/v1/incidents"},
		{method: "POST", path: "/api/v1/incidents/{key}/retry"},
		{method: "GET", path: "/api/v1/trace/{instanceKey}"},
		{method: "POST", path: "/api/v1/backup"},
		{method: "POST", path: "/api/v1/backup/download"},
		{method: "POST", path: "/api/v1/restore"},
		{method: "POST", path: "/api/v1/project/init"},
	}
	for _, route := range routes {
		if !strings.Contains(doc, route.path) {
			t.Errorf("api-reference.md missing route %s", route.path)
		}
		if !strings.Contains(doc, route.method) {
			t.Errorf("api-reference.md missing method %s for %s", route.method, route.path)
		}
	}
}

func TestAPIReferenceDocumentsErrorCodesAndConfirmations(t *testing.T) {
	doc := readAPIDocs(t)
	for _, phrase := range []string{
		"invalid_request",
		"path_forbidden",
		"not_found",
		"payload_too_large",
		"X-Camunda-Lab-CSRF",
		"RESTORE",
		"50 MiB",
		"OIDC",
	} {
		if !strings.Contains(doc, phrase) {
			t.Errorf("api-reference.md missing %q", phrase)
		}
	}
}

func TestAPIReferenceDocumentsResultEnvelope(t *testing.T) {
	doc := readAPIDocs(t)
	for _, field := range []string{`"ok"`, `"status"`, `"complete"`, `"findings"`} {
		if !strings.Contains(doc, field) {
			t.Errorf("api-reference.md missing result field %s", field)
		}
	}
}
