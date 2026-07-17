package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// ChangeKind categorizes a semantic change.
type ChangeKind string

const (
	ElementAdded   ChangeKind = "ElementAdded"
	ElementRemoved ChangeKind = "ElementRemoved"
	AttrChanged    ChangeKind = "AttrChanged"
	FlowAdded      ChangeKind = "FlowAdded"
	FlowRemoved    ChangeKind = "FlowRemoved"
	FlowChanged    ChangeKind = "FlowChanged"
	MessageAdded   ChangeKind = "MessageAdded"
	MessageRemoved ChangeKind = "MessageRemoved"
)

// Change is one semantic difference.
type Change struct {
	Kind    ChangeKind
	ID      string
	Summary string
}

// Compare returns semantic changes from a → b.
func Compare(a, b bpmn.Model) []Change {
	var out []Change

	ae := indexElements(a)
	be := indexElements(b)
	for id, el := range be {
		if _, ok := ae[id]; !ok {
			out = append(out, Change{Kind: ElementAdded, ID: id, Summary: fmt.Sprintf("Added %s %q", el.Type, displayName(el))})
		}
	}
	for id, el := range ae {
		if _, ok := be[id]; !ok {
			out = append(out, Change{Kind: ElementRemoved, ID: id, Summary: fmt.Sprintf("Removed %s %q", el.Type, displayName(el))})
		}
	}
	for id, before := range ae {
		after, ok := be[id]
		if !ok {
			continue
		}
		if before.Name != after.Name {
			out = append(out, Change{Kind: AttrChanged, ID: id, Summary: fmt.Sprintf("Changed name %q -> %q", before.Name, after.Name)})
		}
		if before.Timer != after.Timer {
			out = append(out, Change{Kind: AttrChanged, ID: id, Summary: fmt.Sprintf("Timer changed %s -> %s", empty(before.Timer), empty(after.Timer))})
		}
		if before.RetryCount != after.RetryCount {
			out = append(out, Change{Kind: AttrChanged, ID: id, Summary: fmt.Sprintf("Changed retry policy %s -> %s", empty(before.RetryCount), empty(after.RetryCount))})
		}
		if before.JobType != after.JobType {
			out = append(out, Change{Kind: AttrChanged, ID: id, Summary: fmt.Sprintf("Changed job type %q -> %q", before.JobType, after.JobType)})
		}
		if before.DefaultFlow != after.DefaultFlow {
			out = append(out, Change{Kind: AttrChanged, ID: id, Summary: fmt.Sprintf("Changed default flow %s -> %s", empty(before.DefaultFlow), empty(after.DefaultFlow))})
		}
	}

	af := indexFlows(a)
	bf := indexFlows(b)
	for id, f := range bf {
		if _, ok := af[id]; !ok {
			out = append(out, Change{Kind: FlowAdded, ID: id, Summary: fmt.Sprintf("Added sequence flow %s -> %s", f.Source, f.Target)})
		}
	}
	for id, f := range af {
		if _, ok := bf[id]; !ok {
			out = append(out, Change{Kind: FlowRemoved, ID: id, Summary: fmt.Sprintf("Removed sequence flow %s -> %s", f.Source, f.Target)})
		}
	}
	for id, before := range af {
		after, ok := bf[id]
		if !ok {
			continue
		}
		if before.Condition != after.Condition || before.Source != after.Source || before.Target != after.Target {
			out = append(out, Change{Kind: FlowChanged, ID: id, Summary: fmt.Sprintf("Gateway/flow condition modified (%s)", id)})
		}
	}

	am := indexMessages(a)
	bm := indexMessages(b)
	for id := range bm {
		if _, ok := am[id]; !ok {
			out = append(out, Change{Kind: MessageAdded, ID: id, Summary: "Added message"})
		}
	}
	for id := range am {
		if _, ok := bm[id]; !ok {
			out = append(out, Change{Kind: MessageRemoved, ID: id, Summary: "Removed message"})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func FormatText(changes []Change) string {
	if len(changes) == 0 {
		return "No semantic changes.\n"
	}
	var b strings.Builder
	for _, c := range changes {
		fmt.Fprintf(&b, "✓ %s\n", c.Summary)
	}
	return b.String()
}

func indexElements(m bpmn.Model) map[string]bpmn.Element {
	out := map[string]bpmn.Element{}
	for _, e := range m.Elements {
		out[e.ID] = e
	}
	return out
}

func indexFlows(m bpmn.Model) map[string]bpmn.Flow {
	out := map[string]bpmn.Flow{}
	for _, f := range m.Flows {
		out[f.ID] = f
	}
	return out
}

func indexMessages(m bpmn.Model) map[string]bpmn.Message {
	out := map[string]bpmn.Message{}
	for _, msg := range m.Messages {
		out[msg.ID] = msg
	}
	return out
}

func displayName(e bpmn.Element) string {
	if e.Name != "" {
		return e.Name
	}
	return e.ID
}

func empty(s string) string {
	if s == "" {
		return "(none)"
	}
	return s
}
