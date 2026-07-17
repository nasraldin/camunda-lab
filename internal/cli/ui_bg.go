package cli

import (
	"fmt"
	"os"

	"github.com/nasraldin/camunda-lab/internal/ui"
)

func ensureUIBackground(open bool) {
	opts := ui.EnsureOpts{
		Options: ui.DefaultOptions(),
		Open:    open,
	}
	opts.Version = appVersion
	if err := ui.EnsureBackground(opts); err != nil {
		fmt.Fprintf(os.Stderr, "Note: could not start Lab UI in background: %v\n", err)
		fmt.Fprintf(os.Stderr, "Start manually: camunda ui\n")
	}
}
