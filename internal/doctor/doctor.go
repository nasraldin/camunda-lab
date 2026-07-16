package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/compose"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

type Report struct {
	OK      bool
	Lines   []string
	FixHint string
}

func Run(fix bool) Report {
	var r Report
	r.OK = true
	check := func(name string, err error) {
		if err != nil {
			r.OK = false
			r.Lines = append(r.Lines, fmt.Sprintf("FAIL  %s: %v", name, err))
		} else {
			r.Lines = append(r.Lines, fmt.Sprintf("OK    %s", name))
		}
	}

	check("docker", exec.Command("docker", "version", "--format", "{{.Server.Version}}").Run())
	out, err := exec.Command("docker", "compose", "version").CombinedOutput()
	if err != nil {
		check("docker compose v2", fmt.Errorf("%s", strings.TrimSpace(string(out))))
	} else {
		check("docker compose v2", nil)
		r.Lines[len(r.Lines)-1] = "OK    docker compose v2 (" + strings.TrimSpace(string(out)) + ")"
	}

	cfg, err := config.Load()
	check("config", err)
	if err == nil {
		r.Lines = append(r.Lines, fmt.Sprintf("INFO  version=%s profile=%s resources=%s", cfg.Version, cfg.Profile, cfg.Resources))
		dir := paths.VersionDir(cfg.Version)
		if _, err := os.Stat(dir); err != nil {
			check("version dir "+dir, err)
			r.FixHint = "run: camunda install"
		} else {
			check("version dir", nil)
		}
		if fix {
			_ = compose.NewRunner() // placeholder — restart via lab in CLI layer
		}
	}

	if _, err := exec.LookPath("cosign"); err != nil {
		r.Lines = append(r.Lines, "INFO  cosign not installed (optional zip verify skipped)")
	} else {
		r.Lines = append(r.Lines, "OK    cosign available")
	}
	return r
}
