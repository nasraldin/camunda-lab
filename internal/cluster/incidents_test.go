package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestIncidentGetUsesExactFilterWithoutCompatibilityFallback(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var body struct {
			Filter map[string]any `json:"filter"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Filter["incidentKey"] != "9007199254740993" {
			t.Errorf("filter = %#v", body.Filter)
		}
		http.Error(w, `{"message":"incident key filter unsupported"}`, http.StatusBadRequest)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}

	_, found, err := client.GetIncident(context.Background(), "9007199254740993")
	var apiErr *APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusBadRequest ||
		!strings.Contains(err.Error(), "incident key filter unsupported") || found || calls.Load() != 1 {
		t.Fatalf("found=%v error=%T %v calls=%d", found, err, err, calls.Load())
	}
}

func TestIncidentGetRejectsMismatchedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"items":[{
				"incidentKey":"2","state":"ACTIVE","creationTime":"2026-07-21T12:00:00Z"
			}],
			"page":{"totalItems":1,"endCursor":null}
		}`))
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}

	_, _, err := client.GetIncident(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "mismatched incident key") {
		t.Fatalf("error = %v", err)
	}
}

func TestIncidentGetRejectsPartialExactSearchAsAnomaly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{
			"items":[],
			"page":{"totalItems":1,"hasMoreTotalItems":true,"endCursor":null}
		}`))
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}

	_, found, err := client.GetIncident(context.Background(), "1")
	if err == nil || !strings.Contains(err.Error(), "partial") || found {
		t.Fatalf("found=%v error=%v", found, err)
	}
}

func TestIncidentResolvePreserves404And5xxStatusAndMessage(t *testing.T) {
	for _, status := range []int{http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"message":"resolution unavailable"}`, status)
			}))
			defer server.Close()
			client := &Client{BaseURL: server.URL, HTTPClient: server.Client()}

			err := client.ResolveIncident(context.Background(), "99")
			var apiErr *APIError
			if !errors.As(err, &apiErr) || apiErr.StatusCode != status ||
				!strings.Contains(err.Error(), "resolution unavailable") {
				t.Fatalf("error = %T %v", err, err)
			}
		})
	}
}
