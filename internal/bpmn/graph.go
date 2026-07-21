package bpmn

import (
	"sort"
	"strconv"
	"strings"
)

// Graph is the deterministic sequence-flow graph for one process.
type Graph struct {
	process  Process
	scope    string
	nodes    map[string]Element
	outgoing map[string][]Flow
	order    []string
}

// NewGraph constructs a graph exclusively from process sequence flows.
func NewGraph(process Process) Graph {
	return newGraphForScope(process, "")
}

// NewGraphForScope constructs a graph for a subprocess scope.
func NewGraphForScope(process Process, parentID string) Graph {
	return newGraphForScope(process, parentID)
}

func newGraphForScope(process Process, parentID string) Graph {
	g := Graph{
		process: process, scope: parentID, nodes: map[string]Element{},
		outgoing: map[string][]Flow{},
	}
	for _, element := range process.Elements {
		if element.ParentID != parentID {
			continue
		}
		g.nodes[element.ID] = element
		g.order = append(g.order, element.ID)
	}
	for _, flow := range process.Flows {
		if _, sourceOK := g.nodes[flow.Source]; !sourceOK {
			continue
		}
		if _, targetOK := g.nodes[flow.Target]; !targetOK {
			continue
		}
		g.outgoing[flow.Source] = append(g.outgoing[flow.Source], flow)
	}
	sort.SliceStable(g.order, func(i, j int) bool {
		return graphElementKey(g.nodes[g.order[i]]) < graphElementKey(g.nodes[g.order[j]])
	})
	for source := range g.outgoing {
		sort.SliceStable(g.outgoing[source], func(i, j int) bool {
			return g.edgeKey(g.outgoing[source][i]) < g.edgeKey(g.outgoing[source][j])
		})
	}
	return g
}

// ReachableFrom returns all nodes reachable from the supplied node IDs, including starts.
func (g Graph) ReachableFrom(starts ...string) map[string]bool {
	reachable := map[string]bool{}
	queue := append([]string(nil), starts...)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if _, ok := g.nodes[current]; !ok {
			continue
		}
		if reachable[current] {
			continue
		}
		reachable[current] = true
		for _, flow := range g.outgoing[current] {
			if !reachable[flow.Target] {
				queue = append(queue, flow.Target)
			}
		}
	}
	return reachable
}

// HappyPath returns the preferred start-to-terminal path. Gateway defaults win.
func (g Graph) HappyPath() []string {
	starts := g.starts()
	if len(starts) == 0 {
		return nil
	}
	path := []string{starts[0]}
	seen := map[string]bool{starts[0]: true}
	for {
		current := path[len(path)-1]
		flows := g.preferredOutgoing(current)
		if len(flows) == 0 {
			return path
		}
		next := flows[0].Target
		if seen[next] {
			return path
		}
		path = append(path, next)
		seen[next] = true
	}
}

// AlternatePaths returns finite start-to-terminal paths except HappyPath.
func (g Graph) AlternatePaths() [][]string {
	var paths [][]string
	for _, start := range g.starts() {
		g.walkPaths(start, []string{start}, map[string]bool{start: true}, &paths)
	}
	happy := pathKey(g.HappyPath())
	filtered := paths[:0]
	seen := map[string]bool{}
	for _, path := range paths {
		key := pathKey(path)
		if key == happy || seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, path)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		return g.pathSemanticKey(filtered[i]) < g.pathSemanticKey(filtered[j])
	})
	return filtered
}

// DeadEnds returns non-end flow nodes with no outgoing sequence flow.
func (g Graph) DeadEnds() []string {
	var result []string
	for _, id := range g.order {
		if g.nodes[id].Type != "endEvent" && len(g.outgoing[id]) == 0 {
			result = append(result, id)
		}
	}
	return result
}

// Cycles returns deterministic simple cycles with the first node repeated at the end.
func (g Graph) Cycles() [][]string {
	var result [][]string
	seenCycles := map[string]bool{}
	var visit func(string, []string, map[string]int)
	visit = func(current string, path []string, positions map[string]int) {
		if position, ok := positions[current]; ok {
			cycle := canonicalCycle(append(append([]string(nil), path[position:]...), current), g)
			key := pathKey(cycle)
			if !seenCycles[key] {
				seenCycles[key] = true
				result = append(result, cycle)
			}
			return
		}
		positions[current] = len(path)
		path = append(path, current)
		for _, flow := range g.outgoing[current] {
			visit(flow.Target, path, positions)
		}
		delete(positions, current)
	}
	for _, id := range g.order {
		visit(id, nil, map[string]int{})
	}
	sort.SliceStable(result, func(i, j int) bool {
		return g.pathSemanticKey(result[i]) < g.pathSemanticKey(result[j])
	})
	return result
}

func (g Graph) starts() []string {
	var starts []string
	for _, id := range g.order {
		if g.nodes[id].Type == "startEvent" {
			starts = append(starts, id)
		}
	}
	return starts
}

func (g Graph) preferredOutgoing(source string) []Flow {
	flows := append([]Flow(nil), g.outgoing[source]...)
	defaultFlow := g.nodes[source].DefaultFlow
	sort.SliceStable(flows, func(i, j int) bool {
		leftDefault, rightDefault := flows[i].ID == defaultFlow, flows[j].ID == defaultFlow
		if leftDefault != rightDefault {
			return leftDefault
		}
		leftUnconditional, rightUnconditional := flows[i].Condition == "", flows[j].Condition == ""
		if leftUnconditional != rightUnconditional {
			return leftUnconditional
		}
		return g.edgeKey(flows[i]) < g.edgeKey(flows[j])
	})
	return flows
}

func (g Graph) walkPaths(current string, path []string, seen map[string]bool, paths *[][]string) {
	flows := g.preferredOutgoing(current)
	if len(flows) == 0 {
		*paths = append(*paths, append([]string(nil), path...))
		return
	}
	for _, flow := range flows {
		if seen[flow.Target] {
			continue
		}
		seen[flow.Target] = true
		g.walkPaths(flow.Target, append(path, flow.Target), seen, paths)
		delete(seen, flow.Target)
	}
}

func (g Graph) edgeKey(flow Flow) string {
	return strings.Join([]string{
		graphElementKey(g.nodes[flow.Target]), flow.Condition, flow.Name, flow.ID,
	}, "\x00")
}

func (g Graph) pathSemanticKey(path []string) string {
	parts := make([]string, 0, len(path))
	for _, id := range path {
		parts = append(parts, graphElementKey(g.nodes[id]))
	}
	return strings.Join(parts, "\x01")
}

func pathKey(path []string) string { return strings.Join(path, "\x00") }

func canonicalCycle(cycle []string, g Graph) []string {
	if len(cycle) <= 2 {
		return cycle
	}
	body := cycle[:len(cycle)-1]
	best := 0
	for i := 1; i < len(body); i++ {
		if graphElementKey(g.nodes[body[i]]) < graphElementKey(g.nodes[body[best]]) {
			best = i
		}
	}
	normalized := append([]string(nil), body[best:]...)
	normalized = append(normalized, body[:best]...)
	return append(normalized, normalized[0])
}

func graphElementKey(element Element) string {
	called, message, errorRef := element.CalledElement != "", element.MessageRef != "", element.ErrorRef != ""
	element.CalledElement, element.MessageRef, element.ErrorRef = "", "", ""
	return strings.Join([]string{
		resolvedElementCoreKey(element, referenceIndexes{}),
		strconv.FormatBool(called), strconv.FormatBool(message), strconv.FormatBool(errorRef),
		element.ID,
	}, "\x00")
}
