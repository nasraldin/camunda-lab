package cli

import (
	"context"
	"time"

	"github.com/nasraldin/camunda-lab/internal/backup"
	"github.com/nasraldin/camunda-lab/internal/drift"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/incidents"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

// EnvService is the injectable environment profile surface used by the CLI.
type EnvService interface {
	Resolve(env.ResolveRequest) (env.Resolved, error)
	List(projectRoot string) ([]env.Resolved, error)
	Use(name, projectRoot string) (env.Resolved, error)
	SaveGlobal(env.Profile) error
	SaveProject(projectRoot string, profile env.Profile) error
	Remove(name, projectRoot string, source env.ProfileSource) error
}

// Dependencies injects edge collaborators for tests while production keeps defaults.
type Dependencies struct {
	Toolkit         toolkit.BPMNService
	Doctor          doctorCommandDependencies
	Restore         restoreFunc
	RunningLab      backup.RunningChecker
	Plan            func(context.Context, plan.Request) (plan.Result, error)
	Drift           func(context.Context, drift.Request) (drift.Report, error)
	BackupCreate    func(context.Context, backup.Options) (backup.Manifest, error)
	ListIncidents   func(context.Context, incidents.ListRequest) (incidents.Result, error)
	ResolveIncident func(context.Context, incidents.ResolveRequest) (incidents.Result, error)
	TraceGet        func(context.Context, trace.Request) (trace.Timeline, error)
	TraceFollow     func(context.Context, trace.Request, time.Duration, func(trace.Timeline) error) error
	Env             EnvService
}

func (d Dependencies) toolkitService() toolkit.BPMNService {
	if d.Toolkit != nil {
		return d.Toolkit
	}
	return toolkit.Service{}
}

func (d Dependencies) envService() EnvService {
	if d.Env != nil {
		return d.Env
	}
	return env.NewService(paths.Home())
}
