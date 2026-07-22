package scan

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

var (
	reJSONClient = regexp.MustCompile(`(?i)(?:^|[,{]\s*)"(client[_-]?secret)"\s*:\s*"([^"\r\n]{8,512})`)
	reJSONPass   = regexp.MustCompile(`(?i)(?:^|[,{]\s*)"(password|passwd)"\s*:\s*"([^"\r\n]{8,512})`)
	reJSONToken  = regexp.MustCompile(`(?i)(?:^|[,{]\s*)"(api[_-]?key|webhook[_-]?token)"\s*:\s*"([^"\r\n]{8,512})`)
	reJSONOAuth  = regexp.MustCompile(`(?i)(?:^|[,{]\s*)"(access[_-]?token|id[_-]?token|refresh[_-]?token|oauth[_-]?token)"\s*:\s*"([^"\r\n]{8,1000})`)
	reJSONBearer = regexp.MustCompile(`(?i)(?:^|[,{]\s*)"authorization"\s*:\s*"bearer\s+([A-Za-z0-9._~+/-]{8,1000})`)

	reYAMLClient = regexp.MustCompile(`(?i)^\s*['"]?(client[_-]?secret)['"]?\s*:\s*['"]?([^'"#\s][^'"#\r\n]{7,511})`)
	reYAMLPass   = regexp.MustCompile(`(?i)^\s*['"]?(password|passwd)['"]?\s*:\s*['"]?([^'"#\s][^'"#\r\n]{7,511})`)
	reYAMLToken  = regexp.MustCompile(`(?i)^\s*['"]?(api[_-]?key|webhook[_-]?token)['"]?\s*:\s*['"]?([^'"#\s][^'"#\r\n]{7,511})`)
	reYAMLOAuth  = regexp.MustCompile(`(?i)^\s*['"]?(access[_-]?token|id[_-]?token|refresh[_-]?token|oauth[_-]?token)['"]?\s*:\s*['"]?([^'"#\s][^'"#\r\n]{7,999})`)

	reAssignClient = regexp.MustCompile(`(?i)\b(client[_-]?secret)\b\s*=\s*['"]?([A-Za-z0-9_./+~$:{}@%-]{8,512})`)
	reAssignPass   = regexp.MustCompile(`(?i)\b(password|passwd)\b\s*=\s*['"]?([A-Za-z0-9_./+~$:{}@%-]{8,512})`)
	reAssignToken  = regexp.MustCompile(`(?i)\b(api[_-]?key|webhook[_-]?token)\b\s*=\s*['"]?([A-Za-z0-9_./+~$:{}@%-]{8,512})`)
	reAssignOAuth  = regexp.MustCompile(`(?i)\b(access[_-]?token|id[_-]?token|refresh[_-]?token|oauth[_-]?token)\b\s*=\s*['"]?([A-Za-z0-9._~+/-]{8,1000})`)
	reLineBearer   = regexp.MustCompile(`(?i)^\s*authorization\s*:\s*bearer\s+([A-Za-z0-9._~+/-]{8,1000})`)
	reCodeBearer   = regexp.MustCompile(`(?i)["']authorization["']\s*:\s*["']bearer\s+([A-Za-z0-9._~+/-]{8,1000})`)
	reXML          = regexp.MustCompile(`(?i)(client[_-]?secret|password|passwd|api[_-]?key|webhook[_-]?token|access[_-]?token|id[_-]?token|refresh[_-]?token|oauth[_-]?token)["']?\s+value\s*=\s*["']([^"'\r\n]{8,1000})`)

	reShellRuntime = regexp.MustCompile(`^(?:\$[A-Za-z_][A-Za-z0-9_]*|\$\{[A-Za-z_][A-Za-z0-9_]*\})$`)
	reJSRuntime    = regexp.MustCompile(`^(?:(?:(?:process|Bun)\.env|import\.meta\.env)(?:\.[A-Za-z_][A-Za-z0-9_]*|\[[^\]]+\])|Deno\.env\.get\s*\()`)
	reJavaRuntime  = regexp.MustCompile(`^System\.getenv\s*\(`)
	reGoRuntime    = regexp.MustCompile(`^os\.Getenv\s*\(`)
)

func matchLine(path string, kind SourceKind, line int, text string) []Finding {
	var out []Finding
	try := func(re *regexp.Regexp, rule string, severity Severity) {
		indices := re.FindStringSubmatchIndex(text)
		if indices == nil {
			return
		}
		start, end := indices[len(indices)-2], indices[len(indices)-1]
		value := cleanDetectedValue(text[start:end])
		if isPlaceholder(kind, text, value) {
			return
		}
		out = append(out, newFinding(rule, severity, path, kind, line, value))
	}
	tryBearer := func(re *regexp.Regexp) {
		match := re.FindStringSubmatch(text)
		if match == nil {
			return
		}
		value := cleanDetectedValue(match[len(match)-1])
		if isPlaceholder(kind, text, value) {
			return
		}
		rule, severity := "secret.token", SeverityMedium
		if looksJWT(value, text) {
			rule, severity = "secret.oauth", SeverityHigh
		}
		out = append(out, newFinding(rule, severity, path, kind, line, value))
	}

	switch kind {
	case SourceJSON, SourceForm:
		try(reJSONClient, "secret.client", SeverityHigh)
		try(reJSONPass, "secret.password", SeverityHigh)
		try(reJSONToken, "secret.token", SeverityMedium)
		tryOAuth(reJSONOAuth, path, kind, line, text, &out)
		tryBearer(reJSONBearer)
	case SourceYAML:
		try(reYAMLClient, "secret.client", SeverityHigh)
		try(reYAMLPass, "secret.password", SeverityHigh)
		try(reYAMLToken, "secret.token", SeverityMedium)
		tryOAuth(reYAMLOAuth, path, kind, line, text, &out)
		tryBearer(reLineBearer)
		if len(out) == 0 {
			try(reAssignClient, "secret.client", SeverityHigh)
			try(reAssignPass, "secret.password", SeverityHigh)
			try(reAssignToken, "secret.token", SeverityMedium)
			tryOAuth(reAssignOAuth, path, kind, line, text, &out)
		}
	case SourceBPMN, SourceDMN:
		if match := reXML.FindStringSubmatch(text); match != nil && !isPlaceholder(kind, text, match[2]) {
			rule, severity := xmlRule(match[1])
			out = append(out, newFinding(rule, severity, path, kind, line, match[2]))
		}
	case SourceJavaScript, SourceTypeScript, SourceJava:
		try(reAssignClient, "secret.client", SeverityHigh)
		try(reAssignPass, "secret.password", SeverityHigh)
		try(reAssignToken, "secret.token", SeverityMedium)
		tryOAuth(reAssignOAuth, path, kind, line, text, &out)
		try(reJSONClient, "secret.client", SeverityHigh)
		try(reJSONPass, "secret.password", SeverityHigh)
		try(reJSONToken, "secret.token", SeverityMedium)
		tryOAuth(reJSONOAuth, path, kind, line, text, &out)
		tryBearer(reCodeBearer)
	case SourceProperties, SourceText:
		try(reYAMLClient, "secret.client", SeverityHigh)
		try(reYAMLPass, "secret.password", SeverityHigh)
		try(reYAMLToken, "secret.token", SeverityMedium)
		tryOAuth(reYAMLOAuth, path, kind, line, text, &out)
		try(reAssignClient, "secret.client", SeverityHigh)
		try(reAssignPass, "secret.password", SeverityHigh)
		try(reAssignToken, "secret.token", SeverityMedium)
		tryOAuth(reAssignOAuth, path, kind, line, text, &out)
		tryBearer(reLineBearer)
	default:
		try(reAssignClient, "secret.client", SeverityHigh)
		try(reAssignPass, "secret.password", SeverityHigh)
		try(reAssignToken, "secret.token", SeverityMedium)
		tryOAuth(reAssignOAuth, path, kind, line, text, &out)
		tryBearer(reLineBearer)
	}
	if len(out) == 0 {
		if finding := highEntropyAssignment(path, kind, line, text); finding != nil {
			out = append(out, *finding)
		}
	}
	return out
}

func tryOAuth(
	re *regexp.Regexp,
	path string,
	kind SourceKind,
	line int,
	text string,
	out *[]Finding,
) {
	match := re.FindStringSubmatch(text)
	if match == nil {
		return
	}
	value := cleanDetectedValue(match[len(match)-1])
	key := strings.ToLower(match[1])
	if (!strings.Contains(key, "refresh") && len(value) < 1000 && !looksJWT(value, text)) ||
		isPlaceholder(kind, text, value) {
		return
	}
	*out = append(*out, newFinding("secret.oauth", SeverityHigh, path, kind, line, value))
}

func looksJWT(value, source string) bool {
	parts := strings.SplitN(value, ".", 3)
	if len(parts) == 3 && len(parts[0]) >= 8 && len(parts[1]) >= 8 && len(parts[2]) >= 16 {
		for _, part := range parts {
			for _, character := range part {
				if !unicode.IsLetter(character) && !unicode.IsDigit(character) &&
					character != '-' && character != '_' {
					return false
				}
			}
		}
		return true
	}
	return len(value) == 1000 && strings.Count(source, ".") >= 2
}

func xmlRule(key string) (string, Severity) {
	lower := strings.ToLower(key)
	switch {
	case strings.Contains(lower, "password"), strings.Contains(lower, "passwd"):
		return "secret.password", SeverityHigh
	case strings.Contains(lower, "api"), strings.Contains(lower, "webhook"):
		return "secret.token", SeverityMedium
	case strings.Contains(lower, "refresh"), strings.Contains(lower, "access"),
		strings.Contains(lower, "id_token"), strings.Contains(lower, "oauth"):
		return "secret.oauth", SeverityHigh
	default:
		return "secret.client", SeverityHigh
	}
}

func cleanDetectedValue(value string) string {
	return strings.Trim(strings.TrimSpace(value), `"' ,;`)
}

func newFinding(rule string, severity Severity, path string, kind SourceKind, line int, value string) Finding {
	return Finding{
		RuleID: rule, Rule: rule, Severity: severity, File: path, Line: line,
		SourceKind: kind, Snippet: mask(value),
	}
}

func highEntropyAssignment(path string, kind SourceKind, line int, text string) *Finding {
	parts := strings.SplitN(text, "=", 2)
	if len(parts) != 2 {
		return nil
	}
	key := strings.ToLower(strings.TrimSpace(parts[0]))
	if !strings.Contains(key, "secret") && !strings.Contains(key, "token") &&
		!strings.Contains(key, "password") && !strings.Contains(key, "key") {
		return nil
	}
	value := strings.Trim(strings.TrimSpace(parts[1]), `"'`)
	if len(value) < 20 || isPlaceholder(kind, text, value) || shannon(value) < 3.5 {
		return nil
	}
	finding := newFinding("secret.high-entropy", SeverityMedium, path, kind, line, value)
	return &finding
}

func inlineSuppression(kind SourceKind, text string) (string, bool) {
	switch kind {
	case SourceEnv, SourceShell, SourceYAML:
		if index := commentOutsideQuotes(text, "#"); index >= 0 &&
			hasSuppressionDirective(text[index+1:]) {
			return text[:index], true
		}
	case SourceJavaScript, SourceTypeScript, SourceJava:
		lineIndex := -1
		if index := firstIndex(commentsOutsideJSLiterals(text, "//")); index >= 0 &&
			hasSuppressionDirective(text[index+2:]) {
			lineIndex = index
		}
		blockIndex := directiveBlockCommentAt(text, "/*", "*/", commentsOutsideJSLiterals(text, "/*"))
		index := firstComment(lineIndex, blockIndex)
		if index >= 0 {
			return text[:index], true
		}
	case SourceBPMN, SourceDMN:
		if index := directiveBlockComment(text, "<!--", "-->"); index >= 0 {
			return text[:index], true
		}
	}
	return text, false
}

func directiveBlockComment(text, start, end string) int {
	return directiveBlockCommentAt(text, start, end, commentsOutsideQuotes(text, start))
}

func directiveBlockCommentAt(text, start, end string, indices []int) int {
	for _, index := range indices {
		closeIndex := strings.Index(text[index+len(start):], end)
		if closeIndex < 0 {
			return -1
		}
		closeIndex += index + len(start)
		if hasSuppressionDirective(text[index+len(start) : closeIndex]) {
			return index
		}
	}
	return -1
}

func firstIndex(indices []int) int {
	if len(indices) == 0 {
		return -1
	}
	return indices[0]
}

func commentOutsideQuotes(text, marker string) int {
	indices := commentsOutsideQuotes(text, marker)
	if len(indices) == 0 {
		return -1
	}
	return indices[0]
}

func commentsOutsideQuotes(text, marker string) []int {
	var indices []int
	var quote byte
	escaped := false
	for index := 0; index+len(marker) <= len(text); index++ {
		character := text[index]
		if escaped {
			escaped = false
			continue
		}
		if character == '\\' && quote != 0 {
			escaped = true
			continue
		}
		if character == '\'' || character == '"' {
			if quote == 0 {
				quote = character
			} else if quote == character {
				quote = 0
			}
			continue
		}
		if quote == 0 && strings.HasPrefix(text[index:], marker) {
			indices = append(indices, index)
			index += len(marker) - 1
		}
	}
	return indices
}

func commentsOutsideJSLiterals(text, marker string) []int {
	var (
		indices            []int
		quote              byte
		escaped            bool
		inTemplate         bool
		interpolationDepth int
	)
	for index := 0; index < len(text); index++ {
		character := text[index]
		if escaped {
			escaped = false
			continue
		}
		if quote != 0 {
			if character == '\\' {
				escaped = true
			} else if character == quote {
				quote = 0
			}
			continue
		}
		if inTemplate {
			if interpolationDepth == 0 {
				switch {
				case character == '\\':
					escaped = true
				case character == '`':
					inTemplate = false
				case character == '$' && index+1 < len(text) && text[index+1] == '{':
					interpolationDepth = 1
					index++
				}
				continue
			}
			switch character {
			case '\'', '"', '`':
				quote = character
			case '{':
				interpolationDepth++
			case '}':
				interpolationDepth--
			}
			continue
		}
		switch character {
		case '\'', '"':
			quote = character
			continue
		case '`':
			inTemplate = true
			continue
		}
		if strings.HasPrefix(text[index:], marker) {
			indices = append(indices, index)
			index += len(marker) - 1
		}
	}
	return indices
}

func hasSuppressionDirective(comment string) bool {
	for _, field := range strings.FieldsFunc(comment, func(character rune) bool {
		return !(unicode.IsLetter(character) || unicode.IsDigit(character) || character == '-')
	}) {
		if field == "camunda-scan-ignore" {
			return true
		}
	}
	return false
}

func firstComment(left, right int) int {
	switch {
	case left < 0:
		return right
	case right < 0:
		return left
	case left < right:
		return left
	default:
		return right
	}
}

func isPlaceholder(kind SourceKind, text, value string) bool {
	if looksPlaceholder(value) {
		return true
	}
	rhs := assignmentRHS(text)
	if rhs == "" {
		return false
	}
	if strings.HasPrefix(rhs, "'") {
		return false
	}
	if kind == SourceEnv || kind == SourceShell {
		rhs = strings.Trim(rhs, `"`)
		return reShellRuntime.MatchString(rhs)
	}
	if strings.HasPrefix(rhs, `"`) || strings.HasPrefix(rhs, "`") {
		return false
	}
	switch kind {
	case SourceJavaScript, SourceTypeScript:
		return reJSRuntime.MatchString(rhs)
	case SourceJava:
		return reJavaRuntime.MatchString(rhs)
	case SourceGo:
		return reGoRuntime.MatchString(rhs)
	default:
		return false
	}
}

func assignmentRHS(text string) string {
	equals := strings.IndexByte(text, '=')
	colon := strings.IndexByte(text, ':')
	if colon >= 0 && colon+1 < len(text) && text[colon+1] == '=' {
		colon = -1
	}
	index := equals
	if index < 0 || (colon >= 0 && colon < index) {
		index = colon
	}
	if index < 0 || index+1 >= len(text) {
		return ""
	}
	return strings.TrimSpace(strings.TrimRight(text[index+1:], " \t,;"))
}

func looksPlaceholder(value string) bool {
	lower := strings.ToLower(value)
	for _, placeholder := range []string{"changeme", "example", "xxx", "your-", "todo", "{{"} {
		if strings.Contains(lower, placeholder) {
			return true
		}
	}
	return false
}

func shannon(value string) float64 {
	if value == "" {
		return 0
	}
	frequencies := map[rune]float64{}
	count := 0
	for _, character := range value {
		if unicode.IsSpace(character) {
			continue
		}
		frequencies[character]++
		count++
	}
	if count == 0 {
		return 0
	}
	var entropy float64
	total := float64(count)
	for _, frequency := range frequencies {
		probability := frequency / total
		entropy -= probability * math.Log2(probability)
	}
	return entropy
}

func mask(value string) string {
	if len(value) <= 8 {
		return "****"
	}
	return value[:4] + "…" + value[len(value)-4:]
}
