package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

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
		})
	})
	mux.HandleFunc("/v2/process-definitions/111/xml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<bpmn:definitions><bpmn:process id="orderProcess"/></bpmn:definitions>`))
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
	inv, err := c.RemoteInventory(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(inv) == 0 || inv[0].Digest == "" {
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
