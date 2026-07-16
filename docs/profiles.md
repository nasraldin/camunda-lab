# Profiles and versions

## Profiles

| Profile | Roughly includes | When to use |
| --- | --- | --- |
| `light` (default) | Orchestration, connectors, ES (≤8.9) | Day-to-day coding |
| `full` | + Identity, Keycloak, Optimize, Console, Web Modeler | Platform / IAM / Optimize checks |
| `modeler` | Web Modeler stack only | Modeling without the rest |

```bash
camunda install --profile light --yes
camunda profile full          # recreate with full
```

## Minors we support

| Minor | Notes |
| --- | --- |
| **8.7** | Separate Zeebe / Operate / Tasklist services. Light → `docker-compose-core.yaml`, full → `docker-compose.yaml` |
| **8.8 / 8.9** | Consolidated `orchestration` image. Light / full / modeler files as Camunda documents |
| **8.10** | Preview. ES not bundled; full profile gets our ES overlay |

```bash
camunda switch 8.9 --wipe
camunda switch 8.10 --wipe   # expect preview quirks
```

Without `--wipe`, we warn you: old volumes + new minor often break. Prefer wipe when you’re comparing versions.

## Resource profiles

These don’t rewrite Camunda’s compose topology. They write `~/.camunda-lab/resources.env` with heap settings:

| Profile | Heap-ish setting | Machine vibe |
| --- | --- | --- |
| `small` | `-Xms256m -Xmx512m` | 16 GB laptop, light only |
| `balanced` | `-Xms512m -Xmx1024m` | Default |
| `power` | `-Xms1g -Xmx2g` | You have RAM to spare |

```bash
camunda resources small
camunda restart
```

## Ports (quick map)

Exact ports come from Camunda’s files; `camunda urls` prints what this lab expects.

Typical **light** on 8.8+:

| App | URL |
| --- | --- |
| Operate | http://localhost:8080/operate |
| Tasklist | http://localhost:8080/tasklist |
| gRPC | localhost:26500 |

Typical **full** extras:

| App | URL |
| --- | --- |
| Console | http://localhost:8087 |
| Optimize | http://localhost:8083 |
| Identity | http://localhost:8084 |
| Web Modeler | http://localhost:8070 |
| Keycloak | http://localhost:18080/auth/ |
