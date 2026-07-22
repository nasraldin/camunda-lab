# Roadmap

This is an honest status page — not a delivery calendar. Dates slip; features land when they’re solid.

**Current release:** [v0.7.0](https://github.com/nasraldin/camunda-lab/releases/tag/v0.7.0)  
**Docs site:** [nasraldin.github.io/camunda-lab](https://nasraldin.github.io/camunda-lab/)

## Vision

Camunda Lab starts as the easiest way to run and manage **local** Camunda environments, then evolves into a **productivity toolkit** for developers and platform engineers. It complements official Camunda cluster CLIs (deploy, start instance, resource CRUD) rather than replacing them.

Direction and phase boundaries: [platform toolkit vision](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-platform-toolkit-vision.md) (in-repo, not on the docs site).

## What’s in v0.7.0

### Platform toolkit

- **CLI** — `init`, BPMN `lint` / `diff` / `explain` / `review` / `test generate` / `scan`, `doctor --deep`, `env`, `plan`, `drift`, `backup` / `restore`, `incidents`, `trace`, `k8s`
- **Lab UI** — **BPMN**, **Cluster**, and **Project** pages with `/api/v1` wrappers; developer endpoint health probes; OIDC for full-lab REST

### Monitoring add-on _(idea: @MahmoudSaid037)_

- **`camunda monitoring enable`** — opt-in **Prometheus + Grafana** (loopback only, `admin`/`admin`) with dashboards for Zeebe/orchestration, Elasticsearch, and connectors
- `camunda open grafana`, `camunda urls`, Lab UI **Monitoring** page + Apps cards
- Guide: [Monitoring](monitoring.md)

### Visual BPMN toolkit

- Lint / review findings on an interactive **bpmn-js** diagram, open **history** panel, project-folder upload for multi-file lint

### From v0.6.0 (still included)

- Lab UI auto-start, friendly Docker errors, `camunda ui logs`, local `make dev`
- Camunda Compose **8.7–8.10**, profiles, ElasticVue, AI Agent + MCP
- GitHub Releases, `install.sh`, Homebrew (`camunda-lab`)

## Next up (maintainer / small DX)

Things we’re actively building or next in line — no hard ETA:

- Optional Cosign verify when `cosign` is on your PATH
- Scheduled LIVE smoke in CI (nightly-ish; too heavy for every PR)
- Optional `--write-cursor` to drop MCP JSON into the user’s Cursor config
- Sample AI Agent BPMN deploy helper (thin wrapper around `c8ctl`)
- **Sample-data seeder** — deploy a demo process and start a few instances so Operate/Tasklist aren’t empty on first boot (pairs with the BPMN deploy helper above) _(idea: @MahmoudSaid037)_
- **Port-conflict detection & remap** — `doctor` / install spot colliding host ports (they differ per minor) and offer to remap instead of failing _(idea: @MahmoudSaid037)_

## Later / maybe

- **Console lite (Lab UI scope 3)** — process definitions, start instance, instance/incident views, job retry, richer connector secrets, Operate deep links, plus **compare process versions** (visual `bpmn-js` diff to spot the gaps between two versions) and **promote an older version to latest** (re-deploy its XML as the new highest version — Zeebe assigns versions by deployment order, so this republishes rather than repoints) — inside the same localhost UI, using official Camunda APIs (not a full Optimize/Identity rebuild). Details in the [lab UI design](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-lab-ui-design.md) “Future — option 3” section _(version compare + promote idea: @MahmoudSaid037)_
- **Snapshot / restore lab state** — dump Elasticsearch volumes + deployed BPMN and restore later, for reproducible demos and safer version-switch experiments _(idea: @MahmoudSaid037)_
- **Deeper monitoring dashboards** — per-minor dashboard tuning, alert rules, and a proper Optimize dashboard on top of the shipped Prometheus/Grafana add-on _(idea: @MahmoudSaid037)_
- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s real demand
- Process replay, C7→C8 migration assistant, executive HTML report (explicitly out of Phase 1–3 commitments)
- Worker inspector / variable editor as separate epics

## How to steer this

Open an [issue](https://github.com/nasraldin/camunda-lab/issues) or a PR. Release notes live on [GitHub Releases](https://github.com/nasraldin/camunda-lab/releases).
