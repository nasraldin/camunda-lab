package lab

import (
	"context"
	"errors"
	"io"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/compose"
	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestRunning(t *testing.T) {
	tests := []struct {
		name    string
		psJSON  string
		psErr   error
		want    bool
		wantErr bool
	}{
		{
			name:   "any running container",
			psJSON: "{\"Service\":\"stopped\",\"State\":\"exited\"}\n{\"Service\":\"active\",\"State\":\"running\"}",
			want:   true,
		},
		{
			name:   "stopped containers",
			psJSON: "{\"Service\":\"one\",\"State\":\"exited\"}\n{\"Service\":\"two\",\"State\":\"created\"}",
		},
		{
			name: "empty compose state",
		},
		{
			name:    "compose error",
			psErr:   errors.New("docker unavailable"),
			wantErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			t.Setenv("CAMUNDA_LAB_HOME", home)
			paths.Reset()
			t.Cleanup(paths.Reset)
			cfg := config.Defaults()
			cfg.Version = "8.9"
			cfg.ComposeProject = "safety-test"
			if err := config.Save(cfg); err != nil {
				t.Fatal(err)
			}

			engine := &runningTestEngine{psJSON: tc.psJSON, psErr: tc.psErr}
			got, err := (&Lab{Engine: engine}).Running(context.Background())
			if tc.wantErr {
				if err == nil {
					t.Fatal("Running() error = nil, want compose error")
				}
				return
			}
			if err != nil {
				t.Fatalf("Running() error = %v", err)
			}
			if got != tc.want {
				t.Fatalf("Running() = %v, want %v", got, tc.want)
			}
			if engine.workDir != paths.VersionDir(cfg.Version) {
				t.Fatalf("PsJSON workDir = %q, want %q", engine.workDir, paths.VersionDir(cfg.Version))
			}
			if engine.project != cfg.ComposeProject {
				t.Fatalf("PsJSON project = %q, want %q", engine.project, cfg.ComposeProject)
			}
		})
	}
}

type runningTestEngine struct {
	psJSON  string
	psErr   error
	workDir string
	project string
}

var _ compose.Engine = (*runningTestEngine)(nil)

func (*runningTestEngine) Up(string, []string, []string, string) error { return nil }
func (*runningTestEngine) UpService(string, []string, []string, string, string) error {
	return nil
}
func (*runningTestEngine) Down(string, []string, string, bool) error { return nil }
func (*runningTestEngine) Ps(string, string) (string, error)         { return "", nil }
func (e *runningTestEngine) PsJSON(workDir, project string) (string, error) {
	e.workDir = workDir
	e.project = project
	return e.psJSON, e.psErr
}
func (*runningTestEngine) Logs(string, string, string, bool) error { return nil }
func (*runningTestEngine) LogsTo(context.Context, string, string, string, bool, io.Writer) error {
	return nil
}
