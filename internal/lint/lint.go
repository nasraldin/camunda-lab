package lint

import (
	"sort"

	"github.com/nasraldin/camunda-lab/internal/bpmn"
)

// Run evaluates the registered rules once against the complete document.
func Run(document bpmn.Document, options Options) Result {
	ignored := make(map[string]bool, len(options.Ignore))
	for _, id := range options.Ignore {
		ignored[id] = true
	}

	findings := make([]Finding, 0)
	for _, rule := range Rules() {
		if ignored[rule.ID()] {
			continue
		}
		for _, finding := range rule.Check(document) {
			finding.File = options.File
			findings = append(findings, finding)
		}
	}
	sort.SliceStable(findings, func(i, j int) bool {
		return findingKey(findings[i]) < findingKey(findings[j])
	})
	return Result{
		Failed:   ShouldFail(findings, options.FailOn),
		Findings: findings,
	}
}

func findingKey(finding Finding) string {
	return finding.File + "\x00" + finding.ProcessID + "\x00" + finding.Rule + "\x00" +
		finding.Element + "\x00" + string(finding.Severity) + "\x00" + finding.Message
}

// ShouldFail reports whether findings meet or exceed the configured threshold.
func ShouldFail(findings []Finding, failOn string) bool {
	if failOn == "" {
		failOn = string(SeverityError)
	}
	for _, finding := range findings {
		if finding.Severity == SeverityError ||
			(failOn == string(SeverityWarning) && finding.Severity == SeverityWarning) {
			return true
		}
	}
	return false
}
