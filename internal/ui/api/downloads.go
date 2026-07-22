package api

import (
	"fmt"
	"net/http"

	"github.com/nasraldin/camunda-lab/internal/toolkit"
)

func writeAttachment(w http.ResponseWriter, contentType, filename string) {
	safe := toolkit.SanitizeAttachmentFilename(filename)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, safe))
	w.Header().Set("X-Content-Type-Options", "nosniff")
}
