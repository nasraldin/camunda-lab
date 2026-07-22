package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListIncidentsAPIProvidesKeyAliasesAndNeverBlankRetryKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/incidents/search" {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write([]byte(`{"items":[{
			"incidentKey":"9007199254740993","processInstanceKey":"2","elementId":"pay",
			"errorType":"JOB_NO_RETRIES",
			"errorMessage":"failed bearer leaked-token token=secret123 password=pw",
			"state":"ACTIVE","creationTime":"2026-07-21T12:00:00Z","processDefinitionId":"orders"
		}],"page":{"totalItems":1,"endCursor":null}}`))
	}))
	t.Cleanup(server.Close)

	h := &handler{clusterFactory: &uiTestClusterFactory{baseURL: server.URL + "/v2"}}
	req := httptest.NewRequest(http.MethodGet, "/api/v1/incidents", nil)
	rec := httptest.NewRecorder()
	h.listIncidents(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var body struct {
		OK    bool             `json:"ok"`
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || len(body.Items) != 1 {
		t.Fatalf("body=%#v", body)
	}
	item := body.Items[0]
	if item["key"] != "9007199254740993" || item["id"] != "9007199254740993" ||
		item["processDefinitionId"] != "orders" || item["process"] != "orders" {
		t.Fatalf("aliases missing: %#v", item)
	}
	errorMessage, _ := item["errorMessage"].(string)
	errorAlias, _ := item["error"].(string)
	if errorMessage == "" || errorAlias != errorMessage ||
		strings.Contains(errorMessage, "leaked-token") || strings.Contains(errorMessage, "secret123") {
		t.Fatalf("error fields = %#v", item)
	}

	retry := httptest.NewRequest(http.MethodPost, "/api/v1/incidents/%20/retry", strings.NewReader(`{"confirm":true}`))
	retry.SetPathValue("key", " ")
	retryRec := httptest.NewRecorder()
	h.retryIncident(retryRec, retry)
	if retryRec.Code != http.StatusBadRequest {
		t.Fatalf("blank retry status=%d body=%s", retryRec.Code, retryRec.Body.String())
	}
}
