package doctor

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/config"
	labenv "github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/overlay"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/urls"
	"github.com/nasraldin/camunda-lab/internal/versions"
)

type HTTPClient interface {
	// Implementations must honor req.Context. The production http.Client does.
	Do(*http.Request) (*http.Response, error)
}

type DiagnosticPaths struct {
	ConfigFile  string
	VersionDir  string
	OverlaysDir string
}

type EnvironmentState struct {
	Name            string
	Kind            string
	Endpoint        string
	AuthConfigured  bool
	accessToken     string
	clientID        string
	clientSecret    string
	tokenURL        string
	audience        string
	scope           string
	clientIDEnv     string
	clientSecretEnv string
	tokenURLEnv     string
}

type EnvironmentProvider interface {
	Current(context.Context) (EnvironmentState, error)
}

type EnvironmentErrorKind string

const (
	EnvironmentErrorInvalid EnvironmentErrorKind = "invalid"
	EnvironmentErrorRuntime EnvironmentErrorKind = "runtime"
)

type EnvironmentError struct {
	Kind EnvironmentErrorKind
	Err  error
}

func (e *EnvironmentError) Error() string {
	return e.Err.Error()
}

func (e *EnvironmentError) Unwrap() error {
	return e.Err
}

type staticEnvironment struct {
	Name           string
	Kind           string
	Endpoint       string
	AuthConfigured bool
}

func (s staticEnvironment) Current(context.Context) (EnvironmentState, error) {
	kind := s.Kind
	if kind == "" {
		kind = "lab"
	}
	return EnvironmentState{
		Name: s.Name, Kind: kind, Endpoint: s.Endpoint,
		AuthConfigured: s.AuthConfigured,
	}, nil
}

// DeepOptions configures deep probes.
type DeepOptions struct {
	PerCheckTimeout    time.Duration
	OverallTimeout     time.Duration
	Inspector          Inspector
	HTTPClient         HTTPClient
	DialContext        func(context.Context, string, string) error
	FS                 FileSystem
	Paths              DiagnosticPaths
	Environment        EnvironmentProvider
	RemoteAuth         RemoteAuthenticator
	RemoteReachability RemoteReachability
	Now                func() time.Time
}

// RunDeep performs bounded read-only diagnostics. Probe failures are checks;
// only invalid configuration needed to select checks is returned as fatal.
func RunDeep(ctx context.Context, cfg config.Config, opts DeepOptions) (DeepReport, error) {
	if err := versions.ValidateMinor(cfg.Version); err != nil {
		return DeepReport{}, &FatalError{Code: "invalid_configuration", Message: sanitize(err.Error()), Err: err}
	}
	if err := versions.ValidateProfile(cfg.Profile); err != nil {
		return DeepReport{}, &FatalError{Code: "invalid_configuration", Message: sanitize(err.Error()), Err: err}
	}
	if err := overlay.ValidateResources(cfg.Resources); err != nil {
		return DeepReport{}, &FatalError{Code: "invalid_configuration", Message: sanitize(err.Error()), Err: err}
	}
	if _, err := overlay.ExpectedFiles(cfg.Version, cfg.Profile, cfg.AI.Enabled, cfg.Monitoring.Enabled); err != nil {
		return DeepReport{}, &FatalError{Code: "invalid_configuration", Message: sanitize(err.Error()), Err: err}
	}
	if strings.TrimSpace(cfg.ComposeProject) == "" {
		err := fmt.Errorf("compose project must not be empty")
		return DeepReport{}, &FatalError{Code: "invalid_configuration", Message: err.Error(), Err: err}
	}
	perCheck := opts.PerCheckTimeout
	if perCheck <= 0 {
		perCheck = 5 * time.Second
	}
	overall := opts.OverallTimeout
	if overall <= 0 {
		overall = 30 * time.Second
	}
	if perCheck > overall {
		perCheck = overall
	}
	ctx, cancel := context.WithTimeout(ctx, overall)
	defer cancel()

	now := opts.Now
	if now == nil {
		now = time.Now
	}
	inspector := opts.Inspector
	if inspector == nil {
		inspector = DockerInspector{}
	}
	fs := opts.FS
	if fs == nil {
		fs = osFileSystem{}
	}
	diagnosticPaths := opts.Paths
	if diagnosticPaths.ConfigFile == "" {
		diagnosticPaths.ConfigFile = paths.ConfigFile()
	}
	if diagnosticPaths.VersionDir == "" {
		diagnosticPaths.VersionDir = paths.VersionDir(cfg.Version)
	}
	if diagnosticPaths.OverlaysDir == "" {
		diagnosticPaths.OverlaysDir = paths.OverlaysDir()
	}
	httpClient := opts.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{
			Timeout: perCheck,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	dial := opts.DialContext
	if dial == nil {
		dialer := &net.Dialer{Timeout: perCheck}
		dial = func(ctx context.Context, network, address string) error {
			conn, err := dialer.DialContext(ctx, network, address)
			if err != nil {
				return err
			}
			return conn.Close()
		}
	}
	environment := opts.Environment
	if environment == nil {
		environment = osEnvironmentProvider{}
	}

	var checks []Check
	run := func(check Check, fn func(context.Context) (Status, string)) {
		start := now()
		checkCtx, checkCancel := context.WithTimeout(ctx, perCheck)
		status, detail := StatusFail, ""
		if err := checkCtx.Err(); err != nil {
			detail = probeError(checkCtx, "Diagnostic check did not start", err)
		} else {
			status, detail = fn(checkCtx)
		}
		checkCancel()
		check.Status = status
		check.Detail = nonEmpty(sanitize(detail), "No diagnostic detail was returned")
		check.Remediation = nonEmpty(check.Remediation, "No action is required")
		check.Duration = nonNegative(now().Sub(start))
		checks = append(checks, check)
	}

	run(Check{
		ID: "config.active", Category: "configuration", Summary: "Lab configuration is valid",
		Remediation: "Correct the lab version/profile/resources configuration", Required: true,
	}, func(context.Context) (Status, string) {
		return StatusPass, fmt.Sprintf("version %s, profile %s, resources %s", cfg.Version, cfg.Profile, cfg.Resources)
	})
	run(fileCheck("filesystem.config", "Lab configuration file", diagnosticPaths.ConfigFile, false), func(checkCtx context.Context) (Status, string) {
		return inspectPath(checkCtx, fs, diagnosticPaths.ConfigFile, false)
	})
	run(fileCheck("filesystem.version", "Distribution directory", diagnosticPaths.VersionDir, true), func(checkCtx context.Context) (Status, string) {
		return inspectPath(checkCtx, fs, diagnosticPaths.VersionDir, true)
	})

	var services []ServiceState
	composeKnown := false
	run(Check{
		ID: "compose.services", Category: "compose", Summary: "Compose service state",
		Remediation: "Run `camunda up`, then `camunda wait`; inspect failures with `camunda logs`", Required: true,
	}, func(checkCtx context.Context) (Status, string) {
		var err error
		services, err = inspector.ComposeServices(checkCtx, cfg)
		if err != nil {
			return StatusFail, probeError(checkCtx, "Compose inspection failed", err)
		}
		composeKnown = true
		return composeStatus(services)
	})
	run(Check{
		ID: "docker.disk", Category: "docker", Summary: "Docker disk usage",
		Remediation: "Review Docker disk usage and prune only resources you no longer need", Required: false,
	}, func(checkCtx context.Context) (Status, string) {
		usage, err := inspector.DiskUsage(checkCtx)
		if err != nil {
			if errors.Is(err, ErrCommandOutputOverflow) {
				return StatusFail, probeError(checkCtx, "Docker disk output overflowed", err)
			}
			return StatusWarn, probeError(checkCtx, "Docker disk usage unavailable", err)
		}
		if usage.Percent >= 90 {
			return StatusWarn, fmt.Sprintf("Docker reclaimable usage is %.1f%% (warning threshold 90%%)", usage.Percent)
		}
		return StatusPass, fmt.Sprintf("Docker reclaimable usage is %.1f%%", usage.Percent)
	})
	run(Check{
		ID: "docker.volumes", Category: "docker", Summary: "Compose project volumes",
		Remediation: "Run `camunda up`; verify the Compose project name before restoring data", Required: false,
	}, func(checkCtx context.Context) (Status, string) {
		volumes, err := inspector.Volumes(checkCtx, cfg.ComposeProject)
		if err != nil {
			if errors.Is(err, ErrCommandOutputOverflow) {
				return StatusFail, probeError(checkCtx, "Docker volume output overflowed", err)
			}
			return StatusWarn, probeError(checkCtx, "Docker volume inspection unavailable", err)
		}
		var present []string
		for _, volume := range volumes {
			if volume.Present {
				present = append(present, volume.Name)
			}
		}
		sort.Strings(present)
		if len(present) == 0 {
			return StatusWarn, "No Docker volumes were found for Compose project " + cfg.ComposeProject
		}
		return StatusPass, fmt.Sprintf("%d project volumes present: %s", len(present), strings.Join(present, ", "))
	})
	run(Check{
		ID: "overlay.consistency", Category: "configuration", Summary: "Managed overlay consistency",
		Remediation: "Run `camunda switch` for the selected version/profile to regenerate managed overlays", Required: false,
	}, func(checkCtx context.Context) (Status, string) {
		return inspectOverlays(checkCtx, fs, diagnosticPaths.OverlaysDir, cfg.Version, cfg.Profile, cfg.AI.Enabled, cfg.Monitoring.Enabled)
	})

	var envState EnvironmentState
	var envKnown bool
	var invalidEnvironment error
	run(Check{
		ID: "environment.active", Category: "environment", Summary: "Active environment validity",
		Remediation: "Select an existing valid environment with `camunda env use`", Required: true,
	}, func(checkCtx context.Context) (Status, string) {
		state, err := environment.Current(checkCtx)
		if err != nil {
			var environmentErr *EnvironmentError
			if errors.As(err, &environmentErr) && environmentErr.Kind == EnvironmentErrorInvalid {
				invalidEnvironment = err
				return StatusFail, "Active environment selection is invalid: " + sanitize(err.Error())
			}
			return StatusFail, probeError(checkCtx, "Active environment could not be read", err)
		}
		envState = state
		envKnown = true
		if envState.Name == "" {
			envState.Name = "lab"
		}
		if envState.Kind != "lab" && envState.Kind != "remote" {
			invalidEnvironment = &EnvironmentError{
				Kind: EnvironmentErrorInvalid,
				Err:  fmt.Errorf("active environment %s has invalid kind %s", envState.Name, envState.Kind),
			}
			return StatusFail, sanitize(invalidEnvironment.Error())
		}
		if envState.Kind == "remote" {
			if _, err := cluster.NormalizeBaseURL(envState.Endpoint); err != nil {
				invalidEnvironment = &EnvironmentError{
					Kind: EnvironmentErrorInvalid,
					Err:  fmt.Errorf("active environment %s has an invalid orchestration endpoint", envState.Name),
				}
				return StatusFail, invalidEnvironment.Error()
			}
		}
		return StatusPass, fmt.Sprintf("Active environment %s is valid (%s)", envState.Name, envState.Kind)
	})
	if invalidEnvironment != nil {
		return DeepReport{}, &FatalError{
			Code: "invalid_environment", Message: sanitize(invalidEnvironment.Error()),
			Err: invalidEnvironment,
		}
	}
	remoteAuth := opts.RemoteAuth
	if remoteAuth == nil {
		remoteAuth = httpRemoteAuthenticator{Client: httpClient}
	}
	remoteReachability := opts.RemoteReachability
	if remoteReachability == nil {
		remoteReachability = httpRemoteReachability{Client: httpClient}
	}
	var remoteCredential RemoteCredential
	var remoteAuthOK bool
	run(Check{
		ID: "remote.auth", Category: "environment", Summary: "Remote cluster authentication",
		Remediation: "Configure the remote credential and token endpoint environment variables, then retry", Required: true,
	}, func(checkCtx context.Context) (Status, string) {
		if !envKnown {
			return StatusFail, "Remote authentication could not be selected because the active environment is unavailable"
		}
		if envState.Kind != "remote" {
			return StatusSkipped, "Active environment is local; remote authentication was not requested"
		}
		if !envState.AuthConfigured {
			return StatusFail, "Remote environment credential configuration is incomplete"
		}
		credential, err := remoteAuth.Authenticate(checkCtx, envState)
		if err != nil {
			return StatusFail, probeError(checkCtx, "Remote authentication failed", err)
		}
		remoteCredential = credential
		remoteAuthOK = true
		return StatusPass, "Remote credentials produced a valid access token"
	})
	run(Check{
		ID: "remote.reachability", Category: "environment", Summary: "Protected remote cluster reachability",
		Remediation: "Verify the protected orchestration endpoint, network path, and remote authorization", Required: true,
	}, func(checkCtx context.Context) (Status, string) {
		if !envKnown {
			return StatusFail, "Protected remote reachability could not be selected because the active environment is unavailable"
		}
		if envState.Kind != "remote" {
			return StatusSkipped, "Active environment is local; remote reachability was not requested"
		}
		if strings.TrimSpace(envState.Endpoint) == "" {
			return StatusFail, "Remote environment orchestration endpoint is missing"
		}
		if !remoteAuthOK {
			return StatusFail, "Protected remote probe was not attempted because authentication failed"
		}
		if err := remoteReachability.Probe(checkCtx, envState, remoteCredential); err != nil {
			return StatusFail, probeError(checkCtx, "Protected remote cluster probe failed", err)
		}
		return StatusPass, "Authenticated protected cluster endpoint is reachable"
	})

	labDown := composeKnown && len(services) == 0
	for _, entry := range urls.List(cfg) {
		entry := entry
		kind, target := urls.ProbeTarget(entry)
		run(Check{
			ID: "service." + stableID(entry.Name), Category: "services",
			Summary:     "Local service " + entry.Name,
			Remediation: "Run `camunda up && camunda wait`; inspect with `camunda logs -f " + stableID(entry.Name) + "`",
			Required:    true,
		}, func(checkCtx context.Context) (Status, string) {
			if labDown {
				return StatusSkipped, "Compose lab is down; endpoint probe was not attempted"
			}
			if kind == "tcp" {
				if err := dial(checkCtx, "tcp", target); err != nil {
					return StatusFail, probeError(checkCtx, "TCP probe failed", err)
				}
				return StatusPass, "TCP port is accepting connections"
			}
			req, err := http.NewRequestWithContext(checkCtx, http.MethodGet, target, nil)
			if err != nil {
				return StatusFail, "HTTP probe request is invalid: " + sanitize(err.Error())
			}
			resp, err := httpClient.Do(req)
			if err != nil {
				return StatusFail, probeError(checkCtx, "HTTP probe failed", err)
			}
			_ = resp.Body.Close()
			switch {
			case resp.StatusCode >= 500:
				return StatusFail, fmt.Sprintf("HTTP %d indicates a server failure", resp.StatusCode)
			case resp.StatusCode >= 400:
				return StatusWarn, fmt.Sprintf("HTTP %d is reachable but requires attention or authentication", resp.StatusCode)
			default:
				return StatusPass, fmt.Sprintf("HTTP %d", resp.StatusCode)
			}
		})
	}

	report := DeepReport{Checks: checks}
	report.Aggregate()
	return report, nil
}

// FormatDeep preserves the original rendering entry point.
func FormatDeep(base Report, report DeepReport) string {
	checks := append([]Check(nil), report.Checks...)
	if !base.OK {
		checks = append(checks, Check{
			ID: "shallow.prerequisites", Category: "configuration", Status: StatusFail,
			Summary:     "Basic prerequisites have issues",
			Detail:      "One or more basic doctor prerequisite checks failed",
			Remediation: nonEmpty(base.FixHint, "Run `camunda doctor` for prerequisite details"),
			Required:    true,
		})
	}
	return DeepReport{Checks: checks}.Text()
}

// DeepOK applies required-failure policy.
func DeepOK(report DeepReport) bool {
	report.Aggregate()
	return report.OK
}

func composeStatus(services []ServiceState) (Status, string) {
	if len(services) == 0 {
		return StatusFail, "No Compose services were found; the lab is down"
	}
	var degraded, exited []string
	for _, service := range services {
		switch {
		case service.State == "exited" || service.State == "dead":
			exited = append(exited, fmt.Sprintf("%s exited (code %d)", service.Name, service.ExitCode))
		case service.State != "running":
			degraded = append(degraded, fmt.Sprintf("%s state is %s", service.Name, nonEmpty(service.State, "unknown")))
		case service.Health == "unhealthy" || service.Health == "starting":
			degraded = append(degraded, fmt.Sprintf("%s is %s", service.Name, service.Health))
		}
	}
	sort.Strings(degraded)
	sort.Strings(exited)
	if len(exited) > 0 {
		return StatusFail, strings.Join(exited, "; ")
	}
	if len(degraded) > 0 {
		return StatusWarn, strings.Join(degraded, "; ")
	}
	return StatusPass, fmt.Sprintf("%d Compose services are running", len(services))
}

func inspectPath(ctx context.Context, fs FileSystem, path string, wantDir bool) (Status, string) {
	info, err := fs.Stat(ctx, path)
	if err != nil {
		return StatusFail, probeError(ctx, path+" is unavailable", err)
	}
	if info.IsDir != wantDir {
		kind := "file"
		if wantDir {
			kind = "directory"
		}
		return StatusFail, fmt.Sprintf("%s is not a %s", path, kind)
	}
	return StatusPass, path + " is present"
}

func fileCheck(id, summary, path string, wantDir bool) Check {
	remediation := "Restore or regenerate " + path
	if wantDir {
		remediation = "Run `camunda install` for the configured version"
	}
	return Check{
		ID: id, Category: "filesystem", Summary: summary,
		Remediation: remediation, Required: true,
	}
}

func probeError(ctx context.Context, prefix string, err error) string {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
		return prefix + ": timed out"
	}
	if errors.Is(ctx.Err(), context.Canceled) || errors.Is(err, context.Canceled) {
		return prefix + ": canceled"
	}
	return prefix + ": " + sanitize(err.Error())
}

type osEnvironmentProvider struct{}

func (osEnvironmentProvider) Current(ctx context.Context) (EnvironmentState, error) {
	if err := ctx.Err(); err != nil {
		return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorRuntime, Err: err}
	}
	active, err := labenv.NewService(paths.Home()).Resolve(labenv.ResolveRequest{})
	if err != nil {
		var environmentErr *labenv.Error
		if errors.As(err, &environmentErr) &&
			(environmentErr.Kind == labenv.ErrorInvalid || environmentErr.Kind == labenv.ErrorMigration || environmentErr.Kind == labenv.ErrorMissing) {
			return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorInvalid, Err: err}
		}
		return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorRuntime, Err: err}
	}
	if err := ctx.Err(); err != nil {
		return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorRuntime, Err: err}
	}
	if active.Profile.Name == "lab" {
		return EnvironmentState{Name: "lab", Kind: "lab"}, nil
	}
	profile := active.Profile
	if err := ctx.Err(); err != nil {
		return EnvironmentState{}, &EnvironmentError{Kind: EnvironmentErrorRuntime, Err: err}
	}
	accessToken := strings.TrimSpace(os.Getenv("CAMUNDA_ACCESS_TOKEN"))
	clientID := os.Getenv(profile.Auth.ClientIDEnv)
	clientSecret := os.Getenv(profile.Auth.ClientSecretEnv)
	tokenURL := strings.TrimSpace(profile.Auth.TokenURL)
	if profile.Auth.TokenURLEnv != "" {
		tokenURL = strings.TrimSpace(os.Getenv(profile.Auth.TokenURLEnv))
	}
	audience := strings.TrimSpace(profile.Auth.Audience)
	scope := strings.TrimSpace(profile.Auth.Scope)
	if audience == "" && scope == "" {
		audience = "orchestration-api"
	}
	return EnvironmentState{
		Name: active.Profile.Name, Kind: profile.Kind,
		Endpoint: strings.TrimSpace(profile.Endpoints["orchestration"]),
		AuthConfigured: accessToken != "" ||
			(clientID != "" && clientSecret != "" && tokenURL != ""),
		accessToken: accessToken, tokenURL: strings.TrimSpace(profile.Auth.TokenURL),
		audience: audience, scope: scope,
		clientIDEnv:     profile.Auth.ClientIDEnv,
		clientSecretEnv: profile.Auth.ClientSecretEnv,
		tokenURLEnv:     profile.Auth.TokenURLEnv,
	}, nil
}

var sensitiveAssignment = regexp.MustCompile(`(?i)(access[_-]?token|refresh[_-]?token|client[_-]?secret|password|token|secret)=([^&\s]+)`)
var bearerCredential = regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._~+/=-]+`)

func sanitize(value string) string {
	value = sensitiveAssignment.ReplaceAllString(value, "$1=[REDACTED]")
	value = bearerCredential.ReplaceAllString(value, "$1[REDACTED]")
	fields := strings.Fields(value)
	for i, field := range fields {
		trimmed := strings.Trim(field, `"'(),:`)
		parsed, err := neturl.Parse(trimmed)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			continue
		}
		parsed.User = nil
		query := parsed.Query()
		for key := range query {
			if sensitiveAssignment.MatchString(key + "=x") {
				query.Set(key, "[REDACTED]")
			}
		}
		parsed.RawQuery = query.Encode()
		fields[i] = strings.Replace(field, trimmed, parsed.String(), 1)
	}
	return strings.Join(fields, " ")
}

func stableID(value string) string {
	value = strings.ToLower(value)
	value = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-' {
			return r
		}
		return '-'
	}, value)
	return strings.Trim(value, "-")
}

func nonEmpty(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func nonNegative(value time.Duration) time.Duration {
	if value < 0 {
		return 0
	}
	return value
}
