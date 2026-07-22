package plan

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

func resource(kind inventory.Kind, id, digest, source, environment string) inventory.Resource {
	return inventory.Resource{
		Kind: kind, ID: id, Digest: digest,
		Source: inventory.Source{Type: source, Environment: environment, Endpoint: "https://cluster.example/v2"},
	}
}

func localResource(kind inventory.Kind, id, digest string) inventory.Resource {
	value := resource(kind, id, digest, "local", "")
	value.Source.Endpoint = ""
	value.Source.ProjectRoot = "/project"
	return value
}

func localInventory(resources ...inventory.Resource) inventory.Inventory {
	return inventory.Inventory{
		Source:    inventory.Source{Type: "local", ProjectRoot: "/project"},
		Resources: resources,
	}
}

func remoteInventory(resources ...inventory.Resource) inventory.Inventory {
	return inventory.Inventory{
		Source:    inventory.Source{Type: "remote", Environment: "prod", Endpoint: "https://cluster.example/v2"},
		Resources: resources,
	}
}

func TestBuildCanonicalActionsAndPolicy(t *testing.T) {
	local := localInventory(
		localResource(inventory.KindProcess, "create", "a"),
		localResource(inventory.KindDecision, "same", "b"),
		localResource(inventory.KindForm, "update", "c"),
	)
	remote := remoteInventory(
		resource(inventory.KindProcess, "orphan", "z", "remote", "prod"),
		resource(inventory.KindDecision, "same", "b", "remote", "prod"),
		resource(inventory.KindForm, "update", "old", "remote", "prod"),
	)
	result, err := Build(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, 0, len(result.Actions))
	for _, action := range result.Actions {
		got = append(got, string(action.Resource.Kind)+"/"+action.Resource.ID+":"+string(action.Type))
	}
	want := []string{"decision/same:no-change", "form/update:update", "process/create:create", "process/orphan:remote-only"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("actions=%v", got)
	}
	if !result.Complete || !result.Comparable || result.Policy.Outcome != PolicyChanges ||
		result.Policy.ExitCode != 1 || result.Counts.Total != len(result.Actions) {
		t.Fatalf("result=%+v", result)
	}
}

func TestBuildVersionAndKeyDifferencesAreSemanticNoChange(t *testing.T) {
	local := localInventory(localResource(inventory.KindProcess, "order", "same"))
	older := resource(inventory.KindProcess, "order", "same", "remote", "prod")
	older.Version, older.Key, older.Digest = 6, "100", "old"
	latest := resource(inventory.KindProcess, "order", "same", "remote", "prod")
	latest.Version, latest.Key = 7, "101"
	remote := remoteInventory(latest, older)
	result, err := Build(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Actions) != 1 || result.Actions[0].Type != ActionNoChange {
		t.Fatalf("actions=%+v", result.Actions)
	}
}

func TestBuildNeverNoopsUnknownRemoteState(t *testing.T) {
	local := localInventory(localResource(inventory.KindProcess, "order", "same"))
	remote := remoteInventory()
	remote.Partial = true
	remote.Warnings = []inventory.Warning{{
		Capability: "process-definitions", Message: "item limit reached",
	}}
	result, err := Build(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if result.Comparable || result.Complete || len(result.Actions) != 0 ||
		result.Policy.Outcome != PolicyRefused || result.Policy.ExitCode != 2 {
		t.Fatalf("unsafe result=%+v", result)
	}
}

func TestBuildRefusesMalformedUnsupportedAndDuplicateInventories(t *testing.T) {
	tests := []struct {
		name   string
		local  inventory.Inventory
		remote inventory.Inventory
	}{
		{name: "missing digest", local: localInventory(
			localResource(inventory.KindProcess, "p", ""),
		)},
		{name: "required unsupported", remote: func() inventory.Inventory {
			value := remoteInventory()
			value.Unsupported = []inventory.Unsupported{{Kind: inventory.KindDecision, Required: true, Reason: "not available"}}
			return value
		}()},
		{name: "duplicate local identity", local: localInventory(
			localResource(inventory.KindProcess, "p", "a"), localResource(inventory.KindProcess, "p", "b"),
		)},
		{name: "duplicate remote version", remote: remoteInventory(
			resource(inventory.KindProcess, "p", "a", "remote", "prod"),
			resource(inventory.KindProcess, "p", "b", "remote", "prod"),
		)},
		{name: "mismatched remote environment", remote: remoteInventory(
			resource(inventory.KindProcess, "a", "a", "remote", "prod"),
			resource(inventory.KindProcess, "b", "b", "remote", "other"),
		)},
		{name: "wrong local source", local: localInventory(
			resource(inventory.KindProcess, "a", "a", "remote", "prod"),
		)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := Build(test.local, test.remote)
			if err != nil {
				t.Fatal(err)
			}
			if result.Comparable || len(result.Actions) != 0 || result.Status != StatusRefused {
				t.Fatalf("result=%+v", result)
			}
		})
	}
}

func TestBuildOptionalUnsupportedIsExplicitButPolicyIndependent(t *testing.T) {
	local := localInventory(
		localResource(inventory.KindProcess, "p", "a"),
		localResource(inventory.KindDecision, "decision", "d"),
		localResource(inventory.KindForm, "form", "f"),
	)
	remote := remoteInventory()
	remote.Unsupported = []inventory.Unsupported{
		{Kind: inventory.KindDecision, Reason: "decision content unavailable"},
		{Kind: inventory.KindForm, Reason: "form content unavailable"},
	}
	remote.Warnings = []inventory.Warning{{Capability: "forms", Message: "summary only"}}
	result, err := Build(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if result.Complete || !result.Comparable || result.Status != StatusUnknown ||
		result.Policy.Outcome != PolicyUnknown || result.Policy.ExitCode != 2 ||
		result.Counts.Unknown != 2 || len(result.Actions) != 1 ||
		result.Actions[0].Resource.Kind != inventory.KindProcess {
		t.Fatalf("result=%+v", result)
	}
	if len(result.Warnings) != 3 {
		t.Fatalf("warnings=%+v", result.Warnings)
	}
}

func TestBuildSummariesUseExplicitRoles(t *testing.T) {
	local := localInventory(
		resource(inventory.KindProcess, "p", "a", "remote", "prod"),
	)
	remote := remoteInventory(
		localResource(inventory.KindProcess, "p", "a"),
	)
	result, err := Build(local, remote)
	if err != nil {
		t.Fatal(err)
	}
	if result.Comparable || result.Local.Comparable || result.Remote.Comparable ||
		result.Status != StatusRefused || len(result.Actions) != 0 {
		t.Fatalf("contradictory role result=%+v", result)
	}
}

func TestBuildStableTextAndJSONArrays(t *testing.T) {
	local := localInventory(
		localResource(inventory.KindProcess, "z", "z"),
		localResource(inventory.KindProcess, "a", "a"),
	)
	first, _ := Build(local, remoteInventory())
	second, _ := Build(local, remoteInventory())
	if FormatText(first) != FormatText(second) || !strings.Contains(strings.ToLower(FormatText(first)), "read-only") {
		t.Fatalf("unstable text:\n%s", FormatText(first))
	}
	encoded, err := FormatJSON(first)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]any
	if err := json.Unmarshal(encoded, &decoded); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"actions", "warnings"} {
		if decoded[field] == nil {
			t.Fatalf("%s is null: %s", field, encoded)
		}
	}
}

func TestBuildUpdateIncludesRunningInstanceContextWithoutDelete(t *testing.T) {
	local := localInventory(localResource(inventory.KindProcess, "p", "new"))
	deployed := resource(inventory.KindProcess, "p", "old", "remote", "prod")
	deployed.Version = 4
	result, _ := Build(local, remoteInventory(deployed))
	if len(result.Actions) != 1 || result.Actions[0].Type != ActionUpdate ||
		!strings.Contains(result.Actions[0].Detail, "existing instances") {
		t.Fatalf("actions=%+v", result.Actions)
	}
	for _, action := range result.Actions {
		if action.Destructive || action.Type == "delete" {
			t.Fatalf("destructive action=%+v", action)
		}
	}
}

func TestServicePropagatesCancellationBeforeInventory(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	service := Service{buildLocal: func(inventory.LocalRequest) (inventory.Inventory, error) {
		called = true
		return localInventory(), nil
	}}
	result, err := service.Run(ctx, Request{ProjectRoot: t.TempDir()})
	if !errors.Is(err, context.Canceled) || called || result.Status != StatusError || result.Actions == nil {
		t.Fatalf("result=%+v err=%v called=%v", result, err, called)
	}
}

type planTestFactory struct {
	client   *cluster.Client
	resolved env.Resolved
	request  string
	root     string
}

func (f *planTestFactory) Client(_ context.Context, environment, root string) (*cluster.Client, env.Resolved, error) {
	f.request, f.root = environment, root
	return f.client, f.resolved, nil
}

func TestServiceUsesFactoryInventoryAndReturnsExactResolvedMetadata(t *testing.T) {
	var mutation bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/v2/process-definitions/search" {
			mutation = true
			http.Error(w, "unexpected operation", http.StatusMethodNotAllowed)
			return
		}
		_, _ = io.WriteString(w, `{"items":[],"page":{"totalItems":0,"endCursor":null}}`)
	}))
	defer server.Close()
	resolved := env.Resolved{
		Profile: env.Profile{
			Name: "prod", Kind: "remote",
			Endpoints: map[string]string{"orchestration": server.URL},
			Auth:      env.AuthRefs{ClientIDEnv: "ID", ClientSecretEnv: "SECRET", TokenURLEnv: "TOKEN"},
		},
		Source: env.ProfileSourceProject,
	}
	factory := &planTestFactory{
		client:   &cluster.Client{BaseURL: server.URL + "/v2", Kind: cluster.ClientRemote},
		resolved: resolved,
	}
	service := NewService(factory)
	service.buildLocal = func(inventory.LocalRequest) (inventory.Inventory, error) {
		return localInventory(), nil
	}
	result, err := service.Run(context.Background(), Request{ProjectRoot: "/project", Environment: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	if mutation || factory.request != "prod" || factory.root != "/project" ||
		!reflect.DeepEqual(result.Environment, resolved) || result.Env != "prod" {
		t.Fatalf("result=%+v factory=%+v mutation=%v", result, factory, mutation)
	}
}

func TestServiceRefusesRemoteEndpointMismatch(t *testing.T) {
	resolved := env.Resolved{Profile: env.Profile{
		Name: "prod", Kind: "remote",
		Endpoints: map[string]string{"orchestration": "https://expected.example"},
	}}
	service := NewService(&planTestFactory{resolved: resolved, client: &cluster.Client{}})
	service.buildLocal = func(inventory.LocalRequest) (inventory.Inventory, error) {
		return inventory.Inventory{Source: inventory.Source{
			Type: "local", ProjectRoot: "/project",
		}}, nil
	}
	service.buildRemote = func(context.Context, cluster.Factory, cluster.InventoryRequest) (inventory.Inventory, env.Resolved, error) {
		return inventory.Inventory{Source: inventory.Source{
			Type: "remote", Environment: "prod", Endpoint: "https://other.example/v2",
		}}, resolved, nil
	}
	result, err := service.Run(context.Background(), Request{ProjectRoot: "/project"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Comparable || result.Remote.Comparable || result.Status != StatusRefused {
		t.Fatalf("endpoint mismatch accepted: %+v", result)
	}
}

func TestBuildRequiresLocalProjectSourceMetadata(t *testing.T) {
	local := inventory.Inventory{Source: inventory.Source{Type: "local"}}
	result, err := Build(local, inventory.Inventory{Source: inventory.Source{
		Type: "remote", Environment: "prod", Endpoint: "https://cluster.example/v2",
	}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Comparable || result.Local.Comparable {
		t.Fatalf("missing project metadata accepted: %+v", result)
	}
}

func TestServiceReturnsTypedEmptyResultOnHTTPAndMalformedFailures(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   string
	}{
		{name: "auth", status: http.StatusUnauthorized, body: `{"secret":"must not leak"}`},
		{name: "malformed", status: http.StatusOK, body: `{"items":`},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(test.status)
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			factory := &planTestFactory{
				client:   &cluster.Client{BaseURL: server.URL + "/v2", Kind: cluster.ClientRemote},
				resolved: env.Resolved{Profile: env.Profile{Name: "prod", Kind: "remote"}},
			}
			service := NewService(factory)
			service.buildLocal = func(inventory.LocalRequest) (inventory.Inventory, error) {
				return localInventory(), nil
			}
			result, err := service.Run(context.Background(), Request{})
			if err == nil || result.Status != StatusError || result.Actions == nil || result.Warnings == nil {
				t.Fatalf("result=%+v err=%v", result, err)
			}
			if strings.Contains(err.Error(), "must not leak") {
				t.Fatalf("response body leaked: %v", err)
			}
		})
	}
}
