package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/doctor"
)

func TestWriteErrPreservesSafeDoctorFatalCode(t *testing.T) {
	tests := []struct {
		code string
		want string
	}{
		{"invalid_environment", "active environment configuration is invalid"},
		{"invalid_configuration", "lab configuration is invalid"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			secret := "https://user:password@example.test?token=secret"
			err := fmt.Errorf("doctor failed: %w", &doctor.FatalError{
				Code: tt.code, Message: secret,
			})
			rec := httptest.NewRecorder()
			writeErr(rec, http.StatusBadRequest, err)

			var body struct {
				Code  string `json:"code"`
				Error string `json:"error"`
			}
			if decodeErr := json.NewDecoder(rec.Body).Decode(&body); decodeErr != nil {
				t.Fatal(decodeErr)
			}
			if body.Code != tt.code {
				t.Fatalf("code = %q, want %q; body=%s", body.Code, tt.code, rec.Body.String())
			}
			if !strings.Contains(strings.ToLower(body.Error), tt.want) {
				t.Fatalf("safe error = %q, want phrase %q", body.Error, tt.want)
			}
			for _, leaked := range []string{"password", "token=secret", "example.test"} {
				if strings.Contains(body.Error, leaked) {
					t.Fatalf("response leaked %q: %s", leaked, rec.Body.String())
				}
			}
		})
	}
}
