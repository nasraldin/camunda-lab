# Design: Camunda Lab UI (embedded control panel)

**Status:** Approved for implementation  
**Date:** 2026-07-17  
**Scope:** v1 = control panel + light ops (option 2). Console lite (option 3) is documented for later — do not build it in v1.

This document is the agent/source-of-truth contract for the Lab UI. Prefer this over chat history.

---

## Goals

- Give people who dislike living in the terminal a single local web UI to manage camunda-lab.
- Ship as **`camunda ui`**: Go HTTP server + embedded SPA (`go:embed`). One binary, no extra containers for the control plane.
- No authentication. Bind **loopback only** (`127.0.0.1`).
- UI is a peer to the CLI — same lab packages, not a second compose stack.

## Non-goals (v1)

- Camunda Identity / OIDC inside our UI
- Process instance browser, variable editors, BPMN canvas
- Multi-lab / named labs UI
- LAN / reverse-proxy / remote access
- Replacing Operate, Tasklist, Console, Optimize

---

## Locked decisions

| Topic | Choice |
| --- | --- |
| Hosting | Embedded in CLI (`camunda ui`) |
| Default URL | `http://127.0.0.1:9090` |
| Port override | `--port` / `CAMUNDA_LAB_UI_PORT` |
| Host | `127.0.0.1` only in v1 (reject non-loopback) |
| Auth | None |
| Frontend | Vite + React + TypeScript → `internal/ui/web/dist` embedded |
| Backend | Go stdlib `net/http`; handlers call `internal/lab`, `compose`, `doctor`, `smoke`, `urls`, `ai`, `tools` |
| Daemon | Do not auto-start UI after install; print hint to run `camunda ui` |

---

## Architecture

```text
Browser (localhost:9090)
  ├── static SPA (embedded dist/)
  └── REST /api/v1/*  →  internal/ui/api  →  lab / compose / …
                              ↓
                     docker compose -p camunda-lab
```

Deep links open Camunda apps (Operate, etc.) in a new tab — we do not proxy their UIs.

---

## v1 product surface (scope 2)

1. **Overview** — version/profile/resources, compose summary, up/down/restart, doctor/smoke, about snapshot  
2. **Setup** — install / switch (minor, profile, resources, optional AI secrets)  
3. **Apps** — URL cards from `urls` package  
4. **Containers** — compose ps; per-service restart  
5. **Logs** — stream/tail via SSE  
6. **AI / MCP** — enable/disable/wipe, status, copy MCP JSON  
7. **Tools** — c8ctl status/install; modeler profile; optional BPMN deploy if c8ctl present  
8. **Danger** — nuke with typed confirm  

### UX

- Calm local-tool UI; clear status; one primary action per section  
- Long ops: disable conflicting actions; show progress/errors  
- Mask secrets; never echo full API keys in status  
- Empty state: “No lab yet — Install”

---

## API (`/api/v1`)

Errors: `{ "error": "message" }` + suitable HTTP status.

| Method | Path | Behavior |
| --- | --- | --- |
| GET | `/overview` | config + version + compose ps summary |
| POST | `/install` | body: version, profile, resources, ai?, secrets? |
| POST | `/up` `/down` `/restart` | lifecycle |
| POST | `/switch` | version, wipe?, ai? |
| POST | `/profile` `/resources` | set profile / resources |
| GET | `/urls` | component URLs |
| GET | `/doctor` `/smoke` | diagnostics |
| GET | `/containers` | compose ps JSON |
| POST | `/containers/{service}/restart` | recreate service |
| GET | `/logs/{service}` | SSE log stream |
| GET/POST | `/ai/*` | status / enable / disable / config |
| GET/POST | `/tools/c8ctl/*` | status / install |
| POST | `/tools/modeler/profile` | write Desktop Modeler profile |
| POST | `/tools/deploy` | multipart BPMN if c8ctl installed |
| POST | `/nuke` | body `{ "confirm": "DELETE" }` |

Mutating routes must refuse if the server is not bound to loopback.

---

## Package layout

```text
internal/ui/
  server.go
  api/
  web/           # Vite React source
  web/dist/      # production build (embed)
internal/cli/ui.go
```

---

## Build / release

- `make ui` builds the SPA; `make build` depends on UI dist (or embeds placeholder).
- Commit a minimal `dist` (or generate in CI before GoReleaser) so `go build` / Homebrew work without Node when dist is present.
- Prefer: developers with Node run `make ui`; release CI always builds UI before GoReleaser.

---

## Future — option 3 (Console lite) — DO NOT IMPLEMENT IN V1

When we want richer cluster ops inside Lab UI (still localhost, still not a full Camunda Console):

- Process definitions list + deploy / version history (Orchestration REST and/or c8ctl)
- Start instance form (variables JSON)
- Instance search + basic timeline / incidents
- Job / incident retry helpers
- Connector / secret inventory beyond AI Agent keys
- Read-only Operate deep links with prefilled filters
- Multi-lab named instances UI once CLI supports `--name`

**Constraints for option 3:**

- Prefer official Camunda APIs; do not re-build Optimize or Identity
- Keep localhost / no-auth unless remote access is redesigned with real auth
- Stay complementary to Operate/Tasklist — not a replacement

Track publicly under Roadmap → Later → “Console lite”. Full detail lives in this spec.

---

## Success criteria (v1)

- `camunda ui` opens a browser; user can install/switch/up/down/urls/logs/AI without memorizing CLI flags
- CLI remains fully usable without the UI
- Spec + roadmap clearly separate scope 2 (v1) from scope 3 (later)
