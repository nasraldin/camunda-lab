package drift

import (
	"fmt"
	"strings"
)

// FormatText renders the stable three-way report without claiming sync for an
// incomplete or refused comparison.
func FormatText(report Report) string {
	var output strings.Builder
	fmt.Fprintf(&output, "Drift (env=%s, status=%s)\n", report.Env, report.Status)
	if report.Baseline.Ref != "" {
		fmt.Fprintf(&output, "Baseline: %s (%s)\n", report.Baseline.Ref, report.Baseline.Commit)
	}
	fmt.Fprintf(&output, "Working tree: dirty=%t\n\n", report.Dirty)
	if !report.Comparable {
		output.WriteString("Comparison UNKNOWN: all three inventories were not safely comparable.\n")
		for _, warning := range report.Warnings {
			fmt.Fprintf(&output, "- %s: %s\n", warning.Capability, warning.Message)
		}
		return output.String()
	}
	for _, entry := range report.Entries {
		fmt.Fprintf(&output, "%s\n", entry.Key)
		if entry.SourceDrift == "" && entry.DeploymentDrift == "" && entry.PendingDeployment == "" {
			if entry.GitDigest != "" {
				fmt.Fprintf(&output, "  git:     digest %s\n", entry.GitDigest)
			}
			if entry.ClusterDigest != "" || entry.ClusterVer != "" {
				fmt.Fprintf(&output, "  cluster: version %s digest %s\n",
					empty(entry.ClusterVer, "-"), empty(entry.ClusterDigest, "-"))
			}
			fmt.Fprintf(&output, "  status:  %s\n\n", entry.Status)
			continue
		}
		fmt.Fprintf(&output, "  source drift:       %s\n", entry.SourceDrift)
		fmt.Fprintf(&output, "  deployment drift:   %s\n", entry.DeploymentDrift)
		fmt.Fprintf(&output, "  pending deployment: %s\n", entry.PendingDeployment)
		if entry.Detail != "" {
			fmt.Fprintf(&output, "  detail: %s\n", entry.Detail)
		}
		output.WriteByte('\n')
	}
	fmt.Fprintf(&output, "Source: in-sync=%d drift=%d local-only=%d cluster-only=%d unknown=%d\n",
		report.Source.Counts.InSync, report.Source.Counts.Drift, report.Source.Counts.LocalOnly,
		report.Source.Counts.ClusterOnly, report.Source.Counts.Unknown)
	fmt.Fprintf(&output, "Deployment: in-sync=%d drift=%d local-only=%d cluster-only=%d unknown=%d\n",
		report.Deployment.Counts.InSync, report.Deployment.Counts.Drift, report.Deployment.Counts.LocalOnly,
		report.Deployment.Counts.ClusterOnly, report.Deployment.Counts.Unknown)
	fmt.Fprintf(&output, "Pending: in-sync=%d drift=%d local-only=%d cluster-only=%d unknown=%d\n",
		report.PendingDeployment.Counts.InSync, report.PendingDeployment.Counts.Drift,
		report.PendingDeployment.Counts.LocalOnly, report.PendingDeployment.Counts.ClusterOnly,
		report.PendingDeployment.Counts.Unknown)
	return output.String()
}
