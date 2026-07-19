# Roadmap

This is an honest status page — not a delivery calendar. Dates slip; features land when they’re solid.

**Current release:** [v0.6.0](https://github.com/nasraldin/camunda-lab/releases/tag/v0.6.0)  
**Docs site:** [nasraldin.github.io/camunda-lab](https://nasraldin.github.io/camunda-lab/)

## Vision

Camunda Lab starts as the easiest way to run and manage **local** Camunda environments, then evolves into a **productivity toolkit** for developers and platform engineers. It complements official Camunda cluster CLIs (deploy, start instance, resource CRUD) rather than replacing them.

Direction and phase boundaries: [platform toolkit vision](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-platform-toolkit-vision.md) (in-repo, not on the docs site).

## What’s in v0.6.0 (Phase 1 lab core)

### Lab UI polish

- **Auto-start** — `camunda install`, `up`, `restart`, `switch`, and `profile` start the UI in the background; Homebrew `post_install` does too
- **Friendly Docker errors** — leftover container name conflicts show plain language + **Clean up and try again** in the UI
- **`camunda ui logs`** / **`camunda ui logs -f`** — tail background UI logs (`~/.camunda-lab/logs/ui.log`)
- **Local dev** — `make dev`, `make dev-restart-api`, Vite hot reload on `:5173`

### Maintainer / install

- Homebrew tap publishes automatically on every GitHub Release (no more stale formula)
- CI hardening: concurrency, `make tidy`, embedded UI dist check, govulncheck advisory, Go 1.24.4

### From v0.5.0 (still included)

- **`camunda ui`** — embedded Camunda Lab Console on `http://localhost:9090`
- Home, Get started, Apps (auto sign-in), Services, Logs, AI helpers, Reset lab
- Camunda Compose **8.7–8.10**, profiles, ElasticVue, AI Agent + MCP, official CLI / Modeler tools
- GitHub Releases, `install.sh`, Homebrew (`camunda-lab`)
- Basic `camunda doctor`, smoke/wait, version switch, resource presets

## Phase 1 — lab core

**Shipped:** install / switch / profile / resources, Lab UI, AI/MCP, basic `doctor`, smoke/wait, tools glue, overlays, **`camunda init`** (project scaffold + `.camunda.yaml`).

Plan (historical): [project-init](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/plans/2026-07-17-project-init.md)

## Next up (maintainer / small DX)

Things we’re actively building or next in line — no hard ETA:

- Optional Cosign verify when `cosign` is on your PATH
- Scheduled LIVE smoke in CI (nightly-ish; too heavy for every PR)
- Optional `--write-cursor` to drop MCP JSON into the user’s Cursor config
- Sample AI Agent BPMN deploy helper (thin wrap of official deploy tooling)

## Phase 2 — Developer experience

Shipped on main (MVP): `lint`, `diff`, `explain`, `review`, `test generate`, `scan`, `doctor --deep`.

| Command                 | Intent                                  |
| ----------------------- | --------------------------------------- |
| `camunda diff`          | Semantic BPMN diff (not raw XML)        |
| `camunda lint`          | Deterministic BPMN rules (eslint-style) |
| `camunda review`        | Lint + optional AI review (`--ai`)      |
| `camunda explain`       | Business + technical process summary    |
| `camunda test generate` | Test skeletons from BPMN                |
| `camunda scan`          | Secrets / hardcoded credential scan     |
| `camunda doctor --deep` | Component health beyond Docker/config   |

## Phase 3 — Platform engineering

Shipped on main: `env`, `plan`, `drift`, `backup`/`restore`, `incidents`, `trace`, `k8s`.

`plan` / `drift` / `incidents` / `trace` call the active env’s **Orchestration Cluster REST API** (`/v2`) — lab URLs by default (`camunda urls` → `rest`). Remote profiles use `endpoints.orchestration`.

| Command                      | Intent                                                       |
| ---------------------------- | ------------------------------------------------------------ |
| `camunda env`                | Named lab / remote environment profiles                      |
| `camunda plan`               | Deployment preview vs cluster definitions (does not deploy)  |
| `camunda drift`              | Local project XML digest vs deployed definition XML          |
| `camunda backup` / `restore` | Lab-oriented snapshot MVP                                    |
| `camunda incidents`          | List/resolve via `POST /v2/incidents/search` + `/resolution` |
| `camunda trace`              | Timeline via process instance + element-instances search     |
| `camunda k8s`                | Thin kubectl helpers for Camunda Helm labels                 |

**Lab UI parity (localhost):** sidebar **BPMN**, **Cluster**, and **Project** call the same packages as these CLI commands (upload or absolute project path). Not a full Camunda Console — Operate/Tasklist remain the primary ops UIs.

## Later / maybe

- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s real demand
- Process replay, C7→C8 migration assistant, executive HTML report (explicitly out of Phase 1–3 commitments)
- Worker inspector / variable editor as separate epics

## How to steer this

Open an [issue](https://github.com/nasraldin/camunda-lab/issues) or a PR. Release notes live on [GitHub Releases](https://github.com/nasraldin/camunda-lab/releases).
