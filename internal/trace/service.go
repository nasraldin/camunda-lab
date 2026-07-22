package trace

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
	defaultLimit     = 200
	maxLimit         = 500
	defaultTimeout   = 5 * time.Minute
	defaultMaxEvents = 500
	minPollInterval  = 100 * time.Millisecond
)

var exactKeyPattern = regexp.MustCompile(`^[1-9][0-9]*$`)

type Service struct {
	Factory cluster.Factory
}

func NewService(factory cluster.Factory) *Service {
	return &Service{Factory: factory}
}

func (s *Service) Get(ctx context.Context, request Request) (Timeline, error) {
	result := emptyTimeline()
	if err := ctx.Err(); err != nil {
		return result, err
	}
	if s == nil || s.Factory == nil {
		return result, errors.New("trace service requires a cluster factory")
	}
	if err := validateExactKey(request.ProcessInstanceKey); err != nil {
		return result, err
	}
	limits, err := searchLimits(request)
	if err != nil {
		return result, err
	}
	client, resolved, err := s.Factory.Client(ctx, request.Environment, request.ProjectRoot)
	result.Environment = resolved
	if err != nil {
		return result, fmt.Errorf("resolve trace environment: %w", err)
	}
	result.Source = sourceFor(client, resolved)
	result.InstanceKey = request.ProcessInstanceKey

	instance, found, err := client.GetProcessInstanceExact(ctx, request.ProcessInstanceKey)
	if err != nil {
		return result, fmt.Errorf("get process instance %s: %w", request.ProcessInstanceKey, err)
	}
	if !found {
		return result, &NotFoundError{Key: request.ProcessInstanceKey}
	}
	result.State = instance.State

	events := make([]Event, 0, 8)
	appendProcessEvents(&events, instance)

	elements, elementWarnings, elementPartial := searchScoped(
		ctx, "element-instances",
		func() (cluster.SearchResult[cluster.ElementInstance], error) {
			return client.SearchElementInstancesInventory(ctx, map[string]any{
				"processInstanceKey": request.ProcessInstanceKey,
			}, limits)
		},
	)
	appendWarnings(&result.Warnings, elementWarnings...)
	if elementPartial {
		result.Partial = true
	}
	appendElementEvents(&events, elements)

	incidents, incidentWarnings, incidentPartial := searchScoped(
		ctx, "incidents",
		func() (cluster.SearchResult[cluster.Incident], error) {
			return client.SearchIncidentsInventory(ctx, map[string]any{
				"processInstanceKey": request.ProcessInstanceKey,
			}, limits)
		},
	)
	appendWarnings(&result.Warnings, incidentWarnings...)
	if incidentPartial {
		result.Partial = true
	}
	appendIncidentEvents(&events, incidents)

	jobs, jobWarnings, jobPartial := searchScoped(
		ctx, "jobs",
		func() (cluster.SearchResult[cluster.Job], error) {
			return client.SearchJobsInventory(ctx, map[string]any{
				"processInstanceKey": request.ProcessInstanceKey,
			}, limits)
		},
	)
	appendWarnings(&result.Warnings, jobWarnings...)
	if jobPartial {
		result.Partial = true
	}
	appendJobEvents(&events, jobs)

	sortEvents(events)
	result.Events = events
	result.Steps = stepsFrom(elements, incidents)
	if operateBase := strings.TrimSpace(resolved.Profile.Endpoints["operate"]); operateBase != "" {
		link, linkErr := OperateLink(operateBase, request.ProcessInstanceKey)
		if linkErr != nil {
			appendWarnings(&result.Warnings, inventory.Warning{Capability: "operate-link", Message: linkErr.Error()})
		} else {
			result.OperateURL = link
		}
	}
	sortWarnings(result.Warnings)
	if result.Partial || len(result.Warnings) > 0 {
		result.Status, result.Complete, result.Partial = StatusPartial, false, true
	} else {
		result.Status, result.Complete, result.Partial = StatusCompleted, true, false
	}
	return result, nil
}

func searchScoped[T any](
	ctx context.Context,
	capability string,
	search func() (cluster.SearchResult[T], error),
) ([]T, []inventory.Warning, bool) {
	if err := ctx.Err(); err != nil {
		return nil, []inventory.Warning{{Capability: capability, Message: err.Error()}}, true
	}
	result, err := search()
	if err != nil {
		return nil, []inventory.Warning{{Capability: capability, Message: err.Error()}}, true
	}
	warnings := append([]inventory.Warning{}, result.Warnings...)
	if result.Partial {
		warnings = append(warnings, inventory.Warning{
			Capability: capability,
			Message:    capability + " search was partial",
		})
	}
	return result.Items, warnings, result.Partial
}

func appendProcessEvents(events *[]Event, instance cluster.ProcessInstance) {
	name := instance.ProcessDefinitionID
	if instance.ProcessDefinitionName != nil && strings.TrimSpace(*instance.ProcessDefinitionName) != "" {
		name = *instance.ProcessDefinitionName
	}
	*events = append(*events, Event{
		Kind: EventProcess, Key: instance.Key, Timestamp: instance.StartDate,
		Status: instance.State, Name: name, ProcessInstanceKey: instance.Key,
		Type: "process-instance", Phase: "start",
	})
	if instance.EndDate != "" {
		*events = append(*events, Event{
			Kind: EventProcess, Key: instance.Key, Timestamp: instance.EndDate,
			Status: instance.State, Name: name, ProcessInstanceKey: instance.Key,
			Type: "process-instance", Phase: "end",
		})
	}
}

func appendElementEvents(events *[]Event, elements []cluster.ElementInstance) {
	for _, item := range elements {
		name := item.ElementName
		if name == "" {
			name = item.ElementID
		}
		detail := ""
		status := item.State
		if item.IncidentKey != "" {
			status = "INCIDENT"
			detail = item.IncidentKey
		}
		*events = append(*events, Event{
			Kind: EventElement, Key: item.Key, Timestamp: item.StartDate, Status: status,
			Name: name, Detail: detail, ElementID: item.ElementID,
			ProcessInstanceKey: item.ProcessInstanceKey, Type: item.Type, Phase: "start",
		})
		if item.EndDate != "" {
			*events = append(*events, Event{
				Kind: EventElement, Key: item.Key, Timestamp: item.EndDate, Status: item.State,
				Name: name, ElementID: item.ElementID, ProcessInstanceKey: item.ProcessInstanceKey,
				Type: item.Type, Phase: "end",
			})
		}
	}
}

func appendIncidentEvents(events *[]Event, incidents []cluster.Incident) {
	for _, item := range incidents {
		*events = append(*events, Event{
			Kind: EventIncident, Key: item.Key, Timestamp: item.CreationTime, Status: item.State,
			Name: item.ErrorType, Detail: cluster.RedactAPIMessage(item.ErrorMessage),
			ElementID: item.ElementID, ProcessInstanceKey: item.ProcessInstanceKey,
			Type: item.ErrorType,
		})
	}
}

func appendJobEvents(events *[]Event, jobs []cluster.Job) {
	for _, item := range jobs {
		timestamp, phase := jobEventTimestamp(item)
		if timestamp == "" {
			continue
		}
		*events = append(*events, Event{
			Kind: EventJob, Key: item.Key, Timestamp: timestamp, Status: item.State,
			Name: item.Type, Detail: cluster.RedactAPIMessage(item.ErrorMessage),
			ElementID: item.ElementID, ProcessInstanceKey: item.ProcessInstanceKey,
			Type: item.Type, Phase: phase,
		})
	}
}

// jobEventTimestamp picks the first real timestamp without inventing times.
// Prefer creation → lastUpdate → endTime → deadline; end-phase when only
// endTime/deadline remain.
func jobEventTimestamp(item cluster.Job) (timestamp, phase string) {
	if item.CreationTime != "" {
		return item.CreationTime, ""
	}
	if item.LastUpdateTime != "" {
		return item.LastUpdateTime, ""
	}
	if item.EndTime != "" {
		return item.EndTime, "end"
	}
	if item.Deadline != "" {
		return item.Deadline, "end"
	}
	return "", ""
}

func stepsFrom(elements []cluster.ElementInstance, incidents []cluster.Incident) []Step {
	incidentByElement := map[string]cluster.Incident{}
	for _, item := range incidents {
		if item.ElementInstanceKey != "" {
			incidentByElement[item.ElementInstanceKey] = item
		}
	}
	ordered := append([]cluster.ElementInstance{}, elements...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].StartDate != ordered[j].StartDate {
			return ordered[i].StartDate < ordered[j].StartDate
		}
		return ordered[i].Key < ordered[j].Key
	})
	steps := make([]Step, 0, len(ordered))
	for _, item := range ordered {
		name := item.ElementName
		if name == "" {
			name = item.ElementID
		}
		status := item.State
		detail := ""
		if item.IncidentKey != "" {
			status = "INCIDENT"
			detail = item.IncidentKey
		}
		if incident, ok := incidentByElement[item.Key]; ok {
			status = "INCIDENT"
			if detail == "" {
				detail = cluster.RedactAPIMessage(incident.ErrorMessage)
			}
		}
		steps = append(steps, Step{Name: name, State: status, Detail: detail})
	}
	return steps
}

func sortEvents(events []Event) {
	sort.SliceStable(events, func(i, j int) bool {
		if events[i].Timestamp != events[j].Timestamp {
			if events[i].Timestamp == "" {
				return false
			}
			if events[j].Timestamp == "" {
				return true
			}
			return events[i].Timestamp < events[j].Timestamp
		}
		if rank := eventKindRank(events[i].Kind) - eventKindRank(events[j].Kind); rank != 0 {
			return rank < 0
		}
		if rank := phaseRank(events[i].Phase) - phaseRank(events[j].Phase); rank != 0 {
			return rank < 0
		}
		return events[i].Key < events[j].Key
	})
}

func eventKindRank(kind EventKind) int {
	switch kind {
	case EventProcess:
		return 0
	case EventElement:
		return 1
	case EventIncident:
		return 2
	case EventJob:
		return 3
	default:
		return 9
	}
}

func phaseRank(phase string) int {
	switch phase {
	case "start":
		return 0
	case "":
		return 1
	case "end":
		return 2
	default:
		return 3
	}
}

func searchLimits(request Request) (cluster.InventoryLimits, error) {
	limit := request.MaxItems
	if limit == 0 {
		limit = defaultLimit
	}
	if limit < 1 || limit > maxLimit {
		return cluster.InventoryLimits{}, fmt.Errorf("%w: max items must be between 1 and %d", ErrInvalidRequest, maxLimit)
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
		if maxPages < 1 {
			maxPages = 1
		}
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
		return fmt.Errorf("%w: process instance keys must be canonical positive decimal strings", ErrInvalidRequest)
	}
	if _, err := strconv.ParseUint(key, 10, 64); err != nil {
		return fmt.Errorf("%w: process instance key exceeds unsigned 64-bit range", ErrInvalidRequest)
	}
	return nil
}

func emptyTimeline() Timeline {
	return Timeline{
		Status: StatusCompleted, Complete: true,
		Events: []Event{}, Steps: []Step{}, Warnings: []inventory.Warning{},
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

func appendWarnings(dst *[]inventory.Warning, warnings ...inventory.Warning) {
	*dst = append(*dst, warnings...)
}

func sortWarnings(warnings []inventory.Warning) {
	sort.SliceStable(warnings, func(i, j int) bool {
		if warnings[i].Capability != warnings[j].Capability {
			return warnings[i].Capability < warnings[j].Capability
		}
		return warnings[i].Message < warnings[j].Message
	})
}

// OperateLink builds a validated Operate deep link for a process instance.
func OperateLink(baseURL, processInstanceKey string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", errors.New("operate base URL must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.Fragment != "" {
		return "", errors.New("operate base URL must not contain userinfo or fragment")
	}
	if processInstanceKey == "" {
		return "", errors.New("operate link requires a process instance key")
	}
	basePath := strings.TrimRight(parsed.Path, "/") + "/processes/"
	escapedBasePath := strings.TrimRight(parsed.EscapedPath(), "/") + "/processes/"
	parsed.Path = basePath + processInstanceKey
	parsed.RawPath = escapedBasePath + url.PathEscape(processInstanceKey)
	parsed.RawQuery = ""
	return parsed.String(), nil
}
