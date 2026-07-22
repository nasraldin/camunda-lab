package doctor

import (
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type cancelAwareFS struct{}

func (cancelAwareFS) Stat(ctx context.Context, _ string) (FileInfo, error) {
	<-ctx.Done()
	return FileInfo{}, ctx.Err()
}
func (cancelAwareFS) ReadDir(ctx context.Context, _ string, _ int) ([]DirEntry, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}
func (cancelAwareFS) ReadFile(ctx context.Context, _ string, _ int64) ([]byte, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestFilesystemChecksHonorPerCheckContext(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.FS = cancelAwareFS{}
	opts.PerCheckTimeout = 5 * time.Millisecond
	opts.OverallTimeout = 100 * time.Millisecond

	start := time.Now()
	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if elapsed := time.Since(start); elapsed > 250*time.Millisecond {
		t.Fatalf("filesystem checks exceeded context bound: %s", elapsed)
	}
	check := findCheck(t, report, "filesystem.config")
	if check.Status != StatusFail || !strings.Contains(check.Detail, "timed out") {
		t.Fatalf("filesystem check = %+v", check)
	}
}

type overlayBlockingFS struct {
	fakeFS
}

func (f overlayBlockingFS) ReadDir(ctx context.Context, _ string, _ int) ([]DirEntry, error) {
	<-ctx.Done()
	return nil, ctx.Err()
}

func TestOverlayCheckHonorsPerCheckContext(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.FS = overlayBlockingFS{fakeFS: opts.FS.(fakeFS)}
	opts.PerCheckTimeout = 5 * time.Millisecond
	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	check := findCheck(t, report, "overlay.consistency")
	if check.Status != StatusWarn || !strings.Contains(check.Detail, "timed out") {
		t.Fatalf("overlay check = %+v", check)
	}
}

func TestInvalidEnvironmentIsFatalAndPreservesCause(t *testing.T) {
	cause := errors.New("selected profile is malformed")
	opts := baseOptions(fakeInspector{})
	opts.Environment = environmentFunc(func(context.Context) (EnvironmentState, error) {
		return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorInvalid, Err: cause}
	})

	_, err := RunDeep(context.Background(), testConfig(), opts)
	var fatal *FatalError
	if !errors.As(err, &fatal) || fatal.Code != "invalid_environment" {
		t.Fatalf("error = %#v", err)
	}
	if !errors.Is(err, cause) {
		t.Fatalf("fatal error lost cause: %v", err)
	}
}

func TestRuntimeEnvironmentFailureIsDiagnostic(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.Environment = environmentFunc(func(context.Context) (EnvironmentState, error) {
		return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorRuntime, Err: context.DeadlineExceeded}
	})

	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatalf("runtime environment error became fatal: %v", err)
	}
	check := findCheck(t, report, "environment.active")
	if check.Status != StatusFail || !strings.Contains(check.Detail, "timed out") {
		t.Fatalf("environment check = %+v", check)
	}
}

func TestMalformedRemoteEnvironmentEndpointIsFatal(t *testing.T) {
	opts := baseOptions(fakeInspector{})
	opts.Environment = environmentFunc(func(context.Context) (EnvironmentState, error) {
		return EnvironmentState{
			Name: "prod", Kind: "remote", Endpoint: "://bad endpoint",
			AuthConfigured: true,
		}, nil
	})
	_, err := RunDeep(context.Background(), testConfig(), opts)
	var fatal *FatalError
	if !errors.As(err, &fatal) || fatal.Code != "invalid_environment" {
		t.Fatalf("error = %#v", err)
	}
}

type authFunc func(context.Context, EnvironmentState) (RemoteCredential, error)

func (f authFunc) Authenticate(ctx context.Context, state EnvironmentState) (RemoteCredential, error) {
	return f(ctx, state)
}

type reachabilityFunc func(context.Context, EnvironmentState, RemoteCredential) error

func (f reachabilityFunc) Probe(ctx context.Context, state EnvironmentState, credential RemoteCredential) error {
	return f(ctx, state, credential)
}

func TestRemoteAuthAndReachabilityAreSeparateChecks(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.Environment = environmentFunc(func(context.Context) (EnvironmentState, error) {
		return EnvironmentState{Name: "prod", Kind: "remote", Endpoint: "https://cluster.example/v2", AuthConfigured: true}, nil
	})
	opts.RemoteAuth = authFunc(func(context.Context, EnvironmentState) (RemoteCredential, error) {
		return RemoteCredential{BearerToken: "redacted-token"}, nil
	})
	opts.RemoteReachability = reachabilityFunc(func(_ context.Context, _ EnvironmentState, credential RemoteCredential) error {
		if credential.BearerToken == "" {
			t.Fatal("reachability did not receive acquired credential")
		}
		return nil
	})

	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if findCheck(t, report, "remote.auth").Status != StatusPass {
		t.Fatal("remote auth did not pass")
	}
	if findCheck(t, report, "remote.reachability").Status != StatusPass {
		t.Fatal("remote reachability did not pass")
	}
}

func TestPublic404DoesNotProveRemoteAuthOrReachability(t *testing.T) {
	client := httpClientFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusNotFound, Body: http.NoBody}, nil
	})
	state := EnvironmentState{
		Name: "prod", Kind: "remote", Endpoint: "https://cluster.example/v2",
		AuthConfigured: true, accessToken: "token",
	}
	credential, err := (httpRemoteAuthenticator{Client: client}).Authenticate(context.Background(), state)
	if err != nil {
		t.Fatal(err)
	}
	err = (httpRemoteReachability{Client: client}).Probe(context.Background(), state, credential)
	if err == nil {
		t.Fatal("404 was accepted as authenticated reachability")
	}
}

func TestRemoteReachabilityUsesNormalizedV2Base(t *testing.T) {
	var gotPath string
	client := httpClientFunc(func(req *http.Request) (*http.Response, error) {
		gotPath = req.URL.Path
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
	state := EnvironmentState{
		Name: "prod", Kind: "remote", Endpoint: "https://cluster.example/",
		AuthConfigured: true,
	}
	err := (httpRemoteReachability{Client: client}).Probe(
		context.Background(), state, RemoteCredential{BearerToken: "token"},
	)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/v2/process-definitions/search" {
		t.Fatalf("protected path = %q", gotPath)
	}
}

func TestRemoteAuthFailureMakesReachabilityFailNotSkip(t *testing.T) {
	opts := baseOptions(fakeInspector{services: []ServiceState{{Name: "x", State: "running"}}})
	opts.Environment = environmentFunc(func(context.Context) (EnvironmentState, error) {
		return EnvironmentState{
			Name: "prod", Kind: "remote", Endpoint: "https://cluster.example/v2",
			AuthConfigured: true,
		}, nil
	})
	opts.RemoteAuth = authFunc(func(context.Context, EnvironmentState) (RemoteCredential, error) {
		return RemoteCredential{}, errors.New("token rejected")
	})
	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if findCheck(t, report, "remote.auth").Status != StatusFail {
		t.Fatal("remote auth failure was not reported")
	}
	if findCheck(t, report, "remote.reachability").Status != StatusFail {
		t.Fatal("remote reachability was skipped after auth failure")
	}
}

func TestComposeInspectionErrorStillRunsEndpointProbes(t *testing.T) {
	opts := baseOptions(fakeInspector{serviceErr: errors.New("daemon unavailable")})
	var calls atomic.Int32
	opts.HTTPClient = httpClientFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody, Request: req}, nil
	})
	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() == 0 {
		t.Fatal("endpoint probes were skipped after unknown Compose state")
	}
	if findCheck(t, report, "service.admin").Status == StatusSkipped {
		t.Fatal("endpoint probe was marked skipped")
	}
}

func TestComposeStatusRequiresRunningState(t *testing.T) {
	tests := []struct {
		state  string
		health string
		want   Status
	}{
		{state: "running", health: "healthy", want: StatusPass},
		{state: "created", want: StatusWarn},
		{state: "paused", health: "healthy", want: StatusWarn},
		{state: "restarting", want: StatusWarn},
		{state: "exited", health: "healthy", want: StatusFail},
		{state: "dead", want: StatusFail},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			got, _ := composeStatus([]ServiceState{{Name: "svc", State: tt.state, Health: tt.health}})
			if got != tt.want {
				t.Fatalf("composeStatus(%s/%s) = %s, want %s", tt.state, tt.health, got, tt.want)
			}
		})
	}
}

type commandRunnerFunc func(context.Context, int64, string, ...string) (CommandOutput, error)

func (f commandRunnerFunc) Run(ctx context.Context, limit int64, name string, args ...string) (CommandOutput, error) {
	return f(ctx, limit, name, args...)
}

func TestDockerInspectorReturnsExplicitOutputOverflow(t *testing.T) {
	runner := commandRunnerFunc(func(context.Context, int64, string, ...string) (CommandOutput, error) {
		return CommandOutput{Stdout: []byte(`{"Service":"x"}`), Overflow: true}, ErrCommandOutputOverflow
	})
	_, err := (DockerInspector{Commands: runner, OutputLimit: 16}).ComposeServices(context.Background(), testConfig())
	if !errors.Is(err, ErrCommandOutputOverflow) {
		t.Fatalf("overflow error = %v", err)
	}
}

func TestDeepReportsCommandOutputOverflowPerCheck(t *testing.T) {
	runner := commandRunnerFunc(func(context.Context, int64, string, ...string) (CommandOutput, error) {
		return CommandOutput{Stdout: []byte("truncated"), Overflow: true}, ErrCommandOutputOverflow
	})
	opts := baseOptions(DockerInspector{Commands: runner, OutputLimit: 16})
	report, err := RunDeep(context.Background(), testConfig(), opts)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"compose.services", "docker.disk", "docker.volumes"} {
		check := findCheck(t, report, id)
		if check.Status != StatusFail || !strings.Contains(check.Detail, ErrCommandOutputOverflow.Error()) {
			t.Fatalf("%s did not report overflow: %+v", id, check)
		}
	}
}

func TestBoundedBufferBoundaryAndOverflow(t *testing.T) {
	buffer := newBoundedBuffer(4)
	if _, err := io.WriteString(buffer, "abcd"); err != nil {
		t.Fatal(err)
	}
	if buffer.Overflow() {
		t.Fatal("exact boundary overflowed")
	}
	if _, err := io.WriteString(buffer, "e"); err != nil {
		t.Fatal(err)
	}
	if !buffer.Overflow() || string(buffer.Bytes()) != "abcd" {
		t.Fatalf("buffer = %q overflow=%v", buffer.Bytes(), buffer.Overflow())
	}
}

func TestShallowDoctorTreatsCommandOverflowAsIssue(t *testing.T) {
	runner := commandRunnerFunc(func(context.Context, int64, string, ...string) (CommandOutput, error) {
		return CommandOutput{Overflow: true}, ErrCommandOutputOverflow
	})
	report := runWithCommands(false, runner)
	if report.OK {
		t.Fatal("shallow doctor accepted overflowing Docker command output")
	}
}

func TestExecCommandRunnerBoundsOutputAndPreservesCancellation(t *testing.T) {
	t.Setenv("DOCTOR_HELPER_BYTES", "4")
	out, err := (execCommandRunner{}).Run(context.Background(), 4, os.Args[0], "-test.run=TestDoctorCommandHelper")
	if err != nil || out.Overflow || string(out.Stdout) != "xxxx" {
		t.Fatalf("boundary output = %+v, err=%v", out, err)
	}

	t.Setenv("DOCTOR_HELPER_BYTES", "5")
	out, err = (execCommandRunner{}).Run(context.Background(), 4, os.Args[0], "-test.run=TestDoctorCommandHelper")
	if !errors.Is(err, ErrCommandOutputOverflow) || !out.Overflow || string(out.Stdout) != "xxxx" {
		t.Fatalf("overflow output = %+v, err=%v", out, err)
	}

	t.Setenv("DOCTOR_HELPER_STDERR", "1")
	out, err = (execCommandRunner{}).Run(context.Background(), 4, os.Args[0], "-test.run=TestDoctorCommandHelper")
	if !errors.Is(err, ErrCommandOutputOverflow) || !out.Overflow || string(out.Stderr) != "xxxx" {
		t.Fatalf("stderr overflow output = %+v, err=%v", out, err)
	}
	t.Setenv("DOCTOR_HELPER_STDERR", "")

	t.Setenv("DOCTOR_HELPER_BYTES", "-1")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err = (execCommandRunner{}).Run(ctx, 4, os.Args[0], "-test.run=TestDoctorCommandHelper")
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("command cancellation identity lost: %v", err)
	}

	t.Setenv("DOCTOR_HELPER_BYTES", "5")
	signalPath := t.TempDir() + "/output-written"
	t.Setenv("DOCTOR_HELPER_SIGNAL", signalPath)
	controlled := newControlledDeadlineContext()
	triggered := make(chan struct{})
	go func() {
		defer close(triggered)
		deadline := time.After(5 * time.Second)
		for {
			if _, statErr := os.Stat(signalPath); statErr == nil {
				controlled.Trigger(context.DeadlineExceeded)
				return
			}
			select {
			case <-deadline:
				controlled.Trigger(context.DeadlineExceeded)
				return
			case <-time.After(time.Millisecond):
			}
		}
	}()
	out, err = (execCommandRunner{}).Run(controlled, 4, os.Args[0], "-test.run=TestDoctorCommandHelper")
	<-triggered
	if !out.Overflow ||
		!errors.Is(err, context.DeadlineExceeded) ||
		!errors.Is(err, ErrCommandOutputOverflow) {
		t.Fatalf("noisy timeout output = %+v, err=%v", out, err)
	}
}

func TestDoctorCommandHelper(t *testing.T) {
	raw := os.Getenv("DOCTOR_HELPER_BYTES")
	if raw == "" {
		return
	}
	count, err := strconv.Atoi(raw)
	if err != nil {
		os.Exit(2)
	}
	if count < 0 {
		time.Sleep(time.Second)
		os.Exit(0)
	}
	output := os.Stdout
	if os.Getenv("DOCTOR_HELPER_STDERR") != "" {
		output = os.Stderr
	}
	_, _ = output.WriteString(strings.Repeat("x", count))
	if signal := os.Getenv("DOCTOR_HELPER_SIGNAL"); signal != "" {
		_ = os.WriteFile(signal, []byte("ready"), 0o600)
		time.Sleep(5 * time.Second)
	}
	os.Exit(0)
}

type controlledDeadlineContext struct {
	mu   sync.Mutex
	done chan struct{}
	err  error
	once sync.Once
}

func newControlledDeadlineContext() *controlledDeadlineContext {
	return &controlledDeadlineContext{done: make(chan struct{})}
}

func (c *controlledDeadlineContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *controlledDeadlineContext) Done() <-chan struct{}       { return c.done }
func (c *controlledDeadlineContext) Value(any) any               { return nil }
func (c *controlledDeadlineContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}
func (c *controlledDeadlineContext) Trigger(err error) {
	c.once.Do(func() {
		c.mu.Lock()
		c.err = err
		c.mu.Unlock()
		close(c.done)
	})
}
