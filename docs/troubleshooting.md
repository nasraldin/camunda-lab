# Troubleshooting

## `docker compose` not found

You need Compose **v2** as a Docker plugin:

```bash
docker compose version
```

If that fails, fix Docker first. On Apple Silicon without Desktop, try [docker-lab](https://github.com/nasraldin/docker-lab).

`camunda doctor` should call this out clearly.

## Install hangs or never becomes healthy

Full profile waits on Keycloak + Elasticsearch. Give it time:

```bash
camunda wait --timeout 15m
camunda logs -f keycloak
camunda logs -f elasticsearch
```

Light profile is much kinder for a first try.

## Port already allocated

Something else owns the ports for your minor — see [Ports and URLs](profiles.md#ports-and-urls).

Common conflicts:

| Era | Busy hosts |
| --- | --- |
| 8.7 | `8081` / `8082` / `8088` / `9200` / `18080` |
| 8.8 | `8088` / `8086` / `9200` / `18080` |
| 8.9+ | `8080` / `8086` / `9200` / `18080` |

```bash
camunda urls    # what this lab expects
camunda status
camunda down
# stop the other compose project or Desktop stack fighting for ports
camunda up
```

## Switch to another minor broke everything

Expected without a wipe. Elasticsearch indices and Keycloak DB don’t always travel between minors.

```bash
camunda switch 8.9 --wipe
```

## 8.10 full profile and Elasticsearch

Camunda doesn’t ship ES in the 8.10 compose the same way. We drop in `overlays/elasticsearch-8.10.yaml`. If it still fails:

```bash
camunda logs -f elasticsearch
camunda doctor
```

## ElasticVue cannot connect to the cluster

ElasticVue runs at `http://localhost:9800` and talks to `http://localhost:9200` from your browser (CORS is enabled via our overlay).

```bash
camunda urls          # confirm elasticvue + elasticsearch rows
curl -s http://localhost:9200  # ES must answer on the host
camunda down && camunda up     # recreate so CORS + ElasticVue overlays apply
camunda open elasticvue
```

Not listed for **modeler**, **8.9+ light**, or any profile without host Elasticsearch.

## Connectors URL shows “No static resource”

Connectors is a **runtime** (job workers for BPMN connector tasks), not a web console. Official Camunda docs verify it with Actuator:

`http://localhost:8086/actuator/health` (8.8+/light; older light labs use `8085`).

Lab Apps shows a **Developer endpoints** card (describe + docs + **Test health**). `camunda urls` / `camunda open connectors` point at that health URL. Opening bare `/` on the connectors port is expected to fail.

## Orchestration / REST / gRPC are not websites

These are client endpoints, not Camunda web apps:

| Entry | Display address | How to verify (official) |
| --- | --- | --- |
| Orchestration | `http://localhost:8080` (8.9+) | `GET http://localhost:9600/actuator/health` |
| REST API | `http://localhost:8080/v2` | `GET http://localhost:8080/v2/topology` — bare `GET /v2` is **404 by design** |
| gRPC | `localhost:26500` | TCP open (Camunda / Zeebe client) |

In Lab UI → **Apps → Developer endpoints**, each card explains the endpoint, links to Camunda docs, and has **Test health**.

## `camunda incidents` / `plan` return HTTP 401 on full profile

Full labs protect `/v2` with OIDC. The CLI now fetches a token automatically using the Compose **connectors** client from `~/.camunda-lab/versions/<ver>/.env`.

Override if needed:

```bash
export CAMUNDA_ACCESS_TOKEN=…          # raw bearer
# or
export CAMUNDA_CLIENT_ID=connectors
export CAMUNDA_CLIENT_SECRET=demo-connectors-secret
export CAMUNDA_OAUTH_URL=http://localhost:18080/auth/realms/camunda-platform/protocol/openid-connect/token
```

Light labs leave the API unprotected by default — no token needed.

## `camunda plan` / `drift`: no `.camunda.yaml`

These commands need a project scaffold (not the camunda-lab repo root):

```bash
camunda init /tmp/my-app -y
cd /tmp/my-app   # or: camunda plan --dir /tmp/my-app
camunda plan
```

## Apps ask for login again (401 on `/v2/...` stats)

Camunda 8.9+ Operate/Tasklist store a CSRF header token in **tab** `sessionStorage`. A new tab keeps `camunda-session` but loses that token, so POSTs return **401** and the UI shows the login form even though you are still signed in.

camunda-lab disables CSRF for local Compose labs (`csrf-disabled.yaml` → `CAMUNDA_SECURITY_CSRF_ENABLED=false`). After upgrading or if you still see the bug:

```bash
camunda down && camunda up
```

Then sign in once and open Operate/Tasklist again from Apps — new tabs should stay signed in.

Also keep using `http://localhost:…` (not `127.0.0.1`) so the session cookie matches.

## HTTP 400 Bad Request on Optimize / Identity / apps

Full-profile SSO stores large cookies. Tomcat rejects oversized request headers with **HTTP 400**.

1. In the browser, clear cookies/site data for `localhost` (or at least ports `8080`, `8083`, `8084`, `18080`).
2. Hard-refresh and sign in again with **demo** / **demo**.

camunda-lab raises `SERVER_MAX_HTTP_REQUEST_HEADER_SIZE` on Optimize, Identity, and orchestration. Recreate if you upgraded mid-session:

```bash
camunda down && camunda up
```

## `camunda tools c8ctl install` fails

Needs Node/npm on your PATH. Install Node, or install `@camunda8/cli` yourself, then:

```bash
camunda tools c8ctl status
```

## Wrong Operate URL after profile change

Light and full use different published ports on some minors. Trust:

```bash
camunda urls
```

…after every profile or version change.

## `camunda ai enable` rejected

Needs Camunda **8.9+** and profile **light** or **full** (not modeler). Provide at least one of `--openai-key`, `--anthropic-key`, or `--openai-base-url` (or the matching `SECRET_*` env vars). Details: [AI and MCP](ai-mcp.md).

## MCP endpoint returns 401 (full profile)

Expected with OIDC. Use:

```bash
camunda ai config
camunda tools c8ctl install   # if you need mcp-proxy
```

Light profile can use direct HTTP to `http://localhost:8080/mcp/cluster`.

## Start over

```bash
camunda nuke --yes
camunda install --version 8.9 --profile light --resources small --yes
```

Still stuck? Open an issue with `camunda doctor` output and your OS / Docker version: [github.com/nasraldin/camunda-lab/issues](https://github.com/nasraldin/camunda-lab/issues).
