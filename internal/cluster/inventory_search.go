package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/inventory"
)

// SearchResult preserves bounded-search completeness independently of policy.
type SearchResult[T any] struct {
	Items    []T
	Warnings []inventory.Warning
	Partial  bool
}

// DecisionDefinition is a deployed decision summary.
type DecisionDefinition struct {
	Key                         string
	DecisionDefinitionID        string
	Name                        *string
	Version                     int
	TenantID                    string
	DecisionRequirementsID      *string
	DecisionRequirementsKey     *string
	DecisionRequirementsName    *string
	DecisionRequirementsVersion *int
}

// Job is a normalized runtime job record.
type Job struct {
	Key, ProcessInstanceKey, ElementInstanceKey, ProcessDefinitionKey string
	RootProcessInstanceKey, ProcessDefinitionID, ElementID            string
	Type, State, Worker, TenantID, Kind, ListenerEventType            string
	Retries                                                           int
	Deadline, EndTime, CreationTime, LastUpdateTime                   string
	DeniedReason, ErrorCode, ErrorMessage                             string
	HasFailedWithRetriesLeft                                          bool
	IsDenied                                                          *bool
	CustomHeaders                                                     map[string]string
}

// Topology is the normalized read-only broker/partition view.
type Topology struct {
	Brokers           []Broker
	ClusterSize       int
	PartitionsCount   int
	ReplicationFactor int
	GatewayVersion    string
}

type Broker struct {
	NodeID     int
	Host       string
	Port       int
	Partitions []Partition
}

type Partition struct {
	ID     int
	Role   string
	Health string
}

// SearchDecisionDefinitionsInventory inventories deployed decision summaries
// when the target API version exposes the capability.
func (c *Client) SearchDecisionDefinitionsInventory(ctx context.Context, filter map[string]any, limits InventoryLimits) (SearchResult[DecisionDefinition], error) {
	return searchTyped(ctx, c, "/decision-definitions/search", filter, limits, "decision-definitions",
		func(raw json.RawMessage) (DecisionDefinition, string, error) {
			var wire struct {
				DecisionDefinitionKey       identifier  `json:"decisionDefinitionKey"`
				DecisionDefinitionID        string      `json:"decisionDefinitionId"`
				DecisionRequirementsID      *string     `json:"decisionRequirementsId"`
				DecisionRequirementsKey     *identifier `json:"decisionRequirementsKey"`
				DecisionRequirementsName    *string     `json:"decisionRequirementsName"`
				DecisionRequirementsVersion *int        `json:"decisionRequirementsVersion"`
				Name                        *string     `json:"name"`
				TenantID                    string      `json:"tenantId"`
				Version                     int         `json:"version"`
			}
			if err := decodeStrictJSON(raw, &wire); err != nil {
				return DecisionDefinition{}, "", err
			}
			if wire.DecisionDefinitionKey == "" || strings.TrimSpace(wire.DecisionDefinitionID) == "" || wire.Version < 1 {
				return DecisionDefinition{}, "", errors.New("decision definition requires key, ID, and positive version")
			}
			item := DecisionDefinition{
				Key: string(wire.DecisionDefinitionKey), DecisionDefinitionID: wire.DecisionDefinitionID,
				Name: wire.Name, Version: wire.Version, TenantID: wire.TenantID,
				DecisionRequirementsID:      wire.DecisionRequirementsID,
				DecisionRequirementsName:    wire.DecisionRequirementsName,
				DecisionRequirementsVersion: wire.DecisionRequirementsVersion,
			}
			if wire.DecisionRequirementsKey != nil {
				value := string(*wire.DecisionRequirementsKey)
				item.DecisionRequirementsKey = &value
			}
			return item, item.Key, nil
		})
}

// SearchIncidentsInventory is the canonical bounded incident search used by
// later incident consumers.
func (c *Client) SearchIncidentsInventory(ctx context.Context, filter map[string]any, limits InventoryLimits) (SearchResult[Incident], error) {
	return searchTyped(ctx, c, "/incidents/search", filter, limits, "incidents", decodeIncident)
}

func decodeIncident(raw json.RawMessage) (Incident, string, error) {
	var wire struct {
		IncidentKey            identifier  `json:"incidentKey"`
		ProcessInstanceKey     identifier  `json:"processInstanceKey"`
		RootProcessInstanceKey *identifier `json:"rootProcessInstanceKey"`
		ElementID              string      `json:"elementId"`
		ElementInstanceKey     identifier  `json:"elementInstanceKey"`
		ErrorType              string      `json:"errorType"`
		ErrorMessage           string      `json:"errorMessage"`
		State                  string      `json:"state"`
		CreationTime           string      `json:"creationTime"`
		JobKey                 *identifier `json:"jobKey"`
		ProcessDefinitionID    string      `json:"processDefinitionId"`
		ProcessDefinitionKey   identifier  `json:"processDefinitionKey"`
		TenantID               string      `json:"tenantId"`
	}
	if err := decodeStrictJSON(raw, &wire); err != nil {
		return Incident{}, "", err
	}
	if wire.IncidentKey == "" {
		return Incident{}, "", errors.New("incidentKey is required")
	}
	created, err := normalizedTimestamp(wire.CreationTime)
	if err != nil {
		return Incident{}, "", fmt.Errorf("creationTime: %w", err)
	}
	item := Incident{
		Key: string(wire.IncidentKey), ProcessInstanceKey: string(wire.ProcessInstanceKey),
		ElementID: wire.ElementID, ErrorType: wire.ErrorType, ErrorMessage: wire.ErrorMessage,
		State: normalizedStatus(wire.State), CreationTime: created, JobKey: identifierValue(wire.JobKey),
		ProcessDefinitionID:    wire.ProcessDefinitionID,
		ProcessDefinitionKey:   string(wire.ProcessDefinitionKey),
		RootProcessInstanceKey: identifierValue(wire.RootProcessInstanceKey),
		ElementInstanceKey:     string(wire.ElementInstanceKey), TenantID: wire.TenantID,
	}
	return item, item.Key, nil
}

// SearchProcessInstancesInventory is the canonical bounded instance search.
func (c *Client) SearchProcessInstancesInventory(ctx context.Context, filter map[string]any, limits InventoryLimits) (SearchResult[ProcessInstance], error) {
	return searchTyped(ctx, c, "/process-instances/search", filter, limits, "process-instances", decodeProcessInstance)
}

func decodeProcessInstance(raw json.RawMessage) (ProcessInstance, string, error) {
	var wire struct {
		ProcessInstanceKey          identifier  `json:"processInstanceKey"`
		ProcessDefinitionID         string      `json:"processDefinitionId"`
		ProcessDefinitionKey        identifier  `json:"processDefinitionKey"`
		ProcessDefinitionVersion    int         `json:"processDefinitionVersion"`
		ProcessDefinitionName       *string     `json:"processDefinitionName"`
		ProcessDefinitionVersionTag *string     `json:"processDefinitionVersionTag"`
		StartDate                   string      `json:"startDate"`
		EndDate                     *string     `json:"endDate"`
		State                       string      `json:"state"`
		HasIncident                 bool        `json:"hasIncident"`
		TenantID                    string      `json:"tenantId"`
		ParentProcessInstanceKey    *identifier `json:"parentProcessInstanceKey"`
		ParentElementInstanceKey    *identifier `json:"parentElementInstanceKey"`
		RootProcessInstanceKey      *identifier `json:"rootProcessInstanceKey"`
		Tags                        []string    `json:"tags"`
		BusinessID                  *string     `json:"businessId"`
	}
	if err := decodeStrictJSON(raw, &wire); err != nil {
		return ProcessInstance{}, "", err
	}
	if wire.ProcessInstanceKey == "" {
		return ProcessInstance{}, "", errors.New("processInstanceKey is required")
	}
	start, err := normalizedTimestamp(wire.StartDate)
	if err != nil {
		return ProcessInstance{}, "", fmt.Errorf("startDate: %w", err)
	}
	end, err := normalizedOptionalTimestamp(wire.EndDate)
	if err != nil {
		return ProcessInstance{}, "", fmt.Errorf("endDate: %w", err)
	}
	item := ProcessInstance{
		Key: string(wire.ProcessInstanceKey), ProcessDefinitionID: wire.ProcessDefinitionID,
		ProcessDefinitionName:       wire.ProcessDefinitionName,
		ProcessDefinitionVersion:    wire.ProcessDefinitionVersion,
		ProcessDefinitionVersionTag: wire.ProcessDefinitionVersionTag,
		ProcessDefinitionKey:        string(wire.ProcessDefinitionKey),
		StartDate:                   start, EndDate: end,
		State: normalizedStatus(wire.State), HasIncident: wire.HasIncident,
		TenantID:                 wire.TenantID,
		ParentProcessInstanceKey: identifierValue(wire.ParentProcessInstanceKey),
		ParentElementInstanceKey: identifierValue(wire.ParentElementInstanceKey),
		RootProcessInstanceKey:   identifierValue(wire.RootProcessInstanceKey),
		Tags:                     append([]string(nil), wire.Tags...), BusinessID: pointerString(wire.BusinessID),
	}
	return item, item.Key, nil
}

// SearchElementInstancesInventory is the canonical bounded trace search.
func (c *Client) SearchElementInstancesInventory(ctx context.Context, filter map[string]any, limits InventoryLimits) (SearchResult[ElementInstance], error) {
	return searchTyped(ctx, c, "/element-instances/search", filter, limits, "element-instances", decodeElementInstance)
}

func decodeElementInstance(raw json.RawMessage) (ElementInstance, string, error) {
	var wire struct {
		ElementInstanceKey     identifier  `json:"elementInstanceKey"`
		ProcessInstanceKey     identifier  `json:"processInstanceKey"`
		RootProcessInstanceKey *identifier `json:"rootProcessInstanceKey"`
		ProcessDefinitionKey   identifier  `json:"processDefinitionKey"`
		ProcessDefinitionID    string      `json:"processDefinitionId"`
		ElementID              string      `json:"elementId"`
		ElementName            string      `json:"elementName"`
		Type                   string      `json:"type"`
		State                  string      `json:"state"`
		StartDate              string      `json:"startDate"`
		EndDate                *string     `json:"endDate"`
		HasIncident            bool        `json:"hasIncident"`
		IncidentKey            *identifier `json:"incidentKey"`
		TenantID               string      `json:"tenantId"`
	}
	if err := decodeStrictJSON(raw, &wire); err != nil {
		return ElementInstance{}, "", err
	}
	if wire.ElementInstanceKey == "" {
		return ElementInstance{}, "", errors.New("elementInstanceKey is required")
	}
	start, err := normalizedTimestamp(wire.StartDate)
	if err != nil {
		return ElementInstance{}, "", fmt.Errorf("startDate: %w", err)
	}
	end, err := normalizedOptionalTimestamp(wire.EndDate)
	if err != nil {
		return ElementInstance{}, "", fmt.Errorf("endDate: %w", err)
	}
	item := ElementInstance{
		Key: string(wire.ElementInstanceKey), ProcessInstanceKey: string(wire.ProcessInstanceKey),
		ElementID: wire.ElementID, ElementName: wire.ElementName, Type: wire.Type,
		State: normalizedStatus(wire.State), StartDate: start, EndDate: end,
		IncidentKey:            identifierValue(wire.IncidentKey),
		ProcessDefinitionID:    wire.ProcessDefinitionID,
		ProcessDefinitionKey:   string(wire.ProcessDefinitionKey),
		RootProcessInstanceKey: identifierValue(wire.RootProcessInstanceKey),
		HasIncident:            wire.HasIncident, TenantID: wire.TenantID,
	}
	return item, item.Key, nil
}

// SearchJobsInventory is the canonical bounded job search.
func (c *Client) SearchJobsInventory(ctx context.Context, filter map[string]any, limits InventoryLimits) (SearchResult[Job], error) {
	return searchTyped(ctx, c, "/jobs/search", filter, limits, "jobs",
		func(raw json.RawMessage) (Job, string, error) {
			var wire struct {
				CustomHeaders            map[string]string `json:"customHeaders"`
				Deadline                 *string           `json:"deadline"`
				DeniedReason             *string           `json:"deniedReason"`
				ElementID                *string           `json:"elementId"`
				ElementInstanceKey       identifier        `json:"elementInstanceKey"`
				EndTime                  *string           `json:"endTime"`
				ErrorCode                *string           `json:"errorCode"`
				ErrorMessage             *string           `json:"errorMessage"`
				HasFailedWithRetriesLeft bool              `json:"hasFailedWithRetriesLeft"`
				IsDenied                 *bool             `json:"isDenied"`
				JobKey                   identifier        `json:"jobKey"`
				Kind                     string            `json:"kind"`
				ListenerEventType        string            `json:"listenerEventType"`
				ProcessDefinitionID      string            `json:"processDefinitionId"`
				ProcessDefinitionKey     identifier        `json:"processDefinitionKey"`
				ProcessInstanceKey       identifier        `json:"processInstanceKey"`
				RootProcessInstanceKey   *identifier       `json:"rootProcessInstanceKey"`
				Retries                  int               `json:"retries"`
				State                    string            `json:"state"`
				TenantID                 string            `json:"tenantId"`
				Type                     string            `json:"type"`
				Worker                   string            `json:"worker"`
				CreationTime             *string           `json:"creationTime"`
				LastUpdateTime           *string           `json:"lastUpdateTime"`
			}
			if err := decodeStrictJSON(raw, &wire); err != nil {
				return Job{}, "", err
			}
			if wire.JobKey == "" {
				return Job{}, "", errors.New("jobKey is required")
			}
			deadline, err := normalizedOptionalTimestamp(wire.Deadline)
			if err != nil {
				return Job{}, "", fmt.Errorf("deadline: %w", err)
			}
			endTime, err := normalizedOptionalTimestamp(wire.EndTime)
			if err != nil {
				return Job{}, "", fmt.Errorf("endTime: %w", err)
			}
			creationTime, err := normalizedOptionalTimestamp(wire.CreationTime)
			if err != nil {
				return Job{}, "", fmt.Errorf("creationTime: %w", err)
			}
			lastUpdateTime, err := normalizedOptionalTimestamp(wire.LastUpdateTime)
			if err != nil {
				return Job{}, "", fmt.Errorf("lastUpdateTime: %w", err)
			}
			item := Job{
				Key: string(wire.JobKey), ProcessInstanceKey: string(wire.ProcessInstanceKey),
				ElementInstanceKey:     string(wire.ElementInstanceKey),
				ProcessDefinitionKey:   string(wire.ProcessDefinitionKey),
				RootProcessInstanceKey: identifierValue(wire.RootProcessInstanceKey),
				ProcessDefinitionID:    wire.ProcessDefinitionID, ElementID: pointerString(wire.ElementID),
				Type: wire.Type, State: normalizedStatus(wire.State), Worker: wire.Worker, Retries: wire.Retries,
				Deadline: deadline, EndTime: endTime, CreationTime: creationTime, LastUpdateTime: lastUpdateTime,
				DeniedReason: pointerString(wire.DeniedReason), ErrorCode: pointerString(wire.ErrorCode),
				ErrorMessage: pointerString(wire.ErrorMessage), TenantID: wire.TenantID,
				Kind: wire.Kind, ListenerEventType: wire.ListenerEventType,
				HasFailedWithRetriesLeft: wire.HasFailedWithRetriesLeft, IsDenied: wire.IsDenied,
				CustomHeaders: wire.CustomHeaders,
			}
			return item, item.Key, nil
		})
}

// GetTopologyInventory returns a strict, bounded topology snapshot.
func (c *Client) GetTopologyInventory(ctx context.Context, maxBodyBytes int64) (Topology, error) {
	if maxBodyBytes == 0 {
		maxBodyBytes = defaultInventoryBodySize
	}
	if maxBodyBytes < 1 {
		return Topology{}, errors.New("topology body limit must be positive")
	}
	data, err := c.doInventoryRequest(ctx, http.MethodGet, "/topology", nil, maxBodyBytes)
	if err != nil {
		return Topology{}, err
	}
	var wire struct {
		Brokers []struct {
			NodeID     int    `json:"nodeId"`
			Host       string `json:"host"`
			Port       int    `json:"port"`
			Partitions []struct {
				PartitionID int    `json:"partitionId"`
				Role        string `json:"role"`
				Health      string `json:"health"`
			} `json:"partitions"`
		} `json:"brokers"`
		ClusterSize           int        `json:"clusterSize"`
		PartitionsCount       int        `json:"partitionsCount"`
		ReplicationFactor     int        `json:"replicationFactor"`
		GatewayVersion        string     `json:"gatewayVersion"`
		LastCompletedChangeID identifier `json:"lastCompletedChangeId"`
	}
	if err := decodeStrictJSON(data, &wire); err != nil {
		return Topology{}, fmt.Errorf("decode topology response: %w", err)
	}
	result := Topology{
		ClusterSize: wire.ClusterSize, PartitionsCount: wire.PartitionsCount,
		ReplicationFactor: wire.ReplicationFactor, GatewayVersion: wire.GatewayVersion,
	}
	for _, broker := range wire.Brokers {
		normalized := Broker{NodeID: broker.NodeID, Host: broker.Host, Port: broker.Port}
		for _, partition := range broker.Partitions {
			normalized.Partitions = append(normalized.Partitions, Partition{
				ID: partition.PartitionID, Role: normalizedStatus(partition.Role),
				Health: normalizedStatus(partition.Health),
			})
		}
		sort.SliceStable(normalized.Partitions, func(i, j int) bool {
			return normalized.Partitions[i].ID < normalized.Partitions[j].ID
		})
		result.Brokers = append(result.Brokers, normalized)
	}
	sort.SliceStable(result.Brokers, func(i, j int) bool {
		return result.Brokers[i].NodeID < result.Brokers[j].NodeID
	})
	return result, nil
}

type typedDecoder[T any] func(json.RawMessage) (T, string, error)

func searchTypedPage[T any](
	ctx context.Context,
	client *Client,
	path string,
	filter map[string]any,
	limits InventoryLimits,
	capability string,
	decode typedDecoder[T],
) ([]T, error) {
	normalized, err := normalizeInventoryLimits(limits)
	if err != nil {
		return nil, err
	}
	body, err := json.Marshal(map[string]any{
		"filter": filter,
		"page":   map[string]any{"limit": normalized.PageSize},
	})
	if err != nil {
		return nil, err
	}
	data, err := client.doInventoryRequest(ctx, http.MethodPost, path, body, normalized.MaxBodyBytes)
	if err != nil {
		return nil, err
	}
	rawItems, _, err := decodeSearchEnvelope(data)
	if err != nil {
		return nil, fmt.Errorf("decode %s search response: %w", capability, err)
	}
	if len(rawItems) > normalized.MaxItems {
		rawItems = rawItems[:normalized.MaxItems]
	}
	items := make([]T, 0, len(rawItems))
	for _, raw := range rawItems {
		item, _, err := decode(raw)
		if err != nil {
			return nil, fmt.Errorf("decode %s item: %w", capability, err)
		}
		items = append(items, item)
	}
	return items, nil
}

func searchTyped[T any](
	ctx context.Context,
	client *Client,
	path string,
	filter map[string]any,
	limits InventoryLimits,
	capability string,
	decode typedDecoder[T],
) (SearchResult[T], error) {
	normalized, err := normalizeInventoryLimits(limits)
	if err != nil {
		return SearchResult[T]{}, err
	}
	byID := make(map[string]T)
	cursor := ""
	seenCursors := map[string]struct{}{}
	for pageNumber := 1; pageNumber <= normalized.MaxPages; pageNumber++ {
		pageBody := map[string]any{"limit": normalized.PageSize}
		if cursor != "" {
			pageBody["after"] = cursor
		}
		body, err := json.Marshal(map[string]any{"filter": filter, "page": pageBody})
		if err != nil {
			return SearchResult[T]{}, err
		}
		data, err := client.doInventoryRequest(ctx, http.MethodPost, path, body, normalized.MaxBodyBytes)
		if err != nil {
			return SearchResult[T]{}, err
		}
		rawItems, metadata, err := decodeSearchEnvelope(data)
		if err != nil {
			return SearchResult[T]{}, fmt.Errorf("decode %s search response: %w", capability, err)
		}
		added := 0
		for _, raw := range rawItems {
			item, id, err := decode(raw)
			if err != nil {
				return SearchResult[T]{}, fmt.Errorf("decode %s item: %w", capability, err)
			}
			if _, duplicate := byID[id]; duplicate {
				continue
			}
			if len(byID) == normalized.MaxItems {
				return typedResult(byID, []inventory.Warning{{
					Capability: capability,
					Message:    fmt.Sprintf("item limit %d reached before inventory completed", normalized.MaxItems),
				}}, true), nil
			}
			byID[id] = item
			added++
		}
		if metadata.EndCursor == "" {
			if metadata.HasMoreTotalItems || metadata.TotalItems > uint64(len(byID)) {
				return typedResult(byID, []inventory.Warning{{
					Capability: capability, Message: "missing continuation cursor on a full page",
				}}, true), nil
			}
			return typedResult(byID, nil, false), nil
		}
		if _, repeated := seenCursors[metadata.EndCursor]; repeated || metadata.EndCursor == cursor {
			return typedResult(byID, []inventory.Warning{{
				Capability: capability, Message: "repeated pagination cursor prevented completion",
			}}, true), nil
		}
		if added == 0 {
			return typedResult(byID, []inventory.Warning{{
				Capability: capability, Message: "pagination made no item progress",
			}}, true), nil
		}
		seenCursors[metadata.EndCursor] = struct{}{}
		cursor = metadata.EndCursor
		if pageNumber == normalized.MaxPages {
			return typedResult(byID, []inventory.Warning{{
				Capability: capability,
				Message:    fmt.Sprintf("page limit %d reached before inventory completed", normalized.MaxPages),
			}}, true), nil
		}
	}
	return SearchResult[T]{}, errors.New("inventory pagination terminated unexpectedly")
}

type searchEnvelope struct {
	Items []json.RawMessage `json:"items"`
	Page  *struct {
		TotalItems        *json.Number `json:"totalItems"`
		HasMoreTotalItems *bool        `json:"hasMoreTotalItems"`
		StartCursor       *string      `json:"startCursor"`
		EndCursor         *string      `json:"endCursor"`
	} `json:"page"`
}

type searchPageMetadata struct {
	TotalItems        uint64
	HasMoreTotalItems bool
	EndCursor         string
}

func decodeSearchEnvelope(data []byte) ([]json.RawMessage, searchPageMetadata, error) {
	var envelope searchEnvelope
	if err := decodeStrictJSON(data, &envelope); err != nil {
		return nil, searchPageMetadata{}, err
	}
	if envelope.Items == nil {
		return nil, searchPageMetadata{}, errors.New("response is missing items")
	}
	if envelope.Page == nil {
		return nil, searchPageMetadata{}, errors.New("response is missing page metadata")
	}
	if envelope.Page.TotalItems == nil {
		return nil, searchPageMetadata{}, errors.New("page metadata is missing totalItems")
	}
	totalItems, err := strconv.ParseUint(string(*envelope.Page.TotalItems), 10, 64)
	if err != nil {
		return nil, searchPageMetadata{}, errors.New("totalItems must be a non-negative integer")
	}
	metadata := searchPageMetadata{TotalItems: totalItems}
	if envelope.Page.HasMoreTotalItems != nil {
		metadata.HasMoreTotalItems = *envelope.Page.HasMoreTotalItems
	}
	if envelope.Page.EndCursor != nil {
		metadata.EndCursor = *envelope.Page.EndCursor
	}
	return envelope.Items, metadata, nil
}

func decodeStrictJSON(data []byte, out any) error {
	if err := rejectDuplicateJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("trailing JSON value")
	}
	return nil
}

func rejectDuplicateJSON(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := scanJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return err
		}
		return errors.New("trailing JSON value")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON field %q", key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil || closeToken != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := scanJSONValue(decoder); err != nil {
				return err
			}
		}
		closeToken, err := decoder.Token()
		if err != nil || closeToken != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

type identifier string

func (value *identifier) UnmarshalJSON(raw []byte) error {
	decoded, err := decodeIdentifier(json.NewDecoder(bytes.NewReader(raw)))
	if err != nil {
		return err
	}
	*value = identifier(decoded)
	return nil
}

func typedResult[T any](values map[string]T, warnings []inventory.Warning, partial bool) SearchResult[T] {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	items := make([]T, 0, len(keys))
	for _, key := range keys {
		items = append(items, values[key])
	}
	return SearchResult[T]{Items: items, Warnings: warnings, Partial: partial}
}

func normalizedStatus(value string) string {
	return strings.ToUpper(strings.TrimSpace(value))
}

func normalizedTimestamp(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	parsed, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return "", errors.New("must be an RFC3339 timestamp")
	}
	return parsed.UTC().Format(time.RFC3339Nano), nil
}

func normalizedOptionalTimestamp(value *string) (string, error) {
	if value == nil {
		return "", nil
	}
	return normalizedTimestamp(*value)
}

func identifierValue(value *identifier) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func pointerString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
