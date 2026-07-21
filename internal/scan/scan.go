package scan

import (
	"bufio"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

// Finding is a potential secret.
type Finding struct {
	Rule     string `json:"rule"`
	Severity string `json:"severity"` // low|medium|high
	File     string `json:"file"`
	Line     int    `json:"line"`
	Snippet  string `json:"snippet"`
}

var (
	reClient = regexp.MustCompile(`(?i)(client[_-]?secret|clientSecret)\s*[=:]\s*['"]?([^\s'"]+)`)
	rePass   = regexp.MustCompile(`(?i)(password|passwd)\s*[=:]\s*['"]?([^\s'"]+)`)
	reToken  = regexp.MustCompile(`(?i)(api[_-]?key|apiKey|webhook[_-]?token|bearer)\s*[=:]\s*['"]?([^\s'"]+)`)
	reOAuth  = regexp.MustCompile(`(?i)(refresh[_-]?token)\s*[=:]\s*['"]?([^\s'"]+)`)
	skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, ".camunda-lab": true}
	scanExts = map[string]bool{
		".bpmn": true, ".dmn": true, ".yaml": true, ".yml": true, ".json": true,
		".env": true, ".sh": true, ".js": true, ".ts": true, ".java": true,
		".properties": true, ".form": true, ".txt": true,
	}
)

// Options configures a scan.
type Options struct {
	Root   string
	FailOn string // low|medium|high — default medium
}

// Issue records an input that could not be inspected.
type Issue struct {
	Path string
	Err  error
}

// Result includes findings and honest scan accounting.
type Result struct {
	Findings []Finding
	Issues   []Issue
}

// Walk scans a directory tree.
func Walk(opts Options) ([]Finding, error) {
	result, err := WalkWithReport(opts)
	return result.Findings, err
}

// WalkWithReport scans a tree and reports recoverable per-path failures.
func WalkWithReport(opts Options) (Result, error) {
	root := opts.Root
	if root == "" {
		root = "."
	}
	var result Result
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			if path == root {
				return err
			}
			result.Issues = append(result.Issues, Issue{Path: path, Err: err})
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		base := d.Name()
		if !scanExts[ext] && !strings.HasPrefix(base, ".env") {
			return nil
		}
		fs, err := scanFile(path)
		if err != nil {
			result.Issues = append(result.Issues, Issue{Path: path, Err: err})
			return nil
		}
		result.Findings = append(result.Findings, fs...)
		return nil
	})
	return result, err
}

func scanFile(path string) ([]Finding, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []Finding
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	line := 0
	for sc.Scan() {
		line++
		text := sc.Text()
		if strings.Contains(text, "camunda-scan-ignore") {
			continue
		}
		out = append(out, matchLine(path, line, text)...)
	}
	return out, sc.Err()
}

func matchLine(path string, line int, text string) []Finding {
	var out []Finding
	try := func(re *regexp.Regexp, rule, sev string) {
		m := re.FindStringSubmatch(text)
		if m == nil {
			return
		}
		val := m[len(m)-1]
		if looksPlaceholder(val) {
			return
		}
		out = append(out, Finding{
			Rule:     rule,
			Severity: sev,
			File:     path,
			Line:     line,
			Snippet:  mask(val),
		})
	}
	try(reClient, "secret.client", "high")
	try(rePass, "secret.password", "high")
	try(reToken, "secret.token", "medium")
	try(reOAuth, "secret.oauth", "high")
	if len(out) == 0 {
		if f := highEntropyAssignment(path, line, text); f != nil {
			out = append(out, *f)
		}
	}
	return out
}

func highEntropyAssignment(path string, line int, text string) *Finding {
	parts := strings.SplitN(text, "=", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.Contains(key, "secret") && !strings.Contains(key, "token") &&
		!strings.Contains(key, "password") && !strings.Contains(key, "key") {
		return nil
	}
	val := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	if len(val) < 20 || looksPlaceholder(val) {
		return nil
	}
	if shannon(val) < 3.5 {
		return nil
	}
	return &Finding{Rule: "secret.high-entropy", Severity: "medium", File: path, Line: line, Snippet: mask(val)}
}

func looksPlaceholder(v string) bool {
	l := strings.ToLower(v)
	for _, p := range []string{"changeme", "example", "xxx", "your-", "todo", "${", "{{"} {
		if strings.Contains(l, p) {
			return true
		}
	}
	return false
}

func shannon(s string) float64 {
	if s == "" {
		return 0
	}
	freq := map[rune]float64{}
	n := 0
	for _, r := range s {
		if unicode.IsSpace(r) {
			continue
		}
		freq[r]++
		n++
	}
	if n == 0 {
		return 0
	}
	var h float64
	nf := float64(n)
	for _, c := range freq {
		p := c / nf
		h -= p * math.Log2(p)
	}
	return h
}

func mask(v string) string {
	if len(v) <= 8 {
		return "****"
	}
	return v[:4] + "…" + v[len(v)-4:]
}

// FormatText renders findings with masked snippets.
func FormatText(fs []Finding) string {
	if len(fs) == 0 {
		return "No secrets found.\n"
	}
	var b strings.Builder
	for _, f := range fs {
		fmt.Fprintf(&b, "%s %-6s %s:%d  %s\n", f.Rule, f.Severity, f.File, f.Line, f.Snippet)
	}
	return b.String()
}

// ShouldFail reports CI failure for findings at/above failOn.
func ShouldFail(fs []Finding, failOn string) bool {
	rank := map[string]int{"low": 1, "medium": 2, "high": 3}
	min := rank[failOn]
	if min == 0 {
		min = 2
	}
	for _, f := range fs {
		if rank[f.Severity] >= min {
			return true
		}
	}
	return false
}
