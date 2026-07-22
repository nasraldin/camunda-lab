package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

func TestToolkitExecutionCapturesNormalizedDomainRequests(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validCLIBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte("name: toolkit\ncamundaVersion: \"8.9\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	outDir := filepath.Join(root, "tests")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)

	recorder := &recordingToolkit{}
	var planReq plan.Request
	var driftReq drift.Request
	var listReq incidents.ListRequest
	var resolveReq incidents.ResolveRequest
	var traceReq trace.Request
	var backupOpts backup.Options
	deps := Dependencies{
		Toolkit: recorder,
		Plan: func(_ context.Context, request plan.Request) (plan.Result, error) {
			planReq = request
			return plan.Result{}, nil
		},
		Drift: func(_ context.Context, request drift.Request) (drift.Report, error) {
			driftReq = request
			return drift.Report{}, nil
		},
		ListIncidents: func(_ context.Context, request incidents.ListRequest) (incidents.Result, error) {
			listReq = request
			return incidents.Result{Status: incidents.StatusCompleted, Complete: true}, nil
		},
		ResolveIncident: func(_ context.Context, request incidents.ResolveRequest) (incidents.Result, error) {
			resolveReq = request
			return incidents.Result{Status: incidents.StatusCompleted, Complete: true}, nil
		},
		TraceFollow: func(_ context.Context, request trace.Request, _ time.Duration, emit func(trace.Timeline) error) error {
			traceReq = request
			if emit != nil {
				return emit(trace.Timeline{Status: trace.StatusCompleted, Complete: true})
			}
			return nil
		},
		BackupCreate: func(_ context.Context, options backup.Options) (backup.Manifest, error) {
			backupOpts = options
			return backup.Manifest{Files: []string{}, IncludesSecrets: options.IncludeSecrets}, nil
		},
	}

	tests := []struct {
		name   string
		args   []string
		assert func(t *testing.T)
	}{
		{
			name: "lint",
			args: []string{"lint", path, "--fail-on", "warning", "--ignore", "bpmn/rule", "--json"},
			assert: func(t *testing.T) {
				want := toolkit.LintRequest{
					Inputs: []toolkit.BPMNInput{{Name: path, Path: path}},
					FailOn: toolkit.LintThresholdWarning,
					Ignore: []string{"bpmn/rule"},
				}
				if !reflect.DeepEqual(normalizeLint(recorder.lint), normalizeLint(want)) {
					t.Fatalf("lint request = %+v, want %+v", recorder.lint, want)
				}
			},
		},
		{
			name: "diff",
			args: []string{"diff", "--from", path, "--to", path, "--json"},
			assert: func(t *testing.T) {
				want := toolkit.DiffRequest{
					Before: toolkit.BPMNInput{Name: path, Path: path},
					After:  toolkit.BPMNInput{Name: path, Path: path},
				}
				if !reflect.DeepEqual(recorder.diff, want) {
					t.Fatalf("diff request = %+v, want %+v", recorder.diff, want)
				}
			},
		},
		{
			name: "explain",
			args: []string{"explain", path, "--json"},
			assert: func(t *testing.T) {
				want := toolkit.ExplainRequest{Input: toolkit.BPMNInput{Name: path, Path: path}}
				if !reflect.DeepEqual(recorder.explain, want) {
					t.Fatalf("explain request = %+v, want %+v", recorder.explain, want)
				}
			},
		},
		{
			name: "review",
			args: []string{"review", path, "--fail-on", "warning", "--ignore", "bpmn/rule", "--json"},
			assert: func(t *testing.T) {
				want := toolkit.ReviewRequest{
					Inputs: []toolkit.BPMNInput{{Name: path, Path: path}},
					FailOn: toolkit.LintThresholdWarning,
					Ignore: []string{"bpmn/rule"},
				}
				got := recorder.review
				got.ProjectDir = ""
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("review request = %+v, want %+v", recorder.review, want)
				}
			},
		},
		{
			name: "generate",
			args: []string{"test", "generate", path, "--lang", "python", "--output", outDir, "--force", "--json"},
			assert: func(t *testing.T) {
				want := toolkit.GenerateRequest{
					Input:  toolkit.BPMNInput{Name: path, Path: path},
					OutDir: outDir,
					Lang:   toolkit.GenerateLanguagePython,
					Force:  true,
				}
				if !reflect.DeepEqual(recorder.generate, want) {
					t.Fatalf("generate request = %+v, want %+v", recorder.generate, want)
				}
			},
		},
		{
			name: "scan",
			args: []string{"scan", root, "--fail-on", "low", "--ignore", "*.lock", "--json"},
			assert: func(t *testing.T) {
				want := toolkit.ScanRequest{
					Roots:  []string{root},
					FailOn: toolkit.ScanThresholdLow,
					Ignore: []string{"*.lock"},
				}
				if !reflect.DeepEqual(recorder.scan, want) {
					t.Fatalf("scan request = %+v, want %+v", recorder.scan, want)
				}
			},
		},
		{
			name: "plan",
			args: []string{"plan", "--dir", root, "--env", "staging", "--json"},
			assert: func(t *testing.T) {
				if planReq.Environment != "staging" {
					t.Fatalf("plan request = %+v", planReq)
				}
				if canonical(t, planReq.ProjectRoot) != canonical(t, root) {
					t.Fatalf("plan project = %q, want %q", planReq.ProjectRoot, root)
				}
			},
		},
		{
			name: "drift",
			args: []string{"drift", "--dir", root, "--ref", "HEAD~1", "--env", "staging", "--json"},
			assert: func(t *testing.T) {
				if driftReq.GitRef != "HEAD~1" || driftReq.Environment != "staging" {
					t.Fatalf("drift request = %+v", driftReq)
				}
				if canonical(t, driftReq.ProjectRoot) != canonical(t, root) {
					t.Fatalf("drift project = %q, want %q", driftReq.ProjectRoot, root)
				}
			},
		},
		{
			name: "incidents-list",
			args: []string{"incidents", "list", "--limit", "25", "--env", "staging"},
			assert: func(t *testing.T) {
				want := incidents.ListRequest{
					Environment: "staging", ProjectRoot: root, Limit: 25,
					Filter: incidents.ListFilter{State: "ACTIVE"},
				}
				got := listReq
				got.ProjectRoot = canonical(t, got.ProjectRoot)
				want.ProjectRoot = canonical(t, want.ProjectRoot)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("incidents list = %+v, want %+v", listReq, want)
				}
			},
		},
		{
			name: "incidents-retry-dry-run",
			args: []string{"incidents", "retry", "9007199254740994", "--dry-run", "--env", "staging"},
			assert: func(t *testing.T) {
				want := incidents.ResolveRequest{
					Environment: "staging", ProjectRoot: root, Key: "9007199254740994", DryRun: true,
				}
				got := resolveReq
				got.ProjectRoot = canonical(t, got.ProjectRoot)
				want.ProjectRoot = canonical(t, want.ProjectRoot)
				if !reflect.DeepEqual(got, want) {
					t.Fatalf("incidents resolve = %+v, want %+v", resolveReq, want)
				}
			},
		},
		{
			name: "trace-follow",
			args: []string{"trace", "9007199254740993", "--follow", "--timeout", "30s", "--max-events", "20"},
			assert: func(t *testing.T) {
				if traceReq.ProcessInstanceKey != "9007199254740993" ||
					traceReq.Timeout != 30*time.Second || traceReq.MaxEvents != 20 {
					t.Fatalf("trace follow = %+v", traceReq)
				}
			},
		},
		{
			name: "backup-create",
			args: []string{"backup", "--output", filepath.Join(root, "out.tar.gz")},
			assert: func(t *testing.T) {
				if backupOpts.OutPath == "" || backupOpts.IncludeSecrets {
					t.Fatalf("backup options = %+v", backupOpts)
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			*recorder = recordingToolkit{}
			planReq, driftReq = plan.Request{}, drift.Request{}
			listReq, resolveReq, traceReq = incidents.ListRequest{}, incidents.ResolveRequest{}, trace.Request{}
			backupOpts = backup.Options{}
			command := NewRootWithDependencies(deps)
			var stdout, stderr bytes.Buffer
			command.SetOut(&stdout)
			command.SetErr(&stderr)
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil && ExitCode(err) > 1 {
				t.Fatalf("execute: %v (stderr=%s)", err, stderr.String())
			}
			test.assert(t)
		})
	}
}

type recordingToolkit struct {
	lint     toolkit.LintRequest
	diff     toolkit.DiffRequest
	explain  toolkit.ExplainRequest
	review   toolkit.ReviewRequest
	generate toolkit.GenerateRequest
	scan     toolkit.ScanRequest
}

func (r *recordingToolkit) Lint(_ context.Context, request toolkit.LintRequest) (toolkit.LintResult, error) {
	r.lint = request
	return toolkit.LintResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Findings: []toolkit.LintFinding{}, Inputs: []string{}}, nil
}

func (r *recordingToolkit) Diff(_ context.Context, request toolkit.DiffRequest) (toolkit.DiffResult, error) {
	r.diff = request
	return toolkit.DiffResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Changes: []toolkit.ProcessChange{}}, nil
}

func (r *recordingToolkit) Explain(_ context.Context, request toolkit.ExplainRequest) (toolkit.ExplainResult, error) {
	r.explain = request
	return toolkit.ExplainResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Processes: []toolkit.ProcessExplanation{}}, nil
}

func (r *recordingToolkit) Review(_ context.Context, request toolkit.ReviewRequest) (toolkit.ReviewResult, error) {
	r.review = request
	return toolkit.ReviewResult{
		Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Inputs: []string{},
		Processes: []toolkit.ProcessReview{}, Findings: []toolkit.LintFinding{}, AIStatus: toolkit.AIStatusDisabled,
	}, nil
}

func (r *recordingToolkit) Generate(_ context.Context, request toolkit.GenerateRequest) (toolkit.GenerateResult, error) {
	r.generate = request
	return toolkit.GenerateResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Artifacts: []toolkit.Artifact{}}, nil
}

func (r *recordingToolkit) Scan(_ context.Context, request toolkit.ScanRequest) (toolkit.ScanResult, error) {
	r.scan = request
	return toolkit.ScanResult{Status: toolkit.StatusCompleted, Complete: true, Warnings: []toolkit.Warning{}, Findings: nil, Issues: nil}, nil
}

func normalizeLint(request toolkit.LintRequest) toolkit.LintRequest {
	request.ProjectDir = ""
	return request
}

func canonical(t *testing.T, path string) string {
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
