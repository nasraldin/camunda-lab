package bpmn

import (
	"reflect"
	"testing"
)

func TestGraphReachabilityHappyPathAndAlternates(t *testing.T) {
	p := Process{
		ID: "order",
		Elements: []Element{
			{ID: "end", Type: "endEvent", Name: "Done"},
			{ID: "reject", Type: "task", Name: "Reject"},
			{ID: "gateway", Type: "exclusiveGateway", Name: "Approved?", DefaultFlow: "approved"},
			{ID: "start", Type: "startEvent", Name: "Start"},
			{ID: "work", Type: "serviceTask", Name: "Work"},
		},
		Flows: []Flow{
			{ID: "rejected", Source: "gateway", Target: "reject", Condition: "= not approved"},
			{ID: "finish-rejected", Source: "reject", Target: "end"},
			{ID: "approved", Source: "gateway", Target: "work"},
			{ID: "begin", Source: "start", Target: "gateway"},
			{ID: "finish", Source: "work", Target: "end"},
		},
	}
	g := NewGraph(p)
	if got := g.ReachableFrom("start"); len(got) != 5 || !got["end"] {
		t.Fatalf("reachable = %v", got)
	}
	if got, want := g.HappyPath(), []string{"start", "gateway", "work", "end"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("happy path = %v, want %v", got, want)
	}
	if got, want := g.AlternatePaths(), [][]string{{"start", "gateway", "reject", "end"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("alternates = %v, want %v", got, want)
	}
}

func TestGraphCyclesAndDeadEndsAreFiniteAndDeterministic(t *testing.T) {
	p := Process{
		ID: "cycle",
		Elements: []Element{
			{ID: "s", Type: "startEvent"},
			{ID: "a", Type: "task", Name: "A"},
			{ID: "b", Type: "task", Name: "B"},
			{ID: "dead", Type: "task", Name: "Dead"},
			{ID: "end", Type: "endEvent"},
		},
		Flows: []Flow{
			{ID: "1", Source: "s", Target: "a"},
			{ID: "2", Source: "a", Target: "b"},
			{ID: "3", Source: "b", Target: "a"},
			{ID: "4", Source: "s", Target: "end"},
		},
	}
	g := NewGraph(p)
	if got, want := g.DeadEnds(), []string{"dead"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("dead ends = %v, want %v", got, want)
	}
	if got, want := g.Cycles(), [][]string{{"a", "b", "a"}}; !reflect.DeepEqual(got, want) {
		t.Fatalf("cycles = %v, want %v", got, want)
	}
	if len(g.AlternatePaths()) > 4 {
		t.Fatalf("cycle produced unbounded paths: %v", g.AlternatePaths())
	}
}

func TestGraphTraversalRespectsSubprocessScope(t *testing.T) {
	p := Process{
		ID: "scoped",
		Elements: []Element{
			{ID: "topStart", Type: "startEvent"},
			{ID: "sub", Type: "subProcess"},
			{ID: "topEnd", Type: "endEvent"},
			{ID: "nestedStart", Type: "startEvent", ParentID: "sub"},
			{ID: "nestedTask", Type: "task", ParentID: "sub"},
			{ID: "nestedEnd", Type: "endEvent", ParentID: "sub"},
		},
		Flows: []Flow{
			{ID: "top1", Source: "topStart", Target: "sub"},
			{ID: "top2", Source: "sub", Target: "topEnd"},
			{ID: "nested1", Source: "nestedStart", Target: "nestedTask"},
			{ID: "nested2", Source: "nestedTask", Target: "nestedEnd"},
		},
	}
	top := NewGraph(p)
	if got, want := top.HappyPath(), []string{"topStart", "sub", "topEnd"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("top-level happy path = %v, want %v", got, want)
	}
	if got := top.AlternatePaths(); len(got) != 0 {
		t.Fatalf("nested start leaked into top-level alternates: %v", got)
	}
	nested := NewGraphForScope(p, "sub")
	if got, want := nested.HappyPath(), []string{"nestedStart", "nestedTask", "nestedEnd"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("nested happy path = %v, want %v", got, want)
	}
	if got := nested.ReachableFrom("topStart"); len(got) != 0 {
		t.Fatalf("out-of-scope node became reachable: %v", got)
	}
}
