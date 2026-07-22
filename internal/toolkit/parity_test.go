package toolkit_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/cli"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
	"github.com/nasraldin/camunda-lab/internal/ui/api"
)

func TestParityNormalizedDomainRequestsMatch(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	content := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte("name: toolkit\ncamundaVersion: \"8.9\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "generated")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	canonRoot := mustCanonical(t, root)
	canonPath := mustCanonical(t, path)
	canonOut := mustCanonical(t, outDir)

	cases := []struct {
		name      string
		cliArgs   []string
		apiMethod string
		apiPath   string
		apiBody   string
		capture   func(*parityRecorder) any
	}{
		{
			name:      "lint",
			cliArgs:   []string{"lint", path, "--fail-on", "warning", "--ignore", "bpmn/rule", "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/bpmn/lint",
			apiBody:   `{"path":` + mustJSON(path) + `,"failOn":"warning","ignore":["bpmn/rule"]}`,
			capture: func(r *parityRecorder) any {
				return normalizedLint(r.lint)
			},
		},
		{
			name:      "diff",
			cliArgs:   []string{"diff", "--from", path, "--to", path, "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/bpmn/diff",
			apiBody:   `{"from":` + mustJSON(path) + `,"to":` + mustJSON(path) + `}`,
			capture: func(r *parityRecorder) any {
				return normalizedDiff(r.diff)
			},
		},
		{
			name:      "explain",
			cliArgs:   []string{"explain", path, "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/bpmn/explain",
			apiBody:   `{"path":` + mustJSON(path) + `}`,
			capture: func(r *parityRecorder) any {
				return normalizedExplain(r.explain)
			},
		},
		{
			name:      "review",
			cliArgs:   []string{"review", path, "--fail-on", "warning", "--ignore", "bpmn/rule", "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/bpmn/review",
			apiBody:   `{"path":` + mustJSON(path) + `,"failOn":"warning","ignore":["bpmn/rule"]}`,
			capture: func(r *parityRecorder) any {
				return normalizedReview(r.review)
			},
		},
		{
			name:      "scan",
			cliArgs:   []string{"scan", root, "--fail-on", "high", "--ignore", "*.tmp", "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/bpmn/scan",
			apiBody:   `{"dir":` + mustJSON(root) + `,"failOn":"high","ignore":["*.tmp"]}`,
			capture: func(r *parityRecorder) any {
				return normalizedScan(r.scan)
			},
		},
		{
			name:      "generate",
			cliArgs:   []string{"test", "generate", path, "--lang", "js", "--output", outDir, "--force", "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/bpmn/test-generate",
			apiBody:   `{"path":` + mustJSON(path) + `,"lang":"js","write":true,"output":` + mustJSON(outDir) + `,"force":true}`,
			capture: func(r *parityRecorder) any {
				return normalizedGenerate(r.generate)
			},
		},
		{
			name:      "plan",
			cliArgs:   []string{"plan", "--dir", root, "--env", "prod", "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/plan",
			apiBody:   `{"dir":` + mustJSON(root) + `,"environment":"prod"}`,
			capture: func(r *parityRecorder) any {
				return plan.Request{ProjectRoot: mustCanonical(t, r.plan.ProjectRoot), Environment: r.plan.Environment}
			},
		},
		{
			name:      "drift",
			cliArgs:   []string{"drift", "--dir", root, "--ref", "main", "--env", "prod", "--json"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/drift",
			apiBody:   `{"dir":` + mustJSON(root) + `,"ref":"main","environment":"prod"}`,
			capture: func(r *parityRecorder) any {
				return drift.Request{ProjectRoot: mustCanonical(t, r.drift.ProjectRoot), GitRef: r.drift.GitRef, Environment: r.drift.Environment}
			},
		},
	}

	wantByName := map[string]any{
		"lint": toolkit.LintRequest{
			Inputs: []toolkit.BPMNInput{{Name: path, Path: canonPath}},
			FailOn: toolkit.LintThresholdWarning,
			Ignore: []string{"bpmn/rule"},
		},
		"diff": toolkit.DiffRequest{
			Before: toolkit.BPMNInput{Name: path, Path: canonPath},
			After:  toolkit.BPMNInput{Name: path, Path: canonPath},
		},
		"explain": toolkit.ExplainRequest{Input: toolkit.BPMNInput{Name: path, Path: canonPath}},
		"review": toolkit.ReviewRequest{
			Inputs: []toolkit.BPMNInput{{Name: path, Path: canonPath}},
			FailOn: toolkit.LintThresholdWarning,
			Ignore: []string{"bpmn/rule"},
		},
		"scan": toolkit.ScanRequest{
			Roots: []string{canonRoot}, FailOn: toolkit.ScanThresholdHigh, Ignore: []string{"*.tmp"},
		},
		"generate": toolkit.GenerateRequest{
			Input: toolkit.BPMNInput{Name: path, Path: canonPath}, OutDir: canonOut,
			Lang: toolkit.GenerateLanguageJavaScript, Force: true,
		},
		"plan":  plan.Request{ProjectRoot: canonRoot, Environment: "prod"},
		"drift": drift.Request{ProjectRoot: canonRoot, GitRef: "main", Environment: "prod"},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cliRecorder := &parityRecorder{}
			apiRecorder := &parityRecorder{}
			cliDeps := cli.Dependencies{
				Toolkit: cliRecorder,
				Plan:    cliRecorder.RunPlan,
				Drift:   cliRecorder.RunDrift,
			}
			apiDeps := api.Dependencies{
				Toolkit: apiRecorder,
				Plan:    apiRecorder.RunPlan,
				Drift:   apiRecorder.RunDrift,
			}

			rootCmd := cli.NewRootWithDependencies(cliDeps)
			var stdout, stderr bytes.Buffer
			rootCmd.SetOut(&stdout)
			rootCmd.SetErr(&stderr)
			rootCmd.SetArgs(test.cliArgs)
			if err := rootCmd.Execute(); err != nil && cli.ExitCode(err) > 1 {
				t.Fatalf("CLI execute: %v stderr=%s", err, stderr.String())
			}

			request := httptest.NewRequest(test.apiMethod, test.apiPath, bytes.NewBufferString(test.apiBody))
			request.Header.Set("Content-Type", "application/json")
			response := httptest.NewRecorder()
			api.NewHandler("test", apiDeps).ServeHTTP(response, request)
			if response.Code == http.StatusNotFound {
				t.Fatalf("API route missing: %s", test.apiPath)
			}
			if response.Code >= 400 && test.name != "scan" {
				// scan may still succeed; other ops should reach the injectable service
				if response.Code >= 500 {
					t.Fatalf("API status = %d body=%s", response.Code, response.Body.String())
				}
			}

			cliGot := test.capture(cliRecorder)
			apiGot := test.capture(apiRecorder)
			want := wantByName[test.name]
			if !reflect.DeepEqual(cliGot, want) {
				t.Fatalf("CLI request = %#v, want %#v", cliGot, want)
			}
			if !reflect.DeepEqual(apiGot, want) {
				t.Fatalf("API request = %#v, want %#v (status=%d body=%s)", apiGot, want, response.Code, response.Body.String())
			}
			if !reflect.DeepEqual(cliGot, apiGot) {
				t.Fatalf("CLI/API divergence: cli=%#v api=%#v", cliGot, apiGot)
			}
		})
	}
}

func TestParitySharedResultEnvelopeFields(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	content := `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><endEvent id="end"/><sequenceFlow id="flow" sourceRef="start" targetRef="end"/></process></definitions>`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	command := cli.NewRootWithDependencies(cli.Dependencies{})
	command.SetOut(&stdout)
	command.SetArgs([]string{"lint", path, "--json"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	var cliBody map[string]json.RawMessage
	if err := json.Unmarshal(stdout.Bytes(), &cliBody); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodPost, "/api/v1/bpmn/lint",
		bytes.NewBufferString(`{"path":`+mustJSON(path)+`}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	api.NewHandler("test", api.Dependencies{}).ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", response.Code, response.Body.String())
	}
	var apiBody map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &apiBody); err != nil {
		t.Fatal(err)
	}

	for _, field := range []string{"status", "complete", "warnings", "findings"} {
		if _, ok := cliBody[field]; !ok {
			t.Errorf("CLI JSON missing %s", field)
		}
		if _, ok := apiBody[field]; !ok {
			t.Errorf("API JSON missing %s", field)
		}
	}
}

func TestParityPlatformOpsNormalizedRequestsMatch(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte("name: toolkit\ncamundaVersion: \"8.9\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outPath := filepath.Join(root, "lab-backup.tar.gz")
	if err := os.WriteFile(outPath, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	canonRoot := mustCanonical(t, root)
	canonOut := mustCanonical(t, outPath)
	instanceKey := "9007199254740993"
	incidentKey := "9007199254740994"

	t.Chdir(root)
	t.Setenv("CAMUNDA_LAB_HOME", filepath.Join(root, "lab-home"))

	cases := []struct {
		name      string
		cliArgs   []string
		apiMethod string
		apiPath   string
		apiBody   string
		capture   func(*parityRecorder) any
		want      any
	}{
		{
			name:      "incidents-list",
			cliArgs:   []string{"incidents", "list", "--limit", "25", "--env", "staging"},
			apiMethod: http.MethodGet,
			apiPath:   "/api/v1/incidents?limit=25&environment=staging&dir=" + url.QueryEscape(root),
			capture: func(r *parityRecorder) any {
				req := r.listIncidents
				req.ProjectRoot = mustCanonicalPath(req.ProjectRoot)
				return req
			},
			want: incidents.ListRequest{
				Environment: "staging", ProjectRoot: canonRoot, Limit: 25,
				Filter: incidents.ListFilter{State: "ACTIVE"},
			},
		},
		{
			name:      "incidents-retry-dry-run",
			cliArgs:   []string{"incidents", "retry", incidentKey, "--dry-run", "--env", "staging"},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/incidents/" + incidentKey + "/retry",
			apiBody:   `{"dryRun":true,"environment":"staging","dir":` + mustJSON(root) + `}`,
			capture: func(r *parityRecorder) any {
				req := r.resolveIncident
				req.ProjectRoot = mustCanonicalPath(req.ProjectRoot)
				return req
			},
			want: incidents.ResolveRequest{
				Environment: "staging", ProjectRoot: canonRoot, Key: incidentKey, DryRun: true,
			},
		},
		{
			name:      "trace-follow-aligned",
			cliArgs:   []string{"trace", instanceKey, "--follow", "--timeout", "30s", "--max-events", "20", "--interval", "2s", "--env", "staging"},
			apiMethod: http.MethodGet,
			apiPath: "/api/v1/trace/" + instanceKey +
				"?follow=true&timeout=30s&maxEvents=20&interval=2s&environment=staging&dir=" + url.QueryEscape(root),
			capture: func(r *parityRecorder) any {
				req := r.traceFollow
				req.ProjectRoot = mustCanonicalPath(req.ProjectRoot)
				return followCapture{Request: req, Interval: r.traceInterval}
			},
			want: followCapture{
				Request: trace.Request{
					Environment: "staging", ProjectRoot: canonRoot, ProcessInstanceKey: instanceKey,
					Timeout: 30 * time.Second, MaxEvents: 20, IdleStop: 0,
				},
				Interval: 2 * time.Second,
			},
		},
		{
			name:      "backup-create",
			cliArgs:   []string{"backup", "--output", outPath},
			apiMethod: http.MethodPost,
			apiPath:   "/api/v1/backup",
			apiBody:   `{"output":` + mustJSON(outPath) + `,"dir":` + mustJSON(root) + `}`,
			capture: func(r *parityRecorder) any {
				opts := r.backup
				opts.LabHome = ""
				opts.LabVersion = ""
				opts.LabProfile = ""
				opts.OutPath = mustCanonicalPath(opts.OutPath)
				opts.ProjectDir = mustCanonicalPath(opts.ProjectDir)
				return opts
			},
			want: backup.Options{ProjectDir: canonRoot, OutPath: canonOut, IncludeSecrets: false},
		},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			cliRecorder := &parityRecorder{}
			apiRecorder := &parityRecorder{}
			cliDeps := platformDeps(cliRecorder)
			apiDeps := platformAPIDeps(apiRecorder)

			rootCmd := cli.NewRootWithDependencies(cliDeps)
			var stdout, stderr bytes.Buffer
			rootCmd.SetOut(&stdout)
			rootCmd.SetErr(&stderr)
			rootCmd.SetArgs(test.cliArgs)
			if err := rootCmd.Execute(); err != nil && cli.ExitCode(err) > 1 {
				t.Fatalf("CLI execute: %v stderr=%s", err, stderr.String())
			}

			var body io.Reader
			if test.apiBody != "" {
				body = bytes.NewBufferString(test.apiBody)
			}
			request := httptest.NewRequest(test.apiMethod, test.apiPath, body)
			if test.apiBody != "" {
				request.Header.Set("Content-Type", "application/json")
			}
			response := httptest.NewRecorder()
			api.NewHandler("test", apiDeps).ServeHTTP(response, request)
			if response.Code >= 500 {
				t.Fatalf("API status = %d body=%s", response.Code, response.Body.String())
			}

			cliGot := test.capture(cliRecorder)
			apiGot := test.capture(apiRecorder)
			if !reflect.DeepEqual(cliGot, test.want) {
				t.Fatalf("CLI request = %#v, want %#v", cliGot, test.want)
			}
			if !reflect.DeepEqual(apiGot, test.want) {
				t.Fatalf("API request = %#v, want %#v (status=%d body=%s)", apiGot, test.want, response.Code, response.Body.String())
			}
			if !reflect.DeepEqual(cliGot, apiGot) {
				t.Fatalf("CLI/API divergence: cli=%#v api=%#v", cliGot, apiGot)
			}
		})
	}
}

func TestParityTraceFollowDefaultDivergencesDocumented(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte("name: toolkit\ncamundaVersion: \"8.9\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	instanceKey := "9007199254740993"

	cliRecorder := &parityRecorder{}
	apiRecorder := &parityRecorder{}

	rootCmd := cli.NewRootWithDependencies(platformDeps(cliRecorder))
	rootCmd.SetOut(io.Discard)
	rootCmd.SetErr(io.Discard)
	rootCmd.SetArgs([]string{"trace", instanceKey, "--follow"})
	if err := rootCmd.Execute(); err != nil && cli.ExitCode(err) > 1 {
		t.Fatalf("CLI execute: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/v1/trace/"+instanceKey+"?follow=true", nil)
	response := httptest.NewRecorder()
	api.NewHandler("test", platformAPIDeps(apiRecorder)).ServeHTTP(response, request)
	if response.Code >= 500 {
		t.Fatalf("API status = %d body=%s", response.Code, response.Body.String())
	}

	// Intentional divergences: CLI keeps interactive longer defaults; API/UI stay bounded.
	if cliRecorder.traceFollow.Timeout != 5*time.Minute {
		t.Fatalf("CLI follow timeout = %s, want 5m", cliRecorder.traceFollow.Timeout)
	}
	if cliRecorder.traceFollow.MaxEvents != 0 {
		t.Fatalf("CLI follow max-events = %d, want 0 (domain default)", cliRecorder.traceFollow.MaxEvents)
	}
	if apiRecorder.traceFollow.Timeout != 30*time.Second {
		t.Fatalf("API follow timeout = %s, want 30s", apiRecorder.traceFollow.Timeout)
	}
	if apiRecorder.traceFollow.MaxEvents != 20 {
		t.Fatalf("API follow maxEvents = %d, want 20", apiRecorder.traceFollow.MaxEvents)
	}

	command, _, err := cli.NewRootWithDependencies(cli.Dependencies{}).Find([]string{"trace"})
	if err != nil {
		t.Fatal(err)
	}
	if command.Flags().Lookup("idle-stop") == nil {
		t.Fatal("CLI must advertise --idle-stop")
	}
	if apiRecorder.traceFollow.IdleStop != 0 {
		t.Fatalf("API must not set IdleStop; got %s", apiRecorder.traceFollow.IdleStop)
	}
}

type followCapture struct {
	Request  trace.Request
	Interval time.Duration
}

type parityRecorder struct {
	lint            toolkit.LintRequest
	diff            toolkit.DiffRequest
	explain         toolkit.ExplainRequest
	review          toolkit.ReviewRequest
	generate        toolkit.GenerateRequest
	scan            toolkit.ScanRequest
	plan            plan.Request
	drift           drift.Request
	listIncidents   incidents.ListRequest
	resolveIncident incidents.ResolveRequest
	traceGet        trace.Request
	traceFollow     trace.Request
	traceInterval   time.Duration
	backup          backup.Options
}

func (r *parityRecorder) Lint(_ context.Context, request toolkit.LintRequest) (toolkit.LintResult, error) {
	r.lint = request
	return toolkit.LintResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Findings: []toolkit.LintFinding{}, Inputs: []string{}}, nil
}
func (r *parityRecorder) Diff(_ context.Context, request toolkit.DiffRequest) (toolkit.DiffResult, error) {
	r.diff = request
	return toolkit.DiffResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Changes: []toolkit.ProcessChange{}}, nil
}
func (r *parityRecorder) Explain(_ context.Context, request toolkit.ExplainRequest) (toolkit.ExplainResult, error) {
	r.explain = request
	return toolkit.ExplainResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Processes: []toolkit.ProcessExplanation{}}, nil
}
func (r *parityRecorder) Review(_ context.Context, request toolkit.ReviewRequest) (toolkit.ReviewResult, error) {
	r.review = request
	return toolkit.ReviewResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Inputs: []string{}, Processes: []toolkit.ProcessReview{}, Findings: []toolkit.LintFinding{}, AIStatus: toolkit.AIStatusDisabled}, nil
}
func (r *parityRecorder) Generate(_ context.Context, request toolkit.GenerateRequest) (toolkit.GenerateResult, error) {
	r.generate = request
	return toolkit.GenerateResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Artifacts: []toolkit.Artifact{}}, nil
}
func (r *parityRecorder) Scan(_ context.Context, request toolkit.ScanRequest) (toolkit.ScanResult, error) {
	r.scan = request
	return toolkit.ScanResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}}, nil
}
func (r *parityRecorder) RunPlan(_ context.Context, request plan.Request) (plan.Result, error) {
	r.plan = request
	return plan.Result{}, nil
}
func (r *parityRecorder) RunDrift(_ context.Context, request drift.Request) (drift.Report, error) {
	r.drift = request
	return drift.Report{}, nil
}
func (r *parityRecorder) ListIncidents(_ context.Context, request incidents.ListRequest) (incidents.Result, error) {
	r.listIncidents = request
	return incidents.Result{Status: incidents.StatusCompleted, Complete: true, Warnings: nil, Incidents: nil}, nil
}
func (r *parityRecorder) ResolveIncident(_ context.Context, request incidents.ResolveRequest) (incidents.Result, error) {
	r.resolveIncident = request
	return incidents.Result{Status: incidents.StatusCompleted, Complete: true, Warnings: nil, Incidents: nil}, nil
}
func (r *parityRecorder) TraceGet(_ context.Context, request trace.Request) (trace.Timeline, error) {
	r.traceGet = request
	return trace.Timeline{Status: trace.StatusCompleted, Complete: true, Events: nil, Steps: nil, Warnings: nil}, nil
}
func (r *parityRecorder) TraceFollow(_ context.Context, request trace.Request, interval time.Duration, emit func(trace.Timeline) error) error {
	r.traceFollow = request
	r.traceInterval = interval
	if emit != nil {
		return emit(trace.Timeline{Status: trace.StatusCompleted, Complete: true, Events: nil, Steps: nil, Warnings: nil})
	}
	return nil
}
func (r *parityRecorder) BackupCreate(_ context.Context, options backup.Options) (backup.Manifest, error) {
	r.backup = options
	return backup.Manifest{Files: []string{}, IncludesSecrets: options.IncludeSecrets}, nil
}

func platformDeps(r *parityRecorder) cli.Dependencies {
	return cli.Dependencies{
		ListIncidents:   r.ListIncidents,
		ResolveIncident: r.ResolveIncident,
		TraceGet:        r.TraceGet,
		TraceFollow:     r.TraceFollow,
		BackupCreate:    r.BackupCreate,
	}
}

func platformAPIDeps(r *parityRecorder) api.Dependencies {
	return api.Dependencies{
		ListIncidents:   r.ListIncidents,
		ResolveIncident: r.ResolveIncident,
		TraceGet:        r.TraceGet,
		TraceFollow:     r.TraceFollow,
		BackupCreate:    r.BackupCreate,
	}
}

func normalizedLint(request toolkit.LintRequest) toolkit.LintRequest {
	request.ProjectDir = ""
	for i := range request.Inputs {
		request.Inputs[i].Path = mustCanonicalPath(request.Inputs[i].Path)
	}
	return request
}

func normalizedDiff(request toolkit.DiffRequest) toolkit.DiffRequest {
	request.ProjectDir = ""
	request.Before.Path = mustCanonicalPath(request.Before.Path)
	request.After.Path = mustCanonicalPath(request.After.Path)
	return request
}

func normalizedExplain(request toolkit.ExplainRequest) toolkit.ExplainRequest {
	request.ProjectDir = ""
	request.Input.Path = mustCanonicalPath(request.Input.Path)
	return request
}

func normalizedReview(request toolkit.ReviewRequest) toolkit.ReviewRequest {
	request.ProjectDir = ""
	for i := range request.Inputs {
		request.Inputs[i].Path = mustCanonicalPath(request.Inputs[i].Path)
	}
	return request
}

func normalizedScan(request toolkit.ScanRequest) toolkit.ScanRequest {
	request.ProjectDir = ""
	for i := range request.Roots {
		request.Roots[i] = mustCanonicalPath(request.Roots[i])
	}
	return request
}

func normalizedGenerate(request toolkit.GenerateRequest) toolkit.GenerateRequest {
	request.ProjectDir = ""
	request.Input.Path = mustCanonicalPath(request.Input.Path)
	request.OutDir = mustCanonicalPath(request.OutDir)
	return request
}

func mustCanonical(t *testing.T, path string) string {
	t.Helper()
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			t.Fatal(err)
		}
		return abs
	}
	return resolved
}

func mustCanonicalPath(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		abs, absErr := filepath.Abs(path)
		if absErr != nil {
			return path
		}
		return abs
	}
	return resolved
}

func mustJSON(value string) string {
	encoded, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(encoded)
}
