package smoke

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

type Check struct {
	Name   string
	URL    string
	OK     bool
	Detail string
}

type Result struct {
	Checks []Check
	OK     bool
}

func Run(ctx context.Context, cfg config.Config) error {
	res := Probe(ctx, cfg)
	if !res.OK {
		var fails []string
		for _, c := range res.Checks {
			if !c.OK {
				fails = append(fails, c.Name+": "+c.Detail)
			}
		}
		if len(fails) == 0 {
			return fmt.Errorf("no HTTP endpoints responded successfully")
		}
		return fmt.Errorf("%s", strings.Join(fails, "; "))
	}
	return nil
}

func Probe(ctx context.Context, cfg config.Config) Result {
	client := &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}}
	var res Result
	res.OK = true
	requiredPassed := 0
	requiredTotal := 0
	for _, e := range urls.List(cfg) {
		if !strings.HasPrefix(e.URL, "http") {
			continue
		}
		required := smokeRequired(e.Name)
		if required {
			requiredTotal++
		}
		probe := urls.ProbeURL(e)
		c := Check{Name: e.Name, URL: probe}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, probe, nil)
		if err != nil {
			c.Detail = err.Error()
			res.Checks = append(res.Checks, c)
			if required {
				res.OK = false
			}
			continue
		}
		resp, err := client.Do(req)
		if err != nil {
			c.Detail = err.Error()
			res.Checks = append(res.Checks, c)
			if required {
				res.OK = false
			}
			continue
		}
		_ = resp.Body.Close()
		if resp.StatusCode < 500 {
			c.OK = true
			c.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
			if required {
				requiredPassed++
			}
		} else {
			c.Detail = fmt.Sprintf("HTTP %d", resp.StatusCode)
			if required {
				res.OK = false
			}
		}
		res.Checks = append(res.Checks, c)
	}
	if requiredTotal > 0 && requiredPassed == 0 {
		res.OK = false
	}
	if requiredTotal == 0 && len(res.Checks) > 0 {
		// Fallback: any successful HTTP check counts when no required apps apply.
		any := false
		for _, c := range res.Checks {
			if c.OK {
				any = true
				break
			}
		}
		res.OK = any
	}
	return res
}

// smokeRequired marks UI / auth endpoints that must answer for a healthy lab.
// Infra URLs (ES, connectors, REST roots) are reported but do not fail smoke.
func smokeRequired(name string) bool {
	switch name {
	case "operate", "tasklist", "admin", "console", "optimize", "identity", "web-modeler", "keycloak":
		return true
	default:
		return false
	}
}

func (r Result) Format(cfg config.Config) string {
	var required, optional []string
	for _, c := range r.Checks {
		line := ""
		switch {
		case c.OK:
			line = display.Success(fmt.Sprintf("%s (%s)", c.Name, c.Detail))
		case smokeRequired(c.Name):
			line = display.Fail(fmt.Sprintf("%s — %s", c.Name, c.Detail))
		default:
			line = display.Warn(fmt.Sprintf("%s — %s", c.Name, c.Detail))
		}
		if smokeRequired(c.Name) {
			required = append(required, line)
		} else {
			optional = append(optional, line)
		}
	}
	rep := display.Report{
		Title: "Camunda Lab Smoke",
		Fields: []display.Field{
			display.KV("Version", cfg.Version),
			display.KV("Profile", cfg.Profile),
		},
	}
	if len(required) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Apps", Items: required})
	}
	if len(optional) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "APIs and infra", Items: optional})
	}
	if r.OK {
		rep.Footer = []string{"Result: pass — required apps are reachable."}
	} else {
		rep.Footer = []string{"Result: fail — one or more required apps did not respond."}
	}
	var b strings.Builder
	rep.Write(&b)
	return b.String()
}

func Wait(ctx context.Context, cfg config.Config, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var last error
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		last = Run(ctx, cfg)
		if last == nil {
			return nil
		}
		time.Sleep(5 * time.Second)
	}
	if last == nil {
		last = fmt.Errorf("timeout waiting for healthy lab")
	}
	return fmt.Errorf("wait timed out after %s: %w", timeout, last)
}
