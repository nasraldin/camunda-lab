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

	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

const (
	defaultInventoryPageSize = 100
	defaultInventoryMaxPages = 100
	defaultInventoryMaxItems = 10_000
	defaultInventoryBodySize = 4 << 20
	maxInventoryPageSize     = 500
)

// InventoryLimits bounds every remote search and response.
type InventoryLimits struct {
	PageSize     int
	MaxPages     int
	MaxItems     int
	MaxBodyBytes int64
}

// InventoryRequest selects an environment through the P2 factory.
type InventoryRequest struct {
	Environment string
	ProjectRoot string
	Limits      InventoryLimits
}

// BuildClusterInventory resolves environment metadata and builds one canonical
// read-only cluster snapshot.
func BuildClusterInventory(ctx context.Context, factory Factory, request InventoryRequest) (inventory.Inventory, env.Resolved, error) {
	client, resolved, err := factory.Client(ctx, request.Environment, request.ProjectRoot)
	if err != nil {
		return inventory.Inventory{}, env.Resolved{}, err
	}
	result, err := client.BuildInventory(ctx, inventory.Source{
		Type: string(client.Kind), Environment: resolved.Profile.Name,
		Endpoint: client.BaseURL,
	}, request.Limits)
	if err != nil {
		return inventory.Inventory{}, resolved, err
	}
	return result, resolved, nil
}

// BuildInventory fetches deployed resources without mutating the cluster.
func (c *Client) BuildInventory(ctx context.Context, source inventory.Source, limits InventoryLimits) (inventory.Inventory, error) {
	normalized, err := normalizeInventoryLimits(limits)
	if err != nil {
		return inventory.Inventory{}, err
	}
	definitions, warnings, partial, err := c.searchAllProcessDefinitions(ctx, normalized)
	if err != nil {
		return inventory.Inventory{}, err
	}
	result := inventory.Inventory{
		Source: source, Warnings: warnings, Partial: partial,
		Unsupported: []inventory.Unsupported{
			{Kind: inventory.KindDecision, Reason: "decision definition content inventory is unavailable in this API contract", Required: false},
			{Kind: inventory.KindForm, Reason: "form resource content inventory is unavailable in this API contract", Required: false},
		},
	}
	for _, definition := range definitions {
		xmlBody, err := c.GetProcessDefinitionXML(ctx, definition.Key)
		if err != nil {
			return inventory.Inventory{}, fmt.Errorf("fetch process definition XML for key %s: %w", definition.Key, err)
		}
		digest, err := inventory.DigestCanonicalProcess([]byte(xmlBody), definition.ProcessDefinitionID)
		if err != nil {
			return inventory.Inventory{}, fmt.Errorf(
				"canonicalize process definition XML for key %s and ID %q: %w",
				definition.Key, definition.ProcessDefinitionID, err,
			)
		}
		result.Resources = append(result.Resources, inventory.Resource{
			Kind: inventory.KindProcess, ID: definition.ProcessDefinitionID,
			Key: definition.Key, Name: stringValue(definition.Name), Version: int64(definition.Version),
			Path: definition.ResourceName, Digest: digest, TenantID: definition.TenantID,
			Source: source,
		})
	}
	sortRemoteInventory(&result)
	return result, nil
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func normalizeInventoryLimits(value InventoryLimits) (InventoryLimits, error) {
	if value.PageSize == 0 {
		value.PageSize = defaultInventoryPageSize
	}
	if value.MaxPages == 0 {
		value.MaxPages = defaultInventoryMaxPages
	}
	if value.MaxItems == 0 {
		value.MaxItems = defaultInventoryMaxItems
	}
	if value.MaxBodyBytes == 0 {
		value.MaxBodyBytes = defaultInventoryBodySize
	}
	if value.PageSize < 1 || value.PageSize > maxInventoryPageSize {
		return InventoryLimits{}, fmt.Errorf("inventory page size must be between 1 and %d", maxInventoryPageSize)
	}
	if value.MaxPages < 1 || value.MaxItems < 1 || value.MaxBodyBytes < 1 {
		return InventoryLimits{}, errors.New("inventory page, item, and body limits must be positive")
	}
	return value, nil
}

type processSearchPage struct {
	Items             []ProcessDefinition
	EndCursor         string
	TotalItems        uint64
	HasMoreTotalItems bool
}

func (c *Client) searchAllProcessDefinitions(ctx context.Context, limits InventoryLimits) ([]ProcessDefinition, []inventory.Warning, bool, error) {
	byKey := make(map[string]ProcessDefinition)
	cursor := ""
	seenCursors := make(map[string]struct{})
	for pageNumber := 1; pageNumber <= limits.MaxPages; pageNumber++ {
		page, err := c.searchProcessDefinitionPage(ctx, limits, cursor)
		if err != nil {
			return nil, nil, false, err
		}
		added := 0
		for _, item := range page.Items {
			if _, duplicate := byKey[item.Key]; duplicate {
				continue
			}
			if len(byKey) == limits.MaxItems {
				items := processDefinitionsFromMap(byKey)
				return items, []inventory.Warning{{
					Capability: "process-definitions",
					Message:    fmt.Sprintf("item limit %d reached before inventory completed", limits.MaxItems),
				}}, true, nil
			}
			byKey[item.Key] = item
			added++
		}
		if page.EndCursor == "" {
			if page.HasMoreTotalItems || page.TotalItems > uint64(len(byKey)) {
				return processDefinitionsFromMap(byKey), []inventory.Warning{{
					Capability: "process-definitions",
					Message:    "missing continuation cursor on a full page",
				}}, true, nil
			}
			return processDefinitionsFromMap(byKey), nil, false, nil
		}
		if _, repeated := seenCursors[page.EndCursor]; repeated || page.EndCursor == cursor {
			return processDefinitionsFromMap(byKey), []inventory.Warning{{
				Capability: "process-definitions", Message: "repeated pagination cursor prevented completion",
			}}, true, nil
		}
		if added == 0 {
			return processDefinitionsFromMap(byKey), []inventory.Warning{{
				Capability: "process-definitions", Message: "pagination made no item progress",
			}}, true, nil
		}
		seenCursors[page.EndCursor] = struct{}{}
		cursor = page.EndCursor
		if pageNumber == limits.MaxPages {
			return processDefinitionsFromMap(byKey), []inventory.Warning{{
				Capability: "process-definitions",
				Message:    fmt.Sprintf("page limit %d reached before inventory completed", limits.MaxPages),
			}}, true, nil
		}
	}
	return nil, nil, false, errors.New("process definition pagination terminated unexpectedly")
}

func (c *Client) searchProcessDefinitionPage(ctx context.Context, limits InventoryLimits, cursor string) (processSearchPage, error) {
	page := map[string]any{"limit": limits.PageSize}
	if cursor != "" {
		page["after"] = cursor
	}
	requestBody, err := json.Marshal(map[string]any{"page": page})
	if err != nil {
		return processSearchPage{}, err
	}
	data, err := c.doInventoryRequest(ctx, http.MethodPost, "/process-definitions/search", requestBody, limits.MaxBodyBytes)
	if err != nil {
		return processSearchPage{}, err
	}
	parsed, err := parseProcessSearchResponse(data)
	if err != nil {
		return processSearchPage{}, fmt.Errorf("decode process definitions search response: %w", err)
	}
	return parsed, nil
}

// APIError is an actionable, redacted cluster response failure.
type APIError struct {
	Method     string
	Path       string
	StatusCode int
	Err        error
}

func (e *APIError) Error() string {
	message := fmt.Sprintf("%s %s returned HTTP %d", e.Method, e.Path, e.StatusCode)
	if e.StatusCode == http.StatusNotFound || e.StatusCode == http.StatusMethodNotAllowed {
		message += "; verify the Camunda API version and endpoint capability"
	}
	if e.Err != nil {
		message += ": " + e.Err.Error()
	}
	return message
}

func (e *APIError) Unwrap() error { return e.Err }

func (c *Client) doInventoryRequest(ctx context.Context, method, path string, body []byte, maxBody int64) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create cluster inventory request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	c.applyAuth(request)
	response, err := c.client().Do(request)
	if err != nil {
		return nil, fmt.Errorf("request cluster inventory: %w", err)
	}
	defer response.Body.Close()
	data, err := io.ReadAll(io.LimitReader(response.Body, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("read cluster inventory response: %w", err)
	}
	if int64(len(data)) > maxBody {
		return nil, errors.New("cluster inventory response exceeds size limit")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, &APIError{
			Method: method, Path: path, StatusCode: response.StatusCode,
			Err: boundedAPIMessage(data),
		}
	}
	return data, nil
}

func boundedAPIMessage(data []byte) error {
	var payload struct {
		Message string `json:"message"`
		Detail  string `json:"detail"`
	}
	if len(data) > 0 && json.Unmarshal(data, &payload) == nil {
		message := strings.TrimSpace(payload.Message)
		if message == "" {
			message = strings.TrimSpace(payload.Detail)
		}
		if message != "" {
			return errors.New(redactAPIMessage(message))
		}
	}
	return nil
}

func redactAPIMessage(message string) string {
	return RedactAPIMessage(message)
}

// RedactAPIMessage removes credential-like tokens from actionable messages.
func RedactAPIMessage(message string) string {
	fields := strings.Fields(message)
	for i := range fields {
		lower := strings.ToLower(strings.Trim(fields[i], `"'`))
		if lower == "bearer" && i+1 < len(fields) {
			fields[i+1] = "[REDACTED]"
		}
		if strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") ||
			strings.Contains(lower, "password=") {
			if before, _, found := strings.Cut(fields[i], "="); found {
				fields[i] = before + "=[REDACTED]"
			}
		}
	}
	return strings.Join(fields, " ")
}

func parseProcessSearchResponse(data []byte) (processSearchPage, error) {
	rawItems, metadata, err := decodeSearchEnvelope(data)
	if err != nil {
		return processSearchPage{}, err
	}
	result := processSearchPage{
		EndCursor: metadata.EndCursor, TotalItems: metadata.TotalItems,
		HasMoreTotalItems: metadata.HasMoreTotalItems,
	}
	for _, raw := range rawItems {
		var wire struct {
			Name                 *string     `json:"name"`
			ResourceName         string      `json:"resourceName"`
			Version              int         `json:"version"`
			VersionTag           *string     `json:"versionTag"`
			ProcessDefinitionID  string      `json:"processDefinitionId"`
			TenantID             string      `json:"tenantId"`
			ProcessDefinitionKey identifier  `json:"processDefinitionKey"`
			HasStartForm         *bool       `json:"hasStartForm"`
			StartFormKey         *identifier `json:"startFormKey"`
		}
		if err := decodeStrictJSON(raw, &wire); err != nil {
			return processSearchPage{}, fmt.Errorf("decode process definition item: %w", err)
		}
		if wire.ProcessDefinitionKey == "" || strings.TrimSpace(wire.ProcessDefinitionID) == "" || wire.Version < 1 {
			return processSearchPage{}, errors.New("process definition requires key, ID, and positive integer version")
		}
		item := ProcessDefinition{
			Key: string(wire.ProcessDefinitionKey), ProcessDefinitionID: wire.ProcessDefinitionID,
			Name: wire.Name, Version: wire.Version, VersionTag: wire.VersionTag,
			ResourceName: wire.ResourceName, TenantID: wire.TenantID,
			HasStartForm: wire.HasStartForm,
		}
		if wire.StartFormKey != nil {
			value := string(*wire.StartFormKey)
			item.StartFormKey = &value
		}
		result.Items = append(result.Items, item)
	}
	return result, nil
}

func decodeIdentifier(decoder *json.Decoder) (string, error) {
	var raw json.RawMessage
	if err := decoder.Decode(&raw); err != nil {
		return "", err
	}
	var text string
	if len(raw) != 0 && raw[0] == '"' {
		if err := json.Unmarshal(raw, &text); err != nil {
			return "", err
		}
		if strings.TrimSpace(text) == "" {
			return "", errors.New("identifier is empty")
		}
		return text, nil
	}
	number := string(raw)
	if number == "" || strings.ContainsAny(number, ".eE+-") && strings.HasPrefix(number, "-") ||
		strings.ContainsAny(number, ".eE") {
		return "", errors.New("identifier must be an integer or decimal string")
	}
	if _, err := strconv.ParseUint(number, 10, 64); err != nil {
		return "", errors.New("identifier must be an unsigned integer or decimal string")
	}
	return number, nil
}

func parseProcessXMLResponse(data []byte) (string, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return "", errors.New("process definition XML response is empty")
	}
	if trimmed[0] == '<' {
		return string(trimmed), nil
	}
	decoder := json.NewDecoder(bytes.NewReader(trimmed))
	token, err := decoder.Token()
	if err != nil {
		return "", fmt.Errorf("decode process definition XML response: %w", err)
	}
	if text, ok := token.(string); ok {
		if strings.TrimSpace(text) == "" {
			return "", errors.New("process definition XML response is empty")
		}
		if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
			if err != nil {
				return "", fmt.Errorf("decode process definition XML response: %w", err)
			}
			return "", errors.New("process definition XML response contains a trailing value")
		}
		return text, nil
	}
	if token != json.Delim('{') {
		return "", errors.New("process definition XML response must be XML, a JSON string, or an object")
	}
	xmlBody := ""
	seen := false
	for decoder.More() {
		keyToken, err := decoder.Token()
		if err != nil {
			return "", fmt.Errorf("decode process definition XML response: %w", err)
		}
		key, ok := keyToken.(string)
		if !ok || key != "xml" || seen {
			return "", errors.New("process definition XML response contains an unknown or duplicate field")
		}
		seen = true
		if err := decoder.Decode(&xmlBody); err != nil {
			return "", fmt.Errorf("decode process definition XML field: %w", err)
		}
	}
	if closeToken, err := decoder.Token(); err != nil || closeToken != json.Delim('}') {
		return "", errors.New("process definition XML response object is not closed")
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		if err != nil {
			return "", fmt.Errorf("decode process definition XML response: %w", err)
		}
		return "", errors.New("process definition XML response contains a trailing value")
	}
	if !seen || strings.TrimSpace(xmlBody) == "" {
		return "", errors.New("process definition XML response is missing xml")
	}
	return xmlBody, nil
}

func processDefinitionsFromMap(values map[string]ProcessDefinition) []ProcessDefinition {
	out := make([]ProcessDefinition, 0, len(values))
	for _, value := range values {
		out = append(out, value)
	}
	sortProcessDefinitions(out)
	return out
}

func sortRemoteInventory(value *inventory.Inventory) {
	// Validation also depends on stable kind/ID/version/key ordering.
	sortResources(value.Resources)
}

func sortProcessDefinitions(values []ProcessDefinition) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].ProcessDefinitionID != values[j].ProcessDefinitionID {
			return values[i].ProcessDefinitionID < values[j].ProcessDefinitionID
		}
		if values[i].Version != values[j].Version {
			return values[i].Version < values[j].Version
		}
		return values[i].Key < values[j].Key
	})
}

func sortResources(values []inventory.Resource) {
	sort.SliceStable(values, func(i, j int) bool {
		if values[i].Kind != values[j].Kind {
			return values[i].Kind < values[j].Kind
		}
		if values[i].ID != values[j].ID {
			return values[i].ID < values[j].ID
		}
		if values[i].Version != values[j].Version {
			return values[i].Version < values[j].Version
		}
		return values[i].Key < values[j].Key
	})
}
