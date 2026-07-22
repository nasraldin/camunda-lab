package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func TestNormalizeBaseURL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"origin", "https://cluster.example", "https://cluster.example/v2"},
		{"origin trailing slash", "https://cluster.example/", "https://cluster.example/v2"},
		{"already v2", "https://cluster.example/v2", "https://cluster.example/v2"},
		{"v2 trailing slash", "https://cluster.example/v2/", "https://cluster.example/v2"},
		{"path prefix", "https://gateway.example/camunda", "https://gateway.example/camunda/v2"},
		{"path prefix v2", "https://gateway.example/camunda/v2/", "https://gateway.example/camunda/v2"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizeBaseURL(tt.in)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Fatalf("NormalizeBaseURL(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestNormalizeBaseURLRejectsUnsafeParts(t *testing.T) {
	for _, raw := range []string{
		"cluster.example",
		"ftp://cluster.example",
		"https://cluster.example/v2?token=secret",
		"https://user:pass@cluster.example/v2",
	} {
		if _, err := NormalizeBaseURL(raw); err == nil {
			t.Fatalf("NormalizeBaseURL(%q) succeeded", raw)
		}
	}
}

func TestSearchProcessDefinitionsAndXML(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-definitions/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{
				"processDefinitionKey": "111",
				"processDefinitionId":  "orderProcess",
				"name":                 "Order",
				"version":              2,
				"resourceName":         "order.bpmn",
			}},
			"page": map[string]any{"totalItems": 1, "endCursor": nil},
		})
	})
	mux.HandleFunc("/v2/process-definitions/111/xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="orderProcess"><startEvent id="start"/></process></definitions>`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL + "/v2"}
	defs, err := c.SearchProcessDefinitions(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(defs) != 1 || defs[0].ProcessDefinitionID != "orderProcess" {
		t.Fatalf("%+v", defs)
	}
	xml, err := c.GetProcessDefinitionXML(context.Background(), "111")
	if err != nil || !strings.Contains(xml, "orderProcess") {
		t.Fatal(err, xml)
	}
	inv, err := c.BuildInventory(context.Background(), inventory.Source{
		Type: "remote", Environment: "prod", Endpoint: c.BaseURL,
	}, InventoryLimits{})
	if err != nil {
		t.Fatal(err)
	}
	if len(inv.Resources) == 0 || inv.Resources[0].Digest == "" {
		t.Fatalf("%+v", inv)
	}
}

func TestSearchIncidentsAndResolve(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{{
				"incidentKey":         "99",
				"processInstanceKey":  "55",
				"elementId":           "Payment",
				"errorType":           "JOB_NO_RETRIES",
				"errorMessage":        "timeout",
				"state":               "ACTIVE",
				"creationTime":        "2026-07-17T12:00:00Z",
				"processDefinitionId": "orderProcess",
			}},
			"page": map[string]any{"totalItems": 1, "endCursor": nil},
		})
	})
	mux.HandleFunc("/v2/incidents/99/resolution", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v2"}
	items, err := c.SearchIncidents(context.Background(), 10)
	if err != nil || len(items) != 1 {
		t.Fatal(err, items)
	}
	if err := c.ResolveIncident(context.Background(), "99"); err != nil {
		t.Fatal(err)
	}
}

func TestElementInstancesTimeline(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/process-instances/55", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"processInstanceKey":  "55",
			"processDefinitionId": "orderProcess",
			"state":               "ACTIVE",
			"hasIncident":         true,
		})
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"items": []map[string]any{
				{"elementInstanceKey": "1", "elementId": "Start", "elementName": "OrderCreated", "type": "START_EVENT", "state": "COMPLETED", "startDate": "2026-07-17T12:00:00Z"},
				{"elementInstanceKey": "2", "elementId": "Payment", "elementName": "Payment", "type": "SERVICE_TASK", "state": "ACTIVE", "incidentKey": "99", "startDate": "2026-07-17T12:01:00Z"},
			},
			"page": map[string]any{"totalItems": 2, "endCursor": nil},
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := &Client{BaseURL: srv.URL + "/v2"}
	pi, err := c.GetProcessInstance(context.Background(), "55")
	if err != nil || pi.State != "ACTIVE" {
		t.Fatal(err, pi)
	}
	els, err := c.SearchElementInstances(context.Background(), "55", 50)
	if err != nil || len(els) != 2 {
		t.Fatal(err, els)
	}
}
