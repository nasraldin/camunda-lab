package k8s

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// Runner executes kubectl. Overridable in tests.
type Runner func(args ...string) (string, error)

// DefaultRunner shells out to kubectl on PATH.
func DefaultRunner(args ...string) (string, error) {
	cmd := exec.Command("kubectl", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

// ComponentMaps alias → deployment name pattern.
var ComponentMaps = map[string]string{
	"orchestration": "camunda-zeebe",
	"connectors":    "camunda-connectors",
	"operate":       "camunda-operate",
	"tasklist":      "camunda-tasklist",
	"identity":      "camunda-identity",
	"optimize":      "camunda-optimize",
	"elasticsearch": "elasticsearch-master",
	"workers":       "camunda-workers",
}

// Options for kubectl helpers.
type Options struct {
	Context   string
	Namespace string
	Release   string
	Runner    Runner
}

func (o Options) runner() Runner {
	if o.Runner != nil {
		return o.Runner
	}
	return DefaultRunner
}

func (o Options) baseArgs() []string {
	var args []string
	if o.Context != "" {
		args = append(args, "--context", o.Context)
	}
	ns := o.Namespace
	if ns == "" {
		ns = "camunda"
	}
	args = append(args, "-n", ns)
	return args
}

func releaseSelector(release string) string {
	if release == "" {
		release = "camunda"
	}
	return "app.kubernetes.io/instance=" + release
}

// Status runs kubectl get pods,svc.
func Status(o Options) (string, error) {
	args := append(o.baseArgs(), "get", "pods,svc", "-l", releaseSelector(o.Release))
	return o.runner()(args...)
}

// Logs tails a component.
func Logs(o Options, component string, follow bool, tail int) (string, error) {
	dep, err := resolve(component)
	if err != nil {
		return "", err
	}
	args := append(o.baseArgs(), "logs", "deploy/"+dep, "--tail", fmt.Sprintf("%d", tail))
	if follow {
		args = append(args, "-f")
	}
	return o.runner()(args...)
}

// Restart rollouts a deployment.
func Restart(o Options, component string) (string, error) {
	dep, err := resolve(component)
	if err != nil {
		return "", err
	}
	args := append(o.baseArgs(), "rollout", "restart", "deploy/"+dep)
	return o.runner()(args...)
}

// Scale sets replicas.
func Scale(o Options, component string, replicas int) (string, error) {
	dep, err := resolve(component)
	if err != nil {
		return "", err
	}
	args := append(o.baseArgs(), "scale", "deploy/"+dep, fmt.Sprintf("--replicas=%d", replicas))
	return o.runner()(args...)
}

func resolve(component string) (string, error) {
	dep, ok := ComponentMaps[component]
	if !ok {
		var keys []string
		for k := range ComponentMaps {
			keys = append(keys, k)
		}
		return "", fmt.Errorf("unknown component %q (known: %s)", component, strings.Join(keys, ", "))
	}
	return dep, nil
}

// FakeKubectl installs a temp kubectl that records args (for tests).
func FakeKubectl(t interface {
	TempDir() string
	Helper()
}, record *[]string) (Runner, string) {
	dir := ""
	if td, ok := t.(interface{ TempDir() string }); ok {
		dir = td.TempDir()
	}
	script := filepath.Join(dir, "kubectl")
	body := "#!/bin/sh\necho \"$@\"\n"
	_ = os.WriteFile(script, []byte(body), 0o755)
	return func(args ...string) (string, error) {
		*record = append([]string{}, args...)
		return strings.Join(args, " "), nil
	}, script
}
