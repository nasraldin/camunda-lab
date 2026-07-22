# Architecture

A Go CLI that downloads Camunda’s zip, stores it under your home directory, and runs `docker compose` with the right files plus a few thin overlays.

```text
you
 └── camunda (CLI)
       ├── camunda ui  →  http://localhost:9090  (embedded SPA + /api/v1)
       ├── ~/.camunda-lab/config.yaml
       ├── ~/.camunda-lab/ai.env            ← optional AI Agent SECRET_*
       ├── ~/.camunda-lab/versions/8.9/     ← official zip, extracted
       ├── ~/.camunda-lab/resources.env     ← heap hints
       └── docker compose -p camunda-lab -f …
             └── Camunda containers (+ overlays)
```

## Source of truth

[camunda/camunda-distributions](https://github.com/camunda/camunda-distributions) release assets:

```text
docker-compose-8.8.zip
```

We extract the zip and leave Camunda’s OIDC / Keycloak wiring alone. If Camunda fixes a bug in the zip, the next fetch picks it up.

## What we add

| Piece                                         | Role                                                                 |
| --------------------------------------------- | -------------------------------------------------------------------- |
| Version adapters                              | Map `light` / `full` / `modeler` → the right compose file per minor  |
| `resources.env`                               | Heap hints + `KEYCLOAK_HOST=keycloak` for container→Keycloak on full |
| `elasticsearch-8.10.yaml`                     | Sidecar ES when full profile on 8.10 needs it                        |
| `elasticsearch-cors.yaml` + `elasticvue.yaml` | CORS + ElasticVue when host ES is published                          |
| `http-headers.yaml`                           | Larger Tomcat header limit so full-profile SSO cookies don’t 400     |
| `connectors-ai-secrets.yaml` + `ai.env`       | Opt-in AI Agent `SECRET_*` (`camunda ai`)                            |
| MCP URLs / `camunda ai config`                | Surface `/mcp/cluster` (+ `/mcp/processes` on 8.10+); client JSON    |
| Lab UI (`camunda ui`)                         | Embedded local SPA on loopback; same lab packages via `/api/v1`      |
| doctor / wait / smoke                         | Docker sanity and “are the UIs answering?”                           |
| tools helpers                                 | Point `c8ctl` / Desktop Modeler at this lab                          |

## Compose project name

Everything runs under project name **`camunda-lab`**, so it doesn’t collide with a random `docker compose up` from a hand-extracted zip.

## Toolkit packages (main branch)

Implemented on main with automated Go and Playwright **mock** evidence. Live disposable-cluster acceptance ([Gate 5](acceptance/platform-toolkit-parity.md)) remains open for OIDC, incidents, and trace.

```text
internal/project/     # .camunda.yaml + camunda init scaffold  ✓
internal/bpmn/        # BPMN XML → normalized IR                 ✓
internal/diff/ lint/ review/ explain/ testgen/ scan/           ✓
internal/cluster/     # Orchestration REST /v2 client            ✓
internal/env/ plan/ drift/ backup/ incidents/ trace/               ✓
```

Project-local config lives in `.camunda.yaml` at a repo root; lab state stays under `~/.camunda-lab`.

## Not in scope

- Replacing Helm as the production install path (use Camunda’s official charts for Kubernetes)
- Multi-instance named labs (paths are shaped so we can add that later)
- Rewriting Camunda’s service graph
- Cloning official deploy / start-instance UX
