package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestTraceAPIUsesSharedServiceAndCanonicalSearches(t *testing.T) {
	var sawGetByPath bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/process-instances/search":
			_, _ = w.Write([]byte(`{"items":[{
				"processInstanceKey":"9007199254740993","processDefinitionKey":"8","processDefinitionId":"orders",
				"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
				"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
			}],"page":{"totalItems":1,"endCursor":null}}`))
		case "/v2/element-instances/search", "/v2/incidents/search", "/v2/jobs/search":
			_, _ = w.Write([]byte(`{"items":[],"page":{"totalItems":0,"endCursor":null}}`))
		default:
			if strings.HasPrefix(r.URL.Path, "/v2/process-instances/") {
				sawGetByPath = true
			}
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	h := &handler{clusterFactory: &uiTestClusterFactory{baseURL: server.URL + "/v2"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trace/9007199254740993", nil)
	req.SetPathValue("instanceKey", "9007199254740993")
	rec := httptest.NewRecorder()
	h.traceInstance(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK       bool           `json:"ok"`
		Timeline map[string]any `json:"timeline"`
		Output   string         `json:"output"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Timeline["instanceKey"] != "9007199254740993" ||
		body.Timeline["status"] != "completed" || !strings.Contains(body.Output, "9007199254740993") {
		t.Fatalf("body=%#v", body)
	}
	if sawGetByPath {
		t.Fatal("API trace used ad hoc process-instance GET")
	}
}

func TestTraceAPIReturnsNotFoundForMissingInstance(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/process-instances/search" {
			_, _ = w.Write([]byte(`{"items":[],"page":{"totalItems":0,"endCursor":null}}`))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	h := &handler{clusterFactory: &uiTestClusterFactory{baseURL: server.URL + "/v2"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/trace/55", nil)
	req.SetPathValue("instanceKey", "55")
	rec := httptest.NewRecorder()
	h.traceInstance(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}
