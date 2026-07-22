package inventory

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Kind is a deployable Camunda resource kind.
type Kind string

const (
	KindProcess  Kind = "process"
	KindDecision Kind = "decision"
	KindForm     Kind = "form"
)

var ErrUnsupportedKind = errors.New("unsupported inventory kind")

func (k Kind) String() string { return string(k) }

// Source records where an inventory item was observed.
type Source struct {
	Type        string `json:"type"`
	Environment string `json:"environment,omitempty"`
	Endpoint    string `json:"endpoint,omitempty"`
	ProjectRoot string `json:"projectRoot,omitempty"`
}

// Resource is a canonical local or deployed resource.
// Keys are strings so 64-bit Camunda keys never pass through float64.
type Resource struct {
	Kind         Kind   `json:"kind"`
	ID           string `json:"id"`
	Key          string `json:"key,omitempty"`
	Name         string `json:"name,omitempty"`
	Version      int64  `json:"version,omitempty"`
	Path         string `json:"path,omitempty"`
	Digest       string `json:"digest"`
	TenantID     string `json:"tenantId,omitempty"`
	LastModified string `json:"lastModified,omitempty"`
	Status       string `json:"status,omitempty"`
	Source       Source `json:"source"`
}

// Unsupported describes a capability that could not be inventoried.
type Unsupported struct {
	Kind     Kind   `json:"kind"`
	Reason   string `json:"reason"`
	Required bool   `json:"required"`
}

// Warning makes a bounded or partial result explicit.
type Warning struct {
	Capability string `json:"capability"`
	Message    string `json:"message"`
}

// Inventory is the shared normalized snapshot used by later consumers.
type Inventory struct {
	Source      Source        `json:"source"`
	Resources   []Resource    `json:"resources"`
	Unsupported []Unsupported `json:"unsupported,omitempty"`
	Warnings    []Warning     `json:"warnings,omitempty"`
	Partial     bool          `json:"partial,omitempty"`
}

// ValidateComparable rejects snapshots that could make a comparison lie.
func (i Inventory) ValidateComparable() error {
	if i.Partial {
		return errors.New("inventory is partial")
	}
	for _, unsupported := range i.Unsupported {
		if unsupported.Required {
			return fmt.Errorf("%s inventory is required but unsupported: %s", unsupported.Kind, unsupported.Reason)
		}
	}
	seen := make(map[string]struct{}, len(i.Resources))
	for _, resource := range i.Resources {
		if !validKind(resource.Kind) {
			return fmt.Errorf("%w: %q", ErrUnsupportedKind, resource.Kind)
		}
		if strings.TrimSpace(resource.ID) == "" {
			return fmt.Errorf("%s resource has no canonical ID", resource.Kind)
		}
		if strings.TrimSpace(resource.Digest) == "" {
			return fmt.Errorf("%s %q has an empty canonical digest", resource.Kind, resource.ID)
		}
		identity := resource.Kind.String() + "\x00" + resource.ID
		if resource.Version > 0 {
			identity += "\x00" + strconv.FormatInt(resource.Version, 10)
		}
		if _, duplicate := seen[identity]; duplicate {
			return fmt.Errorf("duplicate %s resource ID %q", resource.Kind, resource.ID)
		}
		seen[identity] = struct{}{}
	}
	return nil
}

func validKind(kind Kind) bool {
	return kind == KindProcess || kind == KindDecision || kind == KindForm
}

func sortInventory(value *Inventory) {
	sort.SliceStable(value.Resources, func(a, b int) bool {
		left, right := value.Resources[a], value.Resources[b]
		if left.Kind != right.Kind {
			return left.Kind < right.Kind
		}
		if left.ID != right.ID {
			return left.ID < right.ID
		}
		if left.Version != right.Version {
			return left.Version < right.Version
		}
		if left.Key != right.Key {
			return left.Key < right.Key
		}
		return left.Path < right.Path
	})
	sort.SliceStable(value.Unsupported, func(a, b int) bool {
		if value.Unsupported[a].Kind != value.Unsupported[b].Kind {
			return value.Unsupported[a].Kind < value.Unsupported[b].Kind
		}
		return value.Unsupported[a].Reason < value.Unsupported[b].Reason
	})
	sort.SliceStable(value.Warnings, func(a, b int) bool {
		if value.Warnings[a].Capability != value.Warnings[b].Capability {
			return value.Warnings[a].Capability < value.Warnings[b].Capability
		}
		return value.Warnings[a].Message < value.Warnings[b].Message
	})
}
