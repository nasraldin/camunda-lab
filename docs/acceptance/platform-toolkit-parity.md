# Platform toolkit parity — acceptance status

**Run ID:** `a9-20260722-114659`  
**Date:** 2026-07-22  
**Artifacts:** `artifacts/acceptance/a9-20260722-114659/` (gitignored)

This document records what passed in the current environment versus what remains for a **disposable live** acceptance run (Chrome DevTools, OIDC, incidents/trace). It does **not** claim live mutation or live cluster workflows passed without evidence.

## Summary

| Layer | Status | Evidence |
|-------|--------|----------|
| A8 harness self-tests | **PASS** | `make acceptance-self-test`, `artifacts/.../a9-selftest.log` |
| `make acceptance-light` | Harness **PASS**; live **UNAVAILABLE** | `light/unavailable-live-light.txt`, `light/install.log` |
| `make acceptance-full` | Harness **PASS**; live **UNAVAILABLE** | `full/unavailable-live-full.txt`, `full/install.log` |
| Go contract/parity suites | **PASS** | `a9-go-tests.log` |
| Playwright mock (A7) | **PASS** (58/58) | `a9-playwright.log`, `internal/ui/web/playwright-report/` |
| Chrome DevTools live | **UNAVAILABLE** | No live UI stack; harness does not emit DevTools artifacts yet |
| Live OIDC / incidents / trace | **UNAVAILABLE** | Requires disposable Camunda stack + cluster credentials |

**Overall A9 gate:** **partial** — automated mock and Go evidence is green; live Chrome DevTools acceptance is **not** complete. **A10** added docs-contract tests, `docs/api-reference.md`, and qualified roadmap/architecture claims; Gate 5 remains open.

## Environment

| Check | Value |
|-------|-------|
| Docker | Available |

## What passed

### A8 ownership-safe harness (self-tests)

Shell self-tests passed on every profile run:

- Rejects missing ownership marker, paths outside temp prefix, compose prefix mismatch, PID pattern/cmdline mismatch
- Valid manifest cleanup removes only owned temp dirs
- No `docker system prune` in harness scripts

### Go suites (platform toolkit parity)

```
go test ./internal/cli/... ./internal/ui/api/... ./internal/toolkit/... ./internal/backup/...
```

All packages **ok** (~4–7s each). Covers CLI developer contracts, API matrix/security/contract tests, toolkit parity, and backup round-trip contracts.

### Playwright mock browser contracts (A7)

```
cd internal/ui/web && npm run test:browser
```

**58/58 passed** on the `mock` project (Chrome channel). Coverage includes:

- BPMN lint/diff/explain/review/test-gen/scan workflows and result states
- Cluster incidents, trace, plan, drift (mocked API)
- Project scaffold, env profiles, backup/restore confirmations
- Navigation, themes, hard refresh, accessibility (axe) on all major routes
- Downloads (backup gzip, test-gen ZIP, action-result text)
- Safety and security (CSRF, confirmation gates, focus trap)

HTML report: `internal/ui/web/playwright-report/index.html`

### Harness cleanup proof

Each profile run wrote `inventory-before.txt` / `inventory-after.txt` and `ownership.json`. Cleanup logged removal of only prefixed compose projects (`camunda-lab-acc-*`) and temp dirs under `/tmp/camunda-lab-acceptance-*`. No foreign Docker prune.

## What was UNAVAILABLE (live)

### `make acceptance-light` — live-light

**Marker:** `light/unavailable-live-light.txt`

```
live install/wait/smoke failed
```

**Root cause** (`light/install.log`):

```
error: docker compose up: service "elasticvue" refers to undefined network camunda-platform: invalid compose project
```

Harness exited 0 after recording UNAVAILABLE (no silent pass).

### `make acceptance-full` — live-full

**Marker:** `full/unavailable-live-full.txt`

**Root cause** (`full/install.log`):

```
error: Another application on your computer is already using the Docker name "orchestration".
```

Likely a pre-existing container/network conflict on the host, not a mocked pass.

### Chrome DevTools live acceptance (Task 9 target)

Not executed. The A8 harness provisions ports (`devtools_port`) but does not yet produce:

- `network.har`
- `console.json`
- `axe.json` (live)
- Playwright `live` project traces
- Light/dark screenshots against a real stack

Playwright **mock** axe/console checks (A7) are linked above; they are not a substitute for live DevTools acceptance.

### Live workflows not verified

Without a disposable live stack, the following were **not** exercised:

| Workflow | Status |
|----------|--------|
| Install / up / wait / smoke (real Compose) | UNAVAILABLE (see install logs) |
| Home deep diagnostics, endpoint cards | UNAVAILABLE |
| OIDC token acquisition | UNAVAILABLE |
| Live incidents list/show/resolve/refresh | UNAVAILABLE |
| Live trace / follow | UNAVAILABLE |
| Live plan / drift against cluster | UNAVAILABLE |
| Backup/restore round trip (live filesystem) | UNAVAILABLE (mock download/restore UI only) |

## Artifact layout

```
artifacts/acceptance/a9-20260722-114659/
├── commands.log
├── summary.json
├── a9-*.log
├── light/   # install.log, unavailable-live-light.txt, inventories, ownership.json
├── full/
└── playwright/README.md
```

## Remaining for disposable live environment

1. **Docker host:** Free conflicting container names (`orchestration`) or use an isolated Docker context; fix or validate `camunda-platform` network wiring for light profile `elasticvue`.
2. **Live stack:** Run `acceptance-light` / `acceptance-full` to completion (install → wait → smoke → down).
3. **Chrome DevTools:** Extend harness or run `npm run test:browser:live` with `CAMUNDA_LAB_UI_URL` and capture HAR, console, axe, traces, screenshots.
4. **OIDC / incidents / trace:** Manual or scripted probes against the live stack with test credentials — record artifacts under a new `artifacts/acceptance/<run-id>/`.

## Commands to reproduce

```bash
make acceptance-self-test
make acceptance-light
make acceptance-full

go test ./internal/cli/... ./internal/ui/api/... ./internal/toolkit/... ./internal/backup/...

cd internal/ui/web && npm run test:browser
```

Expected in this environment: harness self-tests and Go/Playwright **PASS**; all `live-*` steps **UNAVAILABLE** with marker files.
