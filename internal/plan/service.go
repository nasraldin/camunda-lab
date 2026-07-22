package plan

import (
	"context"
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

type Request struct {
	ProjectRoot string `json:"projectRoot"`
	Environment string `json:"environment,omitempty"`
}

type Service struct {
	Factory     cluster.Factory
	buildLocal  func(inventory.LocalRequest) (inventory.Inventory, error)
	buildRemote func(context.Context, cluster.Factory, cluster.InventoryRequest) (inventory.Inventory, env.Resolved, error)
}

func NewService(factory cluster.Factory) *Service {
	return &Service{Factory: factory}
}

func (s *Service) Run(ctx context.Context, request Request) (Result, error) {
	result := emptyResult()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s == nil || s.Factory == nil {
		return result, fmt.Errorf("plan service requires a cluster factory")
	}
	buildLocal := s.buildLocal
	if buildLocal == nil {
		buildLocal = inventory.BuildLocal
	}
	local, err := buildLocal(inventory.LocalRequest{Root: request.ProjectRoot})
	if err != nil {
		return result, fmt.Errorf("build local canonical inventory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	buildRemote := s.buildRemote
	if buildRemote == nil {
		buildRemote = cluster.BuildClusterInventory
	}
	remote, resolved, err := buildRemote(ctx, s.Factory, cluster.InventoryRequest{
		Environment: request.Environment,
		ProjectRoot: request.ProjectRoot,
	})
	result.Environment = resolved
	result.Env = resolved.Profile.Name
	if err != nil {
		return result, fmt.Errorf("build cluster canonical inventory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	result, err = BuildResolved(local, remote, resolved)
	if err != nil {
		return result, err
	}
	return result, nil
}

// BuildResolved binds a pure canonical comparison to the exact P1/P2
// environment metadata and canonical remote endpoint.
func BuildResolved(local, remote inventory.Inventory, resolved env.Resolved) (Result, error) {
	result, err := Build(local, remote)
	result.Environment = resolved
	result.Env = resolved.Profile.Name
	if err != nil {
		return result, err
	}
	if result.Comparable {
		expectedSourceType := resolved.Profile.Kind
		if expectedSourceType == "lab" {
			expectedSourceType = "local"
		}
		sourceMismatch := remote.Source.Environment != resolved.Profile.Name || remote.Source.Type != expectedSourceType
		if resolved.Profile.Kind == "remote" {
			expectedEndpoint, normalizeErr := cluster.NormalizeBaseURL(resolved.Profile.Endpoints["orchestration"])
			sourceMismatch = sourceMismatch || normalizeErr != nil || remote.Source.Endpoint != expectedEndpoint
		}
		if sourceMismatch {
			refuseResolvedSource(&result)
		}
	}
	return result, nil
}

func refuseResolvedSource(result *Result) {
	result.Status = StatusRefused
	result.Complete = false
	result.Comparable = false
	result.Remote.Comparable = false
	result.Actions = make([]Action, 0)
	result.Counts = Counts{}
	result.Policy = Policy{Outcome: PolicyRefused, ExitCode: 2}
	result.Warnings = append(result.Warnings, inventory.Warning{
		Capability: "remote-inventory",
		Message:    "inventory source metadata does not match the exact resolved cluster endpoint",
	})
	sortWarnings(result.Warnings)
}
