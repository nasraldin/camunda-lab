package cluster

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/plan"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

// Client talks to Camunda Orchestration Cluster REST API (/v2).
type Client struct {
	BaseURL    string // e.g. http://localhost:8080/v2
	HTTPClient *http.Client
	Token      string // optional Bearer
	BasicUser  string
	BasicPass  string
}

// ProcessDefinition is a deployed process summary.
type ProcessDefinition struct {
	Key                 string
	ProcessDefinitionID string
	Name                string
	Version             int
	ResourceName        string
	TenantID            string
}

// Incident is a cluster incident.
type Incident struct {
	Key                 string
	ProcessInstanceKey  string
	ElementID           string
	ErrorType           string
	ErrorMessage        string
	State               string
	CreationTime        string
	JobKey              string
	ProcessDefinitionID string
}

// ElementInstance is a flow-node runtime record.
type ElementInstance struct {
	Key                string
	ProcessInstanceKey string
	ElementID          string
	ElementName        string
	Type               string
	State              string
	StartDate          string
	EndDate            string
	IncidentKey        string
}

// ProcessInstance summary.
type ProcessInstance struct {
	Key                 string
	ProcessDefinitionID string
	State               string
	HasIncident         bool
}

type searchPage struct {
	Limit int `json:"limit"`
}

type searchBody struct {
	Filter map[string]any `json:"filter,omitempty"`
	Page   searchPage     `json:"page"`
}

// ResolveBaseURL picks Orchestration /v2 URL for the active env (lab or remote).
func ResolveBaseURL(labHome string, cfg config.Config) (string, error) {
	active := env.GetActive(labHome)
	if active != "" && active != "lab" {
		ps, err := env.ListProfiles(filepath.Join(labHome, "envs"))
		if err != nil {
			return "", err
		}
		for _, p := range ps {
			if p.Name != active {
				continue
			}
			u := strings.TrimRight(p.Endpoints["orchestration"], "/")
			if u == "" {
				return "", fmt.Errorf("profile %q missing endpoints.orchestration", active)
			}
			if !strings.HasSuffix(u, "/v2") {
				u += "/v2"
			}
			return u, nil
		}
		return "", fmt.Errorf("active env %q not found (camunda env use lab)", active)
	}
	for _, e := range urls.List(cfg) {
		if e.Name == "rest" && strings.HasPrefix(e.URL, "http") {
			return strings.TrimRight(e.URL, "/"), nil
		}
	}
	port := 8080
	if cfg.Version == "8.8" {
		port = 8088
	}
	if cfg.Version == "8.7" || cfg.Version == "8.6" || cfg.Version == "8.5" {
		return fmt.Sprintf("http://%s:8088", defaultHost(cfg)), nil
	}
	return fmt.Sprintf("http://%s:%d/v2", defaultHost(cfg), port), nil
}

func defaultHost(cfg config.Config) string {
	if cfg.Host != "" {
		return cfg.Host
	}
	return "localhost"
}

// NewFromLab builds a client for the active environment (with lab OIDC when needed).
func NewFromLab(labHome string, cfg config.Config) (*Client, error) {
	base, err := ResolveBaseURL(labHome, cfg)
	if err != nil {
		return nil, err
	}
	c := &Client{
		BaseURL: base,
		HTTPClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := AttachLabAuth(ctx, c, labHome, cfg); err != nil {
		return nil, err
	}
	return c, nil
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
	var raw struct {
		Items []struct {
			ProcessDefinitionKey string `json:"processDefinitionKey"`
			ProcessDefinitionID  string `json:"processDefinitionId"`
			Name                 string `json:"name"`
			Version              int    `json:"version"`
			ResourceName         string `json:"resourceName"`
			TenantID             string `json:"tenantId"`
		} `json:"items"`
	}
	if err := c.doJSON(ctx, http.MethodPost, "/process-definitions/search", searchBody{Page: searchPage{Limit: limit}}, &raw); err != nil {
		return nil, err
	}
	out := make([]ProcessDefinition, 0, len(raw.Items))
	for _, it := range raw.Items {
		out = append(out, ProcessDefinition{
			Key:                 it.ProcessDefinitionKey,
			ProcessDefinitionID: it.ProcessDefinitionID,
			Name:                it.Name,
			Version:             it.Version,
			ResourceName:        it.ResourceName,
			TenantID:            it.TenantID,
		})
	}
	return out, nil
}

// GetProcessDefinitionXML fetches deployed BPMN XML.
func (c *Client) GetProcessDefinitionXML(ctx context.Context, processDefinitionKey string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(c.BaseURL, "/")+"/process-definitions/"+processDefinitionKey+"/xml", nil)
	if err != nil {
		return "", err
	}
	c.applyAuth(req)
	resp, err := c.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode >= 300 {
		return "", fmt.Errorf("get xml: HTTP %d: %s", resp.StatusCode, truncate(string(data), 200))
	}
	// API may return raw XML or JSON wrapper {"xml":"..."}
	var wrap struct {
		XML string `json:"xml"`
	}
	if json.Unmarshal(data, &wrap) == nil && wrap.XML != "" {
		return wrap.XML, nil
	}
	return string(data), nil
}

// RemoteInventory builds plan.Resource list from cluster (digest = hash of deployed XML).
func (c *Client) RemoteInventory(ctx context.Context) ([]plan.Resource, error) {
	defs, err := c.SearchProcessDefinitions(ctx, 200)
	if err != nil {
		return nil, err
	}
	latest := map[string]ProcessDefinition{}
	for _, d := range defs {
		prev, ok := latest[d.ProcessDefinitionID]
		if !ok || d.Version > prev.Version {
			latest[d.ProcessDefinitionID] = d
		}
	}
	var out []plan.Resource
	for id, d := range latest {
		xmlBody, err := c.GetProcessDefinitionXML(ctx, d.Key)
		digest := ""
		if err == nil {
			sum := sha256.Sum256([]byte(xmlBody))
			digest = hex.EncodeToString(sum[:8])
		}
		key := d.ResourceName
		if key == "" {
			key = id + ".bpmn"
		}
		out = append(out, plan.Resource{
			Key:     key,
			Digest:  digest,
			Version: strconv.Itoa(d.Version),
			Path:    d.Key,
		})
	}
	return out, nil
}

// SearchIncidents lists open incidents.
func (c *Client) SearchIncidents(ctx context.Context, limit int) ([]Incident, error) {
	if limit <= 0 {
		limit = 50
	}
	var raw struct {
		Items []struct {
			IncidentKey         string `json:"incidentKey"`
			ProcessInstanceKey  string `json:"processInstanceKey"`
			ElementID           string `json:"elementId"`
			ErrorType           string `json:"errorType"`
			ErrorMessage        string `json:"errorMessage"`
			State               string `json:"state"`
			CreationTime        string `json:"creationTime"`
			JobKey              string `json:"jobKey"`
			ProcessDefinitionID string `json:"processDefinitionId"`
		} `json:"items"`
	}
	body := searchBody{
		Filter: map[string]any{"state": "ACTIVE"},
		Page:   searchPage{Limit: limit},
	}
	if err := c.doJSON(ctx, http.MethodPost, "/incidents/search", body, &raw); err != nil {
		// Retry without filter for older clusters
		if err2 := c.doJSON(ctx, http.MethodPost, "/incidents/search", searchBody{Page: searchPage{Limit: limit}}, &raw); err2 != nil {
			return nil, err
		}
	}
	out := make([]Incident, 0, len(raw.Items))
	for _, it := range raw.Items {
		out = append(out, Incident{
			Key:                 it.IncidentKey,
			ProcessInstanceKey:  it.ProcessInstanceKey,
			ElementID:           it.ElementID,
			ErrorType:           it.ErrorType,
			ErrorMessage:        it.ErrorMessage,
			State:               it.State,
			CreationTime:        it.CreationTime,
			JobKey:              it.JobKey,
			ProcessDefinitionID: it.ProcessDefinitionID,
		})
	}
	return out, nil
}

// ResolveIncident marks an incident resolved.
func (c *Client) ResolveIncident(ctx context.Context, incidentKey string) error {
	return c.doJSON(ctx, http.MethodPost, "/incidents/"+incidentKey+"/resolution", map[string]any{}, nil)
}

// GetProcessInstance fetches instance state.
func (c *Client) GetProcessInstance(ctx context.Context, key string) (ProcessInstance, error) {
	var raw struct {
		ProcessInstanceKey  string `json:"processInstanceKey"`
		ProcessDefinitionID string `json:"processDefinitionId"`
		State               string `json:"state"`
		HasIncident         bool   `json:"hasIncident"`
	}
	var last error
	for attempt := 0; attempt < 5; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return ProcessInstance{}, ctx.Err()
			case <-time.After(300 * time.Millisecond):
			}
		}
		last = c.doJSON(ctx, http.MethodGet, "/process-instances/"+key, nil, &raw)
		if last == nil {
			return ProcessInstance{
				Key:                 raw.ProcessInstanceKey,
				ProcessDefinitionID: raw.ProcessDefinitionID,
				State:               raw.State,
				HasIncident:         raw.HasIncident,
			}, nil
		}
		// Secondary storage can lag briefly after start-instance.
		if !strings.Contains(last.Error(), "HTTP 404") {
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
	var raw struct {
		Items []struct {
			ElementInstanceKey string `json:"elementInstanceKey"`
			ProcessInstanceKey string `json:"processInstanceKey"`
			ElementID          string `json:"elementId"`
			ElementName        string `json:"elementName"`
			Type               string `json:"type"`
			State              string `json:"state"`
			StartDate          string `json:"startDate"`
			EndDate            string `json:"endDate"`
			IncidentKey        string `json:"incidentKey"`
		} `json:"items"`
	}
	body := searchBody{
		Filter: map[string]any{"processInstanceKey": processInstanceKey},
		Page:   searchPage{Limit: limit},
	}
	if err := c.doJSON(ctx, http.MethodPost, "/element-instances/search", body, &raw); err != nil {
		return nil, err
	}
	out := make([]ElementInstance, 0, len(raw.Items))
	for _, it := range raw.Items {
		out = append(out, ElementInstance{
			Key:                it.ElementInstanceKey,
			ProcessInstanceKey: it.ProcessInstanceKey,
			ElementID:          it.ElementID,
			ElementName:        it.ElementName,
			Type:               it.Type,
			State:              it.State,
			StartDate:          it.StartDate,
			EndDate:            it.EndDate,
			IncidentKey:        it.IncidentKey,
		})
	}
	return out, nil
}
