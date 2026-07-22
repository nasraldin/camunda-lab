package incidents

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

const (
	defaultLimit = 50
	maxLimit     = 500
)

var exactKeyPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

type Service struct {
	Factory cluster.Factory
}

func NewService(factory cluster.Factory) *Service {
	return &Service{Factory: factory}
}

func (s *Service) List(ctx context.Context, request ListRequest) (Result, error) {
	result := emptyResult()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s == nil || s.Factory == nil {
		return result, errors.New("incident service requires a cluster factory")
	}
	filter, err := normalizeFilter(request.Filter)
	if err != nil {
		return result, err
	}
	limits, err := listLimits(request)
	if err != nil {
		return result, err
	}
	client, resolved, err := s.Factory.Client(ctx, request.Environment, request.ProjectRoot)
	result.Environment = resolved
	if err != nil {
		return result, fmt.Errorf("resolve incident environment: %w", err)
	}
	result.Source = sourceFor(client, resolved)
	searched, err := client.SearchIncidentsInventory(ctx, filter, limits)
	if err != nil {
		return result, fmt.Errorf("list incidents: %w", err)
	}
	result.Incidents = make([]Incident, 0, len(searched.Items))
	for _, item := range searched.Items {
		result.Incidents = append(result.Incidents, fromCluster(item, resolved, result.Source))
	}
	sort.SliceStable(result.Incidents, func(i, j int) bool {
		return result.Incidents[i].Key < result.Incidents[j].Key
	})
	result.Warnings = nonNilWarnings(searched.Warnings)
	result.Partial = searched.Partial
	result.Complete = !searched.Partial
	if searched.Partial {
		result.Status = StatusPartial
	}
	return result, nil
}

func (s *Service) Show(ctx context.Context, envName, projectRoot, key string) (Incident, error) {
	if err := ctx.Err(); err != nil {
		return Incident{Warnings: []inventory.Warning{}}, err
	}
	if s == nil || s.Factory == nil {
		return Incident{Warnings: []inventory.Warning{}}, errors.New("incident service requires a cluster factory")
	}
	if err := validateExactKey(key); err != nil {
		return Incident{Warnings: []inventory.Warning{}}, err
	}
	client, resolved, err := s.Factory.Client(ctx, envName, projectRoot)
	if err != nil {
		return Incident{Environment: resolved, Warnings: []inventory.Warning{}}, fmt.Errorf("resolve incident environment: %w", err)
	}
	source := sourceFor(client, resolved)
	raw, found, err := client.GetIncident(ctx, key)
	if err != nil {
		return Incident{Environment: resolved, Source: source, Warnings: []inventory.Warning{}}, fmt.Errorf("get incident %s: %w", key, err)
	}
	if !found {
		return Incident{Environment: resolved, Source: source, Warnings: []inventory.Warning{}}, &NotFoundError{Key: key}
	}
	incident := fromCluster(raw, resolved, source)
	enrichIncident(ctx, client, &incident)
	if operateBase := strings.TrimSpace(resolved.Profile.Endpoints["operate"]); operateBase != "" {
		link, linkErr := OperateLink(operateBase, incident.ProcessInstanceKey, incident.Key)
		if linkErr != nil {
			addWarning(&incident.Warnings, "operate-link", linkErr.Error())
		} else {
			incident.OperateURL = link
		}
	}
	sortWarnings(incident.Warnings)
	if len(incident.Warnings) > 0 {
		incident.Status, incident.Complete, incident.Partial = StatusPartial, false, true
	}
	return incident, nil
}

func (s *Service) Resolve(ctx context.Context, envName, projectRoot, key string) (Result, error) {
	return s.ResolveWithOptions(ctx, ResolveRequest{
		Environment: envName, ProjectRoot: projectRoot, Key: key,
	})
}

func (s *Service) ResolveWithOptions(ctx context.Context, request ResolveRequest) (Result, error) {
	result := emptyResult()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s == nil || s.Factory == nil {
		return result, errors.New("incident service requires a cluster factory")
	}
	if err := validateExactKey(request.Key); err != nil {
		return result, err
	}
	client, resolved, err := s.Factory.Client(ctx, request.Environment, request.ProjectRoot)
	result.Environment = resolved
	if err != nil {
		return result, fmt.Errorf("resolve incident environment: %w", err)
	}
	result.Source = sourceFor(client, resolved)
	raw, found, err := client.GetIncident(ctx, request.Key)
	if err != nil {
		return result, fmt.Errorf("revalidate incident %s: %w", request.Key, err)
	}
	if !found {
		return result, &NotFoundError{Key: request.Key}
	}
	current := fromCluster(raw, resolved, result.Source)
	result.Incidents = []Incident{current}
	result.Outcome.Incident = &result.Incidents[0]
	switch current.State {
	case "RESOLVED":
		result.Policy = Policy{Outcome: PolicyAlreadyResolved, ExitCode: 0}
		result.Outcome = ResolveOutcome{
			Kind: OutcomeAlreadyResolved, Mutated: false, Incident: &result.Incidents[0],
		}
		return result, nil
	case "ACTIVE":
		// The exact active-state result is the mutation precondition.
	default:
		return result, &StateError{Key: request.Key, State: current.State}
	}
	if request.DryRun {
		result.Policy = Policy{Outcome: PolicyWouldResolve, ExitCode: 0}
		result.Outcome = ResolveOutcome{
			Kind: OutcomeDryRun, Mutated: false, Incident: &result.Incidents[0],
		}
		return result, nil
	}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if err := client.ResolveIncident(ctx, request.Key); err != nil {
		return result, fmt.Errorf("resolve incident %s: %w", request.Key, err)
	}
	result.Policy = Policy{Outcome: PolicyResolved, ExitCode: 0}
	result.Outcome = ResolveOutcome{Kind: OutcomeResolved, Mutated: true}

	refreshed, refreshFound, refreshErr := client.GetIncident(ctx, request.Key)
	if refreshErr != nil || !refreshFound {
		message := "incident resolution succeeded but refreshed incident was not found"
		if refreshErr != nil {
			message = "incident resolution succeeded but refresh failed: " + refreshErr.Error()
		}
		markPartial(&result, "incident-refresh", message)
		result.Outcome.Kind = OutcomeResolvedUnverified
		// Do not present the pre-mutation ACTIVE snapshot as current truth.
		unverified := result.Incidents[0]
		unverified.State = ""
		unverified.Status = StatusPartial
		unverified.Complete = false
		unverified.Partial = true
		addWarning(&unverified.Warnings, "incident-refresh", "post-mutation incident state could not be verified")
		result.Incidents = []Incident{unverified}
		result.Outcome.Incident = &result.Incidents[0]
		return result, nil
	}
	updated := fromCluster(refreshed, resolved, result.Source)
	result.Incidents = []Incident{updated}
	result.Outcome.Incident = &result.Incidents[0]
	if updated.State == "ACTIVE" {
		markPartial(&result, "incident-refresh", "incident remained active after accepted resolution")
		result.Outcome.Kind = OutcomeResolvedUnverified
	}
	return result, nil
}

func OperateLink(baseURL, processInstanceKey, incidentKey string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("operate base URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("operate base URL must not contain userinfo or fragment")
	}
	if processInstanceKey == "" || incidentKey == "" {
		return "", errors.New("operate link requires process instance and incident keys")
	}
	basePath := strings.TrimRight(parsed.Path, "/") + "/processes/"
	escapedBasePath := strings.TrimRight(parsed.EscapedPath(), "/") + "/processes/"
	parsed.Path = basePath + processInstanceKey
	parsed.RawPath = escapedBasePath + url.PathEscape(processInstanceKey)
	query := parsed.Query()
	query.Set("incident", incidentKey)
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

func enrichIncident(ctx context.Context, client *cluster.Client, incident *Incident) {
	limits := cluster.InventoryLimits{PageSize: 2, MaxPages: 1, MaxItems: 2, MaxBodyBytes: 4 << 20}
	if incident.ProcessInstanceKey != "" {
		result, err := client.SearchProcessInstancesInventory(ctx, map[string]any{
			"processInstanceKey": incident.ProcessInstanceKey,
		}, limits)
		if err != nil {
			addWarning(&incident.Warnings, "process-instance", err.Error())
		} else if item, ok := exactProcessInstance(result.Items, incident.ProcessInstanceKey); ok {
			incident.ProcessInstance = &ProcessInstanceContext{
				Key: item.Key, ProcessDefinitionID: item.ProcessDefinitionID,
				ProcessDefinitionKey:     item.ProcessDefinitionKey,
				ProcessDefinitionVersion: item.ProcessDefinitionVersion,
				State:                    item.State, HasIncident: item.HasIncident, TenantID: item.TenantID,
				Tags: append([]string{}, item.Tags...),
			}
		} else {
			addWarning(&incident.Warnings, "process-instance", "optional process instance context was not found")
		}
	}
	if incident.ElementInstanceKey != "" {
		result, err := client.SearchElementInstancesInventory(ctx, map[string]any{
			"elementInstanceKey": incident.ElementInstanceKey,
		}, limits)
		if err != nil {
			addWarning(&incident.Warnings, "element-instance", err.Error())
		} else if item, ok := exactElementInstance(result.Items, incident.ElementInstanceKey); ok {
			incident.ElementInstance = &ElementInstanceContext{
				Key: item.Key, ElementID: item.ElementID, Name: item.ElementName,
				Type: item.Type, State: item.State, IncidentKey: item.IncidentKey,
			}
		} else {
			addWarning(&incident.Warnings, "element-instance", "optional element instance context was not found")
		}
	}
	if incident.ProcessDefinitionKey != "" {
		result, err := client.SearchProcessDefinitionsInventory(ctx, map[string]any{
			"processDefinitionKey": incident.ProcessDefinitionKey,
		}, limits)
		if err != nil {
			addWarning(&incident.Warnings, "process-definition", err.Error())
		} else if item, ok := exactProcessDefinition(result.Items, incident.ProcessDefinitionKey); ok {
			incident.ProcessDefinition = &ProcessDefinitionContext{
				Key: item.Key, ID: item.ProcessDefinitionID, Name: pointerValue(item.Name),
				Version: item.Version, ResourceName: item.ResourceName, TenantID: item.TenantID,
			}
		} else {
			addWarning(&incident.Warnings, "process-definition", "optional process definition context was not found")
		}
	}
}

func normalizeFilter(value ListFilter) (map[string]any, error) {
	if strings.TrimSpace(value.RootProcessInstanceKey) != "" {
		return nil, fmt.Errorf("%w: rootProcessInstanceKey is not an official incident filter", ErrInvalidRequest)
	}
	filter := make(map[string]any)
	if state := strings.ToUpper(strings.TrimSpace(value.State)); state != "" {
		switch state {
		case "ACTIVE", "RESOLVED":
			filter["state"] = state
		default:
			return nil, fmt.Errorf("%w: incident state must be ACTIVE or RESOLVED", ErrInvalidRequest)
		}
	}
	for name, item := range map[string]string{
		"incidentKey":          value.IncidentKey,
		"processInstanceKey":   value.ProcessInstanceKey,
		"processDefinitionKey": value.ProcessDefinitionKey,
		"elementInstanceKey":   value.ElementInstanceKey,
		"jobKey":               value.JobKey,
	} {
		if item == "" {
			continue
		}
		if err := validateExactKey(item); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		filter[name] = item
	}
	if value.ErrorType != "" {
		filter["errorType"] = strings.ToUpper(strings.TrimSpace(value.ErrorType))
	}
	for name, item := range map[string]string{
		"processDefinitionId": value.ProcessDefinitionID,
		"elementId":           value.ElementID,
		"errorMessage":        value.ErrorMessage,
	} {
		if item == "" {
			continue
		}
		if item != strings.TrimSpace(item) {
			return nil, fmt.Errorf("%w: %s must be exact", ErrInvalidRequest, name)
		}
		filter[name] = item
	}
	if value.TenantID != "" {
		if value.TenantID != strings.TrimSpace(value.TenantID) {
			return nil, fmt.Errorf("%w: tenant ID must be exact", ErrInvalidRequest)
		}
		filter["tenantId"] = value.TenantID
	}
	after, err := normalizeTime(value.CreatedAfter)
	if err != nil {
		return nil, fmt.Errorf("%w: createdAfter %v", ErrInvalidRequest, err)
	}
	before, err := normalizeTime(value.CreatedBefore)
	if err != nil {
		return nil, fmt.Errorf("%w: createdBefore %v", ErrInvalidRequest, err)
	}
	if after != "" && before != "" {
		afterTime, _ := time.Parse(time.RFC3339Nano, after)
		beforeTime, _ := time.Parse(time.RFC3339Nano, before)
		if afterTime.After(beforeTime) {
			return nil, fmt.Errorf("%w: createdAfter must not exceed createdBefore", ErrInvalidRequest)
		}
	}
	if after != "" || before != "" {
		bounds := map[string]any{}
		if after != "" {
			bounds["$gte"] = after
		}
		if before != "" {
			bounds["$lte"] = before
		}
		filter["creationTime"] = bounds
	}
	return filter, nil
}

func listLimits(request ListRequest) (cluster.InventoryLimits, error) {
	limit := request.Limit
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return cluster.InventoryLimits{}, fmt.Errorf("%w: limit must be between 1 and %d", ErrInvalidRequest, maxLimit)
	}
	pageSize := request.PageSize
	if pageSize == 0 || pageSize > limit {
		pageSize = limit
	}
	if pageSize < 1 || pageSize > maxLimit {
		return cluster.InventoryLimits{}, fmt.Errorf("%w: page size must be between 1 and %d", ErrInvalidRequest, maxLimit)
	}
	maxPages := request.MaxPages
	if maxPages == 0 {
		maxPages = (limit + pageSize - 1) / pageSize
	}
	if maxPages < 1 {
		return cluster.InventoryLimits{}, fmt.Errorf("%w: max pages must be positive", ErrInvalidRequest)
	}
	return cluster.InventoryLimits{
		PageSize: pageSize, MaxPages: maxPages, MaxItems: limit, MaxBodyBytes: 4 << 20,
	}, nil
}

func validateExactKey(key string) error {
	if !exactKeyPattern.MatchString(key) {
		return fmt.Errorf("%w: incident keys must be canonical positive decimal strings", ErrInvalidRequest)
	}
	if _, err := strconv.ParseUint(key, 10, 64); err != nil {
		return fmt.Errorf("%w: incident key exceeds unsigned 64-bit range", ErrInvalidRequest)
	}
	return nil
}

func normalizeTime(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if value != strings.TrimSpace(value) {
		return "", errors.New("must not contain surrounding whitespace")
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", errors.New("must be an RFC3339 timestamp")
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func emptyResult() Result {
	return Result{
		Status: StatusCompleted, Complete: true,
		Warnings: []inventory.Warning{}, Incidents: []Incident{},
		Policy: Policy{Outcome: PolicyReadOnly, ExitCode: 0},
	}
}

func sourceFor(client *cluster.Client, resolved env.Resolved) inventory.Source {
	sourceType := string(client.Kind)
	if sourceType == "" {
		sourceType = resolved.Profile.Kind
		if sourceType == "lab" {
			sourceType = "local"
		}
	}
	return inventory.Source{
		Type: sourceType, Environment: resolved.Profile.Name, Endpoint: client.BaseURL,
	}
}

func fromCluster(item cluster.Incident, resolved env.Resolved, source inventory.Source) Incident {
	return Incident{
		Key: item.Key, ProcessInstanceKey: item.ProcessInstanceKey,
		RootProcessInstanceKey: item.RootProcessInstanceKey,
		ElementInstanceKey:     item.ElementInstanceKey, ElementID: item.ElementID,
		ProcessDefinitionKey: item.ProcessDefinitionKey, ProcessDefinitionID: item.ProcessDefinitionID,
		JobKey: item.JobKey, ErrorType: item.ErrorType,
		ErrorMessage: cluster.RedactAPIMessage(item.ErrorMessage),
		State:        item.State, CreationTime: item.CreationTime, TenantID: item.TenantID,
		Status: StatusCompleted, Complete: true,
		Environment: resolved, Source: source, Warnings: []inventory.Warning{},
	}
}

func exactProcessInstance(items []cluster.ProcessInstance, key string) (cluster.ProcessInstance, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return cluster.ProcessInstance{}, false
}

func exactElementInstance(items []cluster.ElementInstance, key string) (cluster.ElementInstance, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return cluster.ElementInstance{}, false
}

func exactProcessDefinition(items []cluster.ProcessDefinition, key string) (cluster.ProcessDefinition, bool) {
	for _, item := range items {
		if item.Key == key {
			return item, true
		}
	}
	return cluster.ProcessDefinition{}, false
}

func pointerValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func markPartial(result *Result, capability, message string) {
	result.Partial, result.Complete, result.Status = true, false, StatusPartial
	addWarning(&result.Warnings, capability, message)
}

func addWarning(warnings *[]inventory.Warning, capability, message string) {
	*warnings = append(*warnings, inventory.Warning{Capability: capability, Message: message})
}

func nonNilWarnings(warnings []inventory.Warning) []inventory.Warning {
	return append([]inventory.Warning{}, warnings...)
}

func sortWarnings(warnings []inventory.Warning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		if warnings[i].Capability != warnings[j].Capability {
			return warnings[i].Capability < warnings[j].Capability
		}
		return warnings[i].Message < warnings[j].Message
	})
}
