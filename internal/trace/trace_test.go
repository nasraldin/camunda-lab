package trace

import (
	"strings"
	"testing"
)

func TestRenderASCII(t *testing.T) {
	tl := FromActivities("2251799813685248", "ACTIVE", []Step{
		{Name: "OrderCreated", State: "COMPLETED"},
		{Name: "ValidateCustomer", State: "COMPLETED"},
		{Name: "Payment", State: "INCIDENT", Detail: "job timeout"},
	})
	out := RenderASCII(tl)
	if !strings.Contains(out, "↓") || !strings.Contains(out, "INCIDENT") {
		t.Fatal(out)
	}
}

func TestFollowOnce(t *testing.T) {
	a := FromActivities("1", "ACTIVE", []Step{{Name: "A"}})
	b := FromActivities("1", "ACTIVE", []Step{{Name: "A"}, {Name: "B"}})
	_, changed := FollowOnce(a, b)
	if !changed {
		t.Fatal("expected change")
	}
	_, changed = FollowOnce(b, b)
	if changed {
		t.Fatal("expected no change")
	}
}
