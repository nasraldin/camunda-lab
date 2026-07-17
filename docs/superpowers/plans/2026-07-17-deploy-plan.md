# Deployment preview (`camunda plan`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked. Requires `camunda env` + project `.camunda.yaml`.

**Goal:** Terraform-style preview of what would change if project resources were deployed to the active env — **preview only, never deploys**.

**Architecture:** Read local BPMN/DMN/forms via `internal/project` paths. List deployed process/decision definitions from Orchestration API (read). Diff by resource name/id + version/digest. Print create/update/delete/noop + warnings (breaking version, running instances when API allows).

**Tech Stack:** Go, HTTP client, active env from `internal/env`, Cobra.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- **Hard boundary:** `camunda plan` must not upload resources or start instances
- Deploy remains official cluster CLI / UI
- Active env required for remote; lab default uses local orchestration URL from `urls`
- Auth via env var names from profile; clear error if vars unset
- `--json` for CI; text human report default
- Exit `0` always on successful plan computation; `2` on API/tool error (changes are not failures)

---

## Report shape

```text
Plan (env=lab)

Create
  ✓ order.bpmn

Update
  ~ invoice.dmn (cluster v3 → local digest …)

Delete
  - payment.form (present on cluster, missing in git paths)

Noop
  = shipping.bpmn

Warnings
  ! order.bpmn — new version while instances may still run (count=N if available)
```

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/plan/plan.go` | Build plan from local + remote inventories |
| `internal/plan/remote.go` | List definitions API client |
| `internal/plan/local.go` | Scan project resources + digests |
| `internal/plan/format.go` | Text/JSON |
| `internal/plan/plan_test.go` | Fake remote + temp project |
| `internal/cli/plan.go` | `camunda plan` |
| `internal/cli/root.go` | Register |
| `docs/cli-reference.md` | Emphasize preview-only |

---

### Task 1: Local inventory

- [ ] Hash/digest BPMN/DMN/form files (content hash); key by process id / decision id / form id parsed from files when possible, else filename
- [ ] Unit tests with fixtures

### Task 2: Remote inventory

- [ ] HTTP list process definitions (Camunda 8 Orchestration REST — pin concrete paths in impl notes when coding; adapt to 8.8+ consolidated API)
- [ ] Mock server tests
- [ ] Map auth token helper from `internal/env`

### Task 3: Diff + CLI

```bash
camunda plan
camunda plan --env prod
camunda plan --json
```

- [ ] Compare inventories → Plan struct
- [ ] Warnings best-effort for running instance counts
- [ ] cli-reference with big admonition: does not deploy

---

## Out of scope

- `camunda apply` that deploys (explicit non-goal)
- Deleting resources on cluster
- Terraform state files

## Success criteria

- Against fake remote, plan classifies create/update/delete correctly
- Integration docs never show `plan` as a deploy step
