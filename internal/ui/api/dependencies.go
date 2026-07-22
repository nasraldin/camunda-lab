package api

import (
	"context"
	"net/http"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/lab"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

// EnvService is the injectable environment profile surface used by the API.
type EnvService interface {
	Resolve(env.ResolveRequest) (env.Resolved, error)
	List(projectRoot string) ([]env.Resolved, error)
	Use(name, projectRoot string) (env.Resolved, error)
	SaveGlobal(env.Profile) error
	SaveProject(projectRoot string, profile env.Profile) error
	Remove(name, projectRoot string, source env.ProfileSource) error
}

// Dependencies injects edge collaborators for parity and route tests.
type Dependencies struct {
	Toolkit         toolkit.BPMNService
	Doctor          *developerDoctorDependencies
	ClusterFactory  cluster.Factory
	RunningLab      backup.RunningChecker
	BackupCreate    func(context.Context, backup.Options) (backup.Manifest, error)
	Plan            func(context.Context, plan.Request) (plan.Result, error)
	Drift           func(context.Context, drift.Request) (drift.Report, error)
	ListIncidents   func(context.Context, incidents.ListRequest) (incidents.Result, error)
	ResolveIncident func(context.Context, incidents.ResolveRequest) (incidents.Result, error)
	TraceGet        func(context.Context, trace.Request) (trace.Timeline, error)
	TraceFollow     func(context.Context, trace.Request, time.Duration, func(trace.Timeline) error) error
	Env             EnvService
}

// NewHandler builds an API mux with optional injectable dependencies.
func NewHandler(version string, deps Dependencies) http.Handler {
	mux := http.NewServeMux()
	RegisterWithDependencies(mux, version, "", deps)
	return mux
}

// RegisterWithDependencies mounts /api/v1 routes using deps.
func RegisterWithDependencies(mux *http.ServeMux, cliVersion, csrfToken string, deps Dependencies) {
	h := &handler{
		cliVersion:        cliVersion,
		csrfToken:         csrfToken,
		lab:               lab.New(),
		doctor:            deps.Doctor,
		clusterFactory:    deps.ClusterFactory,
		runningLab:        deps.RunningLab,
		backupCreate:      deps.BackupCreate,
		toolkitService:    deps.Toolkit,
		plan:              deps.Plan,
		drift:             deps.Drift,
		listIncidentsFn:   deps.ListIncidents,
		resolveIncidentFn: deps.ResolveIncident,
		traceGetFn:        deps.TraceGet,
		traceFollowFn:     deps.TraceFollow,
		envService:        deps.Env,
	}
	registerCore(mux, h)
	registerToolkit(mux, h)
}

func Register(mux *http.ServeMux, cliVersion, csrfToken string) {
	RegisterWithDependencies(mux, cliVersion, csrfToken, Dependencies{})
}

func (h *handler) bpmnToolkit() toolkit.BPMNService {
	if h != nil && h.toolkitService != nil {
		return h.toolkitService
	}
	return toolkit.Service{}
}

func (h *handler) environments() EnvService {
	if h != nil && h.envService != nil {
		return h.envService
	}
	return env.NewService(paths.Home())
}
