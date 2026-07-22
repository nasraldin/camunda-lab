# Platform Toolkit Completion Program

> **For agentic workers:** REQUIRED SUB-SKILL: Execute the linked plans in order with `superpowers:subagent-driven-development` (recommended) or `superpowers:executing-plans`. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete every committed Phase 1–3 feature in PR #16 and prove CLI, API, and Lab UI behavior in isolated light and full environments.

**Architecture:** Four independently testable plans stabilize safety, finish developer tooling, finish platform operations, and then enforce parity plus live acceptance. Domain contracts land before CLI/API/UI edges. No plan may convert partial, unsupported, or failed work into success, clean, in-sync, or noop.

**Tech Stack:** Go 1.24, Cobra, `net/http`, React 19, TypeScript strict mode, Vite, Playwright with Chrome, Docker Compose, Camunda 8.9 full/light profiles.

## Global Constraints

- Product contract: `docs/superpowers/specs/2026-07-21-platform-toolkit-completion-design.md`.
- Detailed Phase 1–3 plans, user-facing docs, CLI help, API routes, and UI claims jointly define committed behavior.
- Explicit **Later**, **Out of scope**, and hard non-goal items remain excluded.
- Keep CLI commands and HTTP handlers thin; domain behavior stays in focused Go packages.
- Use red-green-refactor for every behavior change.
- No paid AI calls in tests or CI.
- Never expose raw secrets or absolute temporary server paths.
- Live tests use unique `CAMUNDA_LAB_HOME`, project directories, Compose project names, and ownership manifests.
- Never reset, stop, or delete the user’s normal lab or unrelated Docker resources.
- Do not commit unless the user explicitly requests it.

## Plans and dependency order

1. [Safety foundation](./2026-07-21-platform-toolkit-safety.md)
   - Archive/env traversal, transactional restore, Host/Origin/CSRF, symlink authorization, real confirmations.
2. [Developer tooling](./2026-07-21-platform-toolkit-developer-tools.md)
   - Project discovery, BPMN IR, lint, diff, explain, review AI, test generation, scan, deep doctor.
3. [Platform operations](./2026-07-21-platform-toolkit-platform-ops.md)
   - Environment precedence/OIDC, trustworthy inventory/plan/drift, incidents, trace, backup.
4. [Parity and acceptance](./2026-07-21-platform-toolkit-parity-acceptance.md)
   - CLI/API/UI contracts, browser automation, docs truthfulness, isolated light/full verification.

Plans 2 and 3 may proceed in parallel only after Plan 1 has established the security contracts they consume. Plan 4 begins after stable domain request/result contracts exist.

## Program gates

- [ ] **Gate 1: Safety**

  Run:

  ```bash
  go test ./internal/backup ./internal/env ./internal/ui/api ./internal/ui ./internal/cli -count=1
  ```

  Expected: PASS, including malicious archive, profile traversal, Host, Origin, CSRF, and symlink tests.

- [ ] **Gate 2: Developer tools**

  Run:

  ```bash
  go test ./internal/project ./internal/bpmn ./internal/lint ./internal/diff ./internal/explain ./internal/review ./internal/testgen ./internal/scan ./internal/doctor ./internal/toolkit -count=1
  ```

  Expected: PASS with complete BPMN and failure-path fixtures.

- [ ] **Gate 3: Platform operations**

  Run:

  ```bash
  go test ./internal/env ./internal/config ./internal/cluster ./internal/inventory ./internal/plan ./internal/drift ./internal/incidents ./internal/trace ./internal/backup -count=1
  ```

  Expected: PASS; remote failures never produce noop or in-sync.

- [ ] **Gate 4: Repository**

  Run:

  ```bash
  make check
  make ui-check
  cd internal/ui/web && npm run check
  git diff --check
  ```

  Expected: PASS with current embedded UI assets, frontend typecheck/lint/Playwright mock, and no whitespace errors.

- [ ] **Gate 5: Live acceptance**

  Run:

  ```bash
  make acceptance-light
  make acceptance-full
  ```

  Expected: light/full summaries pass. Cleanup proof shows no unrelated resource changed.

## Merge condition

PR #16 remains unmergeable until all four plans and five gates pass. Any skipped acceptance check remains an open gap; documentation must not call the feature shipped.
