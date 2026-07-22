package doctor

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// Status is the stable machine-readable outcome of a diagnostic check.
type Status string

const (
	StatusPass    Status = "pass"
	StatusWarn    Status = "warn"
	StatusFail    Status = "fail"
	StatusSkipped Status = "skipped"
)

// Check is one independently actionable diagnostic result.
type Check struct {
	ID          string        `json:"id"`
	Category    string        `json:"category"`
	Status      Status        `json:"status"`
	Summary     string        `json:"summary"`
	Detail      string        `json:"detail"`
	Remediation string        `json:"remediation"`
	Duration    time.Duration `json:"durationNs"`
	Required    bool          `json:"required"`
}

// Section remains an alias for callers that consumed the original deep API.
type Section = Check

// DeepReport is the stable text/JSON contract returned by deep diagnostics.
type DeepReport struct {
	OK     bool    `json:"ok"`
	Checks []Check `json:"checks"`
}

// Aggregate applies doctor policy: only required failures make the report fail.
func (r *DeepReport) Aggregate() {
	r.OK = true
	if r.Checks == nil {
		r.Checks = []Check{}
	}
	sort.SliceStable(r.Checks, func(i, j int) bool {
		return r.Checks[i].ID < r.Checks[j].ID
	})
	for _, check := range r.Checks {
		if check.Required && check.Status == StatusFail {
			r.OK = false
		}
	}
}

// JSON returns deterministic indented JSON and preserves empty arrays.
func (r DeepReport) JSON() ([]byte, error) {
	r.Aggregate()
	return json.MarshalIndent(r, "", "  ")
}

// Text renders checks in stable category/ID order without terminal styling.
func (r DeepReport) Text() string {
	r.Aggregate()
	checks := append([]Check(nil), r.Checks...)
	sort.SliceStable(checks, func(i, j int) bool {
		if checks[i].Category != checks[j].Category {
			return checks[i].Category < checks[j].Category
		}
		return checks[i].ID < checks[j].ID
	})
	var b strings.Builder
	b.WriteString("Camunda Lab Doctor (deep)\n")
	currentCategory := ""
	for _, check := range checks {
		if check.Category != currentCategory {
			currentCategory = check.Category
			fmt.Fprintf(&b, "\n%s\n", strings.ToUpper(currentCategory))
		}
		fmt.Fprintf(&b, "  [%s] %s — %s\n", check.Status, check.ID, check.Summary)
		fmt.Fprintf(&b, "    %s\n", check.Detail)
		fmt.Fprintf(&b, "    remediation: %s\n", check.Remediation)
	}
	if r.OK {
		b.WriteString("\nResult: healthy (no required checks failed)\n")
	} else {
		b.WriteString("\nResult: issues found (required checks failed)\n")
	}
	return b.String()
}

// FatalError identifies an invalid request/configuration that prevents checks
// from being selected. Runtime probe failures are represented as Check values.
type FatalError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Err     error  `json:"-"`
}

func (e *FatalError) Error() string {
	return e.Message
}

func (e *FatalError) Unwrap() error {
	return e.Err
}

func (e *FatalError) SafeCode() string {
	switch e.Code {
	case "invalid_environment", "invalid_configuration":
		return e.Code
	default:
		return "invalid_request"
	}
}

func (e *FatalError) SafeMessage() string {
	switch e.SafeCode() {
	case "invalid_environment":
		return "The active environment configuration is invalid."
	case "invalid_configuration":
		return "The lab configuration is invalid."
	default:
		return "The request configuration is invalid."
	}
}
