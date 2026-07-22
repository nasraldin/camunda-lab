package explain

import (
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

type scopeSummary struct {
	name       string
	id         string
	happy      []string
	alternates [][]string
	cycles     [][]string
	deadEnds   []string
}

func summarizeScopes(process bpmn.Process) []scopeSummary {
	scopes := []scopeSummary{summarizeScope(process, "", "", "")}
	for _, element := range process.Elements {
		if element.Type != "subProcess" {
			continue
		}
		scopes = append(scopes, summarizeScope(process, element.ID, element.Name, element.ID))
	}
	return scopes
}

func summarizeScope(process bpmn.Process, parentID, name, id string) scopeSummary {
	graph := bpmn.NewGraph(process)
	if parentID != "" {
		graph = bpmn.NewGraphForScope(process, parentID)
	}
	return scopeSummary{
		name: name, id: id, happy: graph.HappyPath(), alternates: graph.AlternatePaths(),
		cycles: graph.Cycles(), deadEnds: graph.DeadEnds(),
	}
}

func formatPath(process bpmn.Process, path []string) string {
	parts := make([]string, 0, len(path))
	for _, id := range path {
		element := process.ElementByID(id)
		if element == nil {
			parts = append(parts, id)
			continue
		}
		parts = append(parts, fmt.Sprintf("%s [%s:%s]", displayName(element.Name, element.ID), element.ID, element.Type))
	}
	return strings.Join(parts, " → ")
}

func displayName(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
