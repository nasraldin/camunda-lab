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

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/doctor"
	"github.com/nasraldin/camunda-lab/internal/paths"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
)

func TestDeveloperAPICompletedContractMatrix(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Run("lint", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/lint", `{"path":`+mustJSON(t, path)+`}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertAPIJSONFields(t, response.Body.Bytes(), "ok", "status", "complete", "warnings", "findings", "inputs", "cli")
		var result lintResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || result.Findings == nil || result.Inputs == nil || result.CLI == "" {
			t.Fatalf("incomplete lint response: %+v", result)
		}
	})

	t.Run("diff", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/diff",
			`{"paths":[`+mustJSON(t, path)+`,`+mustJSON(t, path)+`]}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertAPIJSONFields(t, response.Body.Bytes(), "ok", "status", "complete", "warnings", "changes", "cli")
		var result diffResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || result.Changes == nil || result.CLI == "" {
			t.Fatalf("incomplete diff response: %+v", result)
		}
	})

	t.Run("explain", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/explain",
			`{"path":`+mustJSON(t, path)+`,"format":"json"}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertAPIJSONFields(t, response.Body.Bytes(), "ok", "status", "complete", "warnings", "processes", "output", "cli")
		var result explainResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || len(result.Processes) != 1 ||
			result.Processes[0].ProcessID == "" || result.Processes[0].Markdown == "" ||
			result.Output == "" || result.CLI == "" {
			t.Fatalf("incomplete explain response: %+v", result)
		}
	})

	t.Run("review", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/review", `{"path":`+mustJSON(t, path)+`}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertAPIJSONFields(
			t,
			response.Body.Bytes(),
			"ok",
			"status",
			"complete",
			"warnings",
			"findings",
			"aiStatus",
			"reviews",
			"cli",
		)
		var result reviewResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || result.Findings == nil || result.Reviews == nil ||
			result.AIStatus != toolkit.AIStatusDisabled || result.CLI == "" {
			t.Fatalf("incomplete review response: %+v", result)
		}
	})

	t.Run("generate", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/test-generate",
			`{"path":`+mustJSON(t, path)+`,"lang":"python"}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertAPIJSONFields(
			t,
			response.Body.Bytes(),
			"ok",
			"status",
			"complete",
			"warnings",
			"mode",
			"artifacts",
			"paths",
			"contents",
			"cli",
		)
		var result generateResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != toolkit.StatusCompleted || !result.Complete ||
			result.Warnings == nil || result.Mode != "download" || len(result.Artifacts) == 0 ||
			len(result.Paths) == 0 || len(result.Contents) == 0 || result.CLI == "" ||
			result.Artifacts[0].Path == "" || result.Artifacts[0].MediaType == "" ||
			result.Artifacts[0].Content == "" {
			t.Fatalf("incomplete generate response: %+v", result)
		}
	})

	t.Run("scan", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/scan", `{"dir":`+mustJSON(t, root)+`}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertScanResponseContract(t, response, toolkit.StatusCompleted, true)
	})

	t.Run("doctor", func(t *testing.T) {
		deps := deterministicAPIDoctorDependencies(doctor.StatusPass)
		response := httptest.NewRecorder()
		(&handler{doctor: &deps}).runDoctorDeep(
			response,
			httptest.NewRequest(http.MethodPost, "/api/v1/doctor/deep", nil),
		)
		assertHTTPStatus(t, response, http.StatusOK)
		assertAPIJSONFields(t, response.Body.Bytes(), "ok", "status", "checks", "report", "cli")
		var result doctorDeepResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != "completed" || len(result.Checks) != 1 ||
			result.Report == "" || result.CLI == "" {
			t.Fatalf("incomplete doctor response: %+v", result)
		}
		assertAPIMarshaledFields(
			t,
			result.Checks[0],
			"id",
			"category",
			"status",
			"summary",
			"detail",
			"remediation",
			"durationNs",
			"required",
		)
	})
}

func TestDeveloperAPIFailedAndPartialContractMatrix(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join(root, "valid.bpmn")
	changed := filepath.Join(root, "changed.bpmn")
	policy := filepath.Join(root, "policy.bpmn")
	secretRoot := filepath.Join(root, "secret")
	partialRoot := filepath.Join(root, "partial")
	for _, dir := range []string{secretRoot, partialRoot} {
		if err := os.Mkdir(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	for path, content := range map[string]string{
		valid:                                   validDeveloperBPMN,
		changed:                                 strings.Replace(validDeveloperBPMN, `id="end"`, `id="end" name="changed"`, 1),
		policy:                                  `<definitions xmlns="http://www.omg.org/spec/BPMN/20100524/MODEL"><process id="p"><startEvent id="start"/><serviceTask id="task"/></process></definitions>`,
		filepath.Join(secretRoot, "secret.env"): "CLIENT_SECRET=deterministic-client-secret-value",
	} {
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(filepath.Join(partialRoot, "missing"), filepath.Join(partialRoot, "broken.env")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}

	failed := []struct {
		name     string
		endpoint string
		body     string
		field    string
	}{
		{name: "lint", endpoint: "/api/v1/bpmn/lint", body: `{"path":` + mustJSON(t, policy) + `,"failOn":"warning"}`, field: "findings"},
		{name: "diff", endpoint: "/api/v1/bpmn/diff", body: `{"paths":[` + mustJSON(t, valid) + `,` + mustJSON(t, changed) + `]}`, field: "changes"},
		{name: "review", endpoint: "/api/v1/bpmn/review", body: `{"path":` + mustJSON(t, policy) + `,"failOn":"warning"}`, field: "findings"},
		{name: "scan", endpoint: "/api/v1/bpmn/scan", body: `{"dir":` + mustJSON(t, secretRoot) + `,"failOn":"low"}`, field: "findings"},
	}
	for _, test := range failed {
		t.Run(test.name+" failed", func(t *testing.T) {
			response := runDeveloperJSON(t, test.endpoint, test.body)
			assertHTTPStatus(t, response, http.StatusOK)
			assertAPIJSONFields(t, response.Body.Bytes(), "ok", "status", "complete", test.field)
			var envelope struct {
				OK       bool           `json:"ok"`
				Status   toolkit.Status `json:"status"`
				Complete bool           `json:"complete"`
			}
			decodeAPIJSON(t, response, &envelope)
			if envelope.OK || envelope.Status != toolkit.StatusFailed || !envelope.Complete {
				t.Fatalf("failed envelope = %+v: %s", envelope, response.Body.String())
			}
		})
	}

	t.Run("review partial", func(t *testing.T) {
		withEmptyAIHome(t)
		response := runDeveloperJSON(t, "/api/v1/bpmn/review",
			`{"path":`+mustJSON(t, valid)+`,"ai":true,"provider":"openai","model":"model"}`)
		assertHTTPStatus(t, response, http.StatusOK)
		var result reviewResponse
		decodeAPIJSON(t, response, &result)
		if !result.OK || result.Status != toolkit.StatusPartial || result.Complete ||
			result.Warnings == nil || len(result.Warnings) == 0 ||
			result.Findings == nil || result.Reviews == nil ||
			result.AIStatus != toolkit.AIStatusSkipped {
			t.Fatalf("incomplete partial review response: %+v", result)
		}
	})

	t.Run("scan partial", func(t *testing.T) {
		response := runDeveloperJSON(t, "/api/v1/bpmn/scan",
			`{"dir":`+mustJSON(t, partialRoot)+`}`)
		assertHTTPStatus(t, response, http.StatusOK)
		assertScanResponseContract(t, response, toolkit.StatusPartial, false)
		var result scanResponse
		decodeAPIJSON(t, response, &result)
		if result.OK || result.Complete || len(result.Issues) == 0 ||
			result.Stats.Discovered != 1 || result.Stats.Errored != 1 {
			t.Fatalf("partial scan lost issue/stats: %+v", result)
		}
		assertAPIMarshaledFields(t, result.Issues[0], "path", "kind", "message")
		assertAPIMarshaledFields(t, result.Stats, "discovered", "scanned", "ignored", "errored")
	})

	t.Run("doctor failed", func(t *testing.T) {
		deps := deterministicAPIDoctorDependencies(doctor.StatusFail)
		response := httptest.NewRecorder()
		(&handler{doctor: &deps}).runDoctorDeep(
			response,
			httptest.NewRequest(http.MethodPost, "/api/v1/doctor/deep", nil),
		)
		assertHTTPStatus(t, response, http.StatusOK)
		var result doctorDeepResponse
		decodeAPIJSON(t, response, &result)
		if result.OK || result.Status != "failed" || len(result.Checks) != 1 ||
			result.Checks[0].Status != doctor.StatusFail || result.Report == "" {
			t.Fatalf("failed doctor response: %+v", result)
		}
	})
}

func TestDeveloperAPIToolErrorContractMatrix(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "process.bpmn")
	if err := os.WriteFile(path, []byte(validDeveloperBPMN), 0o600); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		endpoint string
		body     string
		code     string
		status   int
	}{
		{name: "lint", endpoint: "/api/v1/bpmn/lint", body: `{"path":` + mustJSON(t, path) + `,"failOn":"invalid"}`, code: "invalid_request", status: http.StatusUnprocessableEntity},
		{name: "diff", endpoint: "/api/v1/bpmn/diff", body: `{"from":` + mustJSON(t, path) + `}`, code: "invalid_request", status: http.StatusBadRequest},
		{name: "explain", endpoint: "/api/v1/bpmn/explain", body: `{"path":` + mustJSON(t, path) + `,"format":"yaml"}`, code: "invalid_request", status: http.StatusBadRequest},
		{name: "review", endpoint: "/api/v1/bpmn/review", body: `{"path":` + mustJSON(t, path) + `,"ai":true,"provider":"unknown","model":"model"}`, code: "ai_configuration_invalid", status: http.StatusBadRequest},
		{name: "generate", endpoint: "/api/v1/bpmn/test-generate", body: `{"path":` + mustJSON(t, path) + `,"lang":"ruby"}`, code: "invalid_request", status: http.StatusUnprocessableEntity},
		{name: "scan", endpoint: "/api/v1/bpmn/scan", body: `{"dir":` + mustJSON(t, root) + `,"failOn":"invalid"}`, code: "invalid_request", status: http.StatusUnprocessableEntity},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := runDeveloperJSON(t, test.endpoint, test.body)
			assertHTTPStatus(t, response, test.status)
			assertAPIJSONFields(t, response.Body.Bytes(), "ok", "code", "error")
			var result developerErrorResponse
			decodeAPIJSON(t, response, &result)
			if result.OK || result.Code != test.code || result.Error == "" {
				t.Fatalf("error response = %+v", result)
			}
		})
	}

	t.Run("doctor", func(t *testing.T) {
		deps := deterministicAPIDoctorDependencies(doctor.StatusPass)
		deps.runDeep = func(context.Context, config.Config, doctor.DeepOptions) (doctor.DeepReport, error) {
			return doctor.DeepReport{}, &doctor.FatalError{
				Code: "invalid_environment", Message: "internal deterministic detail",
				Err: errors.New("deterministic failure"),
			}
		}
		response := httptest.NewRecorder()
		(&handler{doctor: &deps}).runDoctorDeep(
			response,
			httptest.NewRequest(http.MethodPost, "/api/v1/doctor/deep", nil),
		)
		assertHTTPStatus(t, response, http.StatusBadRequest)
		var result developerErrorResponse
		decodeAPIJSON(t, response, &result)
		if result.Code != "invalid_environment" ||
			result.Error != "The active environment configuration is invalid." {
			t.Fatalf("doctor error leaked or changed: %+v", result)
		}
	})
}

func deterministicAPIDoctorDependencies(status doctor.Status) developerDoctorDependencies {
	return developerDoctorDependencies{
		loadConfig: func() (config.Config, error) { return config.Defaults(), nil },
		runShallow: func(bool) doctor.Report { return doctor.Report{OK: true} },
		runDeep: func(context.Context, config.Config, doctor.DeepOptions) (doctor.DeepReport, error) {
			return doctor.DeepReport{Checks: []doctor.Check{{
				ID: "deterministic.check", Category: "test", Status: status,
				Summary: "summary", Detail: "detail", Remediation: "none", Required: true,
			}}}, nil
		},
	}
}

func withEmptyAIHome(t *testing.T) {
	t.Helper()
	t.Setenv("CAMUNDA_LAB_HOME", t.TempDir())
	paths.Reset()
	t.Cleanup(paths.Reset)
}

func assertScanResponseContract(
	t *testing.T,
	response *httptest.ResponseRecorder,
	status toolkit.Status,
	complete bool,
) {
	t.Helper()
	assertAPIJSONFields(
		t,
		response.Body.Bytes(),
		"ok",
		"status",
		"complete",
		"warnings",
		"scannedRoots",
		"failedRoots",
		"findings",
		"issues",
		"stats",
		"cli",
	)
	var result scanResponse
	decodeAPIJSON(t, response, &result)
	if result.Status != status || result.Complete != complete ||
		result.Warnings == nil || result.ScannedRoots == nil || result.FailedRoots == nil ||
		result.Findings == nil || result.Issues == nil || result.CLI == "" {
		t.Fatalf("incomplete scan response: %+v", result)
	}
}

func assertHTTPStatus(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != want {
		t.Fatalf("status = %d, want %d, body = %s", response.Code, want, response.Body.String())
	}
}

func decodeAPIJSON(t *testing.T, response *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(response.Body.Bytes(), target); err != nil {
		t.Fatalf("invalid JSON %q: %v", response.Body.String(), err)
	}
}

func assertAPIJSONFields(t *testing.T, encoded []byte, fields ...string) {
	t.Helper()
	var object map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &object); err != nil {
		t.Fatalf("invalid JSON %q: %v", encoded, err)
	}
	for _, field := range fields {
		if _, ok := object[field]; !ok {
			t.Errorf("missing JSON field %q in %s", field, encoded)
		}
	}
}

func assertAPIMarshaledFields(t *testing.T, value any, fields ...string) {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	assertAPIJSONFields(t, encoded, fields...)
}
