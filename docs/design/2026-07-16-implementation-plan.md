# Camunda Lab Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `camunda-lab` — a Go CLI (`camunda`) that downloads official Camunda Docker Compose distributions, manages local light/full/modeler stacks for minors 8.7–8.10, and glues developer tools (`c8ctl`, Desktop Modeler profiles).

**Architecture:** Thin Go wrapper over official `camunda-distributions` release zips. Version adapters map CLI profile → compose files. Thin compose overlays handle resource limits and 8.10 Elasticsearch. State lives under `~/.camunda-lab/`.

**Tech Stack:** Go 1.22+, Cobra CLI, yaml.v3, Docker Compose v2 (shell out), goreleaser, Homebrew formula, MkDocs.

## Global Constraints

- Unofficial community project; README must state not affiliated with Camunda GmbH
- Binary name: `camunda`; repo/formula: `camunda-lab`
- Platforms v1: macOS + Linux only (no Windows)
- Supported minors: `8.7`, `8.8`, `8.9`, `8.10` (8.10 labeled preview)
- Default profile: `light`; install must support interactive prompts and `--yes` non-interactive
- Never rewrite upstream OIDC/Keycloak wiring — overlays only for resources and 8.10 ES
- Compose project name: `camunda-lab`
- License: MIT
- Spec: `docs/superpowers/specs/2026-07-16-camunda-lab-design.md`

## File map

| Path                            | Responsibility                         |
| ------------------------------- | -------------------------------------- |
| `cmd/camunda/main.go`           | Entrypoint                             |
| `internal/cli/*.go`             | Cobra commands                         |
| `internal/config/config.go`     | Load/save `~/.camunda-lab/config.yaml` |
| `internal/paths/paths.go`       | Home dir layout helpers                |
| `internal/versions/adapter.go`  | Minor → compose file mapping           |
| `internal/versions/download.go` | Fetch/verify/extract zip               |
| `internal/overlay/overlay.go`   | Pick overlay files                     |
| `internal/compose/compose.go`   | Run `docker compose` with project name |
| `internal/doctor/doctor.go`     | Preflight + health checks              |
| `internal/smoke/smoke.go`       | HTTP smoke checks                      |
| `internal/urls/urls.go`         | Component URL map                      |
| `internal/tools/c8ctl.go`       | c8ctl install/status helpers           |
| `internal/tools/modeler.go`     | Desktop Modeler profile writer         |
| `overlays/*.yaml`               | Resource + 8.10 ES overrides           |
| `Formula/camunda-lab.rb`        | Homebrew                               |
| `.goreleaser.yaml`              | Releases                               |
| `install.sh`                    | One-liner installer                    |
| `docs/` + `mkdocs.yml`          | Docs site                              |

---

### Task 1: Scaffold Go module and config package

**Files:**

- Create: `go.mod`
- Create: `cmd/camunda/main.go`
- Create: `internal/paths/paths.go`
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`
- Create: `Makefile`
- Create: `.gitignore`
- Create: `LICENSE`

**Interfaces:**

- Produces: `paths.Home() string`, `paths.VersionsDir() string`, `paths.ConfigFile() string`
- Produces: `config.Config` with fields `Version`, `Profile`, `Resources`, `Host`, `ComposeProject`
- Produces: `config.Load() (Config, error)`, `config.Save(Config) error`, `config.Defaults() Config`

- [ ] **Step 1: Create module and gitignore**

```bash
cd /Users/nasr/homelab/camunda-lab
go mod init github.com/nasraldin/camunda-lab
```

`.gitignore`:

```
/bin/
/dist/
*.exe
.DS_Store
.venv-docs/
site/
```

`LICENSE`: MIT, Copyright (c) 2026 Nasr Aldin

- [ ] **Step 2: Write failing config tests**

```go
package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/config"
	"github.com/nasraldin/camunda-lab/internal/paths"
)

func TestDefaults(t *testing.T) {
	c := config.Defaults()
	if c.Profile != "light" {
		t.Fatalf("profile=%q", c.Profile)
	}
	if c.Resources != "balanced" {
		t.Fatalf("resources=%q", c.Resources)
	}
	if c.ComposeProject != "camunda-lab" {
		t.Fatalf("project=%q", c.ComposeProject)
	}
}

func TestSaveLoad(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CAMUNDA_LAB_HOME", dir)
	paths.Reset()

	c := config.Defaults()
	c.Version = "8.8"
	if err := config.Save(c); err != nil {
		t.Fatal(err)
	}
	got, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.Version != "8.8" || got.Profile != "light" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.yaml")); err != nil {
		t.Fatal(err)
	}
}
```

- [ ] **Step 3: Run tests — expect FAIL**

Run: `go test ./internal/config/ -v`
Expected: FAIL (packages missing)

- [ ] **Step 4: Implement paths + config**

`internal/paths/paths.go`:

```go
package paths

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	mu   sync.Mutex
	home string
)

func Reset() {
	mu.Lock()
	defer mu.Unlock()
	home = ""
}

func Home() string {
	mu.Lock()
	defer mu.Unlock()
	if home != "" {
		return home
	}
	if v := os.Getenv("CAMUNDA_LAB_HOME"); v != "" {
		home = v
		return home
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		home = filepath.Join(".", ".camunda-lab")
		return home
	}
	home = filepath.Join(userHome, ".camunda-lab")
	return home
}

func ConfigFile() string   { return filepath.Join(Home(), "config.yaml") }
func VersionsDir() string  { return filepath.Join(Home(), "versions") }
func VersionDir(v string) string { return filepath.Join(VersionsDir(), v) }
func OverlaysDir() string  { return filepath.Join(Home(), "overlays") }
func LogsDir() string      { return filepath.Join(Home(), "logs") }
func ActiveFile() string   { return filepath.Join(Home(), "active.yaml") }
```

`internal/config/config.go`:

```go
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/nasraldin/camunda-lab/internal/paths"
	"gopkg.in/yaml.v3"
)

type Config struct {
	Version        string `yaml:"version"`
	Profile        string `yaml:"profile"`
	Resources      string `yaml:"resources"`
	Host           string `yaml:"host"`
	ComposeProject string `yaml:"compose_project"`
}

func Defaults() Config {
	return Config{
		Version:        "8.8",
		Profile:        "light",
		Resources:      "balanced",
		Host:           "localhost",
		ComposeProject: "camunda-lab",
	}
}

func Load() (Config, error) {
	path := paths.ConfigFile()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return Defaults(), nil
		}
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	d := Defaults()
	if c.Version == "" {
		c.Version = d.Version
	}
	if c.Profile == "" {
		c.Profile = d.Profile
	}
	if c.Resources == "" {
		c.Resources = d.Resources
	}
	if c.Host == "" {
		c.Host = d.Host
	}
	if c.ComposeProject == "" {
		c.ComposeProject = d.ComposeProject
	}
	return c, nil
}

func Save(c Config) error {
	if err := os.MkdirAll(filepath.Dir(paths.ConfigFile()), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(paths.ConfigFile(), data, 0o644)
}
```

`cmd/camunda/main.go`:

```go
package main

import (
	"fmt"
	"os"
)

var version = "0.0.0-dev"

func main() {
	if err := execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func execute() error {
	// wired in Task 2
	fmt.Println("camunda", version)
	return nil
}
```

`Makefile`:

```makefile
.PHONY: build test lint
VERSION ?= 0.0.0-dev

build:
	go build -ldflags "-X main.version=$(VERSION)" -o bin/camunda ./cmd/camunda

test:
	go test ./...

lint:
	golangci-lint run ./...
```

- [ ] **Step 5: Add yaml dependency and pass tests**

```bash
go get gopkg.in/yaml.v3
go test ./internal/config/ -v
go build -o bin/camunda ./cmd/camunda
```

Expected: PASS; binary prints `camunda 0.0.0-dev`

- [ ] **Step 6: Commit**

```bash
git add go.mod go.sum cmd/ internal/ Makefile .gitignore LICENSE
git commit -m "feat: scaffold Go module and config package"
```

---

### Task 2: Cobra CLI skeleton with help/version/about

**Files:**

- Create: `internal/cli/root.go`
- Create: `internal/cli/version.go`
- Modify: `cmd/camunda/main.go`
- Test: `internal/cli/root_test.go`

**Interfaces:**

- Consumes: `main.version` via `cli.SetVersion`
- Produces: `cli.NewRoot() *cobra.Command`

- [ ] **Step 1: Add Cobra**

```bash
go get github.com/spf13/cobra@latest
```

- [ ] **Step 2: Write failing test**

```go
package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/nasraldin/camunda-lab/internal/cli"
)

func TestRootHelp(t *testing.T) {
	cli.SetVersion("1.2.3")
	cmd := cli.NewRoot()
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetArgs([]string{"--help"})
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	for _, want := range []string{"camunda", "install", "doctor", "switch"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help missing %q:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 3: Run — expect FAIL**

Run: `go test ./internal/cli/ -v`
Expected: FAIL

- [ ] **Step 4: Implement root command**

`internal/cli/root.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var appVersion = "0.0.0-dev"

func SetVersion(v string) { appVersion = v }

func NewRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "camunda",
		Short: "Local Camunda 8 platform lab (Docker Compose)",
		Long: `camunda-lab — unofficial local Camunda 8 lab.

Wraps official Camunda Docker Compose distributions with install, version
switching, doctor, and developer tool helpers. Not affiliated with Camunda GmbH.`,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newVersionCmd())
	root.AddCommand(newAboutCmd())
	// stubs registered here; real bodies in later tasks
	root.AddCommand(placeholder("install", "Install and start a Camunda lab"))
	root.AddCommand(placeholder("up", "Start the active lab"))
	root.AddCommand(placeholder("start", "Alias for up"))
	root.AddCommand(placeholder("down", "Stop the lab (keep volumes)"))
	root.AddCommand(placeholder("stop", "Alias for down"))
	root.AddCommand(placeholder("restart", "Restart the lab"))
	root.AddCommand(placeholder("status", "Show lab status"))
	root.AddCommand(placeholder("switch", "Switch Camunda minor version"))
	root.AddCommand(placeholder("profile", "Set compose profile (light|full|modeler)"))
	root.AddCommand(placeholder("resources", "Set resource profile"))
	root.AddCommand(placeholder("urls", "Print component URLs"))
	root.AddCommand(placeholder("open", "Open a component URL in the browser"))
	root.AddCommand(placeholder("logs", "Show container logs"))
	root.AddCommand(placeholder("doctor", "Run health diagnostics"))
	root.AddCommand(placeholder("wait", "Wait until the lab is healthy"))
	root.AddCommand(placeholder("smoke", "Run smoke checks"))
	root.AddCommand(placeholder("nuke", "Wipe the lab completely"))
	root.AddCommand(placeholder("tools", "Developer tool helpers"))
	return root
}

func placeholder(use, short string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s: not implemented yet", use)
		},
	}
}
```

`internal/cli/version.go`:

```go
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print CLI version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), "camunda-lab %s\n", appVersion)
		},
	}
}

func newAboutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "about",
		Short: "Project info",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Fprintf(cmd.OutOrStdout(), `Camunda Lab
  CLI       camunda %s
  Project   https://github.com/nasraldin/camunda-lab
  Note      Unofficial community project — not affiliated with Camunda GmbH
`, appVersion)
		},
	}
}
```

Update `cmd/camunda/main.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/nasraldin/camunda-lab/internal/cli"
)

var version = "0.0.0-dev"

func main() {
	cli.SetVersion(version)
	if err := cli.NewRoot().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Pass tests**

```bash
go test ./internal/cli/ -v
./bin/camunda help || (go build -o bin/camunda ./cmd/camunda && ./bin/camunda --help)
```

- [ ] **Step 6: Commit**

```bash
git add cmd/ internal/cli/ go.mod go.sum
git commit -m "feat: add Cobra CLI skeleton with help and version"
```

---

### Task 3: Version adapters

**Files:**

- Create: `internal/versions/adapter.go`
- Create: `internal/versions/adapter_test.go`

**Interfaces:**

- Produces: `versions.Supported = []string{"8.7","8.8","8.9","8.10"}`
- Produces: `versions.IsPreview(minor string) bool` — true for `8.10`
- Produces: `versions.ComposeFiles(minor, profile string) (files []string, err error)`
- Produces: `versions.NeedsElasticsearchOverlay(minor, profile string) bool`

- [ ] **Step 1: Write failing tests**

```go
package versions_test

import (
	"testing"

	"github.com/nasraldin/camunda-lab/internal/versions"
)

func TestComposeFiles88Light(t *testing.T) {
	files, err := versions.ComposeFiles("8.8", "light")
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0] != "docker-compose.yaml" {
		t.Fatalf("%v", files)
	}
}

func TestComposeFiles88Full(t *testing.T) {
	files, err := versions.ComposeFiles("8.8", "full")
	if err != nil {
		t.Fatal(err)
	}
	if files[0] != "docker-compose-full.yaml" {
		t.Fatalf("%v", files)
	}
}

func TestComposeFiles87(t *testing.T) {
	light, _ := versions.ComposeFiles("8.7", "light")
	full, _ := versions.ComposeFiles("8.7", "full")
	if light[0] != "docker-compose-core.yaml" {
		t.Fatalf("light=%v", light)
	}
	if full[0] != "docker-compose.yaml" {
		t.Fatalf("full=%v", full)
	}
}

func Test810NeedsES(t *testing.T) {
	if !versions.NeedsElasticsearchOverlay("8.10", "full") {
		t.Fatal("expected ES overlay")
	}
	if versions.NeedsElasticsearchOverlay("8.10", "light") {
		t.Fatal("light should not force ES overlay")
	}
	if versions.NeedsElasticsearchOverlay("8.9", "full") {
		t.Fatal("8.9 bundles ES")
	}
}

func TestPreview(t *testing.T) {
	if !versions.IsPreview("8.10") {
		t.Fatal("8.10 should be preview")
	}
	if versions.IsPreview("8.8") {
		t.Fatal("8.8 should not be preview")
	}
}
```

- [ ] **Step 2: Run — expect FAIL**

Run: `go test ./internal/versions/ -v`

- [ ] **Step 3: Implement adapter**

```go
package versions

import "fmt"

var Supported = []string{"8.7", "8.8", "8.9", "8.10"}

func IsPreview(minor string) bool { return minor == "8.10" }

func ValidateMinor(minor string) error {
	for _, s := range Supported {
		if s == minor {
			return nil
		}
	}
	return fmt.Errorf("unsupported version %q (supported: %v)", minor, Supported)
}

func ValidateProfile(profile string) error {
	switch profile {
	case "light", "full", "modeler":
		return nil
	default:
		return fmt.Errorf("unsupported profile %q (light|full|modeler)", profile)
	}
}

func ComposeFiles(minor, profile string) ([]string, error) {
	if err := ValidateMinor(minor); err != nil {
		return nil, err
	}
	if err := ValidateProfile(profile); err != nil {
		return nil, err
	}
	switch minor {
	case "8.7":
		switch profile {
		case "light":
			return []string{"docker-compose-core.yaml"}, nil
		case "full":
			return []string{"docker-compose.yaml"}, nil
		case "modeler":
			return []string{"docker-compose-web-modeler.yaml"}, nil
		}
	default: // 8.8, 8.9, 8.10
		switch profile {
		case "light":
			return []string{"docker-compose.yaml"}, nil
		case "full":
			return []string{"docker-compose-full.yaml"}, nil
		case "modeler":
			return []string{"docker-compose-web-modeler.yaml"}, nil
		}
	}
	return nil, fmt.Errorf("internal: unhandled %s/%s", minor, profile)
}

func NeedsElasticsearchOverlay(minor, profile string) bool {
	return minor == "8.10" && profile == "full"
}

func ReleaseTag(minor string) string {
	return "docker-compose-" + minor
}

func ZipURL(minor string) string {
	tag := ReleaseTag(minor)
	return fmt.Sprintf(
		"https://github.com/camunda/camunda-distributions/releases/download/%s/%s.zip",
		tag, tag,
	)
}
```

- [ ] **Step 4: Pass tests and commit**

```bash
go test ./internal/versions/ -v
git add internal/versions/
git commit -m "feat: add Camunda minor version adapters"
```

---

### Task 4: Distribution downloader

**Files:**

- Create: `internal/versions/download.go`
- Create: `internal/versions/download_test.go`
- Create: `internal/versions/testdata/fake-dist.zip` (generated in test)

**Interfaces:**

- Produces: `versions.Ensure(minor string, opts DownloadOptions) (dir string, err error)`
- `DownloadOptions` has `HTTPClient *http.Client`, `SkipIfPresent bool`

- [ ] **Step 1: Write unit test with net/http/httptest**

Use `httptest` to serve a tiny zip containing `docker-compose.yaml` and `.env`, assert extract under `CAMUNDA_LAB_HOME/versions/8.8/`.

```go
func TestEnsureExtractsZip(t *testing.T) {
	// 1. Build an in-memory zip with docker-compose.yaml + .env
	// 2. Serve it via httptest.NewServer
	// 3. t.Setenv("CAMUNDA_LAB_HOME", tempDir); paths.Reset()
	// 4. Call versions.Ensure("8.8", DownloadOptions{URL: server.URL + "/docker-compose-8.8.zip"})
	// 5. Assert versions/8.8/docker-compose.yaml exists on disk
}
```

`DownloadOptions` must include `URL string` (if non-empty, use instead of `ZipURL(minor)`), `HTTPClient *http.Client`, `SkipIfPresent bool`.

Implement `Ensure` to:

1. Skip if `VersionDir(minor)` already has expected compose file and `SkipIfPresent`
2. Download zip to temp
3. Extract to `versions/<minor>/` (handle zip root folder if present)
4. Return absolute dir

Also export `CosignBundleURL(minor string)` for later optional verify — v1 may log "cosign optional" and skip if `cosign` binary missing; document in doctor.

- [ ] **Step 2: Implement download.go with archive/zip + net/http**

Key behavior: on HTTP non-200, return error including status and URL. Create parent dirs with `0755`.

- [ ] **Step 3: Pass tests and commit**

```bash
go test ./internal/versions/ -v
git add internal/versions/
git commit -m "feat: download and extract official Camunda compose zips"
```

---

### Task 5: Overlay selection + checked-in overlay YAMLs

**Files:**

- Create: `overlays/resources-small.yaml`
- Create: `overlays/resources-balanced.yaml`
- Create: `overlays/resources-power.yaml`
- Create: `overlays/elasticsearch-8.10.yaml`
- Create: `internal/overlay/overlay.go`
- Create: `internal/overlay/overlay_test.go`
- Create: `internal/overlay/embed.go` (embed FS)

**Interfaces:**

- Produces: `overlay.Files(resources string, minor string, profile string) ([]string, error)` — returns absolute paths after syncing embeds to `~/.camunda-lab/overlays/`
- Produces: `overlay.SyncToHome() error`

- [ ] **Step 1: Write overlay YAML files**

`overlays/resources-balanced.yaml` (example — tune in implementation):

```yaml
# camunda-lab resource overlay — balanced
services:
  orchestration:
    deploy:
      resources:
        limits:
          memory: 2G
  elasticsearch:
    deploy:
      resources:
        limits:
          memory: 2G
```

For 8.7 use service names `zeebe` / `operate` / `tasklist` in addition via separate files OR keep overlays soft (only apply keys that exist — Compose ignores unknown service keys? Actually Compose fails on unknown services in some versions). **Safer approach:** resource overlays are best-effort documentation + only apply memory env where universal:

Prefer overlays that set only common services per era:

- `resources-*-88.yaml` style OR
- Single overlay using extension fields that won't break

**Simplest safe v1:** overlays only for:

1. `elasticsearch-8.10.yaml` — full ES service definition + network join
2. Resource overlays that only touch `orchestration` + `elasticsearch` + `zeebe` (all optional keys) — Docker Compose v2 **errors** on undefined services.

**Decision for plan:** ship **three resource overlays per era** is too heavy. Instead v1 resource profiles set env file `~/.camunda-lab/resources.env` with `JAVA_TOOL_OPTIONS` / heap hints exported into compose via `--env-file`, and only ship `elasticsearch-8.10.yaml` as a real compose override.

Update design implementation note: `camunda resources` writes `resources.env` and records choice in config; compose runner always passes `--env-file` when present.

`overlays/elasticsearch-8.10.yaml`:

```yaml
services:
  elasticsearch:
    image: docker.elastic.co/elasticsearch/elasticsearch:8.17.10
    container_name: elasticsearch
    environment:
      - discovery.type=single-node
      - xpack.security.enabled=false
      - 'ES_JAVA_OPTS=-Xms1g -Xmx1g'
      - cluster.name=elasticsearch
    ports:
      - '9200:9200'
    healthcheck:
      test: ['CMD-SHELL', 'curl -s http://localhost:9200 >/dev/null || exit 1']
      interval: 30s
      timeout: 10s
      retries: 10
      start_period: 60s
    networks:
      - camunda-platform
```

Ensure network name matches upstream `camunda-platform`.

- [ ] **Step 2: Implement overlay.Files**

Returns paths to compose override files to pass as extra `-f` args. For resources, write `resources.env` with:

| profile  | JAVA_TOOL_OPTIONS / notes |
| -------- | ------------------------- |
| small    | `-Xms256m -Xmx512m`       |
| balanced | `-Xms512m -Xmx1024m`      |
| power    | `-Xms1g -Xmx2g`           |

- [ ] **Step 3: Tests + commit**

```bash
go test ./internal/overlay/ -v
git add overlays/ internal/overlay/
git commit -m "feat: add overlays for resources env and 8.10 Elasticsearch"
```

---

### Task 6: Compose runner

**Files:**

- Create: `internal/compose/compose.go`
- Create: `internal/compose/compose_test.go`

**Interfaces:**

- Produces: `compose.Runner` with methods:
  - `Up(ctx, workDir string, files []string, envFiles []string, project string) error`
  - `Down(ctx, workDir string, files []string, project string, volumes bool) error`
  - `Ps(ctx, workDir, project string) (string, error)`
  - `Logs(ctx, workDir, project, service string, follow bool) error`
- Uses `exec.CommandContext("docker", args...)` with `docker compose`

- [ ] **Step 1: Test arg building**

Export `BuildArgs(subcommand string, project string, files, envFiles []string, extra ...string) []string` and unit-test without Docker:

```go
args := compose.BuildArgs("up", "camunda-lab", []string{"docker-compose.yaml"}, []string{"resources.env"}, "-d")
// expect: compose -p camunda-lab -f docker-compose.yaml --env-file resources.env up -d
```

- [ ] **Step 2: Implement runner**

Working directory = extracted version dir. Stream stdout/stderr to OS. Return wrapped errors.

- [ ] **Step 3: Commit**

```bash
go test ./internal/compose/ -v
git add internal/compose/
git commit -m "feat: add docker compose runner"
```

---

### Task 7: Install + up/down/status commands

**Files:**

- Create: `internal/lab/lab.go` — orchestrates ensure + overlays + compose
- Create: `internal/cli/install.go`
- Create: `internal/cli/lifecycle.go`
- Modify: `internal/cli/root.go` — replace placeholders
- Create: `internal/prompt/prompt.go` — interactive selects (stdin)

**Interfaces:**

- Produces: `lab.Install(ctx, opts lab.InstallOpts) error`
- `InstallOpts`: Version, Profile, Resources, Yes bool
- Produces: `lab.Up/Down/Status`

- [ ] **Step 1: Implement prompt helpers**

If `opts.Yes` or non-TTY: use defaults / flags. Else: survey-style simple numbered menu using only stdlib (`fmt.Scanln`).

- [ ] **Step 2: Implement lab.Install**

1. Validate docker via `doctor.CheckDocker` (stub returning nil until Task 9, or inline `docker compose version`)
2. Resolve version/profile/resources
3. `versions.Ensure`
4. Sync overlays / write resources.env
5. Save config
6. `compose.Up`
7. Print next steps

- [ ] **Step 3: Wire CLI commands `install`, `up`, `start`, `down`, `stop`, `restart`, `status`**

Flags on install: `--version`, `--profile`, `--resources`, `--yes`

- [ ] **Step 4: Manual smoke (no LIVE CI required yet)**

```bash
go build -o bin/camunda ./cmd/camunda
./bin/camunda install --help
```

- [ ] **Step 5: Commit**

```bash
git add internal/lab/ internal/cli/ internal/prompt/
git commit -m "feat: implement install and lifecycle commands"
```

---

### Task 8: switch, profile, resources

**Files:**

- Create: `internal/cli/switch.go`
- Create: `internal/cli/profile.go`
- Create: `internal/cli/resources.go`
- Create: `internal/lab/switch.go`

**Interfaces:**

- `lab.Switch(ctx, minor string, wipe bool) error` — stop; if wipe `Down(volumes=true)`; update config; Ensure; Up
- Warn on cross-minor without wipe to stderr

- [ ] **Step 1: Unit-test Switch config mutation with fake runner interface**

Define `compose.Engine` interface in `internal/compose` so tests can inject a fake. Refactor Runner to implement Engine.

- [ ] **Step 2: Implement commands + tests**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: add switch, profile, and resources commands"
```

---

### Task 9: URLs, open, logs

**Files:**

- Create: `internal/urls/urls.go`
- Create: `internal/urls/urls_test.go`
- Create: `internal/cli/urls.go`
- Create: `internal/cli/logs.go`

**Interfaces:**

- `urls.List(cfg config.Config) []urls.Entry` where `Entry{Name, URL, Notes}`
- Port map must match upstream defaults (Operate/Tasklist differ for light vs full and 8.7 vs 8.8+)

Reference defaults from Camunda docs (8.8+ light):

- Operate/Tasklist/Admin: `http://localhost:8080/...`
- gRPC: `localhost:26500`
- Full: Console `8087`, Optimize `8083`, Identity `8084`, Web Modeler `8070`, Keycloak `18080`

For 8.7 full, use upstream ports from that minor’s compose (verify while implementing by reading extracted `.env` / compose). Adapter function `urls.For(minor, profile)`.

`open` uses `xdg-open` / `open` (macOS) via `exec`.

- [ ] **Step 1: Table-driven URL tests for 8.7/8.8 light and full**

- [ ] **Step 2: Implement + wire CLI**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: add urls, open, and logs commands"
```

---

### Task 10: doctor, wait, smoke

**Files:**

- Create: `internal/doctor/doctor.go`
- Create: `internal/doctor/doctor_test.go`
- Create: `internal/smoke/smoke.go`
- Create: `internal/cli/doctor.go`
- Create: `internal/cli/wait.go`
- Create: `internal/cli/smoke.go`

**Interfaces:**

- `doctor.Run(fix bool) (Report, error)` — docker, compose v2, disk, config present, version dir, optional port check
- `smoke.Run(ctx, cfg) error` — HTTP GET Operate or `/actuator/health` endpoints with timeout; treat 401/302 as success for protected UIs
- `wait.UntilHealthy(ctx, cfg, timeout)` — poll smoke until ok or timeout

- [ ] **Step 1: Tests with httptest for smoke success/failure**

- [ ] **Step 2: Implement CLI `--fix` that restarts docker compose if containers exited (no host package installs)**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: add doctor, wait, and smoke commands"
```

---

### Task 11: tools c8ctl + modeler profile

**Files:**

- Create: `internal/tools/c8ctl.go`
- Create: `internal/tools/modeler.go`
- Create: `internal/tools/modeler_test.go`
- Create: `internal/cli/tools.go`

**Interfaces:**

- `tools.C8ctlStatus() (installed bool, path string, err error)`
- `tools.C8ctlInstall() error` — prefer `npm install -g @camunda8/cli`; clear error if npm missing
- `tools.WriteModelerProfile(cfg config.Config) (path string, err error)` — write JSON profile under OS-specific Camunda Modeler config dir; include gRPC `26500` and REST base URL; auth mode basic for light, OIDC notes for full

Modeler paths:

- macOS: `~/Library/Application Support/camunda-modeler/resources/profiles.json` (confirm during impl; create if missing)
- Linux: `~/.config/camunda-modeler/...`

- [ ] **Step 1: Unit-test JSON merge for modeler profile**

- [ ] **Step 2: Implement tools subcommands: `tools c8ctl install|status`, `tools modeler profile`**

- [ ] **Step 3: Commit**

```bash
git commit -m "feat: add c8ctl and Desktop Modeler tool helpers"
```

---

### Task 12: nuke + active.yaml fingerprint

**Files:**

- Create: `internal/cli/nuke.go`
- Create: `internal/lab/active.go`
- Modify: `internal/lab/lab.go` to write `active.yaml` on up

**Interfaces:**

- `active.yaml` stores: version, profile, workDir, compose files, startedAt
- `nuke`: confirm unless `CONFIRM=yes` or `--yes`; `Down(volumes=true)`; remove `~/.camunda-lab/versions`, overlays copy, config (or entire home)

- [ ] **Step 1: Test nuke refuses without confirmation**

- [ ] **Step 2: Implement + commit**

```bash
git commit -m "feat: add nuke and active lab fingerprint"
```

---

### Task 13: README, install.sh, MkDocs stub

**Files:**

- Create: `README.md`
- Create: `install.sh`
- Create: `mkdocs.yml`
- Create: `docs/index.md`
- Create: `docs/installation.md`
- Create: `docs/cli-reference.md`
- Create: `docs/architecture.md`
- Create: `docs/troubleshooting.md`
- Create: `CONTRIBUTING.md`
- Create: `requirements-docs.txt`

README must include: badges placeholders, quickstart, unofficial disclaimer, link to spec, requirements (Docker Compose v2), examples matching docker-lab tone.

`install.sh`: download latest release binary for `uname` OS/arch from GitHub Releases, install to `~/.local/bin/camunda`.

- [ ] **Step 1: Write docs content from CLI surface in the spec**

- [ ] **Step 2: Commit**

```bash
git add README.md install.sh mkdocs.yml docs/ CONTRIBUTING.md requirements-docs.txt
git commit -m "docs: add README, install script, and MkDocs site stub"
```

---

### Task 14: CI, goreleaser, Homebrew formula

**Files:**

- Create: `.github/workflows/ci.yml`
- Create: `.github/workflows/release.yml`
- Create: `.goreleaser.yaml`
- Create: `Formula/camunda-lab.rb` (template with version/url/sha placeholders + comment that `scripts/publish-homebrew.sh` updates them)

CI: `go test ./...`, `go build`, golangci-lint  
Release: tag `v*` → goreleaser

Formula installs binary named `camunda` from release assets.

- [ ] **Step 1: Add workflows and goreleaser config**

- [ ] **Step 2: Local snapshot**

```bash
goreleaser release --snapshot --clean
```

Expected: binaries under `dist/` for linux/darwin amd64/arm64

- [ ] **Step 3: Commit**

```bash
git add .github/ .goreleaser.yaml Formula/
git commit -m "ci: add test workflow, goreleaser, and Homebrew formula stub"
```

---

### Task 15: LIVE integration test script

**Files:**

- Create: `scripts/live-smoke.sh`
- Create: `internal/lab/lab_live_test.go` with `//go:build live`

Script:

```bash
#!/usr/bin/env bash
set -euo pipefail
export CAMUNDA_LAB_HOME="${CAMUNDA_LAB_HOME:-$(mktemp -d)}"
go build -o bin/camunda ./cmd/camunda
./bin/camunda install --version 8.8 --profile light --resources small --yes
./bin/camunda wait
./bin/camunda smoke
./bin/camunda down
```

Document in CONTRIBUTING: `LIVE=1 ./scripts/live-smoke.sh` needs Docker and ~10–15 minutes + RAM.

- [ ] **Step 1: Add script + docs note**

- [ ] **Step 2: Commit**

```bash
git add scripts/ internal/lab/lab_live_test.go CONTRIBUTING.md
git commit -m "test: add optional LIVE smoke script for light profile"
```

---

## Spec coverage checklist

| Spec requirement                      | Task                   |
| ------------------------------------- | ---------------------- |
| A+ official zip wrapper               | 4, 7                   |
| Versions 8.7–8.10 adapters            | 3                      |
| Profiles light/full/modeler           | 3, 7, 8                |
| Interactive install + `--yes`         | 7                      |
| Resource profiles                     | 5, 8                   |
| 8.10 ES overlay                       | 5                      |
| Lifecycle up/down/status/switch       | 7, 8                   |
| doctor / wait / smoke                 | 10                     |
| urls / open / logs                    | 9                      |
| tools c8ctl + modeler                 | 11                     |
| nuke                                  | 12                     |
| Go CLI + Homebrew + install.sh + docs | 2, 13, 14              |
| Single lab, paths ready for v2        | 1 (`CAMUNDA_LAB_HOME`) |
| Unofficial disclaimer                 | 2, 13                  |

## Plan self-review notes

- Clarified resource overlays: prefer `resources.env` + one real compose overlay for 8.10 ES to avoid Compose errors on missing service names across minors.
- Cosign verification is best-effort in doctor/download (skip if binary absent); not a hard blocker for v1.
- URL ports for 8.7 must be verified against extracted compose during Task 9 implementation.
