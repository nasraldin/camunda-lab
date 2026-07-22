package ai_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/nasraldin/camunda-lab/internal/ai"
)

func TestOpenAICompatibleClientSendsAuthModelAndPrompt(t *testing.T) {
	var authorization, model, prompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.Unmarshal(body, &request)
		model = request.Model
		prompt = request.Messages[0].Content
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"review"}}]}`)
	}))
	defer server.Close()

	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test-model", APIKey: "sk-secret",
		BaseURL: server.URL, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Complete(context.Background(), ai.ChatRequest{Prompt: "stable prompt"})
	if err != nil {
		t.Fatal(err)
	}
	if authorization != "Bearer sk-secret" || model != "test-model" || prompt != "stable prompt" || response.Content != "review" {
		t.Fatalf("authorization=%q model=%q prompt=%q response=%+v", authorization, model, prompt, response)
	}
}

func TestAnthropicClientSendsHeadersModelAndPrompt(t *testing.T) {
	var key, version, model, prompt string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key, version = r.Header.Get("x-api-key"), r.Header.Get("anthropic-version")
		var request struct {
			Model    string `json:"model"`
			Messages []struct {
				Content string `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		model, prompt = request.Model, request.Messages[0].Content
		_, _ = io.WriteString(w, `{"content":[{"type":"text","text":"analysis"}]}`)
	}))
	defer server.Close()

	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "anthropic", Model: "claude-test", APIKey: "ant-secret",
		BaseURL: server.URL, Timeout: time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := client.Complete(context.Background(), ai.ChatRequest{Prompt: "review semantics"})
	if err != nil {
		t.Fatal(err)
	}
	if key != "ant-secret" || version == "" || model != "claude-test" || prompt != "review semantics" || response.Content != "analysis" {
		t.Fatalf("key=%q version=%q model=%q prompt=%q response=%+v", key, version, model, prompt, response)
	}
}

func TestChatClientPreservesCancellationAndRedactsSecrets(t *testing.T) {
	const secret = "do-not-leak"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(200 * time.Millisecond):
		}
	}))
	defer server.Close()
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", APIKey: secret,
		BaseURL: server.URL, Timeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), ai.ChatRequest{Prompt: "prompt " + secret})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("timeout was not preserved: %v", err)
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("secret leaked in error: %v", err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Complete(cancelled, ai.ChatRequest{Prompt: "prompt"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation was not preserved: %v", err)
	}
}

func TestChatClientRejectsActionableInvalidConfiguration(t *testing.T) {
	for _, config := range []ai.ClientConfig{
		{Provider: "mystery", Model: "x", APIKey: "secret"},
		{Provider: "openai", Model: "", APIKey: "secret"},
		{Provider: "anthropic", Model: "x"},
		{Provider: "openai", Model: "x", APIKey: "secret", BaseURL: "://bad"},
		{Provider: "openai", Model: "x", APIKey: "secret", BaseURL: "https://user:pass@example.test/v1"},
		{Provider: "openai", Model: "x", APIKey: "secret", BaseURL: "https://example.test/v1#fragment"},
		{Provider: "openai", Model: "x", APIKey: "secret", BaseURL: "https://example.test/v1?tenant=secret"},
	} {
		_, err := ai.NewChatClient(config)
		var configErr *ai.ConfigError
		if !errors.As(err, &configErr) || configErr.Action == "" {
			t.Fatalf("config=%+v error=%#v", config, err)
		}
		if strings.Contains(err.Error(), config.APIKey) && config.APIKey != "" {
			t.Fatalf("secret leaked in config error: %v", err)
		}
	}
}

func TestChatClientJoinsBasePathAndAllowsLocalEndpoints(t *testing.T) {
	var path string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path = r.URL.Path
		_, _ = io.WriteString(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	}))
	defer server.Close()
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", BaseURL: server.URL + "/proxy/v1/",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), ai.ChatRequest{Prompt: `{"valid":true}`}); err != nil {
		t.Fatal(err)
	}
	if path != "/proxy/v1/chat/completions" {
		t.Fatalf("request path = %q", path)
	}
}

func TestProviderErrorNeverIncludesUntrustedResponseBody(t *testing.T) {
	const nestedSecret = "nested-provider-secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = io.WriteString(w, `{"error":{"details":{"apiKey":"`+nestedSecret+`"},"message":"host internals"}}`)
	}))
	defer server.Close()
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", APIKey: "secret",
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), ai.ChatRequest{Prompt: `{"valid":true}`})
	if err == nil || !strings.Contains(err.Error(), "HTTP 502") {
		t.Fatalf("error = %v", err)
	}
	var providerErr *ai.ProviderError
	if !errors.As(err, &providerErr) || providerErr.Code != "ai_http_502" ||
		!strings.Contains(strings.ToLower(providerErr.Message), "endpoint") {
		t.Fatalf("provider error is not actionable: %#v", err)
	}
	for _, forbidden := range []string{nestedSecret, "host internals", `"details"`, `"message"`} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("provider body leaked through error: %v", err)
		}
	}
}

func TestChatClientRejectsOversizedPromptWithoutCuttingContent(t *testing.T) {
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", APIKey: "secret",
		BaseURL: "http://127.0.0.1", MaxPromptBytes: 32,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), ai.ChatRequest{Prompt: `{"semantics":"` + strings.Repeat("é", 30) + `"}`})
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized prompt error = %v", err)
	}
}

func TestTransportFailureIsSanitizedAndPreservesOriginalCause(t *testing.T) {
	const secret = "transport-internal-secret"
	cause := &deterministicTransportError{secret: secret, cause: syscall.ECONNRESET}
	httpClient := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, cause
	})}
	client, err := ai.NewChatClient(ai.ClientConfig{
		Provider: "openai", Model: "test", APIKey: "not-rendered",
		BaseURL: "http://provider.invalid/v1", HTTPClient: httpClient,
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), ai.ChatRequest{Prompt: `{"safe":true}`})
	var providerErr *ai.ProviderError
	var urlErr *url.Error
	var netErr net.Error
	var original *deterministicTransportError
	if !errors.As(err, &providerErr) || providerErr.Code != "ai_transport_error" ||
		!errors.As(err, &urlErr) || !errors.As(err, &netErr) ||
		!errors.As(err, &original) ||
		!errors.Is(err, syscall.ECONNRESET) {
		t.Fatalf("transport chain was not preserved: %#v", err)
	}
	if original != cause {
		t.Fatalf("original transport error identity changed: %p != %p", original, cause)
	}
	for _, forbidden := range []string{secret, "provider.invalid", "not-rendered", `{"safe":true}`} {
		if strings.Contains(err.Error(), forbidden) {
			t.Fatalf("transport detail leaked in public error: %v", err)
		}
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

type deterministicTransportError struct {
	secret string
	cause  error
}

func (e *deterministicTransportError) Error() string { return e.secret }
func (e *deterministicTransportError) Unwrap() error { return e.cause }
func (e *deterministicTransportError) Timeout() bool { return false }
func (e *deterministicTransportError) Temporary() bool {
	return true
}
