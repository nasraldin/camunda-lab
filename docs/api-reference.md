# Lab API reference

Base URL: **`http://localhost:9090/api/v1`** (loopback only; no authentication).

The Lab UI and CLI share domain packages; toolkit routes mirror `camunda lint`, `plan`, `incidents`, and related commands. See also [CLI reference](cli-reference.md) and [Lab UI](lab-ui.md).

!!! warning "Local only"
The Lab API binds loopback addresses, rejects non-local `Host` headers, and requires Origin + CSRF for mutations. Do not expose it beyond your machine.

## Request boundary

| Check | Rule |
| ----- | ---- |
| Bind | Loopback only (`localhost`, `127.0.0.1`, `[::1]`) |
| `Host` | Must match bind address (optional numeric port) |
| Read methods | `GET`, `HEAD`, `OPTIONS` — no CSRF |
| Mutations | `Origin` must equal `http://<Host>`; send `X-Camunda-Lab-CSRF` from `GET /api/v1/session` |

Fetch the CSRF token once per page load:

```http
GET /api/v1/session
```

Response includes `csrfToken`. The SPA attaches it to JSON, form, multipart, and `DELETE` requests.

## Error envelope

Failed requests return JSON:

```json
{
  "ok": false,
  "code": "invalid_request",
  "error": "Human-readable message",
  "hint": "Optional recovery hint",
  "recoverable": false
}
```

| HTTP status | Typical `code` | Meaning |
| ----------- | ---------------- | ------- |
| 400 | `invalid_request` | Malformed JSON, unknown fields, missing required input |
| 403 | `path_forbidden`, `invalid_origin`, `invalid_csrf` | Path outside allowed roots or failed security boundary |
| 404 | `not_found` | Missing resource (incident, trace instance, env profile) |
| 409 | `conflict`, `artifact` | Env profile in use, artifact already exists |
| 413 | `payload_too_large` | Upload exceeds limit (restore archives capped at **50 MiB** compressed) |
| 422 | `invalid_request`, `invalid` | Unsupported capability (e.g. unknown test-gen language) |
| 502 | `upstream`, `ai` | Orchestration REST or AI provider failure |

Internal failures use `internal_error` with a generic message (no stack traces or temp paths).

## Developer result envelope

BPMN toolkit routes (`/api/v1/bpmn/*`) return a stable success shape:

```json
{
  "ok": true,
  "status": "completed",
  "complete": true,
  "findings": []
}
```

| Field | Meaning |
| ----- | ------- |
| `ok` | `true` when the operation completed without policy failure |
| `status` | Domain status (`completed`, `partial`, `refused`, …) |
| `complete` | `false` when scan/review could not finish (e.g. unreadable files) |
| `findings` | Lint/scan/review findings (shape varies by route) |

Plan, drift, incidents, and trace routes follow their domain schemas but always set `ok` honestly — incomplete remote inventory never reports success.

## Session and overview

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `GET` | `/api/v1/session` | CSRF token + session metadata |
| `GET` | `/api/v1/overview` | Lab status strip (version, profile, health summary) |

## Lab lifecycle (selected)

| Method | Path | Purpose |
| ------ | ---- | ------- |
| `POST` | `/api/v1/install` | Guided install |
| `POST` | `/api/v1/up` / `/down` / `/restart` | Compose lifecycle |
| `POST` | `/api/v1/switch` | Change Camunda minor |
| `POST` | `/api/v1/profile` / `/resources` | Profile and heap presets |
| `GET` | `/api/v1/doctor` / `/smoke` | Shallow health |
| `GET` | `/api/v1/containers` | Service list |
| `POST` | `/api/v1/containers/{service}/restart` | Restart one service (confirmation in UI) |
| `GET` | `/api/v1/logs/{service}` | Container logs |
| `POST` | `/api/v1/nuke` | Wipe lab home (typed `DELETE` + UI confirmation) |

Full lab routes are implemented in `internal/ui/api/handlers.go`; this page focuses on toolkit parity.

## BPMN developer toolkit

| Method | Path | CLI equivalent |
| ------ | ---- | -------------- |
| `POST` | `/api/v1/bpmn/lint` | `camunda lint` |
| `POST` | `/api/v1/bpmn/diff` | `camunda diff` |
| `POST` | `/api/v1/bpmn/explain` | `camunda explain` |
| `POST` | `/api/v1/bpmn/review` | `camunda review` |
| `POST` | `/api/v1/bpmn/test-generate` | `camunda test generate` |
| `POST` | `/api/v1/bpmn/test-generate/download` | In-browser ZIP download |
| `POST` | `/api/v1/bpmn/scan` | `camunda scan` |
| `GET` | `/api/v1/doctor/deep` | `camunda doctor --deep` |

JSON bodies reject unknown fields (`invalid_request`). Paths must be absolute and under allowed roots (home, `/tmp`, lab home).

### Diff modes

Exactly one mode per request:

- Two paths: `paths: ["a.bpmn","b.bpmn"]` or `from` / `to`
- File vs file: `path` + `against`
- Git: `projectDir`, project-relative `path`, and `base` ref

### Test generation download

`POST /api/v1/bpmn/test-generate/download` streams a ZIP attachment (Playwright mock coverage in A7). Writing to disk requires the non-download route with authorized `output`.

## Environment profiles

| Method | Path | CLI equivalent |
| ------ | ---- | -------------- |
| `GET` | `/api/v1/env?dir=` | `camunda env list` |
| `POST` | `/api/v1/env` | `camunda env add` |
| `POST` | `/api/v1/env/use` | `camunda env use` |
| `DELETE` | `/api/v1/env/{name}?dir=` | `camunda env remove` |

Remote profiles store **OIDC** auth by reference (`client-id-env`, `client-secret-env`, `token-url`, `token-url-env`, `audience`, `scope`) — never inline secrets. Full labs resolve tokens from lab `.env` automatically; remote profiles use named environment variables.

Query `dir` must be an absolute authorized project path when scoping project-local profiles.

## Cluster operations

Requires `.camunda.yaml` in the project directory (`dir` field). Uses Orchestration REST `/v2` via the active env profile.

| Method | Path | CLI equivalent |
| ------ | ---- | -------------- |
| `POST` | `/api/v1/plan` | `camunda plan` |
| `POST` | `/api/v1/drift` | `camunda drift` |
| `GET` | `/api/v1/incidents?dir=` | `camunda incidents list` |
| `POST` | `/api/v1/incidents/{key}/retry` | `camunda incidents retry` (UI confirmation gate) |
| `GET` | `/api/v1/trace/{instanceKey}?dir=&follow=` | `camunda trace` |

Upstream cluster failures return **502** with `upstream`. Missing instances return **404** `not_found`.

## Project, backup, restore

| Method | Path | CLI equivalent |
| ------ | ---- | -------------- |
| `POST` | `/api/v1/project/init` | `camunda init` |
| `POST` | `/api/v1/backup` | `camunda backup -o …` (requires `output`) |
| `POST` | `/api/v1/backup/download` | Browser gzip download |
| `POST` | `/api/v1/restore` | `camunda restore` (multipart upload) |

### Backup contents and exclusions

Included: lab `config.yaml`, AI key **names** (not values), project `.camunda.yaml`, BPMN/DMN/form files under configured paths.

**Not** included: Docker volumes, databases, images, downloaded version zips, logs, env profile files, workers, connectors, scripts, Helm charts, or `--include-secrets` opt-in (browser route never ships `ai.env` values).

### Restore limits and confirmations

- Browser restore requires typed **`RESTORE`** in the UI confirmation dialog.
- Upload capped at **50 MiB** compressed; decompressed limits match CLI (64 MiB/entry, 512 MiB total, 10 000 entries).
- Refuses when Compose containers are running unless **force** (same as CLI `--force`).
- Validates manifest, rejects traversal/symlinks, stages privately, rolls back on failure.

Empty `output` on `POST /api/v1/backup` is rejected — use `/backup/download` instead.

## Browser confirmation gates

The Lab UI prompts before: incident retry, env removal, service restart, lab stop/restart, destructive version switch, restore (`RESTORE`), and nuke (`DELETE` + second modal). Direct API calls still require CSRF; server-side confirmation fields apply where implemented.

## Acceptance status

Automated evidence (Go contract tests, Playwright **mock** project) covers these routes. **Live** cluster acceptance (real OIDC, incidents, trace) remains **Gate 5 open** — see [platform toolkit parity acceptance](acceptance/platform-toolkit-parity.md).

## Related docs

- [CLI reference](cli-reference.md) — flags, exit codes, OIDC env vars
- [Lab UI](lab-ui.md) — page-by-page walkthrough
- [Roadmap](roadmap.md) — implementation vs live acceptance status
