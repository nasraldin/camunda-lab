# Profiles and versions

## Profiles

| Profile           | Roughly includes                                               | When to use                      |
| ----------------- | -------------------------------------------------------------- | -------------------------------- |
| `light` (default) | Orchestration, connectors, ES (≤8.9)                           | Day-to-day coding                |
| `full`            | + Identity, Keycloak, Optimize, Console (8.8–8.9), Web Modeler | Platform / IAM / Optimize checks |
| `modeler`         | Web Modeler stack only                                         | Modeling without the rest        |

```bash
camunda install --profile light --yes
camunda profile full          # recreate with full
```

## Minors we support

| Minor    | Notes                                                                                                 |
| -------- | ----------------------------------------------------------------------------------------------------- |
| **8.7**  | Separate Zeebe / Operate / Tasklist. Light → `docker-compose-core.yaml`, full → `docker-compose.yaml` |
| **8.8**  | Consolidated `orchestration`; HTTP on host **8088**                                                   |
| **8.9**  | Consolidated `orchestration`; HTTP on host **8080** (docs default)                                    |
| **8.10** | Preview. Same 8080 mapping as 8.9; ES not bundled; full profile gets our ES overlay                   |

```bash
camunda switch 8.9 --wipe
camunda switch 8.10 --wipe   # expect preview quirks
```

Without `--wipe`, we warn you: old volumes + new minor often break. Prefer wipe when you’re comparing versions.

## Resource profiles

These don’t rewrite Camunda’s compose topology. They write `~/.camunda-lab/resources.env` with heap settings:

| Profile    | Heap setting         | Best for                 |
| ---------- | -------------------- | ------------------------ |
| `small`    | `-Xms256m -Xmx512m`  | 16 GB laptop, light only |
| `balanced` | `-Xms512m -Xmx1024m` | Default                  |
| `power`    | `-Xms1g -Xmx2g`      | Plenty of RAM            |

```bash
camunda resources small
camunda restart
```

## Ports and URLs

Ports come from Camunda’s official compose for each minor
([camunda-distributions](https://github.com/camunda/camunda-distributions)).
`camunda urls` prints the map for **your** active version and profile.

Official docs also differ by version era:

- [8.7 Docker Compose](https://docs.camunda.io/docs/8.7/self-managed/setup/deploy/local/docker-compose/) — separate Operate / Tasklist ports
- [Current Compose configuration](https://docs.camunda.io/docs/self-managed/quickstart/developer-quickstart/docker-compose/configuration.md) — Orchestration Cluster on `:8080` (matches **8.9+**; **8.8** still maps host **8088**)

### 8.7 (and earlier 8.5–8.6 layout)

| App                | URL                                                    |
| ------------------ | ------------------------------------------------------ |
| Operate            | http://localhost:8081                                  |
| Tasklist           | http://localhost:8082                                  |
| Optimize (full)    | http://localhost:8083                                  |
| Identity (full)    | http://localhost:8084                                  |
| Connectors         | http://localhost:8085                                  |
| Zeebe HTTP gateway | http://localhost:8088                                  |
| Web Modeler (full) | http://localhost:8070                                  |
| Keycloak (full)    | http://localhost:18080/auth/                           |
| gRPC               | localhost:26500                                        |
| Elasticsearch      | http://localhost:9200                                  |
| ElasticVue         | http://localhost:9800 (preconfigured → localhost:9200) |

No Console service in 8.7 compose.

### 8.8 (orchestration on host 8088)

| App                | URL                                                                         |
| ------------------ | --------------------------------------------------------------------------- |
| Operate            | http://localhost:8088/operate                                               |
| Tasklist           | http://localhost:8088/tasklist                                              |
| Admin (light)      | http://localhost:8088/admin                                                 |
| REST API           | http://localhost:8088/v2                                                    |
| Connectors         | http://localhost:8086                                                       |
| Console (full)     | http://localhost:8087                                                       |
| Optimize (full)    | http://localhost:8083                                                       |
| Identity (full)    | http://localhost:8084                                                       |
| Web Modeler (full) | http://localhost:8070                                                       |
| Keycloak (full)    | http://localhost:18080/auth/                                                |
| gRPC               | localhost:26500                                                             |
| Elasticsearch      | http://localhost:9200                                                       |
| ElasticVue         | http://localhost:9800 (preconfigured → localhost:9200; light ≤8.8 and full) |

### 8.9 / 8.10 (orchestration on host 8080)

| App                                                 | URL                                                                                                      |
| --------------------------------------------------- | -------------------------------------------------------------------------------------------------------- |
| Operate                                             | http://localhost:8080/operate                                                                            |
| Tasklist                                            | http://localhost:8080/tasklist                                                                           |
| Admin (light)                                       | http://localhost:8080/admin                                                                              |
| REST API                                            | http://localhost:8080/v2                                                                                 |
| Connectors                                          | http://localhost:8086                                                                                    |
| Console (full, 8.9)                                 | http://localhost:8087                                                                                    |
| Optimize / Identity / Web Modeler / Keycloak (full) | same as 8.8 table                                                                                        |
| gRPC                                                | localhost:26500                                                                                          |
| Elasticsearch                                       | http://localhost:9200 (bundled ≤8.8 light / all full ≤8.9; 8.10 full via our overlay; not in 8.9+ light) |
| ElasticVue                                          | http://localhost:9800 when host ES is present (light ≤8.8, full; not modeler / not 8.9+ light)           |
| MCP cluster (AI enabled, 8.9+)                      | http://localhost:8080/mcp/cluster                                                                        |
| MCP processes (AI enabled, 8.10+)                   | http://localhost:8080/mcp/processes                                                                      |

Default web UI credentials: `demo` / `demo`. Keycloak admin: `admin` / `admin`.

ElasticVue ([cars10/elasticvue](https://github.com/cars10/elasticvue)) starts automatically with any profile that exposes Elasticsearch on `:9200`. Open it with `camunda open elasticvue` — cluster **camunda-lab** is preconfigured to `http://localhost:9200`.
