package testgen

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"unicode"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// Artifact is one validated, in-memory generated file.
type Artifact struct {
	Path      string
	MediaType string
	Content   []byte
}

// Options controls rendering. Rendering never writes to the filesystem.
type Options struct {
	Lang string // java|js|python
}

// Render validates a normalized document and returns deterministic artifacts.
func Render(doc bpmn.Document, opts Options) ([]Artifact, error) {
	if opts.Lang == "" {
		opts.Lang = "java"
	}
	if len(doc.Processes) == 0 {
		return nil, fmt.Errorf("render tests: document has no processes")
	}
	artifacts := make([]Artifact, 0, len(doc.Processes))
	seen := make(map[string]string, len(doc.Processes))
	for _, process := range doc.Processes {
		if strings.TrimSpace(process.ID) == "" {
			return nil, fmt.Errorf("render tests: process ID is required")
		}
		var artifact Artifact
		var err error
		switch opts.Lang {
		case "java":
			artifact, err = renderJava(process)
		case "js":
			artifact, err = renderJS(process)
		case "python":
			artifact, err = renderPython(process)
		default:
			return nil, fmt.Errorf("unsupported lang %q (java|js|python)", opts.Lang)
		}
		if err != nil {
			return nil, fmt.Errorf("render process %q: %w", process.ID, err)
		}
		if err := validateRelativePath(artifact.Path); err != nil {
			return nil, fmt.Errorf("render process %q: %w", process.ID, err)
		}
		key := strings.ToLower(filepath.Clean(artifact.Path))
		if previous, exists := seen[key]; exists {
			return nil, fmt.Errorf("render tests: artifact path %q collides for processes %q and %q", artifact.Path, previous, process.ID)
		}
		seen[key] = process.ID
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) == 0 {
		return nil, fmt.Errorf("render tests: no artifacts generated")
	}
	sort.Slice(artifacts, func(i, j int) bool { return artifacts[i].Path < artifacts[j].Path })
	return artifacts, nil
}

func sanitizeIdent(s string) string {
	if s == "" {
		return "Process"
	}
	var b strings.Builder
	separator := false
	capitalize := true
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if b.Len() == 0 && unicode.IsDigit(r) {
				b.WriteByte('P')
			}
			if capitalize {
				r = unicode.ToUpper(r)
			}
			b.WriteRune(r)
			separator = false
			capitalize = false
			continue
		}
		if b.Len() > 0 && !separator {
			b.WriteByte('_')
			separator = true
			capitalize = true
		}
	}
	out := strings.TrimRight(b.String(), "_")
	if out == "" {
		return "Process"
	}
	return out
}

func uniqueJobTypes(process bpmn.Process) []string {
	seen := make(map[string]struct{})
	var jobs []string
	for _, element := range process.ServiceTasks() {
		jobType := strings.TrimSpace(element.JobType)
		if jobType == "" {
			jobType = strings.TrimSpace(element.ID)
		}
		if jobType == "" {
			continue
		}
		if _, exists := seen[jobType]; exists {
			continue
		}
		seen[jobType] = struct{}{}
		jobs = append(jobs, jobType)
	}
	sort.Strings(jobs)
	return jobs
}
