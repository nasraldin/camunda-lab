# CLI reference

Binary: **`camunda`**. Project / Homebrew formula: **`camunda-lab`**.

!!! tip "How to read this"
    `$` lines are what you type. Output is abbreviated and will differ on your machine.

## Command map

| Command | Purpose |
| --- | --- |
| `init` | Scaffold a Camunda application project |
| `install` | Download zip, configure, start |
| `ui` | Local Lab UI (http://localhost:9090) |
| `ai` | MCP + AI Agent connector secrets |
| `lint` | Deterministic BPMN lint |
| `diff` | Semantic BPMN diff (`--from`/`--to`, `--against`, or two files) |
| `explain` | Offline BPMN summary |
| `review` | Lint + optional AI review |
| `test generate` | Test skeletons from BPMN (`-o` output dir) |
| `scan` | Secrets scanner |
| `env` | Environment profiles |
| `plan` | Deployment preview (no deploy; needs `.camunda.yaml`, optional `--dir`) |
| `drift` | Git/project vs cluster drift (optional `--dir`) |
| `backup` / `restore` | Lab-oriented backup |
| `incidents` | Incident list/retry (OIDC on full labs; auto token from lab `.env`) |
| `trace` | Process instance timeline |
| `k8s` | kubectl helpers for Helm releases |
| `up` / `start` | Start |
| `down` / `stop` | Stop (keep volumes) |
| `restart` | Restart |
| `status` | Config + compose ps |
| `switch` | Change minor |
| `profile` | light / full / modeler |
| `resources` | small / balanced / power |
| `urls` / `open` | Component links |
| `logs` | Container logs |
| `doctor` | Diagnostics (`--deep` for component probes) |
| `wait` / `smoke` | Health |
| `tools` | c8ctl + Modeler helpers |
| `nuke` | Wipe lab home |
| `version` / `about` | Meta |

Details for each command are in the sections below.

---

## init

```bash
camunda init
camunda init ./order-service
camunda init ./order-service --name orders --version 8.9 --yes
camunda init ./order-service --force
```

| Flag | Meaning |
|------|---------|
| `--name` | Project name (default: directory basename) |
| `--version` | Camunda version hint in `.camunda.yaml` (default: active lab version, else `8.9`) |
| `--profile` | Lab profile hint (`light` \| `full` \| `modeler`) |
| `--resources` | Lab resources hint (`small` \| `balanced` \| `power`) |
| `--yes` / `-y` | Non-interactive |
| `--force` | Allow scaffolding into a non-empty directory |

Creates `bpmn/`, `dmn/`, `forms/`, `workers/`, `connectors/`, `scripts/`, `tests/`, `environments/`, `helm/`, stub `docker-compose.yml`, `.camunda.yaml`, and `README.md`.

Does **not** start the lab or deploy processes. Use `camunda install` for a local stack; deploy with official Camunda tooling.

---

## install

```bash
camunda install
camunda install --version 8.8 --profile light --resources small --yes
camunda install --version 8.9 --profile light --yes --ai --openai-key "$OPENAI_API_KEY"
```

| Flag | Meaning |
|------|---------|
| `--version` | Minor (`8.7`…`8.10`) |
| `--profile` | `light` \| `full` \| `modeler` |
| `--resources` | `small` \| `balanced` \| `power` |
| `--yes` / `-y` | Non-interactive |
| `--ai` | Enable MCP + AI Agent secrets (8.9+ light/full) |
| `--openai-key` | `SECRET_OPENAI_API_KEY` |
| `--anthropic-key` | `SECRET_ANTHROPIC_API_KEY` |
| `--openai-base-url` | Optional OpenAI-compatible base URL |

See [AI and MCP](ai-mcp.md).

Fetches the official zip into `~/.camunda-lab/versions/<minor>/`, writes config, runs compose up.

---

## ui

```bash
camunda ui
camunda ui --no-open
camunda ui --foreground
camunda ui --stop
camunda ui logs
camunda ui logs -f
camunda ui logs -n 100
camunda ui --port 9091
```

| Flag | Meaning |
|------|---------|
| `--host` | Listen address (loopback only; default `localhost`) |
| `--port` | Port (default `9090`, or `CAMUNDA_LAB_UI_PORT`) |
| `--no-open` | Do not open a browser |
| `--foreground` | Run in the foreground (blocks until Ctrl+C) |
| `--stop` | Stop the background Lab UI |

### ui logs

```bash
camunda ui logs
camunda ui logs -f
camunda ui logs -n 100
```

| Flag | Meaning |
|------|---------|
| `-f`, `--follow` | Follow log output (like `tail -f`) |
| `-n`, `--lines` | Recent lines to show (default `50`; `0` = entire file) |

Reads `~/.camunda-lab/logs/ui.log` from the background UI process.

`camunda install` and `camunda up` start the UI in the background automatically (install also opens the browser).

Serves an embedded SPA + `/api/v1` control plane: Overview, Setup, Apps, Containers, Logs, AI/MCP, Tools, Danger. No auth — refuses non-loopback binds. Spec: [lab UI design](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-lab-ui-design.md).

---

## ai

```bash
camunda ai enable --openai-key "$OPENAI_API_KEY"
camunda ai status
camunda ai config
camunda ai disable
camunda ai disable --wipe-secrets
```

| Subcommand | Meaning |
|------------|---------|
| `enable` | Write `~/.camunda-lab/ai.env`, set `ai.enabled`, recreate connectors, print MCP client JSON |
| `disable` | Clear `ai.enabled` and recreate connectors |
| `disable --wipe-secrets` | Also delete `ai.env` |
| `status` | Masked secrets + MCP HTTP probe |
| `config` | Print Cursor/Claude MCP JSON (HTTP on light; `c8ctl mcp-proxy` on full) |

Requires Camunda **8.9+** and profile **light** or **full**. See [AI and MCP](ai-mcp.md).

---

## up / down / restart

```bash
camunda up
camunda down
camunda restart
```

`down` keeps volumes. Use `nuke` or `switch --wipe` when you want empty disks.

---

## status

```bash
camunda status
```

Prints active version / profile / resources and `docker compose ps` for project `camunda-lab`.

---

## switch

```bash
camunda switch 8.9
camunda switch 8.9 --wipe
camunda switch 8.9 --ai --openai-key "$OPENAI_API_KEY"
```

Changes the active minor, downloads that zip if needed, starts again. `--wipe` runs compose down with volumes first. `--ai` enables MCP + AI Agent secrets after a successful switch (8.9+ only; validated before switch).

---

## profile / resources

```bash
camunda profile full
camunda resources power
camunda restart
```

`profile` recreates the stack with a different compose file set. `resources` rewrites `resources.env`; restart to apply.

---

## urls / open

```bash
camunda urls
camunda open operate
camunda open keycloak
camunda open elasticvue
```

URLs depend on the active **minor** (8.7 vs 8.8 vs 8.9+). See [Ports and URLs](profiles.md#ports-and-urls).

When the profile exposes Elasticsearch, ElasticVue is included at `http://localhost:9800` with cluster **camunda-lab** preconfigured — no manual cluster add.

`open` uses `open` on macOS and `xdg-open` on Linux.

---

## logs

```bash
camunda logs
camunda logs -f orchestration
camunda logs -f keycloak
```

---

## doctor

```bash
camunda doctor
camunda doctor --fix
camunda doctor --deep
camunda doctor --deep --timeout 5s
```

Checks Docker, Compose v2, config, whether the version directory exists. Mentions optional `cosign` if you care about signed zip verify later.

`--deep` also probes lab HTTP/TCP endpoints (Operate, Tasklist, orchestration, gRPC, …) and prints Healthy / Warnings / Failures.

---

## wait / smoke

```bash
camunda wait
camunda wait --timeout 15m
camunda smoke
```

`wait` polls HTTP endpoints until they answer (or timeout). `smoke` is a one-shot check.

---

## tools

```bash
camunda tools c8ctl status
camunda tools c8ctl install
camunda tools modeler profile
```

`c8ctl install` runs `npm install -g @camunda8/cli` when npm is available.  
`modeler profile` writes a Desktop Modeler connection profile named `camunda-lab`.

---

## nuke

```bash
camunda nuke
CONFIRM=yes camunda nuke
camunda nuke --yes
```

Stops with volumes and deletes `~/.camunda-lab` (or `CAMUNDA_LAB_HOME`). Destructive — that’s the point.

---

## version / about

```bash
camunda version
camunda about
```

`about` prints a docker-lab-style info card: author, CLI version, lab paths, active version/profile, Docker/Compose runtime, and feature list.

```console
$ camunda about
Camunda Lab
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  Author      Nasr Aldin
  Website     https://nasraldin.com

  Version     0.5.0
  Tagline     Local Camunda 8 platform lab (Docker Compose)
  CLI path    /opt/homebrew/bin/camunda
  Global      /opt/homebrew/bin/camunda
  Lab home    /Users/you/.camunda-lab
  Config      /Users/you/.camunda-lab/config.yaml
  Versions    /Users/you/.camunda-lab/versions
  Active      version=8.9 profile=light resources=balanced project=camunda-lab

  Docker      Docker 28.x (engine 28.x)
  Compose     Docker Compose v2.x
  Platform    Apple M1 Max
  Memory      64 GB

  Features    compose · profiles · version-switch · overlays · elasticvue · ai · mcp · ui · c8ctl · modeler · doctor · smoke

  Repo        https://github.com/nasraldin/camunda-lab
  Docs        https://nasraldin.github.io/camunda-lab/

  Commands    N available — run: camunda help

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Unofficial community project — wraps Camunda's official Compose. Not affiliated with Camunda GmbH.
```
