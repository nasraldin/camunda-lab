# AI Agent + MCP enablement for Camunda Lab

**Date:** 2026-07-17  
**Status:** Approved  
**Repo:** camunda-lab

## Goal

Give users a single CLI path to make Camunda’s **Orchestration Cluster MCP**, **Processes MCP** (8.10+), and **AI Agent connector** secrets usable in a local lab — at install time or on an already-running stack — without requiring a local LLM install.

## Decisions (approved)

| Decision           | Choice                                                                                     |
| ------------------ | ------------------------------------------------------------------------------------------ |
| Scope              | Both MCP client wiring **and** AI Agent LLM secrets                                        |
| CLI surface        | `--ai` on `install`/`switch` **and** `camunda ai enable\|disable\|status\|config`          |
| Approach           | Lab-owned overlay + secrets file (same pattern as ElasticVue), not docs-only               |
| LLM providers (v1) | OpenAI + Anthropic + optional OpenAI-compatible base URL (e.g. Ollama)                     |
| Local LLM          | Not required; optional via base URL only                                                   |
| Version gate       | Whole feature requires Camunda **8.9+** (cluster MCP). Processes MCP URL only on **8.10+** |
| Profiles           | **light** and **full**; **modeler**-only rejected                                          |

## Non-goals (v1)

- Auto-writing the user’s global Cursor/Claude `mcp.json` (print/copy only; `--write-cursor` can be a later flag)
- Deploying sample AI Agent BPMN blueprints
- Calling paid LLM APIs in smoke tests
- SaaS / Helm enablement
- Disabling official `camunda.mcp.enabled` (already `true` in Docker Compose 8.9+)

## Official Camunda baseline

| Capability                | Availability      | Docker Compose default | Lab endpoint                          |
| ------------------------- | ----------------- | ---------------------- | ------------------------------------- |
| Orchestration Cluster MCP | 8.9+              | Enabled                | `http://localhost:8080/mcp/cluster`   |
| Processes MCP             | 8.10+             | Enabled                | `http://localhost:8080/mcp/processes` |
| AI Agent connector        | connectors-bundle | Needs provider secrets | N/A (process design)                  |

Auth reality (verified on full 8.9 lab): MCP returns **401** when OIDC is on (`unprotectedApi: false`). Light profile uses `unprotectedApi: true` → direct Streamable HTTP works.

Docs:

- [Orchestration Cluster MCP overview](https://docs.camunda.io/docs/apis-tools/orchestration-cluster-api-mcp/orchestration-cluster-api-mcp-overview/)
- [Enable and connect](https://docs.camunda.io/docs/apis-tools/orchestration-cluster-api-mcp/orchestration-cluster-api-mcp-setup/)
- [Processes MCP overview](https://docs.camunda.io/docs/apis-tools/processes-mcp/processes-mcp-overview/)
- [AI agents](https://docs.camunda.io/docs/components/agentic-orchestration/ai-agents/)
- [Connectors secrets (SECRET_ prefix from 8.9)](https://docs.camunda.io/docs/self-managed/components/connectors/connectors-configuration/)

## Architecture

```
camunda install --ai …     ──┐
camunda ai enable          ──┼──► config.ai.enabled = true
camunda ai status/disable  ──┘         │
                                       ▼
                    ~/.camunda-lab/config.yaml   (ai.enabled)
                    ~/.camunda-lab/ai.env        (secrets, mode 0600)
                                       │
                    Compose: connectors env_file / env from ai.env
                                       │
                    urls + smoke + printed MCP client config
                    light → direct HTTP MCP
                    full  → c8ctl mcp-proxy snippet (OIDC)
```

Camunda already enables MCP in compose; camunda-lab’s job is **discoverability, secrets injection, auth bridge guidance, and status**.

## Design

### 1. Config

Extend `internal/config.Config`:

```yaml
version: '8.9'
profile: full
# …
ai:
  enabled: true
```

Use a nested struct (or `AIEnabled bool` with yaml `ai_enabled` if nesting is awkward). Persist via existing `config.Save`.

### 2. Secrets file

Path: `~/.camunda-lab/ai.env` (mode `0600`). Never commit. Document in `.gitignore` if the lab home is ever versioned (lab home is outside the repo).

Supported keys (v1):

```bash
SECRET_OPENAI_API_KEY=
SECRET_ANTHROPIC_API_KEY=
SECRET_OPENAI_BASE_URL=   # optional; e.g. http://host.docker.internal:11434/v1 for Ollama from containers
```

Enable rules:

- At least one provider must be configured: OpenAI key, Anthropic key, or OpenAI-compatible base URL (local Ollama/LM Studio may omit a key).
- Interactive prompts when TTY and values missing.
- Non-interactive: flags `--openai-key`, `--anthropic-key`, `--openai-base-url` and/or pre-set env vars with the same names as the file keys.
- Never print full secret values (mask in `status`).

Camunda 8.9+ connector secret provider default prefix is `SECRET_`; values in `ai.env` must use that prefix so `{{secrets.OPENAI_API_KEY}}` resolves inside connectors.

### 3. Compose overlay

Add `overlays/connectors-ai-secrets.yaml` (mirrored in `internal/overlay/embed/`), selected when `ai.enabled` and version ≥ 8.9 and profile ≠ modeler.

Intent: ensure the **connectors** service loads `ai.env` in addition to upstream `connector-secrets.txt`. Exact merge strategy to verify against official compose for 8.9/8.10 (env_file is a list — append our file path under `~/.camunda-lab/ai.env`, or inject equivalent `environment` entries). Prefer `env_file` append so secrets stay out of process listings where possible.

On enable/disable/secret change: recreate **connectors** only (not full stack wipe), unless compose merge requires a broader recreate — document the command used (`docker compose up -d --force-recreate connectors` via existing lab engine).

### 4. CLI

| Surface                               | Behavior                                                                                                                                                 |
| ------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `camunda install … --ai`              | After successful install path, run the same enable pipeline (prompt/flags for secrets)                                                                   |
| `camunda switch … --ai`               | Same when switching into a supported version/profile                                                                                                     |
| `camunda ai enable`                   | Set `ai.enabled`, write/update `ai.env`, apply overlay, recreate connectors, print next steps + `ai config`                                              |
| `camunda ai disable [--wipe-secrets]` | Clear `ai.enabled`, drop AI overlay from compose selection, recreate connectors; delete `ai.env` only with `--wipe-secrets`                              |
| `camunda ai status`                   | Version/profile gate, `ai.enabled`, masked secret presence, MCP HTTP probe (200 / 401-with-auth / fail), hint for full vs light                          |
| `camunda ai config`                   | Print MCP client JSON for Cursor/Claude/VS Code: light → Streamable HTTP URLs; full → `npx @camunda8/cli mcp-proxy` STDIO template with env placeholders |

Hard failures:

- Version &lt; 8.9 → upgrade hint
- Profile `modeler` → unsupported
- Enable with no provider configured → actionable error

Soft hints:

- Full profile + MCP 401 → use `camunda ai config` / `camunda tools c8ctl install`
- Missing c8ctl when printing full proxy config → install hint

### 5. URLs + open + smoke

When `ai.enabled` (and version allows), `urls.List` adds:

- `mcp-cluster` → `http://{host}:{orchestrationPort}/mcp/cluster` (8.9+)
- `mcp-processes` → `…/mcp/processes` (8.10+ only)

Notes field: light = “direct HTTP”; full = “OIDC — see camunda ai config”.

Smoke:

- Probe MCP endpoints; **200** = pass.
- On full, **401** + Bearer challenge = **warn** (“MCP up, auth required”), not fail.
- Never invoke LLM provider APIs.

### 6. Docs + about

- New MkDocs page (e.g. `docs/ai-mcp.md`): enable/disable, providers, Cursor snippet, full-profile auth, links to official Camunda docs.
- Nav entry under Guides or Features.
- `camunda about` Features line includes `ai · mcp`.

## Testing

- Unit: version/profile gate; URL entries present/absent by version; config round-trip for `ai.enabled`.
- Overlay selection includes `connectors-ai-secrets.yaml` only when enabled + eligible.
- Secret file write uses `0600` (where OS allows).
- Smoke classification: 200 vs 401 vs connection error.
- No live OpenAI/Anthropic calls in CI.

## Success criteria

1. `camunda ai enable` on a running 8.9+ light/full lab configures secrets and prints usable MCP client config without reinstall.
2. `camunda install --version 8.9 --profile light --ai --yes` (with key via env/flag) leaves MCP URL listed and connectors able to resolve `SECRET_*`.
3. Full profile clearly documents the OIDC/`mcp-proxy` path instead of implying raw HTTP will work.
4. No local LLM install is required for the happy path.

## Implementation sketch (for planning)

1. Config field + `ai.env` helpers under `internal/ai` or `internal/config`.
2. Overlay + `ComposeOverrideFiles` / lab up path honor `ai.enabled`.
3. `internal/cli/ai.go` commands; wire `--ai` on install/switch.
4. URLs + smoke + docs + about.
5. Tests + manual verify on light and full 8.9.
