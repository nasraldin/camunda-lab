package plan

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

type ActionType string

const (
	ActionCreate     ActionType = "create"
	ActionUpdate     ActionType = "update"
	ActionNoChange   ActionType = "no-change"
	ActionRemoteOnly ActionType = "remote-only"
)

type Status string

const (
	StatusReady   Status = "ready"
	StatusUnknown Status = "unknown"
	StatusRefused Status = "refused"
	StatusError   Status = "error"
)

type PolicyOutcome string

const (
	PolicyNoChanges PolicyOutcome = "no-changes"
	PolicyChanges   PolicyOutcome = "changes"
	PolicyUnknown   PolicyOutcome = "unknown"
	PolicyRefused   PolicyOutcome = "refused"
	PolicyError     PolicyOutcome = "error"
)

type ResourceIdentity struct {
	Kind inventory.Kind `json:"kind"`
	ID   string         `json:"id"`
}

type Action struct {
	Type          ActionType       `json:"type"`
	Resource      ResourceIdentity `json:"resource"`
	LocalDigest   string           `json:"localDigest,omitempty"`
	RemoteDigest  string           `json:"remoteDigest,omitempty"`
	LocalPath     string           `json:"localPath,omitempty"`
	RemoteKey     string           `json:"remoteKey,omitempty"`
	RemoteVersion int64            `json:"remoteVersion,omitempty"`
	Detail        string           `json:"detail"`
	Destructive   bool             `json:"destructive"`
}

type KindCount struct {
	Kind  inventory.Kind `json:"kind"`
	Count int            `json:"count"`
}

type InventorySummary struct {
	Total       int                     `json:"total"`
	ByKind      []KindCount             `json:"byKind"`
	Complete    bool                    `json:"complete"`
	Comparable  bool                    `json:"comparable"`
	Partial     bool                    `json:"partial"`
	Sources     []inventory.Source      `json:"sources"`
	Unsupported []inventory.Unsupported `json:"unsupported"`
	Warnings    []inventory.Warning     `json:"warnings"`
}

type Counts struct {
	Create     int `json:"create"`
	Update     int `json:"update"`
	NoChange   int `json:"noChange"`
	RemoteOnly int `json:"remoteOnly"`
	Unknown    int `json:"unknown"`
	Total      int `json:"total"`
}

type Policy struct {
	Outcome  PolicyOutcome `json:"outcome"`
	ExitCode int           `json:"exitCode"`
}

type Result struct {
	Environment env.Resolved        `json:"environment"`
	Env         string              `json:"env"`
	Local       InventorySummary    `json:"local"`
	Remote      InventorySummary    `json:"remote"`
	Complete    bool                `json:"complete"`
	Comparable  bool                `json:"comparable"`
	Status      Status              `json:"status"`
	Warnings    []inventory.Warning `json:"warnings"`
	Actions     []Action            `json:"actions"`
	Counts      Counts              `json:"counts"`
	Policy      Policy              `json:"policy"`
}

// Build produces a read-only plan from canonical P3 inventories. Invalid or
// incomplete required state is represented as a typed refusal, not an error.
func Build(local, remote inventory.Inventory) (Result, error) {
	result := emptyResult()
	result.Local = summarize(local, true)
	result.Remote = summarize(remote, false)
	result.Warnings = mergeWarnings(local, remote)

	localErr := validateInventory(local, true)
	remoteErr := validateInventory(remote, false)
	if localErr != nil || remoteErr != nil {
		result.Status = StatusRefused
		result.Policy = Policy{Outcome: PolicyRefused, ExitCode: 2}
		result.Warnings = appendValidationWarnings(result.Warnings, localErr, remoteErr)
		sortWarnings(result.Warnings)
		return result, nil
	}

	result.Comparable = true
	if len(result.Remote.Sources) > 0 {
		result.Env = result.Remote.Sources[0].Environment
	}
	result.Complete = result.Local.Complete && result.Remote.Complete
	unknownKinds := optionalUnsupportedKinds(remote.Unsupported)
	localByID := indexResourcesExcluding(local.Resources, unknownKinds)
	remoteByID := indexLatestResourcesExcluding(remote.Resources, unknownKinds)
	for _, resource := range local.Resources {
		if _, unknown := unknownKinds[resource.Kind]; unknown {
			result.Counts.Unknown++
		}
	}
	keys := make([]string, 0, len(localByID)+len(remoteByID))
	for key := range localByID {
		keys = append(keys, key)
	}
	for key := range remoteByID {
		if _, exists := localByID[key]; !exists {
			keys = append(keys, key)
		}
	}
	sort.Strings(keys)
	for _, key := range keys {
		localResource, hasLocal := localByID[key]
		remoteResource, hasRemote := remoteByID[key]
		action := Action{Resource: identityOf(localResource, remoteResource), Destructive: false}
		switch {
		case hasLocal && !hasRemote:
			action.Type, action.LocalDigest, action.LocalPath = ActionCreate, localResource.Digest, localResource.Path
			action.Detail = "create from canonical local resource"
			result.Counts.Create++
		case hasLocal && hasRemote && localResource.Digest != remoteResource.Digest:
			action.Type = ActionUpdate
			action.LocalDigest, action.RemoteDigest = localResource.Digest, remoteResource.Digest
			action.LocalPath, action.RemoteKey, action.RemoteVersion = localResource.Path, remoteResource.Key, remoteResource.Version
			action.Detail = "deploy changed canonical content as a new version; existing instances may continue on prior versions"
			result.Counts.Update++
		case hasLocal && hasRemote:
			action.Type = ActionNoChange
			action.LocalDigest, action.RemoteDigest = localResource.Digest, remoteResource.Digest
			action.LocalPath, action.RemoteKey, action.RemoteVersion = localResource.Path, remoteResource.Key, remoteResource.Version
			action.Detail = "canonical content is unchanged"
			result.Counts.NoChange++
		default:
			action.Type = ActionRemoteOnly
			action.RemoteDigest, action.RemoteKey, action.RemoteVersion = remoteResource.Digest, remoteResource.Key, remoteResource.Version
			action.Detail = "remote-only observation; no undeploy or delete is planned"
			result.Counts.RemoteOnly++
		}
		result.Actions = append(result.Actions, action)
	}
	result.Counts.Total = len(result.Actions) + result.Counts.Unknown
	if len(unknownKinds) > 0 {
		result.Status = StatusUnknown
		result.Policy = Policy{Outcome: PolicyUnknown, ExitCode: 2}
	} else if result.Counts.Create+result.Counts.Update > 0 {
		result.Status = StatusReady
		result.Policy = Policy{Outcome: PolicyChanges, ExitCode: 1}
	} else {
		result.Status = StatusReady
		result.Policy = Policy{Outcome: PolicyNoChanges, ExitCode: 0}
	}
	return result, nil
}

func emptyResult() Result {
	return Result{
		Status: StatusError, Policy: Policy{Outcome: PolicyError, ExitCode: 2},
		Warnings: make([]inventory.Warning, 0), Actions: make([]Action, 0),
		Local: emptySummary(), Remote: emptySummary(),
	}
}

func emptySummary() InventorySummary {
	return InventorySummary{
		ByKind: make([]KindCount, 0), Sources: make([]inventory.Source, 0),
		Unsupported: make([]inventory.Unsupported, 0), Warnings: make([]inventory.Warning, 0),
	}
}

func summarize(value inventory.Inventory, local bool) InventorySummary {
	summary := emptySummary()
	summary.Total, summary.Partial = len(value.Resources), value.Partial
	summary.Unsupported = append(summary.Unsupported, value.Unsupported...)
	summary.Warnings = append(summary.Warnings, value.Warnings...)
	counts := map[inventory.Kind]int{}
	sourceSet := map[string]inventory.Source{}
	sourceSet[value.Source.Type+"\x00"+value.Source.Environment+"\x00"+value.Source.Endpoint+"\x00"+value.Source.ProjectRoot] = value.Source
	for _, resource := range value.Resources {
		counts[resource.Kind]++
		key := resource.Source.Type + "\x00" + resource.Source.Environment + "\x00" + resource.Source.Endpoint + "\x00" + resource.Source.ProjectRoot
		sourceSet[key] = resource.Source
	}
	for _, kind := range []inventory.Kind{inventory.KindDecision, inventory.KindForm, inventory.KindProcess} {
		if counts[kind] > 0 {
			summary.ByKind = append(summary.ByKind, KindCount{Kind: kind, Count: counts[kind]})
		}
	}
	sourceKeys := make([]string, 0, len(sourceSet))
	for key := range sourceSet {
		sourceKeys = append(sourceKeys, key)
	}
	sort.Strings(sourceKeys)
	for _, key := range sourceKeys {
		summary.Sources = append(summary.Sources, sourceSet[key])
	}
	summary.Comparable = validateInventory(value, local) == nil
	summary.Complete = !value.Partial && len(value.Unsupported) == 0
	return summary
}

func validateInventory(value inventory.Inventory, local bool) error {
	if err := value.ValidateComparable(); err != nil {
		return err
	}
	if local {
		if value.Source.Type != "local" || value.Source.Environment != "" || value.Source.Endpoint != "" ||
			strings.TrimSpace(value.Source.ProjectRoot) == "" || !filepath.IsAbs(value.Source.ProjectRoot) ||
			filepath.Clean(value.Source.ProjectRoot) != value.Source.ProjectRoot {
			return errors.New("local inventory has invalid role/project source metadata")
		}
	} else if (value.Source.Type != "local" && value.Source.Type != "remote") ||
		strings.TrimSpace(value.Source.Environment) == "" || strings.TrimSpace(value.Source.Endpoint) == "" ||
		value.Source.ProjectRoot != "" {
		return errors.New("remote inventory has invalid role/environment/endpoint source metadata")
	}
	if !local {
		canonicalEndpoint, err := cluster.NormalizeBaseURL(value.Source.Endpoint)
		if err != nil || canonicalEndpoint != value.Source.Endpoint {
			return errors.New("remote inventory endpoint is not canonical")
		}
	}
	seen := map[string]struct{}{}
	for _, resource := range value.Resources {
		identity := resource.Kind.String() + "\x00" + resource.ID
		if !local {
			identity += fmt.Sprintf("\x00%d", resource.Version)
		}
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("duplicate %s identity %q", resource.Kind, resource.ID)
		}
		seen[identity] = struct{}{}
		if local {
			if resource.Source != value.Source {
				return fmt.Errorf("local %s %q has mismatched source metadata", resource.Kind, resource.ID)
			}
			continue
		}
		if resource.Source != value.Source {
			return fmt.Errorf("remote %s %q has mismatched source metadata", resource.Kind, resource.ID)
		}
	}
	return nil
}

func indexResourcesExcluding(resources []inventory.Resource, excluded map[inventory.Kind]struct{}) map[string]inventory.Resource {
	result := make(map[string]inventory.Resource, len(resources))
	for _, resource := range resources {
		if _, skip := excluded[resource.Kind]; skip {
			continue
		}
		result[resource.Kind.String()+"\x00"+resource.ID] = resource
	}
	return result
}

func indexLatestResourcesExcluding(resources []inventory.Resource, excluded map[inventory.Kind]struct{}) map[string]inventory.Resource {
	result := make(map[string]inventory.Resource, len(resources))
	for _, resource := range resources {
		if _, skip := excluded[resource.Kind]; skip {
			continue
		}
		key := resource.Kind.String() + "\x00" + resource.ID
		current, exists := result[key]
		if !exists || resource.Version > current.Version ||
			resource.Version == current.Version && resource.Key < current.Key {
			result[key] = resource
		}
	}
	return result
}

func optionalUnsupportedKinds(values []inventory.Unsupported) map[inventory.Kind]struct{} {
	result := make(map[inventory.Kind]struct{})
	for _, value := range values {
		if !value.Required {
			result[value.Kind] = struct{}{}
		}
	}
	return result
}

func identityOf(local, remote inventory.Resource) ResourceIdentity {
	if local.ID != "" {
		return ResourceIdentity{Kind: local.Kind, ID: local.ID}
	}
	return ResourceIdentity{Kind: remote.Kind, ID: remote.ID}
}

func mergeWarnings(local, remote inventory.Inventory) []inventory.Warning {
	result := make([]inventory.Warning, 0, len(local.Warnings)+len(remote.Warnings)+len(local.Unsupported)+len(remote.Unsupported))
	result = append(result, local.Warnings...)
	result = append(result, remote.Warnings...)
	for _, item := range append(append([]inventory.Unsupported(nil), local.Unsupported...), remote.Unsupported...) {
		result = append(result, inventory.Warning{
			Capability: item.Kind.String(),
			Message:    fmt.Sprintf("unsupported inventory capability: %s", item.Reason),
		})
	}
	sortWarnings(result)
	return result
}

func appendValidationWarnings(warnings []inventory.Warning, localErr, remoteErr error) []inventory.Warning {
	if localErr != nil {
		warnings = append(warnings, inventory.Warning{Capability: "local-inventory", Message: localErr.Error()})
	}
	if remoteErr != nil {
		warnings = append(warnings, inventory.Warning{Capability: "remote-inventory", Message: remoteErr.Error()})
	}
	return warnings
}

func sortWarnings(warnings []inventory.Warning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		if warnings[i].Capability != warnings[j].Capability {
			return warnings[i].Capability < warnings[j].Capability
		}
		return warnings[i].Message < warnings[j].Message
	})
}
