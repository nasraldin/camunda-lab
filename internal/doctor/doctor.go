package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/display"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

type Report struct {
	OK      bool
	FixHint string
	cfg     config.Config
	hasCfg  bool
	checks  []string
	notes   []string
}

func Run(fix bool) Report {
	var r Report
	r.OK = true

	check := func(name string, err error) {
		if err != nil {
			r.OK = false
			r.checks = append(r.checks, display.Fail(fmt.Sprintf("%s — %v", name, err)))
			return
		}
		r.checks = append(r.checks, display.Success(name))
	}

	check("Docker Engine reachable", exec.Command("docker", "version", "--format", "{{.Server.Version}}").Run())
	out, err := exec.Command("docker", "compose", "version").CombinedOutput()
	if err != nil {
		check("Docker Compose v2", fmt.Errorf("%s", strings.TrimSpace(string(out))))
	} else {
		ver := strings.TrimSpace(string(out))
		r.checks = append(r.checks, display.Success("Docker Compose v2 ("+ver+")"))
	}

	cfg, err := config.Load()
	if err != nil {
		check("Lab config", err)
	} else {
		r.hasCfg = true
		r.cfg = cfg
		r.checks = append(r.checks, display.Success("Lab config readable"))
		dir := paths.VersionDir(cfg.Version)
		if _, err := os.Stat(dir); err != nil {
			check("Distribution directory", err)
			r.FixHint = "camunda install"
		} else {
			r.checks = append(r.checks, display.Success("Distribution directory present"))
		}
		if fix && r.FixHint == "" && !r.OK {
			r.FixHint = "camunda install && camunda doctor"
		}
	}

	if _, err := exec.LookPath("cosign"); err != nil {
		r.notes = append(r.notes, display.Info("cosign not installed (optional zip verify)"))
	} else {
		r.checks = append(r.checks, display.Success("cosign available"))
	}
	return r
}

func (r Report) Format() string {
	rep := display.Report{Title: "Camunda Lab Doctor"}
	if r.hasCfg {
		rep.Fields = []display.Field{
			display.KV("Version", r.cfg.Version),
			display.KV("Profile", r.cfg.Profile),
			display.KV("Resources", r.cfg.Resources),
			display.KV("Project", r.cfg.ComposeProject),
		}
	}
	if len(r.checks) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Checks", Items: r.checks})
	}
	if len(r.notes) > 0 {
		rep.Sections = append(rep.Sections, display.Section{Title: "Notes", Items: r.notes})
	}
	if r.OK {
		rep.Footer = []string{"Result: healthy — lab prerequisites look good."}
	} else {
		rep.Footer = []string{"Result: issues found."}
		if r.FixHint != "" {
			rep.Footer = append(rep.Footer, "Hint: "+r.FixHint)
		}
	}
	var b strings.Builder
	rep.Write(&b)
	return b.String()
}
