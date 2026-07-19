# Incident explorer (`camunda incidents`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked. CLI-first; Lab UI Console lite reuses this package later.

**Goal:** List and optionally retry/resolve incidents from the terminal against the active env — thin helper over official APIs, not an Operate replacement.

**Architecture:** `internal/incidents` HTTP client using `internal/env.Resolve()`. Table output. Subcommands for retry when API supports. Deep links to Operate when URL known.

**Tech Stack:** Go, HTTP, Cobra, env auth.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)  
**UI later:** [../specs/2026-07-17-lab-ui-design.md](../specs/2026-07-17-lab-ui-design.md) option 3

## Global Constraints

- Complementarity: prefer official Operate for full UX; CLI for speed
- Do not rebuild Optimize analytics
- Auth via env profile; lab uses demo credentials patterns already known to Lab where applicable
- Mutating actions (`retry`) require `--yes` for scripting safety
- Pin concrete Camunda 8 REST paths at implementation time (8.8+ consolidated APIs differ from older Operate-only APIs) — encapsulate in client with version adapter

---

## UX

```bash
camunda incidents
camunda incidents --env prod --limit 50
camunda incidents show 2251799813685249
camunda incidents retry 2251799813685249 --yes
```

Table columns: ID, Created, Job/Worker, Error (truncated), Process, Duration.

`show` prints variables snippet if API returns them (read-only).

---

## File map

| File                                   | Responsibility |
| -------------------------------------- | -------------- |
| `internal/incidents/client.go`         | List/Get/Retry |
| `internal/incidents/format.go`         | Table/JSON     |
| `internal/incidents/incidents_test.go` | Mock HTTP      |
| `internal/cli/incidents.go`            | Commands       |
| `internal/cli/root.go`                 | Register       |
| `docs/cli-reference.md`                | Docs           |

---

### Task 1: List + show

- [ ] Mock API fixtures → table rendering
- [ ] `--json` output
- [ ] Operate deep-link helper when operate base URL in env

### Task 2: Retry / resolve

- [ ] Implement only endpoints confirmed in Camunda 8 docs for the target minors (8.7–8.10)
- [ ] If resolve is unsupported, omit subcommand rather than fake it
- [ ] `--yes` gate

### Task 3: CLI + UI note

- [ ] Wire commands
- [ ] Add “Lab UI may call same package later” note in plan success — no UI work in this plan’s MVP tasks
- [ ] cli-reference

---

## Out of scope

- Full variable editor (`camunda vars` epic — later/maybe)
- Worker fleet inspector
- Process replay

## Success criteria

- List incidents against mock server
- Retry path covered by test when API shape locked
- Docs state Operate remains source of truth for rich ops
