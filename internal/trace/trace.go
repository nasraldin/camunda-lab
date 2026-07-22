package trace

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/nasraldin/camunda-lab/internal/env"
	"github.com/nasraldin/camunda-lab/internal/inventory"
)

type Status string

const (
	StatusCompleted Status = "completed"
	StatusPartial   Status = "partial"
)

type EventKind string

const (
	EventProcess  EventKind = "process"
	EventElement  EventKind = "element"
	EventIncident EventKind = "incident"
	EventJob      EventKind = "job"
)

// Step is one activity on the ASCII timeline.
type Step struct {
	Name   string `json:"name"`
	State  string `json:"state"`
	Detail string `json:"detail,omitempty"`
}

// Event is one deterministic timeline entry derived from canonical searches.
type Event struct {
	Kind               EventKind `json:"kind"`
	Key                string    `json:"key"`
	Timestamp          string    `json:"timestamp"`
	Status             string    `json:"status"`
	Name               string    `json:"name,omitempty"`
	Detail             string    `json:"detail,omitempty"`
	ElementID          string    `json:"elementId,omitempty"`
	ProcessInstanceKey string    `json:"processInstanceKey,omitempty"`
	Type               string    `json:"type,omitempty"`
	Phase              string    `json:"phase,omitempty"`
}

// Timeline is the shared one-shot/follow process-instance contract.
type Timeline struct {
	InstanceKey string              `json:"instanceKey"`
	State       string              `json:"state"`
	Status      Status              `json:"status"`
	Complete    bool                `json:"complete"`
	Partial     bool                `json:"partial"`
	Environment env.Resolved        `json:"environment"`
	Source      inventory.Source    `json:"source"`
	Events      []Event             `json:"events"`
	Steps       []Step              `json:"steps"`
	Warnings    []inventory.Warning `json:"warnings"`
	OperateURL  string              `json:"operateUrl,omitempty"`
}

// Request selects the instance and optional follow bounds.
type Request struct {
	Environment        string
	ProjectRoot        string
	ProcessInstanceKey string
	PageSize           int
	MaxPages           int
	MaxItems           int
	Timeout            time.Duration
	MaxEvents          int
	IdleStop           time.Duration
}

type NotFoundError struct {
	Key string
}

func (e *NotFoundError) Error() string {
	return fmt.Sprintf("process instance %s was not found", e.Key)
}

var ErrInvalidRequest = errors.New("invalid trace request")

// RenderASCII draws a vertical activity flow from derived steps.
func RenderASCII(t Timeline) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Instance %s  (%s)\n\n", t.InstanceKey, t.State)
	steps := t.Steps
	if len(steps) == 0 {
		for _, event := range t.Events {
			if event.Kind != EventElement || event.Phase == "end" {
				continue
			}
			steps = append(steps, Step{Name: event.Name, State: event.Status, Detail: event.Detail})
		}
	}
	for i, s := range steps {
		line := s.Name
		if s.State == "INCIDENT" {
			line += "          ← INCIDENT"
			if s.Detail != "" {
				line += ": " + s.Detail
			}
		}
		b.WriteString(line)
		b.WriteByte('\n')
		if i < len(steps)-1 {
			b.WriteString("↓\n")
		}
	}
	return b.String()
}

// FromActivities builds a timeline from ordered activity names (helper for APIs/tests).
func FromActivities(key, state string, activities []Step) Timeline {
	return Timeline{
		InstanceKey: key, State: state, Status: StatusCompleted, Complete: true,
		Steps: activities, Events: []Event{}, Warnings: []inventory.Warning{},
	}
}

// FollowOnce is a single poll tick for --follow tests.
func FollowOnce(prev, next Timeline) (Timeline, bool) {
	if timelineFingerprint(prev) != timelineFingerprint(next) {
		return next, true
	}
	return next, false
}

// Wait is a context-aware delay used by Follow. Overridable in tests.
var Wait = func(ctx context.Context, d time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if d <= 0 {
		return nil
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// Now is overridable in tests.
var Now = time.Now

func timelineFingerprint(tl Timeline) string {
	var b strings.Builder
	fmt.Fprintf(&b, "%s|%s|%t|%t|%s|", tl.InstanceKey, tl.State, tl.Complete, tl.Partial, tl.Status)
	for _, event := range tl.Events {
		fmt.Fprintf(&b, "%s:%s:%s:%s:%s:%s;", event.Kind, event.Key, event.Timestamp, event.Status, event.Phase, event.Detail)
	}
	for _, step := range tl.Steps {
		fmt.Fprintf(&b, "step:%s:%s:%s;", step.Name, step.State, step.Detail)
	}
	for _, warning := range tl.Warnings {
		fmt.Fprintf(&b, "warn:%s:%s;", warning.Capability, warning.Message)
	}
	return b.String()
}
