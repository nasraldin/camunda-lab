package doctor

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
)

type fakeInspector struct {
	services   []ServiceState
	serviceErr error
	disk       DiskUsage
	diskErr    error
	volumes    []VolumeState
	volumeErr  error
}

func (f fakeInspector) ComposeServices(context.Context, config.Config) ([]ServiceState, error) {
	return f.services, f.serviceErr
}
func (f fakeInspector) DiskUsage(context.Context) (DiskUsage, error) {
	return f.disk, f.diskErr
}
func (f fakeInspector) Volumes(context.Context, string) ([]VolumeState, error) {
	return f.volumes, f.volumeErr
}

type fakeHTTPClient struct {
	status int
	err    error
}

type httpClientFunc func(*http.Request) (*http.Response, error)

func (f httpClientFunc) Do(req *http.Request) (*http.Response, error) {
	return f(req)
}

type environmentFunc func(context.Context) (EnvironmentState, error)

func (f environmentFunc) Current(ctx context.Context) (EnvironmentState, error) {
	return f(ctx)
}

func (f fakeHTTPClient) Do(req *http.Request) (*http.Response, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &http.Response{StatusCode: f.status, Body: http.NoBody, Request: req}, nil
}

type fakeFS struct {
	entries map[string][]DirEntry
	files   map[string][]byte
	stats   map[string]FileInfo
}

func (f fakeFS) Stat(ctx context.Context, path string) (FileInfo, error) {
	if err := ctx.Err(); err != nil {
		return FileInfo{}, err
	}
	info, ok := f.stats[path]
	if !ok {
		return FileInfo{}, errors.New("not found")
	}
	return info, nil
}
func (f fakeFS) ReadDir(ctx context.Context, path string, _ int) ([]DirEntry, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	entries, ok := f.entries[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return entries, nil
}
func (f fakeFS) ReadFile(ctx context.Context, path string, _ int64) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, ok := f.files[path]
	if !ok {
		return nil, errors.New("not found")
	}
	return data, nil
}

func baseOptions(inspector Inspector) DeepOptions {
	return DeepOptions{
		Inspector:   inspector,
		HTTPClient:  fakeHTTPClient{status: http.StatusOK},
		DialContext: func(context.Context, string, string) error { return nil },
		FS: fakeFS{
			stats: map[string]FileInfo{
				"/lab/config.yaml":  {Path: "/lab/config.yaml"},
				"/lab/versions/8.9": {Path: "/lab/versions/8.9", IsDir: true},
			},
			entries: map[string][]DirEntry{"/lab/overlays": {}},
			files:   map[string][]byte{},
		},
		Paths: DiagnosticPaths{
			ConfigFile:  "/lab/config.yaml",
			VersionDir:  "/lab/versions/8.9",
			OverlaysDir: "/lab/overlays",
		},
		Environment:     staticEnvironment{Name: "lab"},
		PerCheckTimeout: time.Second,
		OverallTimeout:  5 * time.Second,
		Now:             func() time.Time { return time.Unix(0, 0) },
	}
}

func TestRunDeepHealthyChecksHaveTypedStableContract(t *testing.T) {
	inspector := fakeInspector{
		services: []ServiceState{{Name: "zeebe", State: "running", Health: "healthy"}},
		disk:     DiskUsage{Percent: 42},
		volumes:  []VolumeState{{Name: "camunda-lab_data", Present: true}},
	}
	result, err := RunDeep(context.Background(), config.Config{
		Version: "8.9", Profile: "modeler", Resources: "balanced",
		Host: "localhost", ComposeProject: "camunda-lab",
	}, baseOptions(inspector))
	if err != nil {
		t.Fatal(err)
	}
	if !result.OK || result.Checks == nil {
		t.Fatalf("result = %+v", result)
	}
	ids := make([]string, 0, len(result.Checks))
	for _, check := range result.Checks {
		ids = append(ids, check.ID)
		if check.ID == "" || check.Category == "" || check.Status == "" ||
			check.Summary == "" || check.Detail == "" || check.Remediation == "" ||
			check.Duration < 0 {
			t.Fatalf("incomplete check: %+v", check)
		}
	}
	if !slices.IsSorted(ids) {
		t.Fatalf("check IDs not deterministic: %v", ids)
	}
	for _, want := range []string{"compose.services", "docker.disk", "docker.volumes", "overlay.consistency"} {
		if !slices.Contains(ids, want) {
			t.Fatalf("missing %q in %v", want, ids)
		}
	}
}

func TestRunDeepDiagnosesComposeStatesAndLabDown(t *testing.T) {
	tests := []struct {
		name     string
		services []ServiceState
		status   Status
		detail   string
	}{
		{"degraded", []ServiceState{{Name: "operate", State: "running", Health: "unhealthy"}}, StatusWarn, "unhealthy"},
		{"exited", []ServiceState{{Name: "operate", State: "exited", ExitCode: 1}}, StatusFail, "exited"},
		{"lab down", []ServiceState{}, StatusFail, "No Compose services"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			opts := baseOptions(fakeInspector{services: tt.services})
			result, err := RunDeep(context.Background(), testConfig(), opts)
			if err != nil {
				t.Fatal(err)
			}
			check := findCheck(t, result, "compose.services")
			if check.Status != tt.status || !strings.Contains(check.Detail, tt.detail) {
				t.Fatalf("check = %+v", check)
			}
			if check.Remediation == "" {
				t.Fatal("missing remediation")
			}
		})
	}
}

func TestRunDeepConvertsProbeFailuresAndTimeoutsToChecks(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.HTTPClient = fakeHTTPClient{err: &url.Error{
		Op: "Get", URL: "https://user:secret@example.test/path?token=abc", Err: context.DeadlineExceeded,
	}}
	opts.DialContext = func(context.Context, string, string) error { return context.DeadlineExceeded }
	result, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	httpCheck := findCheck(t, result, "service.admin")
	if httpCheck.Status != StatusFail || !strings.Contains(httpCheck.Detail, "timed out") {
		t.Fatalf("HTTP check = %+v", httpCheck)
	}
	tcpCheck := findCheck(t, result, "service.grpc")
	if tcpCheck.Status != StatusFail || !strings.Contains(tcpCheck.Detail, "timed out") {
		t.Fatalf("TCP check = %+v", tcpCheck)
	}
	text := result.Text()
	if strings.Contains(text, "secret") || strings.Contains(text, "token=abc") {
		t.Fatalf("secret leaked: %s", text)
	}
}

func TestRunDeepReportsMissingVolumesAndHighDisk(t *testing.T) {
	opts := baseOptions(fakeInspector{
		services: []ServiceState{{Name: "x", State: "running"}},
		disk:     DiskUsage{Percent: 93},
		volumes:  nil,
	})
	result, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := findCheck(t, result, "docker.disk").Status; got != StatusWarn {
		t.Fatalf("disk status = %s", got)
	}
	if got := findCheck(t, result, "docker.volumes").Status; got != StatusWarn {
		t.Fatalf("volume status = %s", got)
	}
}

func TestRunDeepReportsExpectedMissingAndStaleOverlays(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	fs := opts.FS.(fakeFS)
	fs.entries["/lab/overlays"] = []DirEntry{{Name: "stale.yaml"}}
	fs.files["/lab/overlays/stale.yaml"] = []byte("stale")
	opts.FS = fs
	cfg := testConfig()
	cfg.Profile = "full"
	result, err := RunDeep(context.Background(), cfg, opts)
	if err != nil {
		t.Fatal(err)
	}
	check := findCheck(t, result, "overlay.consistency")
	if check.Status != StatusWarn ||
		!strings.Contains(check.Detail, "missing") ||
		!strings.Contains(check.Detail, "stale.yaml") {
		t.Fatalf("overlay check = %+v", check)
	}
}

func TestRunDeepStableTextAndJSONUseEmptyArrays(t *testing.T) {
	result := DeepReport{Checks: []Check{}}
	firstText, secondText := result.Text(), result.Text()
	if firstText != secondText {
		t.Fatal("text output is unstable")
	}
	firstJSON, err := result.JSON()
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, _ := result.JSON()
	if string(firstJSON) != string(secondJSON) {
		t.Fatal("JSON output is unstable")
	}
	var decoded map[string]any
	if err := json.Unmarshal(firstJSON, &decoded); err != nil {
		t.Fatal(err)
	}
	if _, ok := decoded["checks"].([]any); !ok {
		t.Fatalf("checks is not an array: %s", firstJSON)
	}
}

func TestRunDeepInvalidConfigurationIsTypedFatal(t *testing.T) {
	_, err := RunDeep(context.Background(), config.Config{Version: "9.9", Profile: "invalid"}, baseOptions(fakeInspector{}))
	var fatal *FatalError
	if !errors.As(err, &fatal) || fatal.Code != "invalid_configuration" {
		t.Fatalf("error = %#v", err)
	}
}

func TestRequiredFailuresControlAggregate(t *testing.T) {
	report := DeepReport{Checks: []Check{
		{ID: "warn", Status: StatusWarn, Required: true},
		{ID: "skip", Status: StatusSkipped, Required: true},
		{ID: "optional", Status: StatusFail, Required: false},
	}}
	report.Aggregate()
	if !report.OK {
		t.Fatalf("warnings/skips/optional failures should not fail: %+v", report)
	}
	report.Checks = append(report.Checks, Check{ID: "required", Status: StatusFail, Required: true})
	report.Aggregate()
	if report.OK {
		t.Fatal("required failure did not fail aggregate")
	}
}

func TestRunDeepAuthenticatesConfiguredRemoteEnvironment(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.Environment = environmentFunc(func(context.Context) (EnvironmentState, error) {
		return EnvironmentState{
			Name: "prod", Kind: "remote", Endpoint: "https://cluster.example/v2",
			AuthConfigured: true, clientID: "id", clientSecret: "secret",
			tokenURL: "https://identity.example/token",
		}, nil
	})
	var mu sync.Mutex
	var calls []string
	opts.HTTPClient = httpClientFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, req.Method+" "+req.URL.Host)
		if req.URL.Host == "identity.example" {
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"token_type":"Bearer","access_token":"top-secret","expires_in":300}`)),
			}, nil
		}
		if req.URL.Host == "cluster.example" && req.Header.Get("Authorization") != "Bearer top-secret" {
			t.Fatalf("authorization header = %q", req.Header.Get("Authorization"))
		}
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	result, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if got := findCheck(t, result, "remote.auth").Status; got != StatusPass {
		t.Fatalf("remote auth status = %s", got)
	}
	if got := findCheck(t, result, "remote.reachability").Status; got != StatusPass {
		t.Fatalf("remote reachability status = %s", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if !slices.Contains(calls, "POST identity.example") ||
		!slices.Contains(calls, "POST cluster.example") {
		t.Fatalf("remote calls = %v", calls)
	}
}

func TestRunDeepBoundsChecksAndPropagatesCancellation(t *testing.T) {
	inspector := blockingInspector{}
	opts := baseOptions(inspector)
	opts.PerCheckTimeout = 10 * time.Millisecond
	opts.OverallTimeout = 25 * time.Millisecond
	started := time.Now()
	result, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(started); elapsed > 250*time.Millisecond {
		t.Fatalf("deep diagnostics exceeded bound: %s", elapsed)
	}
	if got := findCheck(t, result, "compose.services"); got.Status != StatusFail ||
		!strings.Contains(got.Detail, "timed out") {
		t.Fatalf("compose timeout = %+v", got)
	}
}

type blockingInspector struct{}

func (blockingInspector) ComposeServices(ctx context.Context, _ config.Config) ([]ServiceState, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (blockingInspector) DiskUsage(ctx context.Context) (DiskUsage, error) {
	<-ctx.Done()
	return DiskUsage{}, ctx.Err()
}
func (blockingInspector) Volumes(ctx context.Context, _ string) ([]VolumeState, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestDecodeComposeServicesSupportsArrayAndJSONLines(t *testing.T) {
	array := []byte(`[{"Service":"operate","State":"running","Health":"healthy"},{"Service":"zeebe","State":"exited","ExitCode":2}]`)
	lines := []byte("{\"Service\":\"operate\",\"State\":\"running\",\"Health\":\"healthy\"}\n{\"Service\":\"zeebe\",\"State\":\"exited\",\"ExitCode\":2}\n")
	for _, input := range [][]byte{array, lines} {
		services, err := decodeComposeServices(input)
		if err != nil {
			t.Fatal(err)
		}
		if len(services) != 2 || services[1].ExitCode != 2 {
			t.Fatalf("services = %+v", services)
		}
	}
}

func TestDecodeDockerDiskUsageFindsWarningPercentage(t *testing.T) {
	usage, err := decodeDiskUsage([]byte(`{"Type":"Local Volumes","Reclaimable":"12GB (93%)"}`))
	if err != nil {
		t.Fatal(err)
	}
	if usage.Percent != 93 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestSanitizeRedactsBearerAndURLCredentials(t *testing.T) {
	got := sanitize(`request failed: Authorization: Bearer abc.def.ghi at https://user:pass@example.test/path?access_token=secret`)
	for _, secret := range []string{"abc.def.ghi", "user", "pass", "secret"} {
		if strings.Contains(got, secret) {
			t.Fatalf("sanitize leaked %q in %q", secret, got)
		}
	}
}

func testConfig() config.Config {
	return config.Config{
		Version: "8.9", Profile: "light", Resources: "balanced",
		Host: "localhost", ComposeProject: "camunda-lab",
	}
}

func findCheck(t *testing.T, report DeepReport, id string) Check {
	t.Helper()
	for _, check := range report.Checks {
		if check.ID == id {
			return check
		}
	}
	t.Fatalf("missing check %q in %+v", id, report.Checks)
	return Check{}
}
