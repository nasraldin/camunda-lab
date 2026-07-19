# Process trace (`camunda trace`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked.

**Goal:** `camunda trace <instanceKey>` shows an ASCII activity timeline (and `--follow` live tail) for debugging — kubectl-logs-like for process instances.

**Architecture:** Poll Orchestration/Operate history APIs for the instance; render ordered tree of activities. Follow mode polls until completed/incident/timeout.

**Tech Stack:** Go, HTTP, Cobra, env resolve.

**Depends on:** `internal/env`; may share cluster HTTP helpers with incidents/plan.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Read-only
- `--follow` with `--interval` (default 2s) and `--timeout`
- Clear error if instance not found
- Do not start instances (official tooling)
- ASCII output default; `--json` for machine use

---

## Output sketch

```text
Instance 2251799813685248  (ACTIVE)

OrderCreated
↓
ValidateCustomer
↓
ReserveStock
↓
Payment          ← INCIDENT: job timeout
```

---

## File map

| File                           | Responsibility         |
| ------------------------------ | ---------------------- |
| `internal/trace/trace.go`      | Fetch timeline → model |
| `internal/trace/render.go`     | ASCII tree             |
| `internal/trace/follow.go`     | Poll loop              |
| `internal/trace/trace_test.go` | Fixtures               |
| `internal/cli/trace.go`        | CLI                    |
| `internal/cli/root.go`         | Register               |
| `docs/cli-reference.md`        | Docs                   |

---

### Task 1: Timeline model + render

- [ ] Fixture JSON → ASCII with incident marker
- [ ] Implement renderer (no color required; optional later)

### Task 2: API client

- [ ] Fetch process instance + activity/flow node history for active env
- [ ] Version adapter notes for 8.7 vs 8.8+ URLs
- [ ] Mock tests

### Task 3: Follow + CLI

```bash
camunda trace 2251799813685248
camunda trace 2251799813685248 --follow
camunda trace 2251799813685248 --follow --interval 1s --timeout 5m
camunda trace 2251799813685248 --json
```

- [ ] Redraw or append-only follow mode (prefer append new steps to avoid flicker)
- [ ] Exit `0` on completion, `1` if ends in incident (optional flag `--fail-on-incident`)
- [ ] cli-reference

---

## Out of scope

- BPMN diagram GUI
- Variable watch/edit
- Replaying tokens

## Success criteria

- Static trace renders fixture timeline
- Follow stops on completed/timeout in tests with fake clock or short poll
