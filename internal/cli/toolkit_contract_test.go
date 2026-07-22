package cli

import (
	"strings"
	"testing"
)

func TestToolkitContractInventory(t *testing.T) {
	tests := []struct {
		path  string
		flags []string
	}{
		{path: "lint", flags: []string{"fail-on", "ignore", "json"}},
		{path: "diff", flags: []string{"from", "to", "against", "base", "json"}},
		{path: "explain", flags: []string{"json", "output"}},
		{path: "review", flags: []string{"fail-on", "ignore", "ai", "ai-required", "provider", "model", "json"}},
		{path: "test generate", flags: []string{"lang", "output", "force", "json"}},
		{path: "scan", flags: []string{"fail-on", "ignore", "json"}},
		{path: "doctor", flags: []string{"deep", "json", "timeout"}},
		{path: "env add", flags: []string{"kind", "orchestration", "client-id-env", "client-secret-env", "token-url", "token-url-env", "audience", "scope"}},
		{path: "plan", flags: []string{"dir", "env", "json"}},
		{path: "drift", flags: []string{"dir", "ref", "env", "json"}},
		{path: "backup", flags: []string{"output", "include-secrets"}},
		{path: "restore", flags: []string{"yes", "force", "project"}},
		{path: "incidents", flags: []string{"env", "limit"}},
		{path: "incidents retry", flags: []string{"yes", "dry-run"}},
		{path: "trace", flags: []string{"follow", "json", "env", "interval", "timeout", "idle-stop", "max-events"}},
	}

	root := NewRootWithDependencies(Dependencies{})
	for _, test := range tests {
		parts := strings.Fields(test.path)
		command, _, err := root.Find(parts)
		if err != nil {
			t.Fatalf("%s: %v", test.path, err)
		}
		for _, flag := range test.flags {
			if command.Flags().Lookup(flag) == nil && command.PersistentFlags().Lookup(flag) == nil {
				t.Errorf("%s missing --%s", test.path, flag)
			}
		}
	}
}

func TestToolkitContractExitCodes(t *testing.T) {
	tests := []struct {
		name string
		code int
		desc string
	}{
		{name: "success", code: 0, desc: "clean completion"},
		{name: "policy", code: 1, desc: "findings, diff, drift, or incident policy"},
		{name: "tool", code: 2, desc: "validation, upstream, partial, or unknown tool failure"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if test.code < 0 || test.code > 2 {
				t.Fatalf("unexpected exit class %d (%s)", test.code, test.desc)
			}
			if ExitCode(nil) != 0 {
				t.Fatal("nil error must map to exit 0")
			}
			if test.code == 1 && ExitCode(&ExitError{Code: 1, Err: errString("policy")}) != 1 {
				t.Fatal("policy ExitError must map to 1")
			}
			if test.code == 2 && ExitCode(&ExitError{Code: 2, Err: errString("tool")}) != 2 {
				t.Fatal("tool ExitError must map to 2")
			}
		})
	}
}

func TestToolkitContractTraceFollowDefaults(t *testing.T) {
	root := NewRootWithDependencies(Dependencies{})
	command, _, err := root.Find([]string{"trace"})
	if err != nil {
		t.Fatal(err)
	}
	timeout := command.Flags().Lookup("timeout")
	if timeout == nil || timeout.DefValue != "5m0s" {
		t.Fatalf("CLI --timeout default = %v, want 5m0s (interactive)", timeout)
	}
	maxEvents := command.Flags().Lookup("max-events")
	if maxEvents == nil || maxEvents.DefValue != "0" {
		t.Fatalf("CLI --max-events default = %v, want 0", maxEvents)
	}
	if command.Flags().Lookup("idle-stop") == nil {
		t.Fatal("CLI must keep --idle-stop (CLI-only follow control)")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
