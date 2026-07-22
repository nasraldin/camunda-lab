package incidents

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
)

type incidentFactory struct {
	client   *cluster.Client
	resolved env.Resolved
	calls    atomic.Int32
}

func (f *incidentFactory) Client(context.Context, string, string) (*cluster.Client, env.Resolved, error) {
	f.calls.Add(1)
	return f.client, f.resolved, nil
}

func testIncidentService(t *testing.T, handler http.Handler) (*Service, *incidentFactory) {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	factory := &incidentFactory{
		client: &cluster.Client{BaseURL: server.URL + "/v2", HTTPClient: server.Client(), Kind: cluster.ClientRemote},
		resolved: env.Resolved{
			Profile: env.Profile{Name: "prod", Kind: "remote", Endpoints: map[string]string{"orchestration": server.URL}},
			Source:  env.ProfileSourceProject,
		},
	}
	return NewService(factory), factory
}

func incidentEnvelope(items string, total int, cursor any) string {
	body, _ := json.Marshal(map[string]any{
		"items": json.RawMessage(items),
		"page":  map[string]any{"totalItems": total, "endCursor": cursor},
	})
	return string(body)
}

func TestIncidentListPaginatesFiltersAndNormalizes(t *testing.T) {
	var requests []map[string]any
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		requests = append(requests, body)
		page := body["page"].(map[string]any)
		if page["after"] == nil {
			_, _ = w.Write([]byte(incidentEnvelope(`[{
				"incidentKey":"20","processInstanceKey":"2","elementId":"pay",
				"errorType":"JOB_NO_RETRIES","errorMessage":"timeout","state":" active ",
				"creationTime":"2026-07-21T14:00:00+02:00","processDefinitionId":"orders"
			}]`, 2, "next")))
			return
		}
		_, _ = w.Write([]byte(incidentEnvelope(`[{
			"incidentKey":"10","processInstanceKey":"1","elementId":"ship",
			"errorType":"JOB_NO_RETRIES","errorMessage":"down","state":"ACTIVE",
			"creationTime":"2026-07-21T11:00:00Z","processDefinitionId":"orders"
		}]`, 2, nil)))
	})
	service, _ := testIncidentService(t, mux)

	result, err := service.List(context.Background(), ListRequest{
		Environment: "prod", ProjectRoot: "/project", Limit: 10, PageSize: 1, MaxPages: 2,
		Filter: ListFilter{
			State: " active ", IncidentKey: "20", ProcessInstanceKey: "2",
			ProcessDefinitionID: "orders", ElementID: "pay",
			ErrorType: "JOB_NO_RETRIES", ErrorMessage: "timeout",
			CreatedAfter: "2026-07-21T12:00:00+02:00", CreatedBefore: "2026-07-21T18:00:00+02:00",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Complete || result.Partial || result.Status != StatusCompleted ||
		result.Environment.Profile.Name != "prod" || result.Source.Environment != "prod" ||
		len(result.Incidents) != 2 || result.Incidents[0].Key != "10" ||
		result.Incidents[1].CreationTime != "2026-07-21T12:00:00Z" {
		t.Fatalf("unexpected result: %+v", result)
	}
	filter := requests[0]["filter"].(map[string]any)
	creationTime, _ := filter["creationTime"].(map[string]any)
	if filter["state"] != "ACTIVE" || filter["processInstanceKey"] != "2" ||
		filter["incidentKey"] != "20" || filter["rootProcessInstanceKey"] != nil ||
		filter["processDefinitionId"] != "orders" || filter["elementId"] != "pay" ||
		filter["errorMessage"] != "timeout" || filter["creationTimeFrom"] != nil ||
		filter["creationTimeTo"] != nil || creationTime["$gte"] != "2026-07-21T10:00:00Z" ||
		creationTime["$lte"] != "2026-07-21T16:00:00Z" {
		t.Fatalf("filter was not official IncidentFilter shape: %#v", filter)
	}
	if requests[1]["page"].(map[string]any)["after"] != "next" {
		t.Fatalf("pagination cursor not propagated: %#v", requests)
	}
	text := FormatText(result)
	encoded, err := FormatJSON(result)
	if err != nil || !strings.Contains(text, "JOB_NO_RETRIES") ||
		!strings.Contains(string(encoded), `"warnings": []`) ||
		!strings.Contains(string(encoded), `"incidents": [`) {
		t.Fatalf("formats text=%q json=%s err=%v", text, encoded, err)
	}
}

func TestIncidentListRejectsInvalidLimitAndKeyWithoutRequest(t *testing.T) {
	var requests atomic.Int32
	service, _ := testIncidentService(t, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		requests.Add(1)
	}))
	for _, request := range []ListRequest{
		{Limit: 501},
		{Limit: 1, Filter: ListFilter{ProcessInstanceKey: " 22 "}},
		{Limit: 1, Filter: ListFilter{CreatedBefore: "yesterday"}},
		{Limit: 1, Filter: ListFilter{
			CreatedAfter: "2026-07-21T12:00:00.1Z", CreatedBefore: "2026-07-21T12:00:00Z",
		}},
		{Limit: 1, Filter: ListFilter{RootProcessInstanceKey: "2"}},
	} {
		if _, err := service.List(context.Background(), request); err == nil {
			t.Fatalf("List(%+v) succeeded", request)
		}
	}
	if requests.Load() != 0 {
		t.Fatalf("invalid requests reached cluster %d times", requests.Load())
	}
}

func TestIncidentShowEnrichesCanonicalContextAndWarnsWhenOptionalMissing(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/v2/incidents/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(incidentEnvelope(`[{
			"incidentKey":"99","processInstanceKey":"55","elementInstanceKey":"66",
			"processDefinitionKey":"77","processDefinitionId":"orders","elementId":"pay",
			"errorType":"JOB_NO_RETRIES","errorMessage":"timeout","state":"ACTIVE",
			"creationTime":"2026-07-21T12:00:00Z"
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/process-instances/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(incidentEnvelope(`[{
			"processInstanceKey":"55","processDefinitionKey":"77","processDefinitionId":"orders",
			"processDefinitionVersion":3,"state":"ACTIVE","hasIncident":true,"tags":[]
		}]`, 1, nil)))
	})
	mux.HandleFunc("/v2/element-instances/search", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"message":"optional index unavailable"}`, http.StatusServiceUnavailable)
	})
	mux.HandleFunc("/v2/process-definitions/search", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(incidentEnvelope(`[{
			"processDefinitionKey":"77","processDefinitionId":"orders","name":"Orders",
			"version":3,"resourceName":"orders.bpmn"
		}]`, 1, nil)))
	})
	service, _ := testIncidentService(t, mux)
	got, err := service.Show(context.Background(), "prod", "/project", "99")
	if err != nil {
		t.Fatal(err)
	}
	if got.ProcessInstance == nil || got.ProcessDefinition == nil || got.ElementInstance != nil ||
		len(got.Warnings) != 1 || got.Warnings[0].Capability != "element-instance" ||
		got.Status != StatusPartial || got.Complete || !strings.Contains(FormatIncidentText(got), "Incident 99") {
		t.Fatalf("unexpected detail: %+v", got)
	}
}

func TestIncidentShowReturnsTypedNotFound(t *testing.T) {
	service, _ := testIncidentService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(incidentEnvelope(`[]`, 0, nil)))
	}))
	_, err := service.Show(context.Background(), "", "", "99")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) || notFound.Key != "99" {
		t.Fatalf("error = %T %v", err, err)
	}
}

func TestIncidentResolveDryRunDoesNotMutate(t *testing.T) {
	var mutations atomic.Int32
	service, _ := testIncidentService(t, incidentResolutionHandler(t, &mutations, nil))
	result, err := service.ResolveWithOptions(context.Background(), ResolveRequest{Key: "99", DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 0 || result.Outcome.Kind != OutcomeDryRun || result.Policy.Outcome != PolicyWouldResolve {
		t.Fatalf("result=%+v mutations=%d", result, mutations.Load())
	}
}

func TestIncidentResolveRevalidatesAndRefreshesOnce(t *testing.T) {
	var mutations atomic.Int32
	var searches atomic.Int32
	service, _ := testIncidentService(t, incidentResolutionHandler(t, &mutations, &searches))
	result, err := service.Resolve(context.Background(), "prod", "/project", "99")
	if err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 1 || searches.Load() != 2 || result.Outcome.Kind != OutcomeResolved ||
		result.Outcome.Incident == nil || result.Outcome.Incident.State != "RESOLVED" {
		t.Fatalf("result=%+v mutations=%d searches=%d", result, mutations.Load(), searches.Load())
	}
}

func TestIncidentResolveAlreadyResolvedIsIdempotent(t *testing.T) {
	var mutations atomic.Int32
	service, _ := testIncidentService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/incidents/search":
			_, _ = w.Write([]byte(incidentEnvelope(`[{
				"incidentKey":"99","state":"RESOLVED","creationTime":"2026-07-21T12:00:00Z"
			}]`, 1, nil)))
		case "/v2/incidents/99/resolution":
			mutations.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	result, err := service.Resolve(context.Background(), "", "", "99")
	if err != nil {
		t.Fatal(err)
	}
	if mutations.Load() != 0 || result.Outcome.Kind != OutcomeAlreadyResolved ||
		result.Policy.Outcome != PolicyAlreadyResolved {
		t.Fatalf("result=%+v mutations=%d", result, mutations.Load())
	}
}

func TestIncidentResolveConflictPreservesStatusAndDoesNotRetry(t *testing.T) {
	var mutations atomic.Int32
	mux := incidentResolutionHandler(t, &mutations, nil)
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/resolution") {
			mutations.Add(1)
			http.Error(w, `{"message":"incident changed state"}`, http.StatusConflict)
			return
		}
		mux.ServeHTTP(w, r)
	})
	service, _ := testIncidentService(t, wrapped)
	_, err := service.Resolve(context.Background(), "", "", "99")
	var apiErr *cluster.APIError
	if !errors.As(err, &apiErr) || apiErr.StatusCode != http.StatusConflict ||
		!strings.Contains(err.Error(), "incident changed state") || mutations.Load() != 1 {
		t.Fatalf("error=%T %v mutations=%d", err, err, mutations.Load())
	}
}

func TestIncidentResolveRefreshFailureIsExplicitlyPartial(t *testing.T) {
	var mutations atomic.Int32
	var searches atomic.Int32
	handler := incidentResolutionHandler(t, &mutations, &searches)
	service, _ := testIncidentService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/incidents/search" && searches.Load() > 0 {
			searches.Add(1)
			http.Error(w, `{"message":"refresh unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		handler.ServeHTTP(w, r)
	}))
	result, err := service.Resolve(context.Background(), "", "", "99")
	if err != nil {
		t.Fatal(err)
	}
	if !result.Partial || result.Complete || result.Status != StatusPartial ||
		result.Outcome.Kind != OutcomeResolvedUnverified || len(result.Warnings) == 0 {
		t.Fatalf("unexpected partial result: %+v", result)
	}
	if len(result.Incidents) != 1 || result.Incidents[0].State == "ACTIVE" ||
		result.Outcome.Incident == nil || result.Outcome.Incident.State == "ACTIVE" ||
		strings.Contains(FormatText(result), " ACTIVE") ||
		strings.Contains(FormatJSONMust(t, result), `"state": "ACTIVE"`) {
		t.Fatalf("resolved-unverified still presents ACTIVE truth: %+v text=%q", result, FormatText(result))
	}
}

func TestIncidentErrorMessageIsRedactedInNormalizeAndFormats(t *testing.T) {
	service, _ := testIncidentService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(incidentEnvelope(`[{
			"incidentKey":"99","processInstanceKey":"55","elementId":"pay",
			"errorType":"JOB_NO_RETRIES",
			"errorMessage":"auth failed bearer super-secret token=abc123 password=hunter2",
			"state":"ACTIVE","creationTime":"2026-07-21T12:00:00Z","processDefinitionId":"orders"
		}]`, 1, nil)))
	}))
	result, err := service.List(context.Background(), ListRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	message := result.Incidents[0].ErrorMessage
	if strings.Contains(message, "super-secret") || strings.Contains(message, "abc123") ||
		strings.Contains(message, "hunter2") || !strings.Contains(message, "[REDACTED]") {
		t.Fatalf("errorMessage was not redacted: %q", message)
	}
	text := FormatText(result)
	encoded, err := FormatJSON(result)
	if err != nil || strings.Contains(text, "super-secret") || strings.Contains(string(encoded), "abc123") {
		t.Fatalf("formats leaked credentials text=%q json=%s err=%v", text, encoded, err)
	}
	item := FormatAPIItem(result.Incidents[0])
	if item["errorMessage"] != message || item["error"] != message ||
		item["key"] != "99" || item["id"] != "99" ||
		item["processDefinitionId"] != "orders" || item["process"] != "orders" {
		t.Fatalf("API item adapter = %#v", item)
	}
}

func TestIncidentResolveCancellationAndInvalidKeyDoNotMutate(t *testing.T) {
	var mutations atomic.Int32
	service, _ := testIncidentService(t, incidentResolutionHandler(t, &mutations, nil))
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := service.Resolve(cancelled, "", "", "99"); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled error = %v", err)
	}
	if _, err := service.Resolve(context.Background(), "", "", "../99"); err == nil {
		t.Fatal("invalid key succeeded")
	}
	if mutations.Load() != 0 {
		t.Fatalf("mutation count = %d", mutations.Load())
	}
}

func TestIncidentOperateLinkEscapesValues(t *testing.T) {
	link, err := OperateLink("https://operate.example/base?tenant=a", "12/3", "9 9")
	if err != nil {
		t.Fatal(err)
	}
	if link != "https://operate.example/base/processes/12%2F3?incident=9+9&tenant=a" {
		t.Fatalf("link = %q", link)
	}
}

func FormatJSONMust(t *testing.T, result Result) string {
	t.Helper()
	encoded, err := FormatJSON(result)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}

func incidentResolutionHandler(t *testing.T, mutations, searches *atomic.Int32) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/incidents/search":
			count := int32(0)
			if searches != nil {
				count = searches.Add(1)
			}
			state := "ACTIVE"
			if count > 1 {
				state = "RESOLVED"
			}
			_, _ = w.Write([]byte(incidentEnvelope(`[{
				"incidentKey":"99","processInstanceKey":"55","elementId":"pay",
				"errorType":"JOB_NO_RETRIES","errorMessage":"timeout","state":"`+state+`",
				"creationTime":"2026-07-21T12:00:00Z"
			}]`, 1, nil)))
		case r.URL.Path == "/v2/incidents/99/resolution":
			mutations.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
}
