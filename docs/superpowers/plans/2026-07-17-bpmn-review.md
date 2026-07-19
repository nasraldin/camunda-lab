# BPMN AI review (`camunda review`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked.

**Goal:** `camunda review` layers optional AI risk narrative on top of deterministic `lint` — like golangci-lint + a smart summary, not a replacement for rules.

**Architecture:** Always run `internal/lint`. If `--ai` (or config) and credentials exist, send compact IR + findings to user-configured provider; otherwise print lint-only. Never invent deploy steps.

**Tech Stack:** Go, existing `internal/ai` secrets patterns (`~/.camunda-lab/ai.env`), HTTP to OpenAI-compatible endpoints when enabled.

**Depends on:** `internal/bpmn`, `internal/lint`.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Default mode is **offline** (lint only) — AI is opt-in
- Never call paid LLM APIs in unit tests; mock HTTP transport
- Mask secrets in logs; do not send full `ai.env` file — only model excerpts
- Prompt must ask for risks: infinite loops, missing compensation, unreachable paths, duplicate messages — structured markdown sections
- Do not auto-deploy or mutate files unless a future `--write` is explicitly designed (out of MVP)
- Complementarity: review analyzes files; deploy stays official tooling

---

## File map

| File                             | Responsibility                   |
| -------------------------------- | -------------------------------- |
| `internal/review/review.go`      | Orchestrate lint + optional AI   |
| `internal/review/prompt.go`      | Build prompt from IR + findings  |
| `internal/review/client.go`      | HTTP client for chat completions |
| `internal/review/review_test.go` | Offline path + mocked AI         |
| `internal/cli/review.go`         | `camunda review`                 |
| `internal/cli/root.go`           | Register                         |
| `docs/cli-reference.md`          | Docs                             |

---

### Task 1: Offline review path

- [ ] **Step 1:** `review.Run(paths, opts)` returns lint findings formatted as “Review (lint)”
- [ ] **Step 2:** Test with fixture that has known lint hits
- [ ] **Step 3:** Exit codes mirror lint (`--fail-on`)

### Task 2: AI enrichment

```bash
camunda review bpmn/order.bpmn
camunda review bpmn/order.bpmn --ai
camunda review --ai --model gpt-4o-mini
```

- [ ] **Step 1:** Reuse key resolution from `internal/ai` (OpenAI/Anthropic env secrets); clear error if `--ai` and no key
- [ ] **Step 2:** Compact IR JSON (truncate large models) + lint findings in prompt
- [ ] **Step 3:** Parse model markdown; append under “AI suggestions” without dropping lint section
- [ ] **Step 4:** Mocked client test — assert prompt contains rule IDs and element ids
- [ ] **Step 5:** Timeout + non-zero exit on AI failure only if `--ai-required`; else warn and keep lint results

### Task 3: CLI + docs

- [ ] Wire flags: `--ai`, `--ai-required`, `--fail-on`, `--json` (json = lint findings; AI text field optional)
- [ ] cli-reference + note in `docs/ai-mcp.md` that review uses same secrets when `--ai`

---

## Out of scope

- Replacing `lint` (review always includes lint)
- Writing BPMN fixes automatically
- Camunda-hosted models as default

## Success criteria

- Without `--ai`, behavior equals lint-focused review, offline
- With mocked `--ai`, output contains both lint IDs and AI section
- CI never hits real LLM endpoints
