package api

import (
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
	"github.com/nasraldin/camunda-lab/internal/laberrors"
	"github.com/nasraldin/camunda-lab/internal/toolkit"
	"github.com/nasraldin/camunda-lab/internal/trace"
)

func TestErrorEnvelopeIsStable(t *testing.T) {
	rec := httptest.NewRecorder()
	writeErr(rec, http.StatusBadRequest, &laberrors.UserError{
		Message: "name is required",
		Hint:    "Provide a non-empty name.",
		Code:    "invalid_request",
	})

	var body errorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OK {
		t.Fatalf("ok must be false: %+v", body)
	}
	if body.Code != "invalid_request" {
		t.Fatalf("code = %q", body.Code)
	}
	if body.Error != "name is required" {
		t.Fatalf("error = %q", body.Error)
	}
	if body.Hint != "Provide a non-empty name." {
		t.Fatalf("hint = %q", body.Hint)
	}
}

func TestHTTPStatusMapping(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		want   int
		code   string
		hinted bool
	}{
		{
			name: "400 invalid request",
			err:  errors.New("json: unknown field \"surprise\""),
			want: http.StatusBadRequest, code: "invalid_request", hinted: true,
		},
		{
			name: "403 path forbidden",
			err:  errPathForbidden("/etc/passwd"),
			want: http.StatusForbidden, code: "path_forbidden", hinted: true,
		},
		{
			name: "404 missing resource",
			err:  &trace.NotFoundError{Key: "1"},
			want: http.StatusNotFound, code: "not_found",
		},
		{
			name: "409 conflict",
			err:  &env.Error{Kind: env.ErrorConflict, Operation: "remove", Name: "prod", Err: errors.New("referenced")},
			want: http.StatusConflict, code: "conflict",
		},
		{
			name: "409 artifact conflict",
			err:  &toolkit.Error{Kind: toolkit.ErrorArtifact, Err: errors.New("exists")},
			want: http.StatusConflict, code: "artifact",
		},
		{
			name: "413 payload too large",
			err:  errPayloadTooLarge,
			want: http.StatusRequestEntityTooLarge, code: "payload_too_large", hinted: true,
		},
		{
			name: "422 unprocessable env",
			err:  &env.Error{Kind: env.ErrorInvalid, Operation: "save", Name: "x", Err: errors.New("invalid kind")},
			want: http.StatusUnprocessableEntity, code: "invalid",
		},
		{
			name: "422 unprocessable toolkit",
			err:  &toolkit.Error{Kind: toolkit.ErrorInvalidRequest, Err: errors.New("language must be java, js, or python")},
			want: http.StatusUnprocessableEntity, code: "invalid_request",
		},
		{
			name: "502 upstream cluster",
			err:  &cluster.APIError{Method: "POST", Path: "/v2/x", StatusCode: 503, Err: errors.New("unavailable")},
			want: http.StatusBadGateway, code: "upstream",
		},
		{
			name: "502 upstream AI",
			err:  &toolkit.Error{Kind: toolkit.ErrorAI, Err: errors.New("provider down")},
			want: http.StatusBadGateway, code: "ai",
		},
		{
			name: "500 internal",
			err:  errors.New("unexpected boom"),
			want: http.StatusInternalServerError, code: "internal_error", hinted: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := httpStatusFor(tt.err)
			if got != tt.want {
				t.Fatalf("status = %d, want %d", got, tt.want)
			}
			rec := httptest.NewRecorder()
			writeMappedErr(rec, tt.err)
			if rec.Code != tt.want {
				t.Fatalf("recorded status = %d, want %d; body=%s", rec.Code, tt.want, rec.Body.String())
			}
			var body errorEnvelope
			if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.OK || body.Code != tt.code || body.Error == "" {
				t.Fatalf("envelope = %+v, want code %q", body, tt.code)
			}
			if tt.hinted && body.Hint == "" {
				t.Fatalf("expected hint for %s: %+v", tt.name, body)
			}
		})
	}
}

func TestWriteErrNeverLeaksTempPaths(t *testing.T) {
	leaky := filepath.Join(os.TempDir(), "camunda-lab-a3-secret.tar.gz")
	rec := httptest.NewRecorder()
	writeErr(rec, http.StatusInternalServerError, &os.PathError{
		Op: "rename", Path: leaky, Err: os.ErrPermission,
	})
	body := rec.Body.String()
	for _, needle := range []string{os.TempDir(), leaky, "camunda-lab-a3-secret"} {
		if strings.Contains(body, needle) {
			t.Fatalf("leaked %q in %s", needle, body)
		}
	}
	var envelope errorEnvelope
	if err := json.Unmarshal(rec.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.OK || envelope.Code == "" || envelope.Error == "" {
		t.Fatalf("unstable envelope: %+v", envelope)
	}
}

func TestSecurityErrorEnvelopeIncludesOKFalse(t *testing.T) {
	rec := httptest.NewRecorder()
	writeSecurityError(rec, http.StatusForbidden, "csrf_missing")
	var body errorEnvelope
	if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if body.OK || body.Code != "csrf_missing" || body.Error == "" {
		t.Fatalf("envelope = %+v", body)
	}
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d", rec.Code)
	}
}
