# Deep doctor (`camunda doctor --deep`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked.

**Goal:** Extend `camunda doctor` with `--deep` for component-level lab health (Identity, Operate, Tasklist, Elasticsearch, Zeebe/Orchestration, Connectors, OAuth/Keycloak, disk, overlays) while keeping default doctor fast and mostly offline.

**Architecture:** Keep existing Docker/config checks in `internal/doctor`. Deep mode reuses `internal/urls`, `internal/smoke`, `internal/compose` for HTTP probes, compose ps health, volume disk usage, optional AI/MCP probes. Structured sections: Healthy / Warnings / Failures + fix hints.

**Tech Stack:** Go, existing lab packages, HTTP client with short timeouts.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Default `camunda doctor` remains usable without a running lab (Docker + config)
- `--deep` requires lab config; probes running compose project `camunda-lab`
- Do not require Kubernetes; this is Compose-lab deep check (K8s has Phase 3 `camunda k8s`)
- Mask secrets; never dump Keycloak admin password in report
- HTTP timeouts short (2–5s per endpoint); overall `--timeout` flag
- Fix hints reference real commands: `camunda up`, `camunda wait`, `camunda logs`, `camunda ai status`

---

## Deep checks (MVP)

| Check | Source |
|-------|--------|
| Docker Engine + Compose v2 | existing |
| Lab config + version dir | existing |
| Compose services running / health | `compose ps` |
| Orchestration / Operate / Tasklist / Identity / Keycloak / Connectors / ES HTTP | `urls` + GET or health paths |
| gRPC port open (TCP dial) | urls grpc entry |
| ElasticVue if profile exposes ES | urls |
| MCP endpoints if `ai.enabled` | smoke warn-style |
| Disk usage of Docker volumes for project | `docker system df` / volume inspect best-effort |
| Overlay / version consistency notes | config vs adapter |

Helm version / Ingress / License — **skip** for Compose lab (document as N/A); relevant later under `k8s`.

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/doctor/doctor.go` | Extend Report; call Deep when flag set |
| `internal/doctor/deep.go` | Deep probes |
| `internal/doctor/deep_test.go` | Mock HTTP / fake compose where possible |
| `internal/cli/doctor.go` | `--deep`, `--timeout`, `--json` |
| `internal/smoke/smoke.go` | Reuse helpers if useful (avoid duplication) |
| `docs/cli-reference.md` | Docs |
| `docs/troubleshooting.md` | Point to doctor --deep |

---

### Task 1: Report model

```go
type Section struct {
    Name    string
    Status  string // ok|warn|fail
    Detail  string
    FixHint string
}
```

- [ ] **Step 1:** Extend `Report` with sections + `FormatDeep()`
- [ ] **Step 2:** Unit test formatting Healthy/Warnings/Failures grouping

### Task 2: Deep probes

- [ ] **Step 1:** Implement HTTP probe helper (status code or dial error → fail/warn)
- [ ] **Step 2:** Map profile/version to expected URL set via `urls` package
- [ ] **Step 3:** Compose ps integration — warn on exited services
- [ ] **Step 4:** Disk usage best-effort; warn above threshold (e.g. 90%) when obtainable
- [ ] **Step 5:** Tests with `net/http/httptest` and stubbed exec where needed

### Task 3: CLI

```bash
camunda doctor
camunda doctor --deep
camunda doctor --deep --timeout 60s
camunda doctor --deep --json
camunda doctor --fix   # existing; deep may suggest fixes but auto-fix stays limited
```

- [ ] **Step 1:** Wire `--deep`; if deep and lab down, fail with “run camunda up”
- [ ] **Step 2:** Update cli-reference + troubleshooting
- [ ] **Step 3:** Manual verify against local lab when available

---

## Out of scope

- Remote env deep checks (Phase 3 `env` + reuse probes with remote base URLs later)
- Auto-remediation beyond existing `--fix` hints
- Replacing Operate monitoring

## Success criteria

- Default doctor unchanged in spirit (fast)
- `--deep` against a healthy lab prints mostly OK with clear sections
- Failed Operate probe yields actionable fix hint
