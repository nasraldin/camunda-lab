# Platform Toolkit Platform Operations Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete active-environment authentication, trustworthy plan/drift, incidents, trace, backup, and Kubernetes behavior across CLI, API, and UI.

**Architecture:** Environment resolution and OIDC create active-env cluster clients. Shared canonical inventory feeds plan/drift. Focused services own incidents, trace, backup, and Kubernetes through injected external-system interfaces.

**Tech Stack:** Go 1.24, Cobra, `net/http`, Git, Docker Compose, kubectl, React/TypeScript.

## Global Constraints

- Remote inventory is complete, explicitly unsupported, or failed; missing data never yields noop/in-sync.
- Active environment consistently drives every cluster-aware command.
- Profile files store environment-variable names, never secret values.
- Mutations require edge confirmation and domain validation.
- Kubernetes tests use a recording fake runner; live mutations require a disposable context.
- Helm upgrade, port-forward, volume export, and remote deployed-resource backup remain excluded.
- Do not commit unless explicitly requested.

---

### Task 1: Project/global environment service and config migration

**Files:**
- Create: `internal/env/service.go`
- Modify: `internal/env/profile.go`
- Modify: `internal/env/profile_test.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

**Interfaces:**

```go
type ProfileSource string

type ResolveRequest struct {
    Name        string
    ProjectRoot string
}

type Resolved struct {
    Profile Profile
    Source  ProfileSource
}

func (s *Service) List(projectRoot string) ([]Resolved, error)
func (s *Service) Resolve(ResolveRequest) (Resolved, error)
func (s *Service) SaveGlobal(Profile) error
func (s *Service) SaveProject(projectRoot string, p Profile) error
func (s *Service) Use(name, projectRoot string) (Resolved, error)
func (s *Service) Remove(name, projectRoot string, source ProfileSource) error
```

- [ ] Write failing tests for project-over-global precedence, validated use, atomic active removal, legacy `active-env` migration into `config.yaml`, and secret-reference-only persistence.
- [ ] Run `go test ./internal/env ./internal/config -count=1`; expect missing project profiles/config active state.
- [ ] Implement service and one-time migration that deletes the legacy pointer only after successful config persistence.
- [ ] Re-run tests; expect PASS.

### Task 2: Remote OIDC token source and cluster factory

**Files:**
- Create: `internal/cluster/token.go`
- Create: `internal/cluster/factory.go`
- Create: `internal/cluster/factory_test.go`
- Modify: `internal/cluster/auth.go`
- Modify: `internal/cluster/auth_test.go`

**Interfaces:**

```go
type TokenRequest struct {
    TokenURL, ClientID, ClientSecret, Audience string
}

type TokenSource interface {
    Token(context.Context, TokenRequest) (string, error)
}

type Factory interface {
    Client(context.Context, envName, projectRoot string) (*Client, env.Resolved, error)
}
```

- [ ] Add `httptest` failures/success for grant fields, audience, auth, cancellation, HTTP errors, malformed JSON, missing token, missing named environment variables, access-token override, full-lab OIDC, and project-local profile selection.
- [ ] Run `go test ./internal/cluster -run 'Test(Token|OIDC|Factory|RemoteProfile)' -count=1`; expect missing factory and localhost fallback behavior.
- [ ] Implement explicit token URL resolution; remote profiles never default to localhost.
- [ ] Re-run tests; expect PASS.

### Task 3: Shared canonical inventory

**Files:**
- Create: `internal/inventory/model.go`
- Create: `internal/inventory/canonical.go`
- Create: `internal/inventory/local.go`
- Create: `internal/inventory/inventory_test.go`
- Create fixtures under: `internal/inventory/testdata/`
- Create: `internal/cluster/inventory.go`
- Modify: `internal/cluster/client.go`
- Modify: `internal/cluster/client_test.go`

**Interfaces:**

```go
type Inventory struct {
    Resources   []Resource
    Unsupported []Unsupported
}

func (i Inventory) ValidateComparable() error
func BuildLocal(LocalRequest) (Inventory, error)
func Canonicalize(kind Kind, raw []byte) ([]byte, error)
func DigestCanonical(kind Kind, raw []byte) (string, error)
func ResourceIDs(kind Kind, raw []byte) ([]string, error)
```

- [ ] Write failing tests for configured recursive paths, process-ID identity, semantic-equivalent XML, multi-process files, duplicate IDs, ID-less/invalid resources, list/XML/pagination failures, and explicit unsupported kinds.
- [ ] Include regressions `TestRemoteInventoryXMLFailureIsError`, `TestInventoryRejectsEmptyDigest`, and `TestUnsupportedKindIsNotComparable`.
- [ ] Run `go test ./internal/inventory ./internal/cluster -run 'Test(Inventory|Canonical|Remote)' -count=1`; expect false empty-digest success.
- [ ] Implement canonical local/remote inventory. A successfully empty remote list is valid only after a fully decoded response.
- [ ] Re-run tests; expect PASS.

### Task 4: Trustworthy plan service

**Files:**
- Create: `internal/plan/service.go`
- Create: `internal/plan/format.go`
- Modify: `internal/plan/plan.go`
- Modify: `internal/plan/plan_test.go`

**Interfaces:**

```go
type Request struct {
    ProjectRoot string
    Environment string
}

func (s *Service) Run(context.Context, Request) (Result, error)
func Build(local, remote inventory.Inventory) (Result, error)
func FormatText(Result) string
```

- [ ] Add failing tests for create/update/delete/noop, semantic equivalence, running-instance context, unsupported kinds, missing digest, partial retrieval, auth/HTTP/malformed response, stable ordering, JSON, and no mutation calls.
- [ ] Add regression `TestBuildNeverNoopsUnknownRemoteState`.
- [ ] Run `go test ./internal/plan -count=1`; expect unknown remote state to become noop.
- [ ] Implement plan over validated inventories.
- [ ] Re-run tests; expect PASS.

### Task 5: Git-aware drift service

**Files:**
- Create: `internal/drift/git.go`
- Create: `internal/drift/service.go`
- Create: `internal/drift/format.go`
- Modify: `internal/drift/drift.go`
- Modify: `internal/drift/drift_test.go`

**Interfaces:**

```go
type GitRunner interface {
    Run(context.Context, string, ...string) ([]byte, error)
}

func (s *Service) Run(context.Context, Request) (Report, error)
func HasDrift(Report) bool
func HasUnknown(Report) bool
```

- [ ] Add fake-runner and real temporary Git repo tests for tracked restriction, untracked report, dirty files, `--cached` HEAD content, non-Git mode, semantic equivalence, and remote UNKNOWN.
- [ ] Run `go test ./internal/drift -count=1`; expect missing Git/cached behavior and false in-sync.
- [ ] Implement stable statuses `IN_SYNC|DRIFT|LOCAL_ONLY|CLUSTER_ONLY|UNKNOWN`; unknown/tool failure exits `2`.
- [ ] Re-run tests; expect PASS.

### Task 6: Complete incidents service

**Files:**
- Create: `internal/incidents/service.go`
- Create: `internal/incidents/format.go`
- Modify: `internal/incidents/incidents.go`
- Modify: `internal/incidents/incidents_test.go`
- Create: `internal/cluster/incidents.go`

**Interfaces:**

```go
func (s *Service) List(context.Context, ListRequest) (Result, error)
func (s *Service) Show(ctx context.Context, envName, projectRoot, key string) (Incident, error)
func (s *Service) Resolve(ctx context.Context, envName, projectRoot, key string) (Result, error)
func OperateLink(baseURL, processInstanceKey, incidentKey string) (string, error)
```

- [ ] Write failing tests for limit validation, list/show formats, escaped Operate links, resolve refresh, partial refresh failure, compatibility fallback boundaries, and preserved API status/messages.
- [ ] Run `go test ./internal/incidents ./internal/cluster -run 'Test(Incident|Operate)' -count=1`; expect missing show/refresh/link behavior.
- [ ] Implement service and keep confirmation at CLI/API/UI edges.
- [ ] Re-run tests; expect PASS.

### Task 7: Shared trace derivation and bounded follow

**Files:**
- Create: `internal/trace/service.go`
- Create: `internal/trace/follow.go`
- Create: `internal/trace/format.go`
- Modify: `internal/trace/trace.go`
- Modify: `internal/trace/trace_test.go`
- Create: `internal/cluster/trace.go`

**Interfaces:**

```go
func (s *Service) Get(context.Context, Request) (Timeline, error)
func (s *Service) Follow(
    context.Context,
    Request,
    time.Duration,
    func(Timeline) error,
) error
```

- [ ] Add fake-clock tests for chronological ordering, incident state/message, not-found vs failure, interval, cancellation, completion, timeout, changed-only emission, ASCII, and JSON.
- [ ] Run `go test ./internal/trace ./internal/cluster -run 'Test(Trace|Follow|Timeline)' -count=1`; expect CLI/API state divergence.
- [ ] Implement one derivation path shared by all edges.
- [ ] Re-run tests; expect PASS.

### Task 8: Complete backup feature contract

**Files:**
- Build on: `internal/backup/manifest.go`, `restore.go`
- Create: `internal/backup/create.go`
- Create: `internal/backup/validate.go`
- Modify: `internal/backup/backup_test.go`

- [ ] Add failing tests for `.camunda.yaml`, configured recursive BPMN/DMN/form paths, secret omission/opt-in, atomic mode-`0600` output, running-check errors, and CLI/multipart round trip.
- [ ] Run `go test ./internal/backup -count=1`; expect configured paths and transactional output gaps.
- [ ] Implement context-aware `Create`, `ValidateArchive`, and `Service.Restore` without duplicating safety-plan validation.
- [ ] Re-run tests; expect PASS.

### Task 9: Complete Kubernetes service

**Files:**
- Create: `internal/k8s/runner.go`
- Create: `internal/k8s/components.go`
- Create: `internal/k8s/service.go`
- Create: `internal/k8s/errors.go`
- Modify: `internal/k8s/k8s.go`
- Modify: `internal/k8s/k8s_test.go`

**Interfaces:**

```go
type Runner interface {
    Run(context.Context, ...string) (stdout, stderr string, err error)
    LookPath(string) (string, error)
}

func (s *Service) Status(context.Context, Options) (Result, error)
func (s *Service) Logs(context.Context, Options, string, bool, int) (Result, error)
func (s *Service) Restart(context.Context, Options, string) (Result, error)
func (s *Service) Scale(context.Context, Options, string, int) (Result, error)
```

- [ ] Write recording-runner tests proving context, namespace, release, selector, follow/tail, replicas, aliases, and typed missing-tool/context/auth/resource errors.
- [ ] Run `go test ./internal/k8s -count=1`; expect options to be discarded or static names used.
- [ ] Implement consistent argv construction and preserve stderr.
- [ ] Re-run tests; expect PASS.

### Task 10: Complete CLI/API/UI platform edges

**Files:**
- Split: `internal/cli/toolkit.go` into `env.go`, `plan.go`, `drift.go`, `incidents.go`, `trace.go`, `backup.go`, `k8s.go`
- Create: `internal/cli/dependencies.go`
- Create: `internal/cli/platform_test.go`
- Create: `internal/ui/api/handlers_platform.go`
- Create: `internal/ui/api/platform_types.go`
- Create: `internal/ui/api/handlers_platform_test.go`
- Modify: `internal/ui/web/src/pages/Cluster.tsx`
- Modify: `internal/ui/web/src/pages/Project.tsx`

- [ ] Add failing Cobra tests for every documented env/plan/drift/incidents/trace/backup/restore/k8s flag and exit behavior.
- [ ] Add failing HTTP tests for every field, status, confirmation, attachment, upstream failure, and no temp-path leakage.
- [ ] Wire thin edges to domain services and active environment factory.
- [ ] Expose all UI controls, explicit confirmations, incident refresh, bounded trace follow, backup download, restore upload, and Kubernetes logs/options.
- [ ] Run:

  ```bash
  go test ./internal/env ./internal/config ./internal/cluster ./internal/inventory ./internal/plan ./internal/drift ./internal/incidents ./internal/trace ./internal/backup ./internal/k8s ./internal/cli ./internal/ui/api -count=1
  go test ./...
  git diff --check
  ```

  Expected: all PASS; unknown remote state never reports success.

- [ ] Review checkpoint; commit only if explicitly requested.
