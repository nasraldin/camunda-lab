# Platform Toolkit Safety Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make archive restore, environment storage, filesystem authorization, and the localhost control plane safe before live feature acceptance.

**Architecture:** Add bounded transactional restore, one environment identity/state service, canonical path authorization, and one HTTP security middleware. CLI, handlers, and React consume these primitives without duplicating safety logic.

**Tech Stack:** Go 1.24, `archive/tar`, `net/http`, Cobra, React 19, TypeScript, Playwright.

## Global Constraints

- Validate complete inputs before mutating final destinations.
- Unsupported archive types and unsafe paths fail closed.
- `--force` may bypass a running-lab gate, never archive validation.
- Camunda Lab API CSRF protection is separate from the Compose overlay that disables Camunda application CSRF.
- Browser mutations require deliberate confirmation; a prefilled `confirm: true` is not confirmation.
- Do not commit unless explicitly requested.

---

### Task 1: Transactional bounded archive restore

**Files:**
- Create: `internal/backup/manifest.go`
- Create: `internal/backup/restore.go`
- Create: `internal/backup/restore_test.go`
- Modify: `internal/backup/backup.go`
- Modify: `internal/backup/backup_test.go`

**Interfaces:**
- Produces:

  ```go
  type Limits struct {
      MaxEntries    int
      MaxFileBytes  int64
      MaxTotalBytes int64
  }

  type RestoreOptions struct {
      ArchivePath string
      LabHome     string
      ProjectDir  string
      Force       bool
      Limits      Limits
      Lab         RunningChecker
  }

  type RunningChecker interface {
      Running(context.Context) (bool, error)
  }

  func DefaultLimits() Limits
  func Restore(context.Context, RestoreOptions) (Manifest, error)
  ```

- [ ] **Step 1: Write malicious-archive and atomicity tests**

  Add table cases for absolute paths, `..`, backslashes, mixed separators, symlink, hardlink, device, FIFO, duplicate destination, negative/oversized entries, missing/invalid manifest, unsupported version, and payload mismatch. Assert `config.yaml`, `ai.env`, and project files remain unchanged after every rejection.

- [ ] **Step 2: Verify RED**

  Run:

  ```bash
  go test ./internal/backup -run 'TestRestore(Rejects|Validates|Requires)' -count=1 -v
  ```

  Expected: FAIL because unsafe entries are accepted or final destinations are modified before full validation.

- [ ] **Step 3: Implement validation and staged extraction**

  Use defaults of 10,000 entries, 64 MiB per file, and 512 MiB total. First pass validates and records canonical destinations; second pass extracts regular files/directories into mode-`0700` staging roots. Commit by same-filesystem rename with rollback. Reject every link/device/unknown tar type and every backslash.

- [ ] **Step 4: Harden archive creation**

  Create output with mode `0600`; propagate walk/read/write errors; reject project symlinks; ensure manifest contents exactly match archive payload; keep secret values opt-in.

- [ ] **Step 5: Verify GREEN**

  Run:

  ```bash
  go test ./internal/backup -count=1
  ```

  Expected: PASS with no staging/rollback artifacts.

### Task 2: Running-lab restore gate

**Files:**
- Modify: `internal/lab/containers.go`
- Create: `internal/lab/containers_running_test.go`
- Modify: `internal/cli/toolkit.go`
- Create: `internal/cli/toolkit_safety_test.go`

**Interfaces:**
- Consumes: `backup.Restore(context.Context, backup.RestoreOptions)`.
- Produces:

  ```go
  func (l *Lab) Running(context.Context) (bool, error)
  ```

- [ ] **Step 1: Write running/stopped/error tests**

  Assert any running Compose container returns true, empty/stopped state returns false, Compose errors propagate, restore refuses without force, and `--force` still rejects a malicious archive.

- [ ] **Step 2: Verify RED**

  ```bash
  go test ./internal/lab ./internal/backup ./internal/cli -run 'Test(Running|Restore)' -count=1 -v
  ```

  Expected: FAIL because the running check and `--force` do not exist.

- [ ] **Step 3: Implement thin adapters and CLI flags**

  Add `restore --project`, `--yes/-y`, and `--force`. Interactive mode requires exact `RESTORE`; a running lab returns: `lab is running; stop it first with "camunda down" or retry with --force`.

- [ ] **Step 4: Verify GREEN**

  Re-run the focused command; expected PASS.

### Task 3: Safe environment identities and atomic active state

**Files:**
- Create: `internal/env/name.go`
- Create: `internal/env/active.go`
- Create: `internal/env/name_test.go`
- Create: `internal/env/active_test.go`
- Modify: `internal/env/profile.go`
- Modify: `internal/env/profile_test.go`
- Modify: `internal/cli/toolkit.go`
- Modify: `internal/ui/api/handlers_toolkit.go`

**Interfaces:**
- Produces:

  ```go
  func ValidateName(name string) error
  func ProfilePath(dir, name string) (string, error)
  func LoadNamedProfile(dir, name string) (Profile, error)
  func GetActive(labHome string) (string, error)
  func SetActive(labHome, profilesDir, name string) error
  func RemoveProfile(labHome, profilesDir, name string) error
  ```

- [ ] **Step 1: Write identity and transaction tests**

  Reject empty, dot segments, slash/backslash, percent-encoded separators, controls, surrounding whitespace, over-64-byte names, and reserved stored names `lab`, `config`, `active-env`, `envs`. Test filename/profile-name mismatch, profile symlink, unknown activation, active removal fallback, and rollback when active-state persistence fails.

- [ ] **Step 2: Verify RED**

  ```bash
  go test ./internal/env ./internal/cli ./internal/ui/api -run 'Test(ValidateName|SaveProfile|LoadNamed|SetActive|RemoveActive|EnvUse|EnvRemove)' -count=1 -v
  ```

  Expected: FAIL because traversal names and dangling active profiles are accepted.

- [ ] **Step 3: Implement one validator and atomic operations**

  Stored names use lowercase dot-separated labels beginning/ending in `[a-z0-9]` with internal `-` or `_`. `lab` is allowed only as the implicit active profile. Remove active profiles by tombstone rename, atomic fallback write, then tombstone deletion; roll back on failure.

- [ ] **Step 4: Route all CLI/API callers through the service**

  Remove direct filename joins and direct `active-env` writes from edge code. Propagate corrupted active-state errors.

- [ ] **Step 5: Verify GREEN**

  Re-run the focused command; expected PASS.

### Task 4: Canonical symlink-safe path authorization

**Files:**
- Modify: `internal/ui/api/pathsafety.go`
- Modify: `internal/ui/api/pathsafety_test.go`

**Interfaces:**
- Produces:

  ```go
  func allowPathWithin(path string, roots []string) (string, error)
  func canonicalizeForAuthorization(path string) (string, error)
  ```

- [ ] **Step 1: Write symlink escape tests**

  Cover existing symlink escape, nonexistent child through symlink, inside-root symlink, Darwin `/tmp` canonicalization, and root-prefix collisions.

- [ ] **Step 2: Verify RED**

  ```bash
  go test ./internal/ui/api -run TestAllowPath -count=1 -v
  ```

  Expected: FAIL because lexical containment accepts an escaping symlink.

- [ ] **Step 3: Implement canonical authorization**

  Resolve existing paths with `filepath.EvalSymlinks`. For nonexistent write targets, resolve the nearest existing ancestor and append the missing suffix before `filepath.Rel` containment checks.

- [ ] **Step 4: Verify GREEN**

  Re-run the focused command; expected PASS.

### Task 5: Host, Origin, and CSRF middleware

**Files:**
- Create: `internal/ui/api/security.go`
- Create: `internal/ui/api/security_test.go`
- Modify: `internal/ui/api/handlers.go`
- Modify: `internal/ui/server.go`
- Modify: `internal/ui/server_test.go`

**Interfaces:**
- Produces:

  ```go
  const CSRFHeader = "X-Camunda-Lab-CSRF"

  func NewCSRFToken() (string, error)
  func SecurityMiddleware(csrfToken string, next http.Handler) http.Handler
  ```

- [ ] **Step 1: Write hostile-request tests**

  Test literal loopback hosts with optional ports, DNS-rebinding/malformed hosts, read-only requests, missing/foreign Origin, missing/invalid token, same-origin JSON/form/multipart/DELETE, and `GET /api/v1/session`.

- [ ] **Step 2: Verify RED**

  ```bash
  go test ./internal/ui/api ./internal/ui -run 'Test(Security|Session|ServerHandler)' -count=1 -v
  ```

  Expected: FAIL because hostile mutations currently reach handlers.

- [ ] **Step 3: Implement middleware**

  Accept only `localhost`, `127.0.0.1`, or `[::1]`. Return `421 invalid_host` for Host failures and `403 invalid_origin|csrf_missing|csrf_invalid` for mutation failures. Generate one random 32-byte token per UI process.

- [ ] **Step 4: Verify GREEN**

  Re-run the focused command; expected PASS.

### Task 6: Frontend CSRF transport and deliberate confirmations

**Files:**
- Modify: `internal/ui/web/src/api.ts`
- Modify: `internal/ui/web/vite.config.ts`
- Modify: `internal/ui/web/package.json`
- Modify: `internal/ui/web/package-lock.json`
- Create: `internal/ui/web/playwright.config.ts`
- Create: `internal/ui/web/e2e/safety.spec.ts`
- Create: `internal/ui/web/src/components/ConfirmActionModal.tsx`
- Modify: `internal/ui/web/src/pages/Project.tsx`
- Modify: `internal/ui/web/src/pages/Cluster.tsx`
- Modify: `internal/ui/web/src/pages/Containers.tsx`
- Modify: `internal/ui/web/src/pages/Overview.tsx`
- Modify: `internal/ui/web/src/pages/Setup.tsx`
- Modify: `internal/ui/web/src/pages/Danger.tsx`
- Test through Playwright files created by the parity plan.

**Interfaces:**
- Consumes: `GET /api/v1/session` and `X-Camunda-Lab-CSRF`.
- Produces:

  ```ts
  type ConfirmAction = {
    title: string;
    message: string;
    requiredText?: string;
    confirmLabel: string;
    run: () => Promise<void>;
  };
  ```

- [ ] **Step 1: Add failing browser assertions**

  Assert no request occurs on restore file selection or before confirming incident resolution, env removal, K8s restart/scale, service restart, lab stop/restart, wipe-switch, and reset. Assert mutation requests carry CSRF.

- [ ] **Step 2: Add the minimal Chrome harness**

  Run:

  ```bash
  cd internal/ui/web
  npm install --save-dev @playwright/test
  ```

  Add `test:browser: "playwright test"` and configure `channel: "chrome"`, isolated browser contexts, and trace/screenshot retention on failure.

- [ ] **Step 3: Verify RED**

  ```bash
  cd internal/ui/web && npm run test:browser -- --grep 'confirmation|csrf'
  ```

  Expected: FAIL because actions currently mutate immediately and omit CSRF.

- [ ] **Step 4: Implement transport and modal**

  Cache the session token, attach it to every mutation including multipart and DELETE, refresh once on `csrf_invalid`, preserve multipart boundaries, trap focus, support Escape/cancel, restore focus, and require typed `RESTORE`/`DELETE` where specified.

- [ ] **Step 5: Preserve Vite same-origin behavior**

  Rewrite proxied Host and Origin to `http://127.0.0.1:9090`; do not weaken production middleware.

- [ ] **Step 6: Verify GREEN**

  Re-run the browser subset; expected PASS.

### Task 7: Safety documentation and gate

**Files:**
- Modify: `docs/cli-reference.md`
- Modify: `docs/lab-ui.md`
- Modify: `docs/troubleshooting.md`
- Modify: `internal/ui/web/dist/**`

- [ ] **Step 1: Document exact contracts**

  Document restore limits, `--force`, typed confirmation, running-lab refusal, archive contents/exclusions, Host/Origin/CSRF behavior, and its distinction from Camunda application CSRF.

- [ ] **Step 2: Run the safety gate**

  ```bash
  go test ./internal/backup ./internal/env ./internal/lab ./internal/ui/api ./internal/ui ./internal/cli -count=1
  go test ./...
  make ui-check
  git diff --check
  ```

  Expected: all PASS; no temp archive/staging files remain.

- [ ] **Step 3: Review checkpoint**

  Inspect only this plan’s diff. Commit only if the user explicitly requests it.
