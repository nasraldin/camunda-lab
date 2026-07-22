package cluster

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetProcessInstanceExactUsesCanonicalSearch(t *testing.T) {
	var gotBody map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/process-instances/search" || r.Method != http.MethodPost {
			t.Fatalf("path=%s method=%s", r.URL.Path, r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(`{"items":[{
			"processInstanceKey":"9007199254740993","processDefinitionKey":"8","processDefinitionId":"orders",
			"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
			"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
		}],"page":{"totalItems":1,"endCursor":null}}`))
	}))
	t.Cleanup(server.Close)

	client := &Client{BaseURL: server.URL + "/v2", HTTPClient: server.Client()}
	item, found, err := client.GetProcessInstanceExact(context.Background(), "9007199254740993")
	if err != nil || !found || item.Key != "9007199254740993" {
		t.Fatalf("item=%+v found=%v err=%v", item, found, err)
	}
	filter := gotBody["filter"].(map[string]any)
	if filter["processInstanceKey"] != "9007199254740993" {
		t.Fatalf("filter=%#v", filter)
	}
}

func TestGetProcessInstanceExactRejectsPartialAndDuplicates(t *testing.T) {
	t.Run("partial", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"items":[{
				"processInstanceKey":"55","processDefinitionKey":"8","processDefinitionId":"orders",
				"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
				"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
			}],"page":{"totalItems":2,"endCursor":"next"}}`))
		}))
		t.Cleanup(server.Close)
		_, _, err := (&Client{BaseURL: server.URL + "/v2", HTTPClient: server.Client()}).
			GetProcessInstanceExact(context.Background(), "55")
		if err == nil {
			t.Fatal("expected partial anomaly")
		}
	})
	t.Run("missing", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"items":[],"page":{"totalItems":0,"endCursor":null}}`))
		}))
		t.Cleanup(server.Close)
		_, found, err := (&Client{BaseURL: server.URL + "/v2", HTTPClient: server.Client()}).
			GetProcessInstanceExact(context.Background(), "55")
		if err != nil || found {
			t.Fatalf("found=%v err=%v", found, err)
		}
	})
}
