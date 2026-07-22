package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

const remoteBPMN = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="orders"><startEvent id="start"/></process></definitions>`

func TestInventoryPaginatesDeduplicatesAndSortsDeterministically(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v2/process-definitions/search":
			var body struct {
				Page struct {
					After string `json:"after"`
				} `json:"page"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			calls.Add(1)
			if body.Page.After == "" {
				writeJSON(w, `{"items":[
					{"processDefinitionKey":"9007199254740993","processDefinitionId":"orders","name":"Orders","version":2,"resourceName":"orders.bpmn"},
					{"processDefinitionKey":"10","processDefinitionId":"alpha","name":"Alpha","version":1,"resourceName":"alpha.bpmn"}
				],"page":{"totalItems":4,"endCursor":"next"}}`)
				return
			}
			if body.Page.After != "next" {
				t.Fatalf("after = %q", body.Page.After)
			}
			writeJSON(w, `{"items":[
				{"processDefinitionKey":"10","processDefinitionId":"alpha","name":"Alpha","version":1,"resourceName":"alpha.bpmn"},
				{"processDefinitionKey":"11","processDefinitionId":"orders","name":"Orders","version":1,"resourceName":"orders.bpmn"}
			],"page":{"totalItems":2,"endCursor":null}}`)
		case strings.HasSuffix(r.URL.Path, "/xml"):
			processID := "orders"
			if strings.Contains(r.URL.Path, "/10/") {
				processID = "alpha"
			}
			xmlBody := `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="` +
				processID + `"><startEvent id="start"/></process></definitions>`
			writeJSON(w, strconv.Quote(xmlBody))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	got, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
		context.Background(),
		inventory.Source{Type: "remote", Environment: "prod"},
		InventoryLimits{PageSize: 3, MaxPages: 4, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Partial || len(got.Warnings) != 0 || len(got.Resources) != 3 || calls.Load() != 2 {
		t.Fatalf("inventory = %+v, calls = %d", got, calls.Load())
	}
	if got.Resources[0].ID != "alpha" || got.Resources[1].Version != 1 || got.Resources[2].Version != 2 {
		t.Fatalf("unstable inventory order: %+v", got.Resources)
	}
	if got.Resources[2].Key != "9007199254740993" {
		t.Fatalf("large key lost precision: %+v", got.Resources[2])
	}
	if err := got.ValidateComparable(); err != nil {
		t.Fatal(err)
	}
}

func TestInventoryPaginationAnomaliesAreExplicitlyPartial(t *testing.T) {
	tests := []struct {
		name  string
		pages []string
		limit InventoryLimits
		want  string
	}{
		{
			name: "repeated cursor",
			pages: []string{
				`{"items":[{"processDefinitionKey":"1","processDefinitionId":"orders","version":1}],"page":{"totalItems":2,"endCursor":"same"}}`,
				`{"items":[{"processDefinitionKey":"2","processDefinitionId":"orders","version":2}],"page":{"totalItems":2,"endCursor":"same"}}`,
			},
			limit: InventoryLimits{PageSize: 1, MaxPages: 4, MaxItems: 10, MaxBodyBytes: 1 << 20},
			want:  "cursor",
		},
		{
			name: "missing cursor on full page",
			pages: []string{
				`{"items":[{"processDefinitionKey":"1","processDefinitionId":"orders","version":1}],"page":{"totalItems":2,"endCursor":null}}`,
			},
			limit: InventoryLimits{PageSize: 1, MaxPages: 4, MaxItems: 10, MaxBodyBytes: 1 << 20},
			want:  "missing",
		},
		{
			name: "page bound",
			pages: []string{
				`{"items":[{"processDefinitionKey":"1","processDefinitionId":"orders","version":1}],"page":{"totalItems":2,"endCursor":"more"}}`,
			},
			limit: InventoryLimits{PageSize: 1, MaxPages: 1, MaxItems: 10, MaxBodyBytes: 1 << 20},
			want:  "page limit",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var page atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/xml") {
					writeJSON(w, strconv.Quote(remoteBPMN))
					return
				}
				index := int(page.Add(1) - 1)
				if index >= len(tt.pages) {
					index = len(tt.pages) - 1
				}
				writeJSON(w, tt.pages[index])
			}))
			defer server.Close()
			got, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
				context.Background(), inventory.Source{Type: "remote"}, tt.limit,
			)
			if err != nil {
				t.Fatal(err)
			}
			if !got.Partial || len(got.Warnings) == 0 ||
				!strings.Contains(strings.ToLower(got.Warnings[0].Message), tt.want) {
				t.Fatalf("inventory = %+v, want warning containing %q", got, tt.want)
			}
			if err := got.ValidateComparable(); err == nil {
				t.Fatal("partial inventory is comparable")
			}
		})
	}
}

func TestRemoteInventoryXMLFailureIsError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/search") {
			writeJSON(w, `{"items":[{"processDefinitionKey":"1","processDefinitionId":"orders","version":1}],"page":{"totalItems":1,"endCursor":null}}`)
			return
		}
		http.Error(w, "private stack secret-value", http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
		context.Background(), inventory.Source{Type: "remote"},
		InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err == nil || !strings.Contains(err.Error(), "process definition XML") {
		t.Fatalf("error = %v", err)
	}
	if strings.Contains(err.Error(), "secret-value") {
		t.Fatalf("response body leaked: %v", err)
	}
}

func TestInventoryStrictResponsesBoundsAndCancellation(t *testing.T) {
	for _, payload := range []string{
		`{"items":[],"items":[],"page":{"totalItems":0}}`,
		`{"items":[],"page":{"totalItems":0}} {}`,
		`{"items":[{"processDefinitionKey":1.5,"processDefinitionId":"orders","version":1}],"page":{"totalItems":1}}`,
		`{"items":[],"page":{"totalItems":0},"unknown":true}`,
	} {
		t.Run(payload, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				writeJSON(w, payload)
			}))
			defer server.Close()
			_, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
				context.Background(), inventory.Source{},
				InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
			)
			if err == nil {
				t.Fatalf("accepted %s", payload)
			}
		})
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-r.Context().Done()
	}))
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
		ctx, inventory.Source{}, InventoryLimits{},
	)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}

func TestInventoryResponseBodyBound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, `{"items":[],"page":{"totalItems":0},"padding":"`+strings.Repeat("x", 200)+`"}`)
	}))
	defer server.Close()
	_, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
		context.Background(), inventory.Source{},
		InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 64},
	)
	if err == nil || !strings.Contains(err.Error(), "size limit") {
		t.Fatalf("error = %v", err)
	}
}

func TestInventoryTypedIncidentSearchUsesSharedPagination(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		call := calls.Add(1)
		if call == 1 {
			writeJSON(w, `{"items":[{"incidentKey":"9007199254740993","processInstanceKey":"2","elementId":"task","state":"active","creationTime":"2026-07-21T12:00:00+02:00"}],"page":{"totalItems":1,"endCursor":"next"}}`)
			return
		}
		writeJSON(w, `{"items":[],"page":{"totalItems":0,"endCursor":null}}`)
	}))
	defer server.Close()
	got, err := (&Client{BaseURL: server.URL + "/v2"}).SearchIncidentsInventory(
		context.Background(), map[string]any{"state": "ACTIVE"},
		InventoryLimits{PageSize: 2, MaxPages: 3, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Partial || len(got.Items) != 1 || got.Items[0].Key != "9007199254740993" ||
		got.Items[0].State != "ACTIVE" || got.Items[0].CreationTime != "2026-07-21T10:00:00Z" ||
		calls.Load() != 2 {
		t.Fatalf("result = %+v, calls = %d", got, calls.Load())
	}
}

func TestInventoryTypedDefinitionRuntimeAndTopologySurfaces(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/decision-definitions/search":
			writeJSON(w, `{"items":[{"decisionDefinitionKey":"3","decisionDefinitionId":"risk","name":"Risk","version":1,"decisionRequirementsId":null,"decisionRequirementsKey":null,"decisionRequirementsName":null,"decisionRequirementsVersion":null,"tenantId":"<default>"}],"page":{"totalItems":1,"endCursor":null}}`)
		case "/v2/process-instances/search":
			writeJSON(w, `{"items":[{"processInstanceKey":"5","processDefinitionId":"orders","processDefinitionName":null,"processDefinitionVersion":1,"processDefinitionVersionTag":null,"processDefinitionKey":"8","startDate":"2026-07-21T10:00:00Z","endDate":null,"state":"ACTIVE","hasIncident":false,"tenantId":"<default>","parentProcessInstanceKey":null,"parentElementInstanceKey":null,"rootProcessInstanceKey":null,"tags":[],"businessId":null}],"page":{"totalItems":1,"endCursor":null}}`)
		case "/v2/jobs/search":
			writeJSON(w, `{"items":[{"customHeaders":{},"deadline":null,"deniedReason":null,"elementId":"ship","elementInstanceKey":"7","endTime":null,"errorCode":null,"errorMessage":null,"hasFailedWithRetriesLeft":false,"isDenied":null,"jobKey":"6","kind":"BPMN_ELEMENT","listenerEventType":"UNSPECIFIED","processDefinitionId":"orders","processDefinitionKey":"8","processInstanceKey":"5","rootProcessInstanceKey":null,"retries":3,"state":"CREATED","tenantId":"<default>","type":"shipping","worker":"","creationTime":null,"lastUpdateTime":null}],"page":{"totalItems":1,"endCursor":null}}`)
		case "/v2/topology":
			writeJSON(w, `{"brokers":[{"nodeId":0,"host":"broker","port":26501,"partitions":[{"partitionId":1,"role":"LEADER","health":"HEALTHY"}]}],"clusterSize":1,"partitionsCount":1,"replicationFactor":1,"gatewayVersion":"8.9.0"}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL + "/v2"}
	ctx := context.Background()
	limits := InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20}

	decisions, err := client.SearchDecisionDefinitionsInventory(ctx, nil, limits)
	if err != nil || len(decisions.Items) != 1 || decisions.Items[0].DecisionDefinitionID != "risk" {
		t.Fatal(err, decisions)
	}
	instances, err := client.SearchProcessInstancesInventory(ctx, nil, limits)
	if err != nil || len(instances.Items) != 1 || instances.Items[0].Key != "5" {
		t.Fatal(err, instances)
	}
	jobs, err := client.SearchJobsInventory(ctx, nil, limits)
	if err != nil || len(jobs.Items) != 1 || jobs.Items[0].Key != "6" {
		t.Fatal(err, jobs)
	}
	if jobs.Items[0].CreationTime != "" || jobs.Items[0].LastUpdateTime != "" ||
		jobs.Items[0].EndTime != "" || jobs.Items[0].Deadline != "" {
		t.Fatalf("expected empty nullable job timestamps, got %+v", jobs.Items[0])
	}
	topology, err := client.GetTopologyInventory(ctx, limits.MaxBodyBytes)
	if err != nil || len(topology.Brokers) != 1 || topology.Brokers[0].Partitions[0].Role != "LEADER" {
		t.Fatal(err, topology)
	}
}

func TestInventoryFactoryRemoteLocalParityAndResolvedMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/search") {
			writeJSON(w, `{"items":[{"processDefinitionKey":"1","processDefinitionId":"orders","version":1,"resourceName":"orders.bpmn"}],"page":{"totalItems":1,"endCursor":null}}`)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(remoteBPMN))
	}))
	defer server.Close()

	build := func(kind ClientKind, name string) inventory.Inventory {
		factory := inventoryFactoryStub{
			client:   &Client{BaseURL: server.URL + "/v2", Kind: kind},
			resolved: env.Resolved{Profile: env.Profile{Name: name, Kind: string(kind)}},
		}
		got, resolved, err := BuildClusterInventory(context.Background(), factory, InventoryRequest{
			Environment: name, ProjectRoot: "/project",
			Limits: InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
		})
		if err != nil {
			t.Fatal(err)
		}
		if resolved.Profile.Name != name || len(got.Resources) != 1 ||
			got.Resources[0].Source.Environment != name ||
			got.Resources[0].Source.Type != string(kind) {
			t.Fatalf("inventory/resolution mismatch: %+v %+v", got, resolved)
		}
		return got
	}
	local := build(ClientLocal, "lab")
	remote := build(ClientRemote, "prod")
	local.Resources[0].Source = inventory.Source{}
	remote.Resources[0].Source = inventory.Source{}
	if local.Resources[0] != remote.Resources[0] {
		t.Fatalf("local/remote normalization differs: %+v != %+v", local.Resources[0], remote.Resources[0])
	}
}

type inventoryFactoryStub struct {
	client   *Client
	resolved env.Resolved
	err      error
}

func (s inventoryFactoryStub) Client(context.Context, string, string) (*Client, env.Resolved, error) {
	return s.client, s.resolved, s.err
}

func writeJSON(w http.ResponseWriter, body string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}
