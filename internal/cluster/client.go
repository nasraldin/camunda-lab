package cluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
)

// Client talks to Camunda Orchestration Cluster REST API (/v2).
type Client struct {
	BaseURL    string // e.g. http://localhost:8080/v2
	HTTPClient *http.Client
	Kind       ClientKind
	Token      string // optional Bearer
	BasicUser  string
	BasicPass  string
}

// ProcessDefinition is a deployed process summary.
type ProcessDefinition struct {
	Key                 string
	ProcessDefinitionID string
	Name                *string
	Version             int
	VersionTag          *string
	ResourceName        string
	TenantID            string
	HasStartForm        *bool
	StartFormKey        *string
}

// Incident is a cluster incident.
type Incident struct {
	Key                    string
	ProcessInstanceKey     string
	ElementID              string
	ErrorType              string
	ErrorMessage           string
	State                  string
	CreationTime           string
	JobKey                 string
	ProcessDefinitionID    string
	ProcessDefinitionKey   string
	RootProcessInstanceKey string
	ElementInstanceKey     string
	TenantID               string
}

// ElementInstance is a flow-node runtime record.
type ElementInstance struct {
	Key                    string
	ProcessInstanceKey     string
	ElementID              string
	ElementName            string
	Type                   string
	State                  string
	StartDate              string
	EndDate                string
	IncidentKey            string
	ProcessDefinitionID    string
	ProcessDefinitionKey   string
	RootProcessInstanceKey string
	HasIncident            bool
	TenantID               string
}

// ProcessInstance summary.
type ProcessInstance struct {
	Key                         string
	ProcessDefinitionID         string
	ProcessDefinitionName       *string
	ProcessDefinitionVersion    int
	ProcessDefinitionVersionTag *string
	ProcessDefinitionKey        string
	StartDate                   string
	EndDate                     string
	State                       string
	HasIncident                 bool
	TenantID                    string
	ParentProcessInstanceKey    string
	ParentElementInstanceKey    string
	RootProcessInstanceKey      string
	Tags                        []string
	BusinessID                  string
}

type searchPage struct {
	Limit int `json:"limit"`
}

type searchBody struct {
	Filter map[string]any `json:"filter,omitempty"`
	Page   searchPage     `json:"page"`
}

// NormalizeBaseURL validates an orchestration endpoint and returns its
// canonical REST API base ending in /v2. Path prefixes (for gateways/reverse
// proxies) are preserved.
func NormalizeBaseURL(raw string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", fmt.Errorf("parse orchestration endpoint: %w", err)
	}
	if (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return "", fmt.Errorf("orchestration endpoint must be an absolute HTTP(S) URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", fmt.Errorf("orchestration endpoint must not contain userinfo, query, or fragment")
	}
	basePath := pathpkg.Clean("/" + strings.Trim(parsed.Path, "/"))
	if basePath == "/" {
		basePath = ""
	}
	if basePath != "/v2" && !strings.HasSuffix(basePath, "/v2") {
		basePath += "/v2"
	}
	parsed.Path = basePath
	parsed.RawPath = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

// ResolveBaseURL is a compatibility adapter for global/implicit selection.
// New production code must use Factory.Client with its project root.
func ResolveBaseURL(labHome string, cfg config.Config) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, _, err := NewFactory(labHome, cfg).Client(ctx, "", "")
	if err != nil {
		return "", err
	}
	return client.BaseURL, nil
}

// NewFromLab is a compatibility adapter for global/implicit selection. New
// production code must use Factory.Client to supply project context.
func NewFromLab(labHome string, cfg config.Config) (*Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client, _, err := NewFactory(labHome, cfg).Client(ctx, "", "")
	return client, err
}

func (c *Client) client() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return http.DefaultClient
}

func (c *Client) doJSON(ctx context.Context, method, path string, body any, out any) error {
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(c.BaseURL, "/")+path, rdr)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	c.applyAuth(req)
	resp, err := c.client().Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		msg := fmt.Sprintf("%s %s: HTTP %d: %s", method, path, resp.StatusCode, truncate(string(data), 200))
		if resp.StatusCode == http.StatusUnauthorized {
			msg += " (full labs need OIDC — camunda fetches connectors client token from the lab .env; override with CAMUNDA_ACCESS_TOKEN)"
		}
		return fmt.Errorf("%s", msg)
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// SearchProcessDefinitions lists deployed processes.
func (c *Client) SearchProcessDefinitions(ctx context.Context, limit int) ([]ProcessDefinition, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > maxInventoryPageSize {
		limit = maxInventoryPageSize
	}
	page, err := c.searchProcessDefinitionPage(ctx, InventoryLimits{
		PageSize: limit, MaxPages: 1, MaxItems: limit, MaxBodyBytes: defaultInventoryBodySize,
	}, "")
	if err != nil {
		return nil, err
	}
	if len(page.Items) > limit {
		page.Items = page.Items[:limit]
	}
	return page.Items, nil
}

// GetProcessDefinitionXML fetches deployed BPMN XML.
func (c *Client) GetProcessDefinitionXML(ctx context.Context, processDefinitionKey string) (string, error) {
	path := "/process-definitions/" + url.PathEscape(processDefinitionKey) + "/xml"
	data, err := c.doInventoryRequest(ctx, http.MethodGet, path, nil, defaultInventoryBodySize)
	if err != nil {
		return "", err
	}
	return parseProcessXMLResponse(data)
}

// SearchIncidents lists open incidents.
func (c *Client) SearchIncidents(ctx context.Context, limit int) ([]Incident, error) {
	if limit <= 0 {
		limit = 50
	}
	if limit > maxInventoryPageSize {
		limit = maxInventoryPageSize
	}
	result, err := c.SearchIncidentsInventory(ctx, map[string]any{"state": "ACTIVE"}, InventoryLimits{
		PageSize: limit, MaxPages: 1, MaxItems: limit, MaxBodyBytes: defaultInventoryBodySize,
	})
	return result.Items, err
}

// ResolveIncident marks an incident resolved.
func (c *Client) ResolveIncident(ctx context.Context, incidentKey string) error {
	body, err := json.Marshal(map[string]any{})
	if err != nil {
		return err
	}
	_, err = c.doInventoryRequest(
		ctx, http.MethodPost, "/incidents/"+url.PathEscape(incidentKey)+"/resolution",
		body, defaultInventoryBodySize,
	)
	return err
}

// GetProcessInstance fetches instance state.
func (c *Client) GetProcessInstance(ctx context.Context, key string) (ProcessInstance, error) {
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ProcessInstance{}, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		data, err := c.doInventoryRequest(
			ctx, http.MethodGet, "/process-instances/"+url.PathEscape(key), nil, defaultInventoryBodySize,
		)
		if err == nil {
			item, _, decodeErr := decodeProcessInstance(data)
			if decodeErr != nil {
				return ProcessInstance{}, fmt.Errorf("decode process instance response: %w", decodeErr)
			}
			return item, nil
		}
		last = err
		// Secondary storage can lag briefly after start-instance.
		var apiError *APIError
		if !errors.As(last, &apiError) || apiError.StatusCode != http.StatusNotFound {
			return ProcessInstance{}, last
		}
	}
	return ProcessInstance{}, last
}

// SearchElementInstances returns activity timeline for an instance.
func (c *Client) SearchElementInstances(ctx context.Context, processInstanceKey string, limit int) ([]ElementInstance, error) {
	if limit <= 0 {
		limit = 200
	}
	if limit > maxInventoryPageSize {
		limit = maxInventoryPageSize
	}
	result, err := searchTypedPage(ctx, c, "/element-instances/search",
		map[string]any{"processInstanceKey": processInstanceKey},
		InventoryLimits{
			PageSize: limit, MaxPages: 1, MaxItems: limit, MaxBodyBytes: defaultInventoryBodySize,
		}, "element-instances", decodeElementInstance)
	if err != nil {
		return nil, err
	}
	return result, nil
}
