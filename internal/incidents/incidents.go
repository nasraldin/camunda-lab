package incidents

import (
	"errors"
	"fmt"

	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

type Status string

const (
	StatusCompleted Status = "completed"
	StatusPartial   Status = "partial"
)

type PolicyOutcome string

const (
	PolicyReadOnly        PolicyOutcome = "read-only"
	PolicyWouldResolve    PolicyOutcome = "would-resolve"
	PolicyResolved        PolicyOutcome = "resolved"
	PolicyAlreadyResolved PolicyOutcome = "already-resolved"
)

type Policy struct {
	Outcome  PolicyOutcome `json:"outcome"`
	ExitCode int           `json:"exitCode"`
}

type ProcessInstanceContext struct {
	Key                      string   `json:"key"`
	ProcessDefinitionID      string   `json:"processDefinitionId,omitempty"`
	ProcessDefinitionKey     string   `json:"processDefinitionKey,omitempty"`
	ProcessDefinitionVersion int      `json:"processDefinitionVersion,omitempty"`
	State                    string   `json:"state,omitempty"`
	HasIncident              bool     `json:"hasIncident"`
	TenantID                 string   `json:"tenantId,omitempty"`
	Tags                     []string `json:"tags"`
}

type ElementInstanceContext struct {
	Key         string `json:"key"`
	ElementID   string `json:"elementId,omitempty"`
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	State       string `json:"state,omitempty"`
	IncidentKey string `json:"incidentKey,omitempty"`
}

type ProcessDefinitionContext struct {
	Key          string `json:"key"`
	ID           string `json:"id"`
	Name         string `json:"name,omitempty"`
	Version      int    `json:"version"`
	ResourceName string `json:"resourceName,omitempty"`
	TenantID     string `json:"tenantId,omitempty"`
}

// Incident is the stable incident list/detail contract. Camunda identifiers
// remain decimal strings and timestamps remain normalized RFC3339 strings.
type Incident struct {
	Key                    string `json:"key"`
	ProcessInstanceKey     string `json:"processInstanceKey,omitempty"`
	RootProcessInstanceKey string `json:"rootProcessInstanceKey,omitempty"`
	ElementInstanceKey     string `json:"elementInstanceKey,omitempty"`
	ElementID              string `json:"elementId,omitempty"`
	ProcessDefinitionKey   string `json:"processDefinitionKey,omitempty"`
	ProcessDefinitionID    string `json:"processDefinitionId,omitempty"`
	JobKey                 string `json:"jobKey,omitempty"`
	ErrorType              string `json:"errorType,omitempty"`
	ErrorMessage           string `json:"errorMessage,omitempty"`
	State                  string `json:"state"`
	CreationTime           string `json:"creationTime,omitempty"`
	TenantID               string `json:"tenantId,omitempty"`

	Status            Status                    `json:"status"`
	Complete          bool                      `json:"complete"`
	Partial           bool                      `json:"partial"`
	Environment       env.Resolved              `json:"environment"`
	Source            inventory.Source          `json:"source"`
	ProcessInstance   *ProcessInstanceContext   `json:"processInstance,omitempty"`
	ElementInstance   *ElementInstanceContext   `json:"elementInstance,omitempty"`
	ProcessDefinition *ProcessDefinitionContext `json:"processDefinition,omitempty"`
	OperateURL        string                    `json:"operateUrl,omitempty"`
	Warnings          []inventory.Warning       `json:"warnings"`
}

type ListFilter struct {
	IncidentKey            string `json:"incidentKey,omitempty"`
	State                  string `json:"state,omitempty"`
	ProcessInstanceKey     string `json:"processInstanceKey,omitempty"`
	RootProcessInstanceKey string `json:"rootProcessInstanceKey,omitempty"`
	ProcessDefinitionKey   string `json:"processDefinitionKey,omitempty"`
	ProcessDefinitionID    string `json:"processDefinitionId,omitempty"`
	ElementInstanceKey     string `json:"elementInstanceKey,omitempty"`
	ElementID              string `json:"elementId,omitempty"`
	JobKey                 string `json:"jobKey,omitempty"`
	ErrorType              string `json:"errorType,omitempty"`
	ErrorMessage           string `json:"errorMessage,omitempty"`
	TenantID               string `json:"tenantId,omitempty"`
	CreatedAfter           string `json:"createdAfter,omitempty"`
	CreatedBefore          string `json:"createdBefore,omitempty"`
}

type ListRequest struct {
	Environment string     `json:"environment,omitempty"`
	ProjectRoot string     `json:"projectRoot,omitempty"`
	Filter      ListFilter `json:"filter"`
	Limit       int        `json:"limit,omitempty"`
	PageSize    int        `json:"pageSize,omitempty"`
	MaxPages    int        `json:"maxPages,omitempty"`
}

type Result struct {
	Environment env.Resolved        `json:"environment"`
	Source      inventory.Source    `json:"source"`
	Status      Status              `json:"status"`
	Complete    bool                `json:"complete"`
	Partial     bool                `json:"partial"`
	Warnings    []inventory.Warning `json:"warnings"`
	Incidents   []Incident          `json:"incidents"`
	Policy      Policy              `json:"policy"`
	Outcome     ResolveOutcome      `json:"outcome"`
}

type OutcomeKind string

const (
	OutcomeNone               OutcomeKind = ""
	OutcomeDryRun             OutcomeKind = "dry-run"
	OutcomeResolved           OutcomeKind = "resolved"
	OutcomeAlreadyResolved    OutcomeKind = "already-resolved"
	OutcomeResolvedUnverified OutcomeKind = "resolved-unverified"
)

type ResolveOutcome struct {
	Kind     OutcomeKind `json:"kind,omitempty"`
	Mutated  bool        `json:"mutated"`
	Incident *Incident   `json:"incident,omitempty"`
}

type ResolveRequest struct {
	Environment string `json:"environment,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
	Key         string `json:"key"`
	DryRun      bool   `json:"dryRun"`
}

type NotFoundError struct {
	Key string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("incident %s was not found", e.Key)
}

type StateError struct {
	Key   string
	State string
}

func (e *StateError) Error() string {
	return fmt.Sprintf("incident %s is not active (state %s)", e.Key, e.State)
}

var ErrInvalidRequest = errors.New("invalid incident request")
