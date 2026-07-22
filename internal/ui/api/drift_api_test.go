package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cluster"
	"github.com/nasraldin/camunda-lab/internal/env"
)

func TestDriftAPIReturnsTypedPreEnvironmentRefusal(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte(
		"name: api-drift\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "bpmn"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "bpmn", "invalid.bpmn"), []byte("<secret-token>"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/drift",
		strings.NewReader(`{"dir":`+mustProjectJSON(t, root)+`,"gitRef":"HEAD"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&handler{}).runDrift(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	var body struct {
		OK     bool `json:"ok"`
		Status string
		Policy struct {
			Outcome  string `json:"outcome"`
			ExitCode int    `json:"exitCode"`
		}
		Error *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
		Drift struct {
			Status string `json:"status"`
			Policy struct {
				ExitCode int `json:"exitCode"`
			} `json:"policy"`
		} `json:"drift"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.OK || body.Status != "refused" || body.Policy.Outcome != "refused" ||
		body.Policy.ExitCode != 2 || body.Drift.Status != "refused" ||
		body.Drift.Policy.ExitCode != 2 || body.Error == nil ||
		body.Error.Code != "drift_refused" || body.Error.Message == "" {
		t.Fatalf("inconsistent typed refusal: %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "secret-token") {
		t.Fatalf("operational cause leaked: %s", response.Body.String())
	}
}

func TestDriftAPIRedactsWrappedOperationalCause(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".camunda.yaml"), []byte(
		"name: api-drift\ncamundaVersion: \"8.9\"\npaths:\n  bpmn: bpmn\n  dmn: dmn\n  forms: forms\n  tests: tests\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/drift",
		strings.NewReader(`{"dir":`+mustProjectJSON(t, root)+`,"gitRef":"HEAD"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	(&handler{clusterFactory: secretDriftFactory{}}).runDrift(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", response.Code, response.Body.String())
	}
	if strings.Contains(response.Body.String(), "supersecretvalue") {
		t.Fatalf("operational cause leaked: %s", response.Body.String())
	}
	var body struct {
		Status string `json:"status"`
		Error  *struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Status != "unknown" || body.Error == nil || body.Error.Code != "drift_unknown" {
		t.Fatalf("typed unknown envelope missing: %s", response.Body.String())
	}
}

type secretDriftFactory struct{}

func (secretDriftFactory) Client(context.Context, string, string) (*cluster.Client, env.Resolved, error) {
	return nil, env.Resolved{}, errors.New("supersecretvalue")
}
