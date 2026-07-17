# Roadmap

This is an honest status page — not a delivery calendar. Dates slip; features land when they’re solid.

**Current release:** [v0.6.0](https://github.com/nasraldin/camunda-lab/releases/tag/v0.6.0)  
**Docs site:** [nasraldin.github.io/camunda-lab](https://nasraldin.github.io/camunda-lab/)

## What’s in v0.6.0

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
- Camunda Compose **8.7–8.10**, profiles, ElasticVue, AI Agent + MCP, `c8ctl` / Modeler tools
- GitHub Releases, `install.sh`, Homebrew (`camunda-lab`)

## Next up

Things we’re actively building or next in line — no hard ETA:

- Optional Cosign verify when `cosign` is on your PATH
- Scheduled LIVE smoke in CI (nightly-ish; too heavy for every PR)
- Optional `--write-cursor` to drop MCP JSON into the user’s Cursor config
- Sample AI Agent BPMN deploy helper (thin wrapper around `c8ctl`)

## Later / maybe

- **Console lite (Lab UI scope 3)** — process definitions, start instance, instance/incident views, job retry, richer connector secrets, Operate deep links — inside the same localhost UI, using official Camunda APIs (not a full Optimize/Identity rebuild). Details in the [lab UI design](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-lab-ui-design.md) “Future — option 3” section
- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s real demand
- Thin Kind/Helm bridge that keeps the same CLI verbs for people who outgrew Compose

## How to steer this

Open an [issue](https://github.com/nasraldin/camunda-lab/issues) or a PR. Release notes live on [GitHub Releases](https://github.com/nasraldin/camunda-lab/releases).
