package drift

import (
	"fmt"
	"strings"

	"github.com/nasraldin/camunda-lab/internal/plan"
)

// Entry is per-resource drift status.
type Entry struct {
	Key           string
	GitDigest     string
	ClusterDigest string
	ClusterVer    string
	Status        string // IN_SYNC|DRIFT|LOCAL_ONLY|CLUSTER_ONLY
}

// Report summarizes drift.
type Report struct {
	Env     string
	Entries []Entry
}

// Compare builds a drift report from inventories.
func Compare(env string, local, remote []plan.Resource) Report {
	lm := map[string]plan.Resource{}
	rm := map[string]plan.Resource{}
	for _, r := range local {
		lm[r.Key] = r
	}
	for _, r := range remote {
		rm[r.Key] = r
	}
	var entries []Entry
	seen := map[string]bool{}
	for k, l := range lm {
		seen[k] = true
		r, ok := rm[k]
		e := Entry{Key: k, GitDigest: l.Digest}
		if !ok {
			e.Status = "LOCAL_ONLY"
		} else {
			e.ClusterDigest = r.Digest
			e.ClusterVer = r.Version
			if l.Digest == r.Digest || (r.Digest == "" && r.Version == "") {
				e.Status = "IN_SYNC"
			} else {
				e.Status = "DRIFT"
			}
		}
		entries = append(entries, e)
	}
	for k, r := range rm {
		if seen[k] {
			continue
		}
		entries = append(entries, Entry{
			Key:           k,
			ClusterDigest: r.Digest,
			ClusterVer:    r.Version,
			Status:        "CLUSTER_ONLY",
		})
	}
	return Report{Env: env, Entries: entries}
}

// HasDrift is true when any entry is not IN_SYNC.
func HasDrift(r Report) bool {
	for _, e := range r.Entries {
		if e.Status != "IN_SYNC" {
			return true
		}
	}
	return false
}

// FormatText renders drift report.
func FormatText(r Report) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Drift (env=%s)\n\n", r.Env)
	for _, e := range r.Entries {
		fmt.Fprintf(&b, "%s\n", e.Key)
		if e.GitDigest != "" {
			fmt.Fprintf(&b, "  git:     digest %s\n", e.GitDigest)
		}
		if e.ClusterDigest != "" || e.ClusterVer != "" {
			fmt.Fprintf(&b, "  cluster: version %s digest %s\n", empty(e.ClusterVer, "-"), empty(e.ClusterDigest, "-"))
		}
		fmt.Fprintf(&b, "  status:  %s\n\n", e.Status)
	}
	return b.String()
}

func empty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
