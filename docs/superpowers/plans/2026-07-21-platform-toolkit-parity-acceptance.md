# Platform Toolkit Parity and Acceptance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans` to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Lock CLI/API/React parity, add repeatable Chrome coverage, make documentation truthful, and prove completed features in isolated light and full Camunda profiles.

**Architecture:** Stable domain requests/results feed thin Cobra and HTTP edges; typed React clients render explicit states. Go integration tests cover routes and security, while Playwright drives installed Chrome and captures network, console, accessibility, download, and cleanup evidence.

**Tech Stack:** Go `testing`/`httptest`, React 19, TypeScript strict mode, ESLint flat config, Playwright with Chrome, `@axe-core/playwright`, Docker Compose.

## Global Constraints

- Do not add a second frontend unit-test framework; use Go route tests plus Playwright browser workflows.
- Browser tests fail on unexpected console errors/warnings, page errors, or unhandled `4xx/5xx`.
- Downloads contain no absolute server paths or traversal entries.
- “Shipped” claims require automated and live acceptance evidence.
- Live scripts use ownership markers and never run broad Docker/Kubernetes cleanup.
- Do not commit unless explicitly requested.

---

### Task 1: Characterize complete CLI/API parity

**Files:**
- Create: `internal/cli/toolkit_contract_test.go`
- Create: `internal/cli/toolkit_execution_test.go`
- Create: `internal/ui/api/routes_toolkit_test.go`
- Create: `internal/toolkit/parity_test.go`

**Interfaces:**
- Consumes stable domain request/result types from the developer-tools and platform-ops plans.
- Produces an executable inventory of every promised command, flag, route, field, status, JSON shape, and exit code.

- [ ] **Step 1: Write failing table-driven contract tests**

  Cover lint, diff, explain, review, test generation, scan, deep doctor, env, plan, drift, incidents, trace, backup/restore, and k8s. For equivalent CLI/API inputs, assert normalized domain requests/results match.

- [ ] **Step 2: Verify RED**

  ```bash
  go test ./internal/cli ./internal/ui/api ./internal/toolkit -run 'Test(ToolkitContract|ToolkitExecution|ToolkitRoutes|Parity)' -count=1 -v
  ```

  Expected: FAIL for missing flags, routes, status mappings, and divergent trace/result behavior.

- [ ] **Step 3: Wire thin edge dependencies**

  Add `NewRootWithDependencies` and `api.NewHandler(version, dependencies)` so tests inject fake services. Keep formatting at the edge.

- [ ] **Step 4: Verify GREEN**

  Re-run the focused command; expected PASS.

### Task 2: CLI exit and JSON contracts

**Files:**
- Create: `internal/cli/execute.go`
- Create: `internal/cli/exit_test.go`
- Modify: `cmd/camunda/main.go`

**Interfaces:**

```go
func Run(ctx context.Context, args []string, stdout, stderr io.Writer) int
func ExitCode(error) int
```

- [ ] Write failing tests for clean `0`, findings/diff/drift/incident `1`, and validation/upstream/partial/unknown tool errors `2`; assert JSON has no human preamble.
- [ ] Run `go test ./internal/cli -run TestExit -count=1`; expect all errors to map to `1`.
- [ ] Implement typed exit mapping and make `main` call `os.Exit(cli.Run(...))`.
- [ ] Re-run tests; expect PASS.

### Task 3: Full HTTP integration and stable errors

**Files:**
- Create: `internal/ui/api/dependencies.go`
- Create: `internal/ui/api/errors_test.go`
- Expand: `internal/ui/api/routes_toolkit_test.go`
- Modify: `internal/ui/api/handlers.go`
- Modify: `internal/ui/api/handlers_toolkit.go`
- Modify: `internal/ui/server_test.go`

**Wire contract:**

```json
{
  "ok": false,
  "code": "invalid_request",
  "error": "name is required",
  "hint": "..."
}
```

- [ ] Add route cases for valid success, invalid/unknown fields, missing resources, conflicts, unsupported capabilities, upstream failures, mutation security, stable schemas, no leaked temp paths, and no leftover files.
- [ ] Add SPA hard-refresh tests for `/`, `/bpmn`, `/cluster`, and `/project`; missing `/api/...` remains `404`.
- [ ] Run `go test ./internal/ui/api ./internal/ui -count=1`; expect missing route/error/hard-refresh coverage to fail.
- [ ] Implement status mapping: `400`, `403`, `404`, `409`, `413`, `422`, `502`, `500`; completed drift may return `200` with drift, but incomplete remote state must not.
- [ ] Re-run tests; expect PASS.

### Task 4: Secure browser artifact downloads

**Files:**
- Create: `internal/toolkit/artifact.go`
- Create: `internal/ui/api/downloads_test.go`
- Create: `internal/testgen/artifact_test.go`
- Modify: `internal/ui/api/handlers_toolkit.go`
- Modify: `internal/backup/backup_test.go`

**Routes:**

```text
POST /api/v1/bpmn/test-generate/download -> application/zip
POST /api/v1/backup/download             -> application/gzip
```

- [ ] Write failing tests for ZIP/gzip signatures, sanitized `Content-Disposition`, `nosniff`, deterministic entries, secret opt-in, permissions, cleanup, traversal rejection, and no server-local path.
- [ ] Run `go test ./internal/testgen ./internal/backup ./internal/ui/api -run 'Test(Artifact|Download|Backup)' -count=1`; expect JSON temp-path responses and leaks.
- [ ] Implement stream/download routes and authorized write routes separately; delete temporary server files after streaming.
- [ ] Re-run tests; expect PASS.

### Task 5: Typed React API and explicit result states

**Files:**
- Create: `internal/ui/web/src/api/client.ts`
- Create: `internal/ui/web/src/api/toolkit.ts`
- Create: `internal/ui/web/src/api/types.ts`
- Create: `internal/ui/web/src/components/ActionResult.tsx`
- Create: `internal/ui/web/src/components/ConfirmDialog.tsx`
- Create: `internal/ui/web/src/components/DownloadButton.tsx`
- Modify: `internal/ui/web/src/api.ts`
- Modify: `internal/ui/web/src/pages/Bpmn.tsx`
- Modify: `internal/ui/web/src/pages/Cluster.tsx`
- Modify: `internal/ui/web/src/pages/Project.tsx`
- Modify: `internal/ui/web/src/pages/Overview.tsx`
- Modify: `internal/ui/web/src/components/Modal.tsx`

- [ ] Add Playwright fixtures that return loading, empty, success, findings, partial, unsupported, and failure responses.
- [ ] Run the targeted browser specs from Task 7; expect untyped/merged states and missing controls.
- [ ] Implement typed requests/results, every UI-relevant CLI option, copy/text download, ZIP/gzip download, controlled trace refresh, deliberate confirmations, focus trap/Escape/focus restoration, and dynamic Camunda version defaults.
- [ ] Re-run targeted specs; expect PASS.

### Task 6: Frontend quality and Chrome setup

**Files:**
- Modify: `internal/ui/web/package.json`
- Modify: `internal/ui/web/package-lock.json`
- Create: `internal/ui/web/eslint.config.js`
- Modify: `internal/ui/web/playwright.config.ts`
- Create: `internal/ui/web/e2e/helpers/api.ts`
- Create: `internal/ui/web/e2e/helpers/console.ts`

**Dependencies:**

```bash
npm install --save-dev @axe-core/playwright eslint typescript-eslint eslint-plugin-react-hooks
```

- [ ] Add scripts:

  ```json
  {
    "typecheck": "tsc --noEmit",
    "lint": "eslint .",
    "test:browser": "playwright test",
    "test:browser:live": "playwright test --project=live",
    "check": "npm run typecheck && npm run lint && npm run test:browser"
  }
  ```

- [ ] Configure `channel: "chrome"`, isolated contexts, trace/screenshot/HAR on failure, download directories, and zero local retries.
- [ ] Run:

  ```bash
  cd internal/ui/web
  npm run typecheck
  npm run lint
  npx playwright test --list
  ```

  Expected: typecheck/lint PASS and all browser specs listed.

### Task 7: Browser workflow coverage

**Files:**
- Create: `internal/ui/web/e2e/navigation.spec.ts`
- Create: `internal/ui/web/e2e/bpmn.spec.ts`
- Create: `internal/ui/web/e2e/cluster.spec.ts`
- Create: `internal/ui/web/e2e/project.spec.ts`
- Create: `internal/ui/web/e2e/downloads.spec.ts`
- Create: `internal/ui/web/e2e/security.spec.ts`

- [ ] Add assertions that fail on `pageerror`, unexpected console warning/error, uncaught `4xx/5xx`, failed axe checks, keyboard/focus defects, route hard refresh, invisible borders, and unsafe cross-origin mutations.
- [ ] Cover all BPMN workflows, environment/plan/drift/incidents/trace, backup/restore, Kubernetes status/logs/options/mutations, light/dark themes, and all explicit result states.
- [ ] Validate downloaded ZIP/gzip opens, is nonempty, and has no unsafe entries.
- [ ] Run:

  ```bash
  cd internal/ui/web && npm run test:browser
  ```

  Expected: PASS with Chrome.

### Task 8: Ownership-safe acceptance harness

**Files:**
- Create: `scripts/acceptance/lib.sh`
- Create: `scripts/acceptance/platform-toolkit-light.sh`
- Create: `scripts/acceptance/platform-toolkit-full.sh`
- Create: `scripts/acceptance/platform-toolkit-k8s.sh`
- Create: `scripts/acceptance/cleanup.sh`
- Create fixtures under: `scripts/acceptance/fixtures/`
- Modify: `Makefile`

**Run contract:**

```text
unique CAMUNDA_LAB_HOME
unique project root
unique Compose project
random UI/DevTools ports
isolated Chrome profile
ownership manifest
```

- [ ] Write shell self-tests that reject cleanup without the ownership marker, outside the accepted temp prefix, against mismatched PIDs/Compose projects, or against an unlabeled Kubernetes namespace.
- [ ] Run those tests; expect RED until ownership validation exists.
- [ ] Implement preflight that fails rather than stopping occupied ports, `trap EXIT INT TERM`, before/created/after inventories, exact PID command-line checks, and exact Compose project cleanup. Never use `docker system prune`.
- [ ] Add Make targets:

  ```make
  acceptance-light:
  	./scripts/acceptance/platform-toolkit-light.sh

  acceptance-full:
  	./scripts/acceptance/platform-toolkit-full.sh

  acceptance-k8s:
  	./scripts/acceptance/platform-toolkit-k8s.sh
  ```

- [ ] Re-run shell tests; expect PASS.

### Task 9: Chrome DevTools live acceptance

**Files:**
- Create: `docs/acceptance/platform-toolkit-parity.md`
- Write ignored artifacts under: `artifacts/acceptance/<run-id>/`

- [ ] Run `make acceptance-light`.

  Verify install/up/wait/smoke, Home deep diagnostics, endpoint cards, all BPMN workflows, project/env workflows, test ZIP, backup/restore round trip, hard refresh, themes, network, console, and accessibility.

- [ ] Run `make acceptance-full`.

  Verify OIDC token acquisition, endpoint probes, representative process seeding via official tooling, incidents list/show/resolve/refresh, trace/follow, plan, drift, and browser checks.

- [ ] Run `make acceptance-k8s`.

  Use an explicitly disposable context; otherwise record deterministic fake-runner evidence and mark live mutation unavailable rather than passing it silently.

- [ ] Confirm each artifact directory contains:

  ```text
  commands.log
  network.har
  console.json
  axe.json
  Playwright trace
  light/dark screenshots
  generated test ZIP
  backup gzip
  Docker/Kubernetes inventories
  summary.json
  ```

- [ ] Confirm cleanup proof shows only owned resources were removed.

### Task 10: Documentation truthfulness and final gate

**Files:**
- Create: `docs/api-reference.md`
- Create: `internal/cli/docs_contract_test.go`
- Create: `internal/ui/api/docs_contract_test.go`
- Modify: `README.md`
- Modify: `docs/cli-reference.md`
- Modify: `docs/lab-ui.md`
- Modify: `docs/architecture.md`
- Modify: `docs/roadmap.md`
- Modify relevant checklists under: `docs/superpowers/plans/`
- Modify: `.github/workflows/ci.yml`

- [ ] Add failing docs-contract tests requiring every public toolkit command/flag and API route in reference docs.
- [ ] Change unverified “shipped” claims to “in progress”; restore “shipped” only after Task 9 passes.
- [ ] Document exit codes, API fields/statuses, unsupported inventory, OIDC references, backup exclusions/limits, downloads, and confirmations.
- [ ] Add CI frontend typecheck/lint/browser mocked-contract gates; keep heavy live acceptance scheduled/manual.
- [ ] Run:

  ```bash
  go test ./...
  make check
  make ui-check
  cd internal/ui/web && npm run check
  git diff --check
  ```

  Expected: all PASS.

- [ ] Review all four plan checklists against `docs/superpowers/specs/2026-07-21-platform-toolkit-completion-design.md`; no uncovered requirement or placeholder remains.
- [ ] Review checkpoint; commit only if explicitly requested.
