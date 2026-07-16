# Roadmap

This is an honest status page — not a delivery calendar. Dates slip; features land when they’re solid.

**Current release:** [v0.4.0](https://github.com/nasraldin/camunda-lab/releases/tag/v0.4.0)  
**Docs site:** [nasraldin.github.io/camunda-lab](https://nasraldin.github.io/camunda-lab/)

## What’s in v0.4.0

### Lab lifecycle

- Download and run official Camunda Compose for **8.7–8.10**
- Profiles: **light**, **full**, **modeler**
- Resource presets: **small** / **balanced** / **power**
- `install`, `up` / `down`, `switch`, `profile`, `resources`
- `wait`, `smoke`, `doctor`, `urls`, `open`, `logs`, `nuke`
- State under `~/.camunda-lab/` (override with `CAMUNDA_LAB_HOME`)

### Overlays and helpers

- **ElasticVue** when the stack publishes Elasticsearch on the host (CORS + preconfigured cluster)
- **8.10 full** Elasticsearch sidecar when the zip doesn’t ship ES the same way
- Larger HTTP header limit so full-profile SSO cookies don’t 400
- **`camunda ai`** — MCP URLs + AI Agent connector secrets (`ai.env`, 8.9+ light/full)
- `camunda ai config` for Cursor/Claude (HTTP on light; `c8ctl mcp-proxy` on full)

### Packaging and docs

- GitHub Releases via GoReleaser (`checksums.txt`)
- `install.sh` one-liner with checksum verify
- Homebrew: `brew tap nasraldin/tools && brew install camunda-lab`
- Docs site (MkDocs Material) + CI
- `c8ctl` and Desktop Modeler helpers under `camunda tools`

## Next up

Things we’re actively interested in — no ETA:

- Optional Cosign verify when `cosign` is on your PATH
- Scheduled LIVE smoke in CI (nightly-ish; too heavy for every PR)
- Optional `--write-cursor` to drop MCP JSON into the user’s Cursor config
- Sample AI Agent BPMN deploy helper (thin wrapper around `c8ctl`)

## Later / maybe

- Named labs (`camunda --name upgrade-test`) for side-by-side minors
- Windows support if there’s real demand
- Thin Kind/Helm bridge that keeps the same CLI verbs for people who outgrew Compose

## How to steer this

Open an [issue](https://github.com/nasraldin/camunda-lab/issues) or a PR. Release notes live on [GitHub Releases](https://github.com/nasraldin/camunda-lab/releases).
