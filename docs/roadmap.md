# Roadmap

This is an honest status page — not a delivery calendar. Dates slip; features land when they’re solid.

**Current release:** [v0.5.0](https://github.com/nasraldin/camunda-lab/releases/tag/v0.5.0)  
**Docs site:** [nasraldin.github.io/camunda-lab](https://nasraldin.github.io/camunda-lab/)

## What’s in v0.5.0

### Lab UI

- **`camunda ui`** — embedded Camunda Lab Console on `http://localhost:9090` (loopback only, no auth)
- Home: start/stop/restart, doctor, smoke, CLI update check
- Get started: install / switch version, profile, resources, optional AI
- Apps: grouped cards, **Auto sign-in** (Keycloak warm as `demo`/`demo`, opt-out remembered in the browser)
- Sign out / Fix broken session for stuck SSO cookies
- Logins, Services (filter/search/restart), Logs (filter/highlight), AI helpers, Extras, Reset lab
- Light / dark theme
- Guide + screenshots: [Lab UI](lab-ui.md)

### From v0.4.0 (still included)

- Camunda Compose **8.7–8.10**, profiles light / full / modeler, resource presets
- ElasticVue, AI Agent + MCP helpers, `c8ctl` / Modeler tools
- GitHub Releases, `install.sh`, Homebrew (`camunda-lab`)

## Next up

Things we’re actively building or next in line — no hard ETA:

- Optional Cosign verify when `cosign` is on your PATH
- Scheduled LIVE smoke in CI (nightly-ish; too heavy for every PR)
- Optional `--write-cursor` to drop MCP JSON into the user’s Cursor config
- Sample AI Agent BPMN deploy helper (thin wrapper around `c8ctl`)
- **Monitoring add-on** — optional profile that wires Prometheus + Grafana with pre-provisioned dashboards for Zeebe and Elasticsearch (official dashboards + the ES exporter); opt-in like AI/ElasticVue, opened via `camunda open grafana` and a Lab UI card. Metric paths shift per minor, so dashboards are best-effort per supported version _(idea: @MahmoudSaid037)_
- **Sample-data seeder** — deploy a demo process and start a few instances so Operate/Tasklist aren’t empty on first boot (pairs with the BPMN deploy helper above) _(idea: @MahmoudSaid037)_
- **Port-conflict detection & remap** — `doctor` / install spot colliding host ports (they differ per minor) and offer to remap instead of failing _(idea: @MahmoudSaid037)_

## Later / maybe

- **Console lite (Lab UI scope 3)** — process definitions, start instance, instance/incident views, job retry, richer connector secrets, Operate deep links, plus **compare process versions** (visual `bpmn-js` diff to spot the gaps between two versions) and **promote an older version to latest** (re-deploy its XML as the new highest version — Zeebe assigns versions by deployment order, so this republishes rather than repoints) — inside the same localhost UI, using official Camunda APIs (not a full Optimize/Identity rebuild). Details in the [lab UI design](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-lab-ui-design.md) “Future — option 3” section _(version compare + promote idea: @MahmoudSaid037)_
- **Snapshot / restore lab state** — dump Elasticsearch volumes + deployed BPMN and restore later, for reproducible demos and safer version-switch experiments _(idea: @MahmoudSaid037)_
- **Full monitoring dashboards** — extend the Prometheus/Grafana add-on to connectors and Optimize once the core Zeebe/ES dashboards land _(idea: @MahmoudSaid037)_
- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s real demand
- Thin Kind/Helm bridge that keeps the same CLI verbs for people who outgrew Compose

## How to steer this

Open an [issue](https://github.com/nasraldin/camunda-lab/issues) or a PR. Release notes live on [GitHub Releases](https://github.com/nasraldin/camunda-lab/releases).
