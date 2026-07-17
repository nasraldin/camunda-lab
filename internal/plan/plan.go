package plan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Resource is a local or remote definition inventory entry.
type Resource struct {
	Key     string // process/decision/form id or filename
	Path    string
	Digest  string
	Version string
}

// Action is create|update|delete|noop.
type Action struct {
	Key     string
	Kind    string
	Detail  string
	Warning string
}

// Plan compares local vs remote inventories.
type Plan struct {
	Env     string
	Actions []Action
}

// LocalInventory walks bpmn/processes/dmn/forms under root.
func LocalInventory(root string) ([]Resource, error) {
	var out []Resource
	for _, sub := range []string{"bpmn", "processes", "dmn", "forms"} {
		dir := filepath.Join(root, sub)
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			path := filepath.Join(dir, e.Name())
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, err
			}
			sum := sha256.Sum256(data)
			out = append(out, Resource{
				Key:    e.Name(),
				Path:   path,
				Digest: hex.EncodeToString(sum[:8]),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

// Build compares local to remote (keyed by Key).
func Build(env string, local, remote []Resource) Plan {
	lm := map[string]Resource{}
	rm := map[string]Resource{}
	for _, r := range local {
		lm[r.Key] = r
	}
	for _, r := range remote {
		rm[r.Key] = r
	}
	var actions []Action
	for k, l := range lm {
		r, ok := rm[k]
		if !ok {
			actions = append(actions, Action{Key: k, Kind: "create", Detail: "new local resource"})
			continue
		}
		if l.Digest != r.Digest && r.Digest != "" {
			a := Action{Key: k, Kind: "update", Detail: fmt.Sprintf("cluster %s → local %s", empty(r.Version, r.Digest), l.Digest)}
			if r.Version != "" {
				a.Warning = "new version while instances may still run"
			}
			actions = append(actions, a)
			continue
		}
		actions = append(actions, Action{Key: k, Kind: "noop", Detail: "in sync"})
	}
	for k := range rm {
		if _, ok := lm[k]; !ok {
			actions = append(actions, Action{Key: k, Kind: "delete", Detail: "present on cluster, missing locally"})
		}
	}
	sort.Slice(actions, func(i, j int) bool {
		if actions[i].Kind != actions[j].Kind {
			return actions[i].Kind < actions[j].Kind
		}
		return actions[i].Key < actions[j].Key
	})
	return Plan{Env: env, Actions: actions}
}

// FormatText renders terraform-like preview.
func FormatText(p Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan (env=%s)\n\n", p.Env)
	groups := map[string][]Action{}
	order := []string{"create", "update", "delete", "noop"}
	for _, a := range p.Actions {
		groups[a.Kind] = append(groups[a.Kind], a)
	}
	titles := map[string]string{"create": "Create", "update": "Update", "delete": "Delete", "noop": "Noop"}
	marks := map[string]string{"create": "✓", "update": "~", "delete": "-", "noop": "="}
	var warnings []string
	for _, kind := range order {
		items := groups[kind]
		if len(items) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s\n", titles[kind])
		for _, a := range items {
			fmt.Fprintf(&b, "  %s %s  %s\n", marks[kind], a.Key, a.Detail)
			if a.Warning != "" {
				warnings = append(warnings, a.Key+" — "+a.Warning)
			}
		}
		b.WriteByte('\n')
	}
	if len(warnings) > 0 {
		b.WriteString("Warnings\n")
		for _, w := range warnings {
			fmt.Fprintf(&b, "  ! %s\n", w)
		}
	}
	b.WriteString("\nPreview only — does not deploy. Use official Camunda tooling to apply.\n")
	return b.String()
}

func empty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
