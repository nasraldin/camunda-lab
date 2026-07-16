# Architecture

Nothing fancy under the hood: a Go CLI that downloads Camunda’s zip, drops it under your home dir, and runs `docker compose` with the right files.

```text
you
 └── camunda (CLI)
       ├── ~/.camunda-lab/config.yaml
       ├── ~/.camunda-lab/versions/8.8/     ← official zip, extracted
       ├── ~/.camunda-lab/resources.env     ← heap hints
       └── docker compose -p camunda-lab -f …
             └── Camunda containers
```

## Source of truth

[camunda/camunda-distributions](https://github.com/camunda/camunda-distributions) release assets:

```text
docker-compose-8.8.zip
```

We verify what we can, extract, and leave Camunda’s OIDC / Keycloak wiring alone. If Camunda fixes a bug in the zip, the next fetch picks it up.

## What we add

| Piece | Role |
| --- | --- |
| Version adapters | Map `light` / `full` / `modeler` → the right compose file per minor |
| `resources.env` | `JAVA_TOOL_OPTIONS` for small / balanced / power |
| `elasticsearch-8.10.yaml` | Sidecar ES when full profile on 8.10 needs it |
| doctor / wait / smoke | “Is Docker fine?” and “are the UIs answering?” |
| tools helpers | Point `c8ctl` / Desktop Modeler at this lab |

## Compose project name

Everything runs under project name **`camunda-lab`**, so it doesn’t collide with a random `docker compose up` you ran from a hand-extracted zip.

## Not in scope

- Replacing Helm
- Multi-instance named labs (paths are shaped so we can add that later)
- Rewriting Camunda’s service graph
