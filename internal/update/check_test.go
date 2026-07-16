package update_test

import (
	"testing"

	"github.com/nasraldin/camunda-lab/internal/update"
)

func TestNewer(t *testing.T) {
	if !update.Newer("0.4.0", "v0.5.0") {
		t.Fatal("expected 0.5.0 newer than 0.4.0")
	}
	if update.Newer("v0.5.0", "0.4.0") {
		t.Fatal("expected not newer")
	}
	if update.Newer("0.4.0", "0.4.0") {
		t.Fatal("same version")
	}
}

func TestDetectChannel(t *testing.T) {
	if update.DetectChannel("0.0.0-dev", "/tmp/camunda") != update.ChannelDev {
		t.Fatal("dev")
	}
	if update.DetectChannel("0.4.0", "/opt/homebrew/Cellar/camunda-lab/0.4.0/bin/camunda") != update.ChannelHomebrew {
		t.Fatal("homebrew cellar")
	}
	if update.DetectChannel("0.4.0", "/Users/me/.local/bin/camunda") != update.ChannelRelease {
		t.Fatal("release")
	}
}
