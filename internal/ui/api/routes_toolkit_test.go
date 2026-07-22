package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

func TestToolkitRoutesInventory(t *testing.T) {
	routes := []struct {
		method string
		path   string
	}{
		{method: http.MethodPost, path: "/api/v1/bpmn/lint"},
		{method: http.MethodPost, path: "/api/v1/bpmn/diff"},
		{method: http.MethodPost, path: "/api/v1/bpmn/explain"},
		{method: http.MethodPost, path: "/api/v1/bpmn/review"},
		{method: http.MethodPost, path: "/api/v1/bpmn/test-generate"},
		{method: http.MethodPost, path: "/api/v1/bpmn/test-generate/download"},
		{method: http.MethodPost, path: "/api/v1/bpmn/scan"},
		{method: http.MethodGet, path: "/api/v1/doctor/deep"},
		{method: http.MethodGet, path: "/api/v1/env"},
		{method: http.MethodPost, path: "/api/v1/env"},
		{method: http.MethodPost, path: "/api/v1/env/use"},
		{method: http.MethodDelete, path: "/api/v1/env/{name}"},
		{method: http.MethodPost, path: "/api/v1/plan"},
		{method: http.MethodPost, path: "/api/v1/drift"},
		{method: http.MethodGet, path: "/api/v1/incidents"},
		{method: http.MethodPost, path: "/api/v1/incidents/{key}/retry"},
		{method: http.MethodGet, path: "/api/v1/trace/{instanceKey}"},
		{method: http.MethodPost, path: "/api/v1/backup"},
		{method: http.MethodPost, path: "/api/v1/backup/download"},
		{method: http.MethodPost, path: "/api/v1/restore"},
		{method: http.MethodPost, path: "/api/v1/project/init"},
	}

	handler := NewHandler("test", Dependencies{})
	for _, route := range routes {
		path := route.path
		path = strings.ReplaceAll(path, "{name}", "demo")
		path = strings.ReplaceAll(path, "{key}", "1")
		path = strings.ReplaceAll(path, "{instanceKey}", "1")
		path = strings.ReplaceAll(path, "{component}", "zeebe")
		request := httptest.NewRequest(route.method, path, strings.NewReader("{}"))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		// ServeMux missing-route 404 text; application NotFound JSON still proves the route exists.
		if response.Code == http.StatusNotFound && strings.Contains(response.Body.String(), "404 page not found") {
			t.Errorf("%s %s returned 404 (route missing)", route.method, route.path)
		}
	}
}

func TestToolkitRoutesRejectUnknownJSONFields(t *testing.T) {
	endpoints := []string{
		"/api/v1/bpmn/lint",
		"/api/v1/bpmn/diff",
		"/api/v1/bpmn/explain",
		"/api/v1/bpmn/review",
		"/api/v1/bpmn/test-generate",
		"/api/v1/bpmn/scan",
		"/api/v1/plan",
		"/api/v1/drift",
	}
	handler := NewHandler("test", Dependencies{})
	for _, endpoint := range endpoints {
		request := httptest.NewRequest(http.MethodPost, endpoint, strings.NewReader(`{"surprise":true}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		assertErrorEnvelope(t, response, http.StatusBadRequest, "invalid_request")
	}
}

func TestToolkitRouteIntegrationCases(t *testing.T) {
	home := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", home)
	paths.Reset()
	t.Cleanup(paths.Reset)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte("version: \"8.9\"\nprofile: light\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	project := t.TempDir()
	if err := os.WriteFile(filepath.Join(project, ".camunda.yaml"), []byte(
		"name: a3\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(project, "bpmn"), 0o700); err != nil {
		t.Fatal(err)
	}
	bpmnPath := filepath.Join(project, "bpmn", "order.bpmn")
	if err := os.WriteFile(bpmnPath, []byte(`<?xml version="1.0"?>
<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL" xmlns:bpmndi="http://www.omg.org/spec/BPMN/20100524/DI">
  <process id="order" isExecutable="true">
    <startEvent id="start"/><endEvent id="end"/>
    <sequenceFlow id="f1" sourceRef="start" targetRef="end"/>
  </process>
</definitions>`), 0o600); err != nil {
		t.Fatal(err)
	}

	handler := NewHandler("test", Dependencies{
		Toolkit: fakeToolkitOK{},
		Plan: func(context.Context, plan.Request) (plan.Result, error) {
			return plan.Result{
				Status: plan.StatusReady,
				Policy: plan.Policy{Outcome: plan.PolicyNoChanges, ExitCode: 0},
			}, nil
		},
		Drift: func(context.Context, drift.Request) (drift.Report, error) {
			return drift.Report{
				Status:     drift.StatusReady,
				Complete:   true,
				Comparable: true,
				Policy:     drift.Policy{Outcome: "drift", ExitCode: 1},
			}, nil
		},
		TraceGet: func(context.Context, trace.Request) (trace.Timeline, error) {
			return trace.Timeline{}, &trace.NotFoundError{Key: "missing"}
		},
		Env: &fakeEnvConflict{},
	})

	t.Run("valid success lint", func(t *testing.T) {
		body := `{"path":` + mustJSON(t, bpmnPath) + `}`
		rec := postJSON(t, handler, "/api/v1/bpmn/lint", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload map[string]any
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload["ok"] != true || payload["complete"] != true {
			t.Fatalf("success schema incomplete: %s", rec.Body.String())
		}
	})

	t.Run("missing resource 404", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/trace/missing", nil)
		handler.ServeHTTP(rec, req)
		assertErrorEnvelope(t, rec, http.StatusNotFound, "not_found")
	})

	t.Run("env conflict 409", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodDelete, "/api/v1/env/prod", nil)
		handler.ServeHTTP(rec, req)
		assertErrorEnvelope(t, rec, http.StatusConflict, "conflict")
	})

	t.Run("unsupported capability 422", func(t *testing.T) {
		// Use real toolkit validation (no fake) so unsupported languages are rejected.
		real := NewHandler("test", Dependencies{})
		body := `{"path":` + mustJSON(t, bpmnPath) + `,"lang":"ruby"}`
		rec := postJSON(t, real, "/api/v1/bpmn/test-generate", body)
		assertErrorEnvelope(t, rec, http.StatusUnprocessableEntity, "invalid_request")
	})

	t.Run("upstream failure 502", func(t *testing.T) {
		failing := NewHandler("test", Dependencies{
			Toolkit: fakeToolkitUpstream{},
		})
		body := `{"path":` + mustJSON(t, bpmnPath) + `,"ai":true,"provider":"openai","model":"gpt"}`
		rec := postJSON(t, failing, "/api/v1/bpmn/review", body)
		assertErrorEnvelope(t, rec, http.StatusBadGateway, "ai")
	})

	t.Run("completed drift may be 200 with drift", func(t *testing.T) {
		body := `{"dir":` + mustJSON(t, project) + `}`
		rec := postJSON(t, handler, "/api/v1/drift", body)
		if rec.Code != http.StatusOK {
			t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
		}
		var payload struct {
			OK     bool   `json:"ok"`
			Status string `json:"status"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.OK || payload.Status != string(drift.StatusReady) {
			t.Fatalf("want completed drift with ok=false: %+v body=%s", payload, rec.Body.String())
		}
	})

	t.Run("incomplete remote state fails closed", func(t *testing.T) {
		incomplete := NewHandler("test", Dependencies{
			Plan: func(context.Context, plan.Request) (plan.Result, error) {
				return plan.Result{
					Status: plan.StatusRefused,
					Policy: plan.Policy{Outcome: plan.PolicyRefused, ExitCode: 2},
				}, nil
			},
			Drift: func(context.Context, drift.Request) (drift.Report, error) {
				return drift.Report{
					Status:     drift.StatusUnknown,
					Complete:   false,
					Comparable: false,
					Policy:     drift.Policy{Outcome: "unknown", ExitCode: 2},
				}, errors.New("remote incomplete")
			},
		})
		planRec := postJSON(t, incomplete, "/api/v1/plan", `{"dir":`+mustJSON(t, project)+`}`)
		if planRec.Code != http.StatusOK {
			t.Fatalf("plan status=%d body=%s", planRec.Code, planRec.Body.String())
		}
		var planBody struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal(planRec.Body.Bytes(), &planBody); err != nil {
			t.Fatal(err)
		}
		if planBody.OK {
			t.Fatalf("incomplete plan claimed success: %s", planRec.Body.String())
		}

		driftRec := postJSON(t, incomplete, "/api/v1/drift", `{"dir":`+mustJSON(t, project)+`}`)
		if driftRec.Code != http.StatusOK {
			t.Fatalf("drift status=%d body=%s", driftRec.Code, driftRec.Body.String())
		}
		var driftBody struct {
			OK bool `json:"ok"`
		}
		if err := json.Unmarshal(driftRec.Body.Bytes(), &driftBody); err != nil {
			t.Fatal(err)
		}
		if driftBody.OK {
			t.Fatalf("incomplete drift claimed success: %s", driftRec.Body.String())
		}
	})

	t.Run("mutation security 403", func(t *testing.T) {
		mux := http.NewServeMux()
		Register(mux, "test", "token")
		secured := SecurityMiddleware("token", mux)
		req := httptest.NewRequest(http.MethodPost, "/api/v1/plan", strings.NewReader(`{}`))
		req.Host = "localhost:9090"
		req.Header.Set("Origin", "http://attacker.example")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		secured.ServeHTTP(rec, req)
		assertErrorEnvelope(t, rec, http.StatusForbidden, "invalid_origin")
	})

	t.Run("path forbidden 403", func(t *testing.T) {
		rec := postJSON(t, handler, "/api/v1/plan", `{"dir":"/etc"}`)
		assertErrorEnvelope(t, rec, http.StatusForbidden, "path_forbidden")
	})

	t.Run("backup rejects empty output without temp invent", func(t *testing.T) {
		rec := postJSON(t, handler, "/api/v1/backup", `{}`)
		if rec.Code < 400 {
			t.Fatalf("status=%d, want rejection", rec.Code)
		}
		body := rec.Body.String()
		if strings.Contains(body, os.TempDir()) || strings.Contains(body, home) {
			t.Fatalf("leaked temp/home path: %s", body)
		}
		assertErrorEnvelope(t, rec, http.StatusBadRequest, "invalid_request")
	})
}

func assertErrorEnvelope(t *testing.T, rec *httptest.ResponseRecorder, status int, code string) {
	t.Helper()
	if rec.Code != status {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, status, rec.Body.String())
	}
	var body errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rec.Body.String())
	}
	if body.OK {
		t.Fatalf("ok must be false: %+v", body)
	}
	if body.Code != code {
		t.Fatalf("code = %q, want %q; body=%s", body.Code, code, rec.Body.String())
	}
	if body.Error == "" {
		t.Fatalf("empty error message: %+v", body)
	}
}

func postJSON(t *testing.T, h http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

type fakeToolkitOK struct{}

func (fakeToolkitOK) Lint(context.Context, toolkit.LintRequest) (toolkit.LintResult, error) {
	return toolkit.LintResult{Status: toolkit.StatusCompleted, Complete: true}, nil
}
func (fakeToolkitOK) Diff(context.Context, toolkit.DiffRequest) (toolkit.DiffResult, error) {
	return toolkit.DiffResult{Status: toolkit.StatusCompleted, Complete: true}, nil
}
func (fakeToolkitOK) Explain(context.Context, toolkit.ExplainRequest) (toolkit.ExplainResult, error) {
	return toolkit.ExplainResult{Status: toolkit.StatusCompleted, Complete: true}, nil
}
func (fakeToolkitOK) Review(context.Context, toolkit.ReviewRequest) (toolkit.ReviewResult, error) {
	return toolkit.ReviewResult{Status: toolkit.StatusCompleted, Complete: true}, nil
}
func (fakeToolkitOK) Generate(context.Context, toolkit.GenerateRequest) (toolkit.GenerateResult, error) {
	return toolkit.GenerateResult{Status: toolkit.StatusCompleted, Complete: true}, nil
}
func (fakeToolkitOK) Scan(context.Context, toolkit.ScanRequest) (toolkit.ScanResult, error) {
	return toolkit.ScanResult{Status: toolkit.StatusCompleted, Complete: true}, nil
}

type fakeToolkitUpstream struct{ fakeToolkitOK }

func (fakeToolkitUpstream) Review(context.Context, toolkit.ReviewRequest) (toolkit.ReviewResult, error) {
	return toolkit.ReviewResult{}, &toolkit.Error{Kind: toolkit.ErrorAI, Operation: toolkit.OperationReview, Err: errors.New("provider down")}
}

type fakeEnvConflict struct{}

func (fakeEnvConflict) Resolve(env.ResolveRequest) (env.Resolved, error) {
	return env.Resolved{}, nil
}
func (fakeEnvConflict) List(string) ([]env.Resolved, error) { return nil, nil }
func (fakeEnvConflict) Use(string, string) (env.Resolved, error) {
	return env.Resolved{}, nil
}
func (fakeEnvConflict) SaveGlobal(env.Profile) error          { return nil }
func (fakeEnvConflict) SaveProject(string, env.Profile) error { return nil }
func (fakeEnvConflict) Remove(string, string, env.ProfileSource) error {
	return &env.Error{Kind: env.ErrorConflict, Operation: "remove", Name: "prod", Err: errors.New("referenced")}
}
