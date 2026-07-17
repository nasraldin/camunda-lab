package doctor

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/urls"
)

// Section is a deep-check result row.
type Section struct {
	Name    string
	Status  string // ok|warn|fail
	Detail  string
	FixHint string
}

// DeepOptions configures deep probes.
type DeepOptions struct {
	Timeout time.Duration
	Client  *http.Client // optional for tests
	Dial    func(network, address string) error
}

// Deep runs HTTP/TCP probes against lab URLs. Requires readable lab config.
func Deep(ctx context.Context, cfg config.Config, opts DeepOptions) []Section {
	if opts.Timeout <= 0 {
		opts.Timeout = 5 * time.Second
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{
			Timeout: opts.Timeout,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	dial := opts.Dial
	if dial == nil {
		dial = func(network, address string) error {
			c, err := net.DialTimeout(network, address, opts.Timeout)
			if err != nil {
				return err
			}
			return c.Close()
		}
	}

	var sections []Section
	for _, e := range urls.List(cfg) {
		name := e.Name
		kind, target := urls.ProbeTarget(e)
		if kind == "http" {
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
			if err != nil {
				sections = append(sections, Section{Name: name, Status: "fail", Detail: err.Error(), FixHint: "camunda up && camunda wait"})
				continue
			}
			resp, err := client.Do(req)
			if err != nil {
				sections = append(sections, Section{Name: name, Status: "fail", Detail: err.Error(), FixHint: "camunda up && camunda wait"})
				continue
			}
			_ = resp.Body.Close()
			if resp.StatusCode >= 500 {
				sections = append(sections, Section{Name: name, Status: "fail", Detail: fmt.Sprintf("HTTP %d", resp.StatusCode), FixHint: "camunda logs -f " + name})
				continue
			}
			st := "ok"
			detail := fmt.Sprintf("HTTP %d", resp.StatusCode)
			if resp.StatusCode >= 400 {
				st = "warn"
			}
			sections = append(sections, Section{Name: name, Status: st, Detail: detail})
			continue
		}
		if err := dial("tcp", target); err != nil {
			sections = append(sections, Section{Name: name, Status: "fail", Detail: err.Error(), FixHint: "camunda up"})
		} else {
			sections = append(sections, Section{Name: name, Status: "ok", Detail: "tcp open"})
		}
	}

	if cfg.AI.Enabled {
		sections = append(sections, Section{Name: "ai", Status: "ok", Detail: "ai.enabled in lab config (probe MCP via camunda ai status)"})
	}

	sections = append(sections, Section{
		Name:   "version-profile",
		Status: "ok",
		Detail: fmt.Sprintf("%s / %s / %s", cfg.Version, cfg.Profile, cfg.Resources),
	})
	return sections
}

// FormatDeep renders Healthy / Warnings / Failures.
func FormatDeep(base Report, sections []Section) string {
	var healthy, warnings, failures []string
	ok := base.OK
	for _, s := range sections {
		line := fmt.Sprintf("%s — %s", s.Name, s.Detail)
		switch s.Status {
		case "ok":
			healthy = append(healthy, display.Success(line))
		case "warn":
			warnings = append(warnings, display.Info(line))
			if s.FixHint != "" {
				warnings = append(warnings, "  hint: "+s.FixHint)
			}
		default:
			ok = false
			failures = append(failures, display.Fail(line))
			if s.FixHint != "" {
				failures = append(failures, "  hint: "+s.FixHint)
			}
		}
	}
	rep := display.Report{Title: "Camunda Lab Doctor (deep)"}
	if base.hasCfg {
		rep.Fields = []display.Field{
			display.KV("Version", base.cfg.Version),
			display.KV("Profile", base.cfg.Profile),
			display.KV("Resources", base.cfg.Resources),
		}
	}
	if len(base.checks) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Prerequisites", Items: base.checks})
	}
	if len(healthy) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Healthy", Items: healthy})
	}
	if len(warnings) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Warnings", Items: warnings})
	}
	if len(failures) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Failures", Items: failures})
	}
	if ok && len(failures) == 0 {
		rep.Footer = []string{"Result: healthy"}
	} else {
		rep.Footer = []string{"Result: issues found", "Hints: camunda up · camunda wait · camunda logs"}
	}
	var b strings.Builder
	rep.Write(&b)
	return b.String()
}

// DeepOK is false if any section failed.
func DeepOK(sections []Section) bool {
	for _, s := range sections {
		if s.Status == "fail" {
			return false
		}
	}
	return true
}
