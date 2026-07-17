# BPMN explain (`camunda explain`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked.

**Goal:** `camunda explain file.bpmn` produces Business Summary, Technical Summary, Risks, Missing Paths from the model — offline template first, optional AI enrichment.

**Architecture:** Walk `internal/bpmn` IR to extract happy-path outline, gateways, external/service tasks, timers, messages. Render markdown. Optional AI polish via same secrets pattern as review.

**Tech Stack:** Go, Cobra, shared IR, optional HTTP AI client (share with `internal/review` where practical).

**Depends on:** `internal/bpmn`.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Default **offline**; `--ai` opt-in
- No cluster calls; no deploy
- No paid LLM in tests
- Output markdown sections fixed for scripting:

```markdown
## Business Summary
## Technical Summary
## Risks
## Missing Paths
## Optimization Suggestions
```

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/explain/explain.go` | Offline summary from IR |
| `internal/explain/graph.go` | Simple path / dead-end heuristics |
| `internal/explain/ai.go` | Optional enrichment |
| `internal/explain/explain_test.go` | Fixture snapshots |
| `internal/cli/explain.go` | CLI |
| `internal/cli/root.go` | Register |
| `docs/cli-reference.md` | Docs |

---

### Task 1: Offline explainer

- [ ] **Step 1:** Fixture process with gateway + service task + timer
- [ ] **Step 2:** Failing test — Business section lists human-readable task names; Technical lists types/ids; Risks notes gateway without default if detectable (may call lint helper)
- [ ] **Step 3:** Implement graph walk for missing paths / dead ends (best-effort; document limits)
- [ ] **Step 4:** `go test ./internal/explain/ -count=1`

### Task 2: AI optional + CLI

```bash
camunda explain bpmn/order.bpmn
camunda explain bpmn/order.bpmn --ai
camunda explain bpmn/order.bpmn -o explain.md
```

- [ ] **Step 1:** `--ai` enriches Optimization Suggestions; keep offline sections as source of truth
- [ ] **Step 2:** `-o` write file; default stdout
- [ ] **Step 3:** cli-reference

---

## Out of scope

- Full formal model checking
- Generating new BPMN
- Executive multi-process HTML report (later/maybe on roadmap)

## Success criteria

- Offline explain on fixture is stable enough for golden/snapshot test
- AI path mocked in tests; default path needs no network
