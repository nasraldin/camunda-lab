package drift

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	pathpkg "path"
	"sort"
	"strconv"
	"strings"
)

const (
	maxAttributeFileBytes       = 1 << 20
	maxBaselineAttributeFiles   = 1024
	maxBaselineAttributeBytes   = 4 << 20
	maxCheckAttrPaths           = 10_000
	maxCheckAttrInputBytes      = 4 << 20
	maxCheckAttrBatchPaths      = 256
	maxCheckAttrBatchInputBytes = 64 << 10
)

type attributeFile struct {
	Path string
	Data []byte
}

type attributeBudget struct {
	files    int
	bytes    int
	maxFiles int
	maxBytes int
}

func (b *attributeBudget) reserveFiles(count int) error {
	if count < 0 || count > b.maxFiles {
		return fmt.Errorf("baseline attribute file count %d exceeds limit %d", count, b.maxFiles)
	}
	b.files = count
	return nil
}

func (b *attributeBudget) addBytes(count int) error {
	if count < 0 || count > b.maxBytes-b.bytes {
		return fmt.Errorf("baseline attribute aggregate bytes exceed limit %d", b.maxBytes)
	}
	b.bytes += count
	return nil
}

func baselineAttributeFiles(
	ctx context.Context,
	runner GitRunner,
	repository string,
	commit string,
	projectPrefix string,
	scoped []treeEntry,
) ([]attributeFile, error) {
	byPath := make(map[string]treeEntry)
	for _, entry := range scoped {
		if pathpkg.Base(entry.Path) == ".gitattributes" && entry.Type == "blob" {
			byPath[entry.Path] = entry
		}
	}
	for _, candidate := range ancestorAttributePaths(projectPrefix) {
		if _, found := byPath[candidate]; found {
			continue
		}
		output, err := runSafeGit(ctx, runner, repository,
			"ls-tree", "-z", "--full-tree", commit, "--", candidate)
		if err != nil {
			return nil, fmt.Errorf("inspect baseline attributes %s: %w", candidate, err)
		}
		entries, err := parseTreeEntries(output)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if entry.Path == candidate && entry.Type == "blob" {
				byPath[candidate] = entry
			}
		}
	}
	paths := make([]string, 0, len(byPath))
	for path := range byPath {
		paths = append(paths, path)
	}
	sortAttributePaths(paths)
	budget := attributeBudget{
		maxFiles: maxBaselineAttributeFiles,
		maxBytes: maxBaselineAttributeBytes,
	}
	if err := budget.reserveFiles(len(paths)); err != nil {
		return nil, reportError("baseline Git attributes exceed the file-count budget", err)
	}
	sizes := make(map[string]int, len(paths))
	for _, path := range paths {
		sizeOutput, err := runSafeGit(ctx, runner, repository, "cat-file", "-s", byPath[path].Object)
		if err != nil {
			return nil, fmt.Errorf("inspect baseline attributes %s size: %w", path, err)
		}
		sizeText := strings.TrimSpace(string(sizeOutput))
		size, err := strconv.Atoi(sizeText)
		if err != nil || size < 0 || strconv.Itoa(size) != sizeText {
			return nil, fmt.Errorf("baseline attributes %s returned an invalid blob size", path)
		}
		if size > maxAttributeFileBytes {
			return nil, fmt.Errorf("baseline attributes %s exceed size limit", path)
		}
		if err := budget.addBytes(size); err != nil {
			return nil, reportError("baseline Git attributes exceed the aggregate byte budget", err)
		}
		sizes[path] = size
	}
	files := make([]attributeFile, 0, len(paths))
	for _, path := range paths {
		data, err := runSafeGit(ctx, runner, repository, "cat-file", "blob", byPath[path].Object)
		if err != nil {
			return nil, fmt.Errorf("read baseline attributes %s: %w", path, err)
		}
		if len(data) != sizes[path] {
			return nil, fmt.Errorf("baseline attributes %s size changed during read", path)
		}
		files = append(files, attributeFile{Path: path, Data: data})
	}
	return files, nil
}

func inspectFilterRisk(files []attributeFile, assets []string) error {
	active := make(map[string]string, len(assets))
	for _, file := range files {
		lines := strings.Split(string(file.Data), "\n")
		for lineNumber, raw := range lines {
			line := strings.TrimSpace(raw)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			fields := strings.Fields(line)
			if len(fields) < 2 {
				continue
			}
			state, filterName, hasFilter := parseFilterAttribute(fields[1:])
			if !hasFilter {
				continue
			}
			for _, asset := range assets {
				matches, certain := attributePatternMatches(file.Path, fields[0], asset)
				if !certain {
					detail := fmt.Errorf("cannot safely evaluate filter attribute in %s:%d", file.Path, lineNumber+1)
					return reportError("configured git filter pattern cannot be evaluated safely", detail)
				}
				if !matches {
					continue
				}
				if state {
					active[asset] = filterName
				} else {
					delete(active, asset)
				}
			}
		}
	}
	if len(active) == 0 {
		return nil
	}
	paths := make([]string, 0, len(active))
	for path := range active {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	detail := fmt.Errorf("configured asset %s has Git filter %q", paths[0], active[paths[0]])
	return reportError("configured git filter prevents canonical comparison", detail)
}

func inspectEffectiveWorkingFilter(
	ctx context.Context,
	runner GitRunner,
	repository string,
	assets []string,
) error {
	assets = sortedUniqueStrings(assets)
	if len(assets) > maxCheckAttrPaths {
		detail := fmt.Errorf("configured asset count %d exceeds check-attr limit %d", len(assets), maxCheckAttrPaths)
		return reportError("effective Git filter attributes could not be inspected safely", detail)
	}
	totalInputBytes := 0
	for _, asset := range assets {
		if len(asset)+1 > maxCheckAttrInputBytes-totalInputBytes {
			detail := errors.New("configured asset paths exceed aggregate check-attr input limit")
			return reportError("effective Git filter attributes could not be inspected safely", detail)
		}
		totalInputBytes += len(asset) + 1
	}
	for start := 0; start < len(assets); {
		end := start
		inputSize := 0
		for end < len(assets) && end-start < maxCheckAttrBatchPaths {
			nextSize := len(assets[end]) + 1
			if nextSize > maxCheckAttrBatchInputBytes {
				detail := fmt.Errorf("configured asset path exceeds check-attr input bound")
				return reportError("effective Git filter attributes could not be inspected safely", detail)
			}
			if end > start && inputSize+nextSize > maxCheckAttrBatchInputBytes {
				break
			}
			inputSize += nextSize
			end++
		}
		var input bytes.Buffer
		for _, asset := range assets[start:end] {
			if strings.IndexByte(asset, 0) >= 0 {
				detail := fmt.Errorf("configured asset path contains NUL")
				return reportError("effective Git filter attributes could not be inspected safely", detail)
			}
			input.WriteString(asset)
			input.WriteByte(0)
		}
		output, err := runSafeGitInput(
			ctx, runner, repository, input.Bytes(),
			"check-attr", "-z", "--stdin", "filter",
		)
		if err != nil {
			return reportError("effective Git filter attributes could not be inspected safely", err)
		}
		values, err := parseCheckAttrOutput(output, assets[start:end])
		if err != nil {
			return reportError("effective Git filter attributes could not be inspected safely", err)
		}
		for path, value := range values {
			if value != "unspecified" && value != "unset" {
				detail := fmt.Errorf("configured asset %s has effective Git filter %q", path, value)
				return reportError("configured git filter prevents canonical comparison", detail)
			}
		}
		start = end
	}
	return nil
}

func parseCheckAttrOutput(output []byte, expected []string) (map[string]string, error) {
	if len(output) == 0 && len(expected) == 0 {
		return map[string]string{}, nil
	}
	if len(output) == 0 || output[len(output)-1] != 0 {
		return nil, errors.New("Git check-attr returned an unterminated record")
	}
	fields := bytes.Split(output[:len(output)-1], []byte{0})
	if len(fields) != len(expected)*3 {
		return nil, errors.New("Git check-attr returned an unexpected record count")
	}
	result := make(map[string]string, len(expected))
	for index, path := range expected {
		offset := index * 3
		if string(fields[offset]) != path || string(fields[offset+1]) != "filter" {
			return nil, errors.New("Git check-attr returned mismatched path or attribute data")
		}
		value := string(fields[offset+2])
		if value == "" || strings.IndexByte(value, 0) >= 0 {
			return nil, errors.New("Git check-attr returned an invalid attribute value")
		}
		result[path] = value
	}
	return result, nil
}

func sortedUniqueStrings(values []string) []string {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		set[value] = struct{}{}
	}
	result := make([]string, 0, len(set))
	for value := range set {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func parseFilterAttribute(fields []string) (active bool, name string, found bool) {
	for _, field := range fields {
		switch {
		case field == "filter":
			active, name, found = true, "set", true
		case strings.HasPrefix(field, "filter=") && len(field) > len("filter="):
			active, name, found = true, strings.TrimPrefix(field, "filter="), true
		case field == "-filter" || field == "!filter":
			active, name, found = false, "", true
		}
	}
	return active, name, found
}

func attributePatternMatches(attributePath, pattern, asset string) (bool, bool) {
	directory := pathpkg.Dir(attributePath)
	if directory == "." {
		directory = ""
	}
	relative := asset
	if directory != "" {
		if !pathWithinGitRoot(asset, directory) || asset == directory {
			return false, true
		}
		relative = strings.TrimPrefix(asset, directory+"/")
	}
	if strings.ContainsAny(pattern, `\]"'`) || strings.HasPrefix(pattern, "!") {
		return false, false
	}
	pattern = strings.TrimPrefix(pattern, "/")
	if strings.HasSuffix(pattern, "/") {
		return false, true
	}
	if !strings.Contains(pattern, "/") {
		matches, err := pathpkg.Match(pattern, pathpkg.Base(relative))
		return matches, err == nil
	}
	if strings.Contains(pattern, "**") {
		return doublestarMatch(pattern, relative), true
	}
	matches, err := pathpkg.Match(pattern, relative)
	return matches, err == nil
}

func doublestarMatch(pattern, value string) bool {
	patternParts := strings.Split(pattern, "/")
	valueParts := strings.Split(value, "/")
	var match func(int, int) bool
	match = func(patternIndex, valueIndex int) bool {
		if patternIndex == len(patternParts) {
			return valueIndex == len(valueParts)
		}
		if patternParts[patternIndex] == "**" {
			for next := valueIndex; next <= len(valueParts); next++ {
				if match(patternIndex+1, next) {
					return true
				}
			}
			return false
		}
		if valueIndex == len(valueParts) {
			return false
		}
		matched, err := pathpkg.Match(patternParts[patternIndex], valueParts[valueIndex])
		return err == nil && matched && match(patternIndex+1, valueIndex+1)
	}
	return match(0, 0)
}

func ancestorAttributePaths(projectPrefix string) []string {
	paths := []string{".gitattributes"}
	if projectPrefix == "" {
		return paths
	}
	current := ""
	for _, component := range strings.Split(projectPrefix, "/") {
		current = joinGitPath(current, component)
		paths = append(paths, joinGitPath(current, ".gitattributes"))
	}
	return paths
}

func sortAttributePaths(paths []string) {
	sort.SliceStable(paths, func(i, j int) bool {
		leftDepth := strings.Count(paths[i], "/")
		rightDepth := strings.Count(paths[j], "/")
		if leftDepth != rightDepth {
			return leftDepth < rightDepth
		}
		return paths[i] < paths[j]
	})
}

func isLFSPointer(data []byte) bool {
	return bytes.HasPrefix(data, []byte("version https://git-lfs.github.com/spec/v1\n"))
}
