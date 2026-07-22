package scan

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/sys/unix"
)

type ignoreRule struct {
	pattern  string
	source   string
	base     string
	negated  bool
	dirOnly  bool
	anchored bool
	hasSlash bool
	matcher  *regexp.Regexp
}

type ignoreRules []ignoreRule

type ignoreState struct {
	git     ignoreRules
	camunda ignoreRules
	user    ignoreRules
}

func readIgnoreAt(
	ctx context.Context,
	filesystem *secureFS,
	directory *secureDir,
	name, source, base string,
	afterLine func(string, int) error,
) (ignoreRules, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	opened, err := filesystem.openFile(directory, name)
	if errors.Is(err, unix.ENOENT) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	defer opened.file.Close()

	var lines []string
	scanner := bufio.NewScanner(opened.file)
	scanner.Buffer(make([]byte, 0, 16*1024), 256*1024)
	for scanner.Scan() {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		lines = append(lines, scanner.Text())
		if afterLine != nil {
			if err := afterLine(source, len(lines)); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", source, err)
	}
	if err := filesystem.verify(); err != nil {
		return nil, fmt.Errorf("read %s: anchored path changed: %w", source, err)
	}
	if err := verifySecureFile(directory, name, opened); err != nil {
		return nil, fmt.Errorf("read %s: path changed: %w", source, err)
	}
	return parseIgnoreLinesContext(ctx, lines, source, base)
}

func parseIgnoreLines(lines []string, source, base string) (ignoreRules, error) {
	return parseIgnoreLinesContext(context.Background(), lines, source, base)
}

func parseIgnoreLinesContext(
	ctx context.Context,
	lines []string,
	source, base string,
) (ignoreRules, error) {
	rules := make(ignoreRules, 0, len(lines))
	for number, raw := range lines {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rule, err := compileIgnoreRule(line, source, base)
		if err != nil {
			return nil, fmt.Errorf("unsafe ignore pattern in %s:%d: %w", source, number+1, err)
		}
		rules = append(rules, rule)
	}
	return rules, nil
}

func compileIgnoreRule(value, source, base string) (ignoreRule, error) {
	rule := ignoreRule{source: source, base: base}
	if strings.HasPrefix(value, "!") {
		rule.negated = true
		value = strings.TrimPrefix(value, "!")
	}
	if value == "" || strings.ContainsAny(value, "\x00\r\n\\") {
		return ignoreRule{}, fmt.Errorf("pattern is empty or contains an unsafe character")
	}
	if strings.HasPrefix(value, "//") || filepath.VolumeName(value) != "" {
		return ignoreRule{}, fmt.Errorf("absolute patterns are not allowed")
	}
	rule.anchored = strings.HasPrefix(value, "/")
	value = strings.TrimPrefix(value, "/")
	rule.dirOnly = strings.HasSuffix(value, "/")
	value = strings.TrimSuffix(value, "/")
	clean := path.Clean(value)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") {
		return ignoreRule{}, fmt.Errorf("pattern escapes the project")
	}
	for _, component := range strings.Split(clean, "/") {
		if component == ".." {
			return ignoreRule{}, fmt.Errorf("pattern escapes the project")
		}
	}
	matcher, err := compileGlob(clean)
	if err != nil {
		return ignoreRule{}, err
	}
	rule.pattern = clean
	rule.hasSlash = strings.Contains(clean, "/")
	rule.matcher = matcher
	return rule, nil
}

func compileGlob(pattern string) (*regexp.Regexp, error) {
	// path.Match provides strict validation for bracket expressions.
	if _, err := path.Match(pattern, "validation"); err != nil {
		return nil, fmt.Errorf("invalid glob: %w", err)
	}
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(pattern); {
		switch pattern[index] {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				if index+2 < len(pattern) && pattern[index+2] == '/' {
					expression.WriteString("(?:.*/)?")
					index += 3
				} else {
					expression.WriteString(".*")
					index += 2
				}
			} else {
				expression.WriteString("[^/]*")
				index++
			}
		case '?':
			expression.WriteString("[^/]")
			index++
		case '[':
			end := index + 1
			for end < len(pattern) && pattern[end] != ']' {
				end++
			}
			class := pattern[index : end+1]
			if strings.HasPrefix(class, "[!") {
				class = "[^" + class[2:]
			}
			expression.WriteString(class)
			index = end + 1
		default:
			expression.WriteString(regexp.QuoteMeta(string(pattern[index])))
			index++
		}
	}
	expression.WriteString("$")
	return regexp.Compile(expression.String())
}

func (state ignoreState) match(relative string) (bool, string) {
	ignored, reason := false, ""
	for _, rules := range []ignoreRules{state.git, state.camunda, state.user} {
		for _, rule := range rules {
			if !rule.matches(relative) {
				continue
			}
			ignored = !rule.negated
			if ignored {
				reason = rule.source + ":" + rule.pattern
			} else {
				reason = ""
			}
		}
	}
	return ignored, reason
}

func (rule ignoreRule) matches(relative string) bool {
	local := relative
	if rule.base != "" {
		prefix := strings.TrimSuffix(rule.base, "/") + "/"
		if !strings.HasPrefix(relative, prefix) {
			return false
		}
		local = strings.TrimPrefix(relative, prefix)
	}
	parts := strings.Split(local, "/")
	last := len(parts)
	if rule.dirOnly {
		last--
	}
	for end := 1; end <= last; end++ {
		if rule.matchesPath(strings.Join(parts[:end], "/")) {
			return true
		}
	}
	return false
}

func (rule ignoreRule) matchesPath(candidate string) bool {
	if rule.anchored || rule.hasSlash {
		return rule.matcher.MatchString(candidate)
	}
	for _, component := range strings.Split(candidate, "/") {
		if rule.matcher.MatchString(component) {
			return true
		}
	}
	return false
}
