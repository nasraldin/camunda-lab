package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestPlatformOpsUseProjectLocalFactoryResolution(t *testing.T) {
	var projectCalls atomic.Int32
	clusterServer := func(calls *atomic.Int32) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			calls.Add(1)
			w.Header().Set("Content-Type", "application/json")
			switch r.URL.Path {
			case "/v2/process-definitions/search",
				"/v2/decision-definitions/search",
				"/v2/form-definitions/search",
				"/v2/incidents/search",
				"/v2/element-instances/search",
				"/v2/jobs/search":
				_, _ = io.WriteString(w, `{"items":[],"page":{"totalItems":0,"endCursor":null}}`)
			case "/v2/process-instances/search":
				_, _ = io.WriteString(w, `{"items":[{
					"processInstanceKey":"9007199254740993","processDefinitionKey":"8","processDefinitionId":"orders",
					"processDefinitionVersion":1,"state":"ACTIVE","hasIncident":false,"tags":[],
					"startDate":"2026-07-21T12:00:00Z","tenantId":"<default>"
				}],"page":{"totalItems":1,"endCursor":null}}`)
			default:
				http.NotFound(w, r)
			}
		}))
	}
	projectCluster := clusterServer(&projectCalls)
	defer projectCluster.Close()

	home := t.TempDir()
	projectRoot := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	t.Setenv("CAMUNDA_ACCESS_TOKEN", "test-token")
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := os.WriteFile(filepath.Join(projectRoot, ".camunda.yaml"), []byte("name: adapter-test\ncamundaVersion: \"8.9\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := env.NewService(home)
	profile := func(name, endpoint string) env.Profile {
		return env.Profile{
			Name: name, Kind: "remote",
			Endpoints: map[string]string{"orchestration": endpoint},
			Auth: env.AuthRefs{
				ClientIDEnv: "TEST_CLIENT_ID", ClientSecretEnv: "TEST_CLIENT_SECRET",
			},
		}
	}
	if err := service.SaveGlobal(profile("global", "https://global.example")); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Use("global", ""); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveProject(projectRoot, profile("project-local", projectCluster.URL)); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Use("project-local", projectRoot); err != nil {
		t.Fatal(err)
	}

	factory := &uiTestClusterFactory{home: home, baseURL: projectCluster.URL + "/v2"}
	h := &handler{clusterFactory: factory}
	dirJSON := mustProjectJSON(t, projectRoot)
	dirQuery := url.QueryEscape(projectRoot)
	for _, test := range []struct {
		name    string
		handler func(http.ResponseWriter, *http.Request)
		req     *http.Request
		field   string
		envPath bool
	}{
		{
			name: "plan", handler: h.runPlan, field: "plan", envPath: true,
			req: httptest.NewRequest(http.MethodPost, "/api/v1/plan",
				strings.NewReader(`{"dir":`+dirJSON+`}`)),
		},
		{
			name: "drift", handler: h.runDrift, field: "drift", envPath: true,
			req: httptest.NewRequest(http.MethodPost, "/api/v1/drift",
				strings.NewReader(`{"dir":`+dirJSON+`}`)),
		},
		{
			name: "incidents", handler: h.listIncidents, field: "result",
			req: httptest.NewRequest(http.MethodGet, "/api/v1/incidents?dir="+dirQuery, nil),
		},
		{
			name: "trace", handler: h.traceInstance, field: "timeline",
			req: func() *http.Request {
				req := httptest.NewRequest(http.MethodGet, "/api/v1/trace/9007199254740993?dir="+dirQuery, nil)
				req.SetPathValue("instanceKey", "9007199254740993")
				return req
			}(),
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			if test.req.Header.Get("Content-Type") == "" && test.req.Method == http.MethodPost {
				test.req.Header.Set("Content-Type", "application/json")
			}
			rec := httptest.NewRecorder()
			test.handler(rec, test.req)
			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
			}
			var body map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
				t.Fatal(err)
			}
			if test.envPath {
				var result struct {
					Env string
				}
				if err := json.Unmarshal(body[test.field], &result); err != nil {
					t.Fatal(err)
				}
				if result.Env != "project-local" {
					t.Fatalf("%s environment = %q", test.name, result.Env)
				}
				return
			}
			var result struct {
				Environment struct {
					Profile struct {
						Name string `json:"name"`
					} `json:"profile"`
				} `json:"environment"`
			}
			if err := json.Unmarshal(body[test.field], &result); err != nil {
				t.Fatal(err)
			}
			if result.Environment.Profile.Name != "project-local" {
				t.Fatalf("%s environment = %q", test.name, result.Environment.Profile.Name)
			}
		})
	}
	if projectCalls.Load() < 4 {
		t.Fatalf("project calls=%d", projectCalls.Load())
	}
	canonicalRoot, err := filepath.EvalSymlinks(projectRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(factory.projectRoots) != 4 {
		t.Fatalf("factory project roots=%v", factory.projectRoots)
	}
	for i, root := range factory.projectRoots {
		if root != canonicalRoot {
			t.Fatalf("factory project roots[%d]=%q, want %q", i, root, canonicalRoot)
		}
	}
}

func mustProjectJSON(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

type uiTestClusterFactory struct {
	home         string
	baseURL      string
	projectRoots []string
}

func (f *uiTestClusterFactory) Client(_ context.Context, name, projectRoot string) (*cluster.Client, env.Resolved, error) {
	f.projectRoots = append(f.projectRoots, projectRoot)
	resolved, err := env.NewService(f.home).Resolve(env.ResolveRequest{Name: name, ProjectRoot: projectRoot})
	if err != nil {
		return nil, env.Resolved{}, err
	}
	return &cluster.Client{BaseURL: f.baseURL, HTTPClient: http.DefaultClient, Kind: cluster.ClientRemote}, resolved, nil
}

var _ cluster.Factory = (*uiTestClusterFactory)(nil)
