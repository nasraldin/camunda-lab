package toolkit

import (
	"path/filepath"
	"strings"
	"unicode"

	"github.com/nasraldin/camunda-lab/internal/testgen"
)

// PackArtifactsZIP builds a deterministic ZIP of toolkit artifacts for browser download.
func PackArtifactsZIP(artifacts []Artifact) ([]byte, error) {
	converted := make([]testgen.Artifact, len(artifacts))
	for index, artifact := range artifacts {
		converted[index] = testgen.Artifact{
			Path: artifact.Path, MediaType: artifact.MediaType, Content: artifact.Content,
		}
	}
	return testgen.PackZIP(converted)
}

// SanitizeAttachmentFilename returns a single-segment Content-Disposition filename.
// Path separators, quotes, and control characters are stripped.
func SanitizeAttachmentFilename(name string) string {
	name = strings.TrimSpace(name)
	name = strings.ReplaceAll(name, `\`, `/`)
	name = filepath.Base(filepath.FromSlash(name))
	var b strings.Builder
	for _, r := range name {
		switch {
		case r < 0x20 || r == 0x7f:
			continue
		case r == '"' || r == '\'' || r == ';' || r == '\\' || r == '/':
			continue
		case unicode.IsSpace(r):
			b.WriteByte('-')
		default:
			b.WriteRune(r)
		}
	}
	cleaned := strings.Trim(b.String(), ".-")
	if cleaned == "" || cleaned == "." || cleaned == ".." {
		return "download"
	}
	return cleaned
}
