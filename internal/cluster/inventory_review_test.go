package cluster

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func TestProcessDefinitionOfficial88And89Responses(t *testing.T) {
	tests := []struct {
		file             string
		wantName         *string
		wantVersionTag   *string
		wantHasStart     *bool
		wantStartFormKey *string
	}{
		{
			file:             "process-definitions-8.8.json",
			wantStartFormKey: ptr("9007199254740994"),
		},
		{
			file:     "process-definitions-8.9.json",
			wantName: ptr("Orders"), wantVersionTag: ptr("release-2026-07"),
			wantHasStart: ptr(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.file, func(t *testing.T) {
			page, err := parseProcessSearchResponse(readFixture(t, tt.file))
			if err != nil {
				t.Fatal(err)
			}
			if len(page.Items) != 1 {
				t.Fatalf("items = %+v", page.Items)
			}
			got := page.Items[0]
			if !reflect.DeepEqual(got.Name, tt.wantName) ||
				!reflect.DeepEqual(got.VersionTag, tt.wantVersionTag) ||
				!reflect.DeepEqual(got.HasStartForm, tt.wantHasStart) ||
				!reflect.DeepEqual(got.StartFormKey, tt.wantStartFormKey) {
				t.Fatalf("process definition = %+v", got)
			}
		})
	}
}

func TestInventoryFullFinalPageUsesOfficialTotalItems(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/search") {
			writeJSON(w, `{"items":[{"name":"Orders","resourceName":"orders.bpmn","version":1,"versionTag":null,"processDefinitionId":"orders","tenantId":"<default>","processDefinitionKey":"1","hasStartForm":false}],"page":{"totalItems":1,"hasMoreTotalItems":false,"startCursor":null,"endCursor":null}}`)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, remoteBPMN)
	}))
	defer server.Close()
	got, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
		context.Background(), inventory.Source{Type: "remote"},
		InventoryLimits{PageSize: 1, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if got.Partial || len(got.Warnings) != 0 {
		t.Fatalf("inventory = %+v", got)
	}
}

func TestDecisionDefinitionOfficial89Response(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readFixture(t, "decision-definitions-8.9.json"))
	}))
	defer server.Close()
	result, err := (&Client{BaseURL: server.URL + "/v2"}).SearchDecisionDefinitionsInventory(
		context.Background(), nil, InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items = %+v", result.Items)
	}
	item := result.Items[0]
	if item.DecisionDefinitionID != "risk" || item.Key != "9007199254740996" ||
		item.DecisionRequirementsID != nil || item.DecisionRequirementsKey != nil ||
		item.DecisionRequirementsName != nil || item.DecisionRequirementsVersion != nil {
		t.Fatalf("decision definition = %+v", item)
	}
}

func TestDecisionDefinitionOfficial89NullNameAndStableSorting(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(readFixture(t, "decision-definitions-8.9-null-name.json"))
	}))
	defer server.Close()
	result, err := (&Client{BaseURL: server.URL + "/v2"}).SearchDecisionDefinitionsInventory(
		context.Background(), nil, InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Items) != 2 || result.Items[0].Key != "10" || result.Items[1].Key != "2" {
		t.Fatalf("items = %+v", result.Items)
	}
	if result.Items[0].Name != nil {
		t.Fatalf("null name = %#v", result.Items[0].Name)
	}
	if result.Items[1].Name == nil || *result.Items[1].Name != "Risk Two" {
		t.Fatalf("non-null name = %#v", result.Items[1].Name)
	}
}

func TestRemoteAndLocalMultiProcessDigestParity(t *testing.T) {
	const multi = `<?xml version="1.0"?><definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL">
	  <message id="submitted" name="Submitted"/>
	  <process id="orders"><startEvent id="order-start"/></process>
	  <process id="refunds"><startEvent id="refund-start"><messageEventDefinition messageRef="submitted"/></startEvent></process>
	</definitions>`
	root := t.TempDir()
	writeReviewFile(t, filepath.Join(root, ".camunda.yaml"), `name: multi
paths: {bpmn: workflows, dmn: decisions, forms: forms, tests: tests}
`)
	writeReviewFile(t, filepath.Join(root, "workflows", "multi.bpmn"), multi)
	local, err := inventory.BuildLocal(inventory.LocalRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	var localDigest string
	for _, resource := range local.Resources {
		if resource.ID == "refunds" {
			localDigest = resource.Digest
		}
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/search") {
			writeJSON(w, `{"items":[{"name":"Refunds","resourceName":"multi.bpmn","version":1,"versionTag":null,"processDefinitionId":"refunds","tenantId":"<default>","processDefinitionKey":"42","hasStartForm":false}],"page":{"totalItems":1,"hasMoreTotalItems":false,"startCursor":null,"endCursor":null}}`)
			return
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = io.WriteString(w, multi)
	}))
	defer server.Close()
	remote, err := (&Client{BaseURL: server.URL + "/v2"}).BuildInventory(
		context.Background(), inventory.Source{Type: "remote"},
		InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(remote.Resources) != 1 || localDigest == "" || remote.Resources[0].Digest != localDigest {
		t.Fatalf("local digest %q, remote = %+v", localDigest, remote.Resources)
	}
}

func TestCanonicalProcessSelectionFailuresAreTyped(t *testing.T) {
	tests := []struct {
		name string
		xml  string
	}{
		{
			name: "missing",
			xml:  `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="other"><startEvent id="s"/></process></definitions>`,
		},
		{
			name: "duplicate",
			xml:  `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="orders"><startEvent id="a"/></process><process id="orders"><startEvent id="b"/></process></definitions>`,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := inventory.CanonicalizeProcess([]byte(tt.xml), "orders")
			var selection *inventory.ProcessSelectionError
			if !errors.As(err, &selection) || selection.ProcessID != "orders" {
				t.Fatalf("error = %#v", err)
			}
		})
	}
}

func TestLegacySearchAdaptersPreserveMaxOrderAndExactIncidentFilter(t *testing.T) {
	t.Run("process maximum and native order", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var body struct {
				Page struct {
					Limit int `json:"limit"`
				} `json:"page"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.Page.Limit != 2 {
				t.Fatalf("limit = %d", body.Page.Limit)
			}
			writeJSON(w, `{"items":[
			  {"name":"Ten","resourceName":"ten.bpmn","version":1,"versionTag":null,"processDefinitionId":"ten","tenantId":"<default>","processDefinitionKey":"10","hasStartForm":false},
			  {"name":"Two","resourceName":"two.bpmn","version":1,"versionTag":null,"processDefinitionId":"two","tenantId":"<default>","processDefinitionKey":"2","hasStartForm":false},
			  {"name":"One","resourceName":"one.bpmn","version":1,"versionTag":null,"processDefinitionId":"one","tenantId":"<default>","processDefinitionKey":"1","hasStartForm":false}
			],"page":{"totalItems":3,"hasMoreTotalItems":false,"startCursor":null,"endCursor":"more"}}`)
		}))
		defer server.Close()
		got, err := (&Client{BaseURL: server.URL + "/v2"}).SearchProcessDefinitions(context.Background(), 2)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("items = %+v", got)
		}
		if keys := []string{got[0].Key, got[1].Key}; !reflect.DeepEqual(keys, []string{"10", "2"}) {
			t.Fatalf("keys = %v", keys)
		}
	})

	t.Run("trace native chronology", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeJSON(w, `{"items":[
			  {"processDefinitionId":"orders","startDate":"2026-07-21T10:00:00Z","endDate":null,"elementId":"first","elementName":"First","type":"START_EVENT","state":"COMPLETED","hasIncident":false,"tenantId":"<default>","elementInstanceKey":"2","processInstanceKey":"1","rootProcessInstanceKey":null,"processDefinitionKey":"9","incidentKey":null},
			  {"processDefinitionId":"orders","startDate":"2026-07-21T10:01:00Z","endDate":null,"elementId":"second","elementName":"Second","type":"SERVICE_TASK","state":"ACTIVE","hasIncident":false,"tenantId":"<default>","elementInstanceKey":"10","processInstanceKey":"1","rootProcessInstanceKey":null,"processDefinitionKey":"9","incidentKey":null}
			],"page":{"totalItems":2,"hasMoreTotalItems":false,"startCursor":null,"endCursor":null}}`)
		}))
		defer server.Close()
		got, err := (&Client{BaseURL: server.URL + "/v2"}).SearchElementInstances(context.Background(), "1", 2)
		if err != nil {
			t.Fatal(err)
		}
		if keys := []string{got[0].Key, got[1].Key}; !reflect.DeepEqual(keys, []string{"2", "10"}) {
			t.Fatalf("keys = %v", keys)
		}
	})

	t.Run("active incident filter has no broadening fallback", func(t *testing.T) {
		calls := 0
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls++
			var body struct {
				Filter map[string]any `json:"filter"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			if body.Filter["state"] != "ACTIVE" {
				t.Fatalf("filter = %v", body.Filter)
			}
			http.Error(w, `{"message":"unsupported active filter"}`, http.StatusBadRequest)
		}))
		defer server.Close()
		_, err := (&Client{BaseURL: server.URL + "/v2"}).SearchIncidents(context.Background(), 10)
		var apiError *APIError
		if !errors.As(err, &apiError) || apiError.StatusCode != http.StatusBadRequest {
			t.Fatalf("error = %v", err)
		}
		if calls != 1 {
			t.Fatalf("calls = %d", calls)
		}
	})
}

func TestRuntimeOfficialFieldsAndTimestampsAreRetained(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v2/process-instances/search":
			writeJSON(w, `{"items":[{"processDefinitionId":"orders","processDefinitionName":null,"processDefinitionVersion":3,"processDefinitionVersionTag":null,"startDate":"2026-07-21T12:00:00+02:00","endDate":"2026-07-21T12:05:00+02:00","state":"COMPLETED","hasIncident":false,"tenantId":"tenant-a","processInstanceKey":"9007199254740993","processDefinitionKey":"8","parentProcessInstanceKey":null,"parentElementInstanceKey":null,"rootProcessInstanceKey":"9007199254740993","tags":["vip"],"businessId":"order-7"}],"page":{"totalItems":1,"hasMoreTotalItems":false,"startCursor":null,"endCursor":null}}`)
		case "/v2/process-instances/9007199254740993":
			writeJSON(w, `{"processDefinitionId":"orders","processDefinitionName":null,"processDefinitionVersion":3,"processDefinitionVersionTag":null,"startDate":"2026-07-21T12:00:00+02:00","endDate":"2026-07-21T12:05:00+02:00","state":"COMPLETED","hasIncident":false,"tenantId":"tenant-a","processInstanceKey":"9007199254740993","processDefinitionKey":"8","parentProcessInstanceKey":null,"parentElementInstanceKey":null,"rootProcessInstanceKey":"9007199254740993","tags":["vip"],"businessId":"order-7"}`)
		case "/v2/jobs/search":
			writeJSON(w, `{"items":[{"customHeaders":{"region":"eu"},"deadline":"2026-07-21T12:01:00+02:00","deniedReason":null,"elementId":"ship","elementInstanceKey":"7","endTime":"2026-07-21T12:03:00+02:00","errorCode":null,"errorMessage":null,"hasFailedWithRetriesLeft":false,"isDenied":null,"jobKey":"9007199254740994","kind":"BPMN_ELEMENT","listenerEventType":"UNSPECIFIED","processDefinitionId":"orders","processDefinitionKey":"8","processInstanceKey":"9007199254740993","rootProcessInstanceKey":"9007199254740993","retries":3,"state":"COMPLETED","tenantId":"tenant-a","type":"shipping","worker":"worker-a","creationTime":"2026-07-21T11:59:00+02:00","lastUpdateTime":"2026-07-21T12:03:00+02:00"}],"page":{"totalItems":1,"hasMoreTotalItems":false,"startCursor":null,"endCursor":null}}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL + "/v2"}
	limits := InventoryLimits{PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20}
	instances, err := client.SearchProcessInstancesInventory(context.Background(), nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	instance := instances.Items[0]
	if instance.StartDate != "2026-07-21T10:00:00Z" || instance.EndDate != "2026-07-21T10:05:00Z" ||
		instance.RootProcessInstanceKey != "9007199254740993" || instance.BusinessID != "order-7" {
		t.Fatalf("instance = %+v", instance)
	}
	point, err := client.GetProcessInstance(context.Background(), "9007199254740993")
	if err != nil {
		t.Fatal(err)
	}
	if point.StartDate != instance.StartDate || point.EndDate != instance.EndDate ||
		point.RootProcessInstanceKey != instance.RootProcessInstanceKey {
		t.Fatalf("point instance = %+v, search instance = %+v", point, instance)
	}
	jobs, err := client.SearchJobsInventory(context.Background(), nil, limits)
	if err != nil {
		t.Fatal(err)
	}
	job := jobs.Items[0]
	if job.Deadline != "2026-07-21T10:01:00Z" || job.EndTime != "2026-07-21T10:03:00Z" ||
		job.CreationTime != "2026-07-21T09:59:00Z" || job.LastUpdateTime != "2026-07-21T10:03:00Z" ||
		job.RootProcessInstanceKey != "9007199254740993" {
		t.Fatalf("job = %+v", job)
	}
}

func TestInventoryJobsPreserveEndTimeDeadlineWhenCreationLastUpdateNull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/jobs/search" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, `{"items":[{
			"customHeaders":{},"deadline":"2026-07-21T12:01:00+02:00","deniedReason":null,
			"elementId":"ship","elementInstanceKey":"7","endTime":"2026-07-21T12:03:00+02:00",
			"errorCode":null,"errorMessage":null,"hasFailedWithRetriesLeft":false,"isDenied":null,
			"jobKey":"9007199254740994","kind":"BPMN_ELEMENT","listenerEventType":"UNSPECIFIED",
			"processDefinitionId":"orders","processDefinitionKey":"8","processInstanceKey":"5",
			"rootProcessInstanceKey":null,"retries":0,"state":"COMPLETED","tenantId":"<default>",
			"type":"shipping","worker":"worker-a","creationTime":null,"lastUpdateTime":null
		}],"page":{"totalItems":1,"endCursor":null}}`)
	}))
	defer server.Close()
	client := &Client{BaseURL: server.URL + "/v2"}
	jobs, err := client.SearchJobsInventory(context.Background(), nil, InventoryLimits{
		PageSize: 10, MaxPages: 2, MaxItems: 10, MaxBodyBytes: 1 << 20,
	})
	if err != nil || len(jobs.Items) != 1 {
		t.Fatal(err, jobs)
	}
	job := jobs.Items[0]
	if job.CreationTime != "" || job.LastUpdateTime != "" {
		t.Fatalf("expected null creation/lastUpdate, got %+v", job)
	}
	if job.EndTime != "2026-07-21T10:03:00Z" || job.Deadline != "2026-07-21T10:01:00Z" ||
		job.Key != "9007199254740994" {
		t.Fatalf("expected preserved endTime/deadline, got %+v", job)
	}
}

func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func writeReviewFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func ptr[T any](value T) *T { return &value }
