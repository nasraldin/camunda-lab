package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
	"github.com/nasraldin/camunda-lab/internal/lint"
)

// ChatClient is the provider-neutral AI boundary used by application services.
// Provider adapters are added separately; callers can always omit the client.
type ChatClient interface {
	Complete(context.Context, ChatRequest) (ChatResponse, error)
}

// ChatRequest carries domain data without coupling callers to a provider.
type ChatRequest struct {
	Purpose  string
	Prompt   string
	Document bpmn.Document
	Findings []lint.Finding
}

// ChatResponse is provider-neutral generated content.
type ChatResponse struct {
	Content string
}

// ClientConfig selects an HTTP provider adapter outside domain orchestration.
type ClientConfig struct {
	Provider       string
	Model          string
	APIKey         string
	BaseURL        string
	Timeout        time.Duration
	MaxPromptBytes int
	HTTPClient     *http.Client
}

// ConfigError describes an actionable provider configuration failure.
type ConfigError struct {
	Field   string
	Detail  string
	Action  string
	Code    string
	Message string
}

func (e *ConfigError) Error() string {
	return e.SafeMessage()
}

// SafeCode is a stable public classification that contains no credentials.
func (e *ConfigError) SafeCode() string {
	if e.Code != "" {
		return e.Code
	}
	return "ai_configuration_invalid"
}

// SafeMessage is an actionable public detail constructed only from validated config metadata.
func (e *ConfigError) SafeMessage() string {
	if e.Message != "" {
		return e.Message
	}
	action := e.Action
	if action == "" {
		action = "verify provider, model, endpoint, and credentials"
	}
	return fmt.Sprintf("invalid AI configuration (%s); %s", e.Field, action)
}

// ProviderError exposes provider status/remediation separately from its wrapped cause.
// Message never includes a provider response body, prompt, or credential.
type ProviderError struct {
	StatusCode int
	Code       string
	Message    string
	Err        error
}

func (e *ProviderError) Error() string { return e.SafeMessage() }
func (e *ProviderError) Unwrap() error { return e.Err }

// SafeCode returns the stable public provider failure classification.
func (e *ProviderError) SafeCode() string {
	if e.Code != "" {
		return e.Code
	}
	if e.StatusCode > 0 {
		return fmt.Sprintf("ai_http_%d", e.StatusCode)
	}
	return "ai_provider_failed"
}

// SafeMessage returns actionable status guidance without untrusted response content.
func (e *ProviderError) SafeMessage() string {
	if e.Message != "" {
		return e.Message
	}
	switch e.StatusCode {
	case http.StatusUnauthorized:
		return "AI provider returned HTTP 401 (unauthorized); verify provider credentials and model access"
	case http.StatusTooManyRequests:
		return "AI provider returned HTTP 429 (rate limited); retry later or check provider quota"
	default:
		if e.StatusCode > 0 {
			return fmt.Sprintf(
				"AI provider returned HTTP %d; verify the provider endpoint, model, and service availability",
				e.StatusCode,
			)
		}
		return "AI provider request failed; verify the provider endpoint, model, credentials, and service availability"
	}
}

type httpChatClient struct {
	provider       string
	model          string
	apiKey         string
	endpoint       string
	timeout        time.Duration
	maxPromptBytes int
	httpClient     *http.Client
}

const (
	defaultTimeout        = 30 * time.Second
	defaultMaxPromptBytes = 128 * 1024
)

// NewChatClient validates configuration and constructs a provider-neutral client.
func NewChatClient(config ClientConfig) (ChatClient, error) {
	provider := strings.ToLower(strings.TrimSpace(config.Provider))
	if provider != "openai" && provider != "anthropic" {
		return nil, &ConfigError{
			Field: "provider", Detail: fmt.Sprintf("%q is not supported", provider),
			Action: `choose "openai" or "anthropic"`,
			Code:   "ai_configuration_invalid",
			Message: fmt.Sprintf(
				"AI provider %q is not supported; choose \"openai\" or \"anthropic\"",
				safeConfigIdentifier(provider),
			),
		}
	}
	if strings.TrimSpace(config.Model) == "" {
		return nil, &ConfigError{
			Field: "model", Detail: "model is empty", Action: "set an AI model name",
		}
	}
	customBase := strings.TrimSpace(config.BaseURL) != ""
	if strings.TrimSpace(config.APIKey) == "" && (provider == "anthropic" || !customBase) {
		envName := "SECRET_OPENAI_API_KEY"
		if provider == "anthropic" {
			envName = "SECRET_ANTHROPIC_API_KEY"
		}
		return nil, &ConfigError{
			Field: "api key", Detail: "credential is empty", Action: "set " + envName,
		}
	}
	baseURL := strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if baseURL == "" {
		if provider == "openai" {
			baseURL = "https://api.openai.com/v1"
		} else {
			baseURL = "https://api.anthropic.com"
		}
	}
	parsed, err := url.Parse(baseURL)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return nil, &ConfigError{
			Field: "base URL", Detail: "URL must be an absolute HTTP(S) URL",
			Action: "set a valid provider endpoint such as https://api.openai.com/v1",
		}
	}
	if parsed.User != nil {
		return nil, &ConfigError{
			Field: "base URL", Detail: "userinfo is not allowed",
			Action: "remove credentials from the URL and configure the API key separately",
		}
	}
	if parsed.Fragment != "" {
		return nil, &ConfigError{
			Field: "base URL", Detail: "fragments are not allowed",
			Action: "remove the URL fragment",
		}
	}
	if parsed.RawQuery != "" {
		return nil, &ConfigError{
			Field: "base URL", Detail: "query parameters are not supported",
			Action: "remove the query string and use a path-based provider endpoint",
		}
	}
	suffix := "v1/messages"
	if provider == "openai" {
		suffix = "chat/completions"
	}
	if !strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/"+suffix) {
		parsed.Path = path.Join(parsed.Path, suffix)
	}
	endpoint := parsed.String()
	timeout := config.Timeout
	if timeout <= 0 {
		timeout = defaultTimeout
	}
	maxPromptBytes := config.MaxPromptBytes
	if maxPromptBytes <= 0 {
		maxPromptBytes = defaultMaxPromptBytes
	}
	httpClient := config.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	return &httpChatClient{
		provider: provider, model: config.Model, apiKey: config.APIKey, endpoint: endpoint,
		timeout: timeout, maxPromptBytes: maxPromptBytes, httpClient: httpClient,
	}, nil
}

func safeConfigIdentifier(value string) string {
	if value == "" || len(value) > 64 {
		return "[redacted]"
	}
	for _, char := range value {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || strings.ContainsRune(".-_", char) {
			continue
		}
		return "[redacted]"
	}
	return value
}

// NewChatClientFromSecrets maps the existing secret configuration to a provider adapter.
func NewChatClientFromSecrets(provider, model string, secrets Secrets) (ChatClient, error) {
	config := ClientConfig{Provider: provider, Model: model}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai":
		config.APIKey, config.BaseURL = secrets.OpenAIKey, secrets.OpenAIBaseURL
	case "anthropic":
		config.APIKey = secrets.AnthropicKey
	}
	return NewChatClient(config)
}

func (c *httpChatClient) Complete(ctx context.Context, request ChatRequest) (ChatResponse, error) {
	if err := ctx.Err(); err != nil {
		return ChatResponse{}, err
	}
	prompt := request.Prompt
	if len([]byte(prompt)) > c.maxPromptBytes {
		return ChatResponse{}, fmt.Errorf(
			"AI prompt exceeds configured limit: %d bytes > %d bytes; compact the structured request before calling the provider",
			len([]byte(prompt)), c.maxPromptBytes,
		)
	}
	var payload any
	if c.provider == "openai" {
		payload = struct {
			Model    string        `json:"model"`
			Messages []chatMessage `json:"messages"`
		}{Model: c.model, Messages: []chatMessage{{Role: "user", Content: prompt}}}
	} else {
		payload = struct {
			Model     string        `json:"model"`
			MaxTokens int           `json:"max_tokens"`
			Messages  []chatMessage `json:"messages"`
		}{Model: c.model, MaxTokens: 2048, Messages: []chatMessage{{Role: "user", Content: prompt}}}
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return ChatResponse{}, fmt.Errorf("encode AI request: %w", err)
	}
	callCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(callCtx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("create AI request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if c.provider == "openai" {
		if c.apiKey != "" {
			httpRequest.Header.Set("Authorization", "Bearer "+c.apiKey)
		}
	} else {
		httpRequest.Header.Set("x-api-key", c.apiKey)
		httpRequest.Header.Set("anthropic-version", "2023-06-01")
	}
	response, err := c.httpClient.Do(httpRequest)
	if err != nil {
		if callCtx.Err() != nil {
			return ChatResponse{}, callCtx.Err()
		}
		return ChatResponse{}, &ProviderError{
			Code:    "ai_transport_error",
			Message: "AI provider transport failed; verify network connectivity, DNS, proxy, TLS, and endpoint availability",
			Err:     err,
		}
	}
	defer response.Body.Close()
	responseBody, err := io.ReadAll(io.LimitReader(response.Body, 2*1024*1024))
	if err != nil {
		return ChatResponse{}, fmt.Errorf("read AI provider response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		providerErr := &ProviderError{StatusCode: response.StatusCode}
		providerErr.Code = providerErr.SafeCode()
		providerErr.Message = providerErr.SafeMessage()
		return ChatResponse{}, providerErr
	}
	content, err := decodeResponse(c.provider, responseBody)
	if err != nil {
		return ChatResponse{}, err
	}
	if strings.TrimSpace(content) == "" {
		return ChatResponse{}, errors.New("AI provider returned an empty completion")
	}
	return ChatResponse{Content: content}, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

func decodeResponse(provider string, body []byte) (string, error) {
	if provider == "openai" {
		var response struct {
			Choices []struct {
				Message chatMessage `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return "", fmt.Errorf("decode OpenAI-compatible response: %w", err)
		}
		if len(response.Choices) == 0 {
			return "", errors.New("OpenAI-compatible response contained no choices")
		}
		return response.Choices[0].Message.Content, nil
	}
	var response struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("decode Anthropic response: %w", err)
	}
	var content strings.Builder
	for _, block := range response.Content {
		if block.Type == "text" {
			content.WriteString(block.Text)
		}
	}
	return content.String(), nil
}
