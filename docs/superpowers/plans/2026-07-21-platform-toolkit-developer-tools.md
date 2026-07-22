# Platform Toolkit Developer Tools Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete project discovery, the shared BPMN model, and every promised Phase 2 developer command with matching CLI/API/UI behavior.

**Architecture:** Project configuration resolves safe recursive assets; one multi-process BPMN document IR and graph feed lint, diff, explain, review, and test generation. A typed `internal/toolkit.Service` coordinates domain packages for CLI and API.

**Tech Stack:** Go 1.24, Cobra, XML, Git subprocess abstraction, provider-neutral AI HTTP adapters, React/TypeScript.

## Global Constraints

- Offline deterministic behavior is always available.
- AI is optional unless `--ai-required` is requested.
- Unknown BPMN extensions are retained; unsupported semantics never become valid empty data.
- Tool failures exit `2`; findings/differences exit `1`; clean success exits `0`.
- DMN/form semantic diff, SARIF, auto-fix, visual BPMN canvas, and generated-test execution remain excluded.
- Do not commit unless explicitly requested.

---

### Task 1: Safe project configuration and recursive discovery

**Files:**
- Create: `internal/project/discovery.go`
- Create: `internal/project/discovery_test.go`
- Modify: `internal/project/config.go`
- Modify: `internal/project/config_test.go`
- Modify: `internal/project/scaffold.go`
- Modify: `internal/project/scaffold_test.go`
- Create fixtures under: `testdata/projects/toolkit/`

**Interfaces:**

```go
type Project struct {
    Root   string
    Config Config
}

func FindRoot(start string) (string, error)
func Open(start string) (Project, error)
func (p Project) Resolve(configuredPath string) (string, error)
func (p Project) Discover(kind AssetKind) ([]string, error)
func (p Project) ResolveInput(kind AssetKind, input string) (string, error)
func (o ScaffoldOpts) Validate() error
```

- [ ] Write failing tests for `paths.tests`, `lint.ignore`, unsafe configured paths, recursive sorted discovery, project-relative inputs, scaffold validation, and scaffold/load round trip.
- [ ] Run `go test ./internal/project -count=1`; expect failures for missing fields/discovery and accepted traversal.
- [ ] Implement safe root/config resolution and recursive extension-filtered discovery.
- [ ] Re-run the focused tests; expect PASS.

### Task 2: Multi-process BPMN document IR and graph

**Files:**
- Modify: `internal/bpmn/model.go`
- Modify: `internal/bpmn/parse.go`
- Create: `internal/bpmn/normalize.go`
- Create: `internal/bpmn/graph.go`
- Modify: `internal/bpmn/parse_test.go`
- Create: `internal/bpmn/graph_test.go`
- Add fixtures under: `testdata/bpmn/`

**Interfaces:**

```go
type Document struct {
    Processes         []Process
    Messages          []Message
    Errors            []Error
    UnknownExtensions []Extension
}

func Parse(io.Reader) (Document, error)
func ParseFile(string) (Document, error)
func NewGraph(Process) Graph
func (g Graph) ReachableFrom(starts ...string) map[string]bool
func (g Graph) HappyPath() []string
func (g Graph) AlternatePaths() [][]string
func (g Graph) DeadEnds() []string
```

- [ ] Add failing fixtures/tests for multiple processes, nested subprocesses, call activities, boundary message/timer/error events, retries, conditions, unknown extensions, non-BPMN XML, no process, malformed XML, cycles, alternate paths, and XML/ID-order independence.
- [ ] Run `go test ./internal/bpmn -count=1`; expect failures because only the first process and limited nodes are parsed.
- [ ] Implement the document model, namespace validation, typed errors, stable normalization, and sequence-flow graph.
- [ ] Re-run the focused tests; expect PASS.

### Task 3: Typed toolkit application service

**Files:**
- Create: `internal/toolkit/contracts.go`
- Create: `internal/toolkit/dependencies.go`
- Create: `internal/toolkit/errors.go`
- Create: `internal/toolkit/service.go`
- Create: `internal/toolkit/service_test.go`

**Interfaces:**

```go
type Service struct {
    Git GitReader
    AI  ai.ChatClient
}

func (s Service) Lint(context.Context, LintRequest) (LintResult, error)
func (s Service) Diff(context.Context, DiffRequest) (DiffResult, error)
func (s Service) Explain(context.Context, ExplainRequest) (ExplainResult, error)
func (s Service) Review(context.Context, ReviewRequest) (ReviewResult, error)
func (s Service) Generate(context.Context, GenerateRequest) (GenerateResult, error)
func (s Service) Scan(context.Context, ScanRequest) (ScanResult, error)
```

- [ ] Write failing service tests for explicit/project discovery, malformed input, Git failure, optional/required AI, artifact output, partial scans, and typed statuses.
- [ ] Run `go test ./internal/toolkit -count=1`; expect compile failure because the package/contracts do not exist.
- [ ] Implement thin orchestration over domain packages with no text/HTTP formatting.
- [ ] Re-run the focused tests; expect PASS as each downstream domain task lands.

### Task 4: Complete deterministic lint

**Files:**
- Create: `internal/lint/rule.go`
- Create: `internal/lint/rules.go`
- Create: `internal/lint/rules_process.go`
- Create: `internal/lint/rules_gateway.go`
- Create: `internal/lint/rules_message.go`
- Create: `internal/lint/rules_task.go`
- Create: `internal/lint/rules_timer.go`
- Create: `internal/lint/report.go`
- Modify: `internal/lint/lint.go`
- Modify: `internal/lint/lint_test.go`
- Add fixtures under: `testdata/bpmn/lint/`

**Interfaces:**

```go
type Rule interface {
    ID() string
    Check(bpmn.Document) []Finding
}

func Run(bpmn.Document, Options) Result
```

- [ ] Add one positive and negative fixture per planned rule, plus tests for multi-process attribution, ignore filtering, stable sorting, threshold, text, and JSON.
- [ ] Run `go test ./internal/lint -count=1`; expect missing rule/ignore failures.
- [ ] Implement the registry and all seven plan rules using the graph.
- [ ] Re-run tests; expect PASS.

### Task 5: Complete semantic diff and Git modes

**Files:**
- Create: `internal/diff/change.go`
- Create: `internal/diff/format.go`
- Create: `internal/diff/git.go`
- Modify: `internal/diff/diff.go`
- Modify: `internal/diff/diff_test.go`
- Create: `internal/diff/git_test.go`

**Interfaces:**

```go
type Change struct {
    Kind, ProcessID, ElementID, ElementType string
    Field, Before, After                    string
    Summary                                string
}

func Compare(before, after bpmn.Document) []Change
```

- [ ] Add failing tests for process/element add-remove, type/name/job/retry/default, messages, events, conditions, attachments, call targets, flow endpoints, formatting-only equivalence, stable JSON, and Git errors.
- [ ] Run `go test ./internal/diff -count=1`; expect missing semantic changes and false formatting diffs.
- [ ] Implement normalized comparison and Git `show` abstraction for documented file/from/to/against/base modes.
- [ ] Re-run tests; expect PASS.

### Task 6: Graph-based explain and provider-neutral review AI

**Files:**
- Create: `internal/ai/chat.go`
- Create: `internal/ai/chat_test.go`
- Create: `internal/explain/graph.go`
- Create: `internal/explain/ai.go`
- Modify: `internal/explain/explain.go`
- Modify: `internal/explain/explain_test.go`
- Create: `internal/review/prompt.go`
- Create: `internal/review/client.go`
- Modify: `internal/review/review.go`
- Modify: `internal/review/review_test.go`

**Interfaces:**

```go
type ChatClient interface {
    Complete(context.Context, ChatRequest) (ChatResponse, error)
}

type AIStatus string // disabled|skipped|succeeded|failed
```

- [ ] Write golden explain tests proving graph order, alternate paths, all processes, and stable sections.
- [ ] Write `httptest` AI tests for auth headers, model, timeout, masking, prompt truncation, optional failure, required failure, skip, and success.
- [ ] Run `go test ./internal/ai ./internal/explain ./internal/review -count=1`; expect graph-order and unwired-client failures.
- [ ] Implement OpenAI-compatible and Anthropic adapters using existing AI environment configuration; no live calls.
- [ ] Re-run tests; expect PASS.

### Task 7: Render-first test generation

**Files:**
- Create: `internal/testgen/java.go`
- Create: `internal/testgen/js.go`
- Create: `internal/testgen/write.go`
- Create: `internal/testgen/templates/`
- Modify: `internal/testgen/generate.go`
- Modify: `internal/testgen/generate_test.go`
- Add golden files under: `testdata/golden/testgen/`

**Interfaces:**

```go
type Artifact struct {
    Path      string
    MediaType string
    Content   []byte
}

func Generate(bpmn.Document, Options) ([]Artifact, error)
func Write(root string, artifacts []Artifact, force bool) ([]string, error)
```

- [ ] Write failing tests for deterministic Java/JS output, multiple processes, duplicate job types, invalid language, safe relative paths, overwrite refusal, and no partial writes.
- [ ] Run `go test ./internal/testgen -count=1`; expect compile/behavior failures.
- [ ] Implement in-memory generation first and a separate authorized writer.
- [ ] Re-run tests; expect PASS.

### Task 8: Complete scan accounting and ignore behavior

**Files:**
- Create: `internal/scan/walk.go`
- Create: `internal/scan/ignore.go`
- Create: `internal/scan/rules.go`
- Create: `internal/scan/report.go`
- Modify: `internal/scan/scan.go`
- Modify: `internal/scan/scan_test.go`
- Add fixtures under: `testdata/scan/project/`

**Interfaces:**

```go
type Result struct {
    Findings []Finding `json:"findings"`
    Issues   []Issue   `json:"issues"`
    Complete bool      `json:"complete"`
    Stats    Stats     `json:"stats"`
}
```

- [ ] Add failing tests for `.gitignore`, `.camunda-scanignore`, negation, directory patterns, inline suppression, binary/large-file skips, unreadable files, truncation, deterministic output, masking, and severity.
- [ ] Run `go test ./internal/scan -count=1`; expect ignored read/walk failures and missing ignore behavior.
- [ ] Implement explicit issue accounting; partial scans set `complete:false` and never print “No secrets found.”
- [ ] Re-run tests; expect PASS.

### Task 9: Complete `doctor --deep`

**Files:**
- Create: `internal/doctor/report.go`
- Create: `internal/doctor/compose.go`
- Create: `internal/doctor/docker.go`
- Create: `internal/doctor/overlay.go`
- Modify: `internal/doctor/deep.go`
- Modify: `internal/doctor/deep_test.go`
- Modify: `internal/overlay/overlay.go`

**Interfaces:**

```go
type Inspector interface {
    ComposeServices(context.Context, config.Config) ([]ServiceState, error)
    DiskUsage(context.Context) (DiskUsage, error)
    Volumes(context.Context, string) ([]VolumeState, error)
}
```

- [ ] Add failing tests for healthy/degraded/exited/no services, HTTP/TCP failure, timeout, missing volumes, disk warning, expected/missing/stale overlays, and stable text/JSON.
- [ ] Run `go test ./internal/doctor -count=1`; expect absent inspection categories.
- [ ] Implement injected Compose/Docker/overlay inspection and explicit lab-down diagnosis.
- [ ] Re-run tests; expect PASS.

### Task 10: Thin CLI/API/UI edges and gate

**Files:**
- Split: `internal/cli/toolkit.go` into focused command files
- Create: `internal/cli/exit.go`
- Modify: `cmd/camunda/main.go`
- Create: `internal/ui/api/handlers_developer.go`
- Create: `internal/ui/api/contracts_developer.go`
- Modify: `internal/ui/web/src/pages/Bpmn.tsx`
- Modify: `internal/ui/web/src/pages/Overview.tsx`
- Modify: `internal/ui/web/src/pages/Project.tsx`
- Modify: `docs/cli-reference.md`
- Modify: `docs/lab-ui.md`

- [ ] Add failing Cobra/API contract tests for every advertised flag, JSON schema, status, and exit `0/1/2`.
- [ ] Wire edges to `toolkit.Service`; return in-memory artifacts or authorized writes without temp leaks.
- [ ] Expose lint threshold/ignores, Git diff, AI controls, artifact download/write, scan partial status, and structured deep diagnostics.
- [ ] Remove hardcoded Camunda `8.9`; obtain supported/default version from configuration/overview.
- [ ] Run:

  ```bash
  go test ./internal/project ./internal/bpmn ./internal/lint ./internal/diff ./internal/explain ./internal/review ./internal/testgen ./internal/scan ./internal/doctor ./internal/toolkit ./internal/cli ./internal/ui/api -count=1
  go test ./...
  git diff --check
  ```

  Expected: all PASS.

- [ ] Review checkpoint; commit only if explicitly requested.
