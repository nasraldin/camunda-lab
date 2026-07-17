package trace

import (
	"fmt"
	"strings"
	"time"
)

// Step is one activity on the timeline.
type Step struct {
	Name   string
	State  string // ACTIVE|COMPLETED|INCIDENT
	Detail string
}

// Timeline is an instance view.
type Timeline struct {
	InstanceKey string
	State       string
	Steps       []Step
}

// RenderASCII draws a vertical flow.
func RenderASCII(t Timeline) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Instance %s  (%s)\n\n", t.InstanceKey, t.State)
	for i, s := range t.Steps {
		line := s.Name
		if s.State == "INCIDENT" {
			line += "          ← INCIDENT"
			if s.Detail != "" {
				line += ": " + s.Detail
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if i < len(t.Steps)-1 {
			b.WriteString("↓\n")
		}
	}
	return b.String()
}

// FromActivities builds a timeline from ordered activity names (helper for APIs/tests).
func FromActivities(key, state string, activities []Step) Timeline {
	return Timeline{InstanceKey: key, State: state, Steps: activities}
}

// FollowOnce is a single poll tick for --follow tests.
func FollowOnce(prev, next Timeline) (Timeline, bool) {
	if len(next.Steps) > len(prev.Steps) || next.State != prev.State {
		return next, true
	}
	return next, false
}

// Sleep is overridable in tests.
var Sleep = time.Sleep
