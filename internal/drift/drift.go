package drift

import (
	"fmt"

	envpkg "github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
	"github.com/nasraldin/camunda-lab/internal/plan"
)

type Status string

const (
	StatusReady   Status = "ready"
	StatusUnknown Status = "unknown"
	StatusRefused Status = "refused"
)

type ComparisonStatus string

const (
	ComparisonInSync      ComparisonStatus = "IN_SYNC"
	ComparisonDrift       ComparisonStatus = "DRIFT"
	ComparisonLocalOnly   ComparisonStatus = "LOCAL_ONLY"
	ComparisonClusterOnly ComparisonStatus = "CLUSTER_ONLY"
	ComparisonUnknown     ComparisonStatus = "UNKNOWN"
)

type Baseline struct {
	Ref        string `json:"ref"`
	Commit     string `json:"commit"`
	Repository string `json:"repository"`
}

type Snapshot struct {
	Source inventory.Source `json:"source"`
	Total  int              `json:"total"`
}

type Change struct {
	Path        string `json:"path"`
	Index       string `json:"index,omitempty"`
	Worktree    string `json:"worktree,omitempty"`
	Untracked   bool   `json:"untracked,omitempty"`
	Deleted     bool   `json:"deleted,omitempty"`
	RenamedFrom string `json:"renamedFrom,omitempty"`
}

type Counts struct {
	InSync      int `json:"inSync"`
	Drift       int `json:"drift"`
	LocalOnly   int `json:"localOnly"`
	ClusterOnly int `json:"clusterOnly"`
	Unknown     int `json:"unknown"`
	Total       int `json:"total"`
}

type Comparison struct {
	Name       string `json:"name"`
	Comparable bool   `json:"comparable"`
	Counts     Counts `json:"counts"`
}

type Policy struct {
	ExitCode int    `json:"exitCode"`
	Outcome  string `json:"outcome"`
}

// Entry is per-resource drift status.
type Entry struct {
	Key               string           `json:"key"`
	BaselineDigest    string           `json:"baselineDigest,omitempty"`
	WorkingDigest     string           `json:"workingDigest,omitempty"`
	DeployedDigest    string           `json:"deployedDigest,omitempty"`
	DeployedVersion   string           `json:"deployedVersion,omitempty"`
	BaselinePath      string           `json:"baselinePath,omitempty"`
	WorkingPath       string           `json:"workingPath,omitempty"`
	DeployedPath      string           `json:"deployedPath,omitempty"`
	SourceDrift       ComparisonStatus `json:"sourceDrift"`
	DeploymentDrift   ComparisonStatus `json:"deploymentDrift"`
	PendingDeployment ComparisonStatus `json:"pendingDeployment"`
	Detail            string           `json:"detail"`

	// Deprecated two-way aliases retained for compatibility with P4 consumers.
	GitDigest     string `json:"gitDigest,omitempty"`
	ClusterDigest string `json:"clusterDigest,omitempty"`
	ClusterVer    string `json:"clusterVersion,omitempty"`
	Status        string `json:"status,omitempty"`
}

// Report summarizes drift.
type Report struct {
	Environment       envpkg.Resolved     `json:"environment"`
	Env               string              `json:"env"`
	Baseline          Baseline            `json:"baseline"`
	Working           Snapshot            `json:"working"`
	Deployed          Snapshot            `json:"deployed"`
	Dirty             bool                `json:"dirty"`
	Changes           []Change            `json:"changes"`
	Status            Status              `json:"status"`
	Complete          bool                `json:"complete"`
	Comparable        bool                `json:"comparable"`
	Unknown           int                 `json:"unknown"`
	Warnings          []inventory.Warning `json:"warnings"`
	Entries           []Entry             `json:"entries"`
	Source            Comparison          `json:"source"`
	Deployment        Comparison          `json:"deployment"`
	PendingDeployment Comparison          `json:"pendingDeployment"`
	Policy            Policy              `json:"policy"`
}

// Compare builds a drift report from inventories.
func Compare(env string, local, remote inventory.Inventory) Report {
	profileKind := remote.Source.Type
	if profileKind == "local" {
		profileKind = "lab"
	}
	result, _ := plan.BuildResolved(local, remote, envpkg.Resolved{Profile: envpkg.Profile{
		Name: env, Kind: profileKind,
		Endpoints: map[string]string{"orchestration": remote.Source.Endpoint},
	}})
	return fromPlan(result)
}

// CompareResolved additionally binds the remote source to the exact resolved
// P1/P2 environment and canonical endpoint.
func CompareResolved(resolved envpkg.Resolved, local, remote inventory.Inventory) Report {
	result, _ := plan.BuildResolved(local, remote, resolved)
	return fromPlan(result)
}

func fromPlan(result plan.Result) Report {
	report := Report{
		Env: result.Env, Complete: result.Complete, Comparable: result.Comparable,
		Unknown:  result.Counts.Unknown,
		Warnings: append(make([]inventory.Warning, 0, len(result.Warnings)), result.Warnings...),
		Entries:  make([]Entry, 0, len(result.Actions)),
		Changes:  make([]Change, 0),
	}
	switch result.Status {
	case plan.StatusRefused, plan.StatusError:
		report.Status = StatusRefused
	case plan.StatusUnknown:
		report.Status = StatusUnknown
	default:
		report.Status = StatusReady
	}
	for _, action := range result.Actions {
		entry := Entry{
			Key:       action.Resource.Kind.String() + "/" + action.Resource.ID,
			GitDigest: action.LocalDigest, ClusterDigest: action.RemoteDigest,
		}
		if action.RemoteVersion > 0 {
			entry.ClusterVer = fmt.Sprintf("%d", action.RemoteVersion)
		}
		switch action.Type {
		case plan.ActionCreate:
			entry.Status = "LOCAL_ONLY"
		case plan.ActionUpdate:
			entry.Status = "DRIFT"
		case plan.ActionNoChange:
			entry.Status = "IN_SYNC"
		case plan.ActionRemoteOnly:
			entry.Status = "CLUSTER_ONLY"
		}
		entry.WorkingDigest = entry.GitDigest
		entry.DeployedDigest = entry.ClusterDigest
		entry.DeployedVersion = entry.ClusterVer
		report.Entries = append(report.Entries, entry)
	}
	if report.Status == StatusReady {
		if HasDrift(report) {
			report.Policy = Policy{Outcome: "drift", ExitCode: 1}
		} else {
			report.Policy = Policy{Outcome: "in-sync", ExitCode: 0}
		}
	} else {
		report.Policy = Policy{Outcome: string(report.Status), ExitCode: 2}
	}
	return report
}

// HasDrift is true when any entry is not IN_SYNC.
func HasDrift(r Report) bool {
	if !r.Comparable || r.Status == StatusUnknown || r.Status == StatusRefused {
		return true
	}
	for _, e := range r.Entries {
		if e.SourceDrift != "" || e.DeploymentDrift != "" || e.PendingDeployment != "" {
			if e.SourceDrift != ComparisonInSync || e.DeploymentDrift != ComparisonInSync ||
				e.PendingDeployment != ComparisonInSync {
				return true
			}
			continue
		}
		if e.Status != string(ComparisonInSync) {
			return true
		}
	}
	return false
}

func HasUnknown(r Report) bool {
	return r.Status == StatusUnknown || r.Status == StatusRefused || !r.Comparable || r.Unknown > 0
}

func empty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
