# CLI reference

Binary: **`camunda`**. Project / Homebrew formula: **`camunda-lab`**.

!!! tip "How to read this"
`$` lines are what you type. Output is abbreviated and will differ on your machine.

## Command map

| Command              | Purpose                                                                 |
| -------------------- | ----------------------------------------------------------------------- |
| `init`               | Scaffold a Camunda application project                                  |
| `install`            | Download zip, configure, start                                          |
| `ui`                 | Local Lab UI (http://localhost:9090)                                    |
| `ai`                 | MCP + AI Agent connector secrets                                        |
| `monitoring`         | Prometheus + Grafana dashboards                                         |
| `lint`               | Deterministic BPMN lint                                                 |
| `diff`               | Semantic BPMN diff (files, `--against`, or Git `--base`)                |
| `explain`            | Offline BPMN summary                                                    |
| `review`             | Lint + optional AI review                                               |
| `test generate`      | Test skeletons from BPMN (`-o` output dir)                              |
| `scan`               | Secrets scanner                                                         |
| `env`                | Environment profiles                                                    |
| `plan`               | Deployment preview (no deploy; `--dir`, `--env`, `--json`)              |
| `drift`              | Git/project vs cluster drift (`--dir`, `--ref`, `--env`, `--json`)      |
| `backup` / `restore` | Lab-oriented backup                                                     |
| `incidents`          | Incident list/retry (OIDC on full labs; auto token from lab `.env`)     |
| `trace`              | Process instance timeline                                               |
| `up` / `start`       | Start                                                                   |
| `down` / `stop`      | Stop (keep volumes)                                                     |
| `restart`            | Restart                                                                 |
| `status`             | Config + compose ps                                                     |
| `switch`             | Change minor                                                            |
| `profile`            | light / full / modeler                                                  |
| `resources`          | small / balanced / power                                                |
| `urls` / `open`      | Component links                                                         |
| `logs`               | Container logs                                                          |
| `doctor`             | Diagnostics (`--deep` for component probes)                             |
| `wait` / `smoke`     | Health                                                                  |
| `tools`              | c8ctl + Modeler helpers                                                 |
| `nuke`               | Wipe lab home                                                           |
| `version` / `about`  | Meta                                                                    |

Details for each command are in the sections below.

---

## init

```bash
camunda init
camunda init ./order-service
camunda init ./order-service --name orders --version 8.10 --yes
camunda init ./order-service --force
```

| Flag           | Meaning                                                                           |
| -------------- | --------------------------------------------------------------------------------- |
| `--name`       | Project name (default: directory basename)                                        |
| `--version`    | Camunda version hint in `.camunda.yaml` (default: active lab version or configured default) |
| `--profile`    | Lab profile hint (`light` \| `full` \| `modeler`)                                 |
| `--resources`  | Lab resources hint (`small` \| `balanced` \| `power`)                             |
| `--yes` / `-y` | Non-interactive                                                                   |
| `--force`      | Allow scaffolding into a non-empty directory                                      |

Creates `bpmn/`, `dmn/`, `forms/`, `workers/`, `connectors/`, `scripts/`, `tests/`, `environments/`, `helm/`, stub `docker-compose.yml`, `.camunda.yaml`, and `README.md`.

Does **not** start the lab or deploy processes. Use `camunda install` for a local stack; deploy with official Camunda tooling.

---

## install

```bash
camunda install
camunda install --version 8.8 --profile light --resources small --yes
camunda install --version 8.9 --profile light --yes --ai --openai-key "$OPENAI_API_KEY"
```

| Flag                | Meaning                                         |
| ------------------- | ----------------------------------------------- |
| `--version`         | Minor (`8.7`…`8.10`)                            |
| `--profile`         | `light` \| `full` \| `modeler`                  |
| `--resources`       | `small` \| `balanced` \| `power`                |
| `--yes` / `-y`      | Non-interactive                                 |
| `--ai`              | Enable MCP + AI Agent secrets (8.9+ light/full) |
| `--openai-key`      | `SECRET_OPENAI_API_KEY`                         |
| `--anthropic-key`   | `SECRET_ANTHROPIC_API_KEY`                      |
| `--openai-base-url` | Optional OpenAI-compatible base URL             |

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

| Flag           | Meaning                                             |
| -------------- | --------------------------------------------------- |
| `--host`       | Listen address (loopback only; default `localhost`) |
| `--port`       | Port (default `9090`, or `CAMUNDA_LAB_UI_PORT`)     |
| `--no-open`    | Do not open a browser                               |
| `--foreground` | Run in the foreground (blocks until Ctrl+C)         |
| `--stop`       | Stop the background Lab UI                          |

### ui logs

```bash
camunda ui logs
camunda ui logs -f
camunda ui logs -n 100
```

| Flag             | Meaning                                                |
| ---------------- | ------------------------------------------------------ |
| `-f`, `--follow` | Follow log output (like `tail -f`)                     |
| `-n`, `--lines`  | Recent lines to show (default `50`; `0` = entire file) |

Reads `~/.camunda-lab/logs/ui.log` from the background UI process.

`camunda install` and `camunda up` start the UI in the background automatically (install also opens the browser).

Serves an embedded SPA + `/api/v1` control plane: Overview, Setup, Apps, Containers, Logs, AI/MCP, Tools, Danger. There is no authentication, so the server refuses non-loopback binds. It also rejects requests whose HTTP `Host` is not the literal `localhost`, `127.0.0.1`, or `[::1]` (with an optional numeric port). `GET`, `HEAD`, and `OPTIONS` are read-only; every other method must have an `Origin` exactly equal to `http://<Host>` and the process CSRF token in `X-Camunda-Lab-CSRF`. The browser fetches that token from `GET /api/v1/session`; the token is a same-origin request defense, not authentication.

This Lab API protection is separate from the `csrf-disabled.yaml` Compose overlay. That overlay sets `CAMUNDA_SECURITY_CSRF_ENABLED=false` for local Camunda application UIs to avoid new-tab session failures; it does not disable the Lab API's Host, Origin, or token checks. Spec: [lab UI design](https://github.com/nasraldin/camunda-lab/blob/main/docs/superpowers/specs/2026-07-17-lab-ui-design.md).

---

## ai

```bash
camunda ai enable --openai-key "$OPENAI_API_KEY"
camunda ai status
camunda ai config
camunda ai disable
camunda ai disable --wipe-secrets
```

| Subcommand               | Meaning                                                                                     |
| ------------------------ | ------------------------------------------------------------------------------------------- |
| `enable`                 | Write `~/.camunda-lab/ai.env`, set `ai.enabled`, recreate connectors, print MCP client JSON |
| `disable`                | Clear `ai.enabled` and recreate connectors                                                  |
| `disable --wipe-secrets` | Also delete `ai.env`                                                                        |
| `status`                 | Masked secrets + MCP HTTP probe                                                             |
| `config`                 | Print Cursor/Claude MCP JSON (HTTP on light; `c8ctl mcp-proxy` on full)                     |

Requires Camunda **8.9+** and profile **light** or **full**. See [AI and MCP](ai-mcp.md).

---

## monitoring

```bash
camunda monitoring enable
camunda monitoring status
camunda monitoring disable
camunda open grafana
```

| Subcommand | Meaning |
|------------|---------|
| `enable` | Set `monitoring.enabled`, start Prometheus + Grafana with provisioned dashboards |
| `disable` | Clear `monitoring.enabled` and remove the monitoring containers |
| `status` | Enablement + Grafana health probe |

Grafana on `http://localhost:3000` (`admin`/`admin`), Prometheus on `http://localhost:9490`. See [Monitoring](monitoring.md).

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

## BPMN developer toolkit

These local developer commands use the same toolkit services as the Lab API and
UI. Policy findings exit `1`; invalid input, configuration, Git, incomplete
scans, and other tool failures exit `2`; successful checks exit `0`.

```bash
camunda lint bpmn/order.bpmn --fail-on warning --ignore bpmn/rule --json
camunda explain bpmn/order.bpmn
camunda explain bpmn/order.bpmn --json
camunda review bpmn/order.bpmn --ai --provider openai --model gpt-4o-mini
camunda review bpmn/order.bpmn --ai-required --provider anthropic --model MODEL
camunda test generate bpmn/order.bpmn --lang python -o tests
```

- `lint` accepts repeatable `--ignore`, `--fail-on error|warning`, and `--json`.
- `explain` emits text by default, supports `--json`, and writes Markdown only
  when `--output` is provided. `--json` and `--output` are mutually exclusive
  and combining them exits `2`.
- `review` accepts lint threshold/ignore controls. `--ai` is optional
  enrichment; `--ai-required` makes unavailable or failed enrichment a tool
  failure. Provider response bodies and credentials are never printed.
- `test generate` supports `java`, `js`, and `python`. It writes to explicit
  `--output`, otherwise to configured `paths.tests` (default `tests`), and
  requires `--force` to replace existing artifacts.

These commands do not deploy resources or perform platform operations.

---

## scan

```bash
camunda scan
camunda scan ./connectors
camunda scan --json
camunda scan --fail-on high
camunda scan --ignore 'fixtures/**'
```

The scan recursively inspects supported BPMN, DMN, form, YAML, JSON, environment,
script, JavaScript, TypeScript, Java, Go, properties, and text sources. Built-in
VCS, dependency, vendor, generated, and build-directory exclusions cannot be
negated. When the requested path is inside a project, the nearest
`.camunda.yaml` governs path attribution and ignore loading. Root and nested
`.gitignore` files use directory-relative semantics and are applied first,
followed by the project-root `.camunda-scanignore`, then repeatable `--ignore`
rules; the last matching project/user rule wins. Directory patterns with or
without a trailing slash apply to descendants. Unsafe absolute, traversing, or
malformed patterns are rejected.

The scanner detects bounded source-aware assignments, quoted JSON/YAML keys,
standard `Authorization: Bearer ...` headers, and JWT-shaped access, ID, or
OAuth tokens. It deliberately avoids credential-looking prose, placeholders,
and malformed JWT examples. Bounded match prefixes still report values longer
than detector caps, so an oversized credential cannot become a silent clean
result. Values remain masked.

Runtime references are classified after parsing the source-specific
right-hand-side expression. Shell/environment `$VAR` and `${VAR}` are excluded
when unquoted or double-quoted, where expansion is active; single-quoted forms
are literal credentials. JSON, form JSON, properties, and YAML quoted forms are
also literals. JavaScript/TypeScript `process.env`, `Bun.env`,
`import.meta.env`, and `Deno.env.get`, Java `System.getenv`, and Go `os.Getenv`
are excluded only when they are the actual unquoted right-hand-side expression.
Compound or quoted literals that merely contain those strings are still scanned
as credentials.

Inline suppression is same-line and must follow the credential in valid source
comment syntax: `# camunda-scan-ignore` for shell/environment/YAML,
`// camunda-scan-ignore` or `/* camunda-scan-ignore */` for JavaScript,
TypeScript, or Java, and `<!-- camunda-scan-ignore -->` for BPMN/DMN XML.
JavaScript/TypeScript comment recognition tracks escaped backticks, template
literals, and `${...}` interpolation conservatively, so directive-looking text
inside a template is never a suppression comment.
JSON, form JSON, properties, and plain text have no inline suppression syntax;
directive text inside a value never suppresses a finding.

Text and JSON reports use project-relative slash paths, masked values,
deterministic ordering, source kinds, and explicit scanned/ignored/errored
accounting. A candidate is either a supported regular source file or one
built-in-pruned subtree represented by a single terminal record; vendor and
dependency contents are never recursively enumerated. Binary and oversized
files are reported as accepted ignores. Unreadable or truncated candidates
make the report incomplete and produce a non-zero exit; an incomplete scan
never prints `No secrets found.` The `--fail-on low|medium|high` finding
threshold remains effective even when the scan is incomplete.

Directory traversal and file opening are descriptor-relative and no-follow on
Darwin/Linux. Explicit root symlinks are rejected, and parent/file replacement
races are reported without reading outside the governing project boundary.

---

## plan / drift

```bash
camunda plan
camunda plan --dir /tmp/my-app
camunda plan --env prod
camunda plan --json

camunda drift
camunda drift --dir /tmp/my-app --ref HEAD
camunda drift --env prod --json
```

Both commands require a project with `.camunda.yaml` (discovered by walking up
from the current directory, or via `--dir`). `--env` selects an exact
environment profile; otherwise the project-local then global active profile is
used. `--json` emits the stable domain result. Exit codes follow the comparison
policy (`0` no-changes, `1` changes, `2` unknown/refused/error).

---

## env

```bash
camunda env list
camunda env show
camunda env use staging
camunda env add remote-prod --kind remote --orchestration https://cluster.example/v2 \
  --client-id-env CAMUNDA_CLIENT_ID --client-secret-env CAMUNDA_CLIENT_SECRET \
  --token-url https://login.example/oauth/token --audience zeebe-api --scope openid
camunda env remove remote-prod
```

| Subcommand | Purpose |
| ---------- | ------- |
| `list` | List global and project-local profiles |
| `show` | Print active profile name |
| `use` | Set active profile (project-local when run inside a project) |
| `add` | Create a global profile |
| `remove` | Delete a profile (refuses when referenced) |

| Flag (`env add`) | Meaning |
| ---------------- | ------- |
| `--kind` | `lab` or `remote` (default `remote`) |
| `--orchestration` | Orchestration REST base URL |
| `--client-id-env` | Env var holding OIDC client id (default `CAMUNDA_CLIENT_ID`) |
| `--client-secret-env` | Env var holding OIDC client secret |
| `--token-url` | HTTPS OIDC token endpoint |
| `--token-url-env` | Env var holding token URL instead of `--token-url` |
| `--audience` | OIDC audience for cluster API token |
| `--scope` | OIDC scope |

Secrets are **never** stored in profile YAML — only env-var references. Full labs with Keycloak resolve tokens from lab `.env` automatically for local Orchestration REST calls.

---

## incidents

```bash
camunda incidents list
camunda incidents list --env prod --limit 20
camunda incidents show 9007199254740993
camunda incidents retry 9007199254740993 --yes
camunda incidents retry 9007199254740993 --dry-run
```

| Flag | Meaning |
| ---- | ------- |
| `--env` | Environment profile (default: resolved active) |
| `--limit` | Max incidents returned (default `50`) |
| `--yes` / `-y` | Confirm resolve (`retry` only) |
| `--dry-run` | Validate resolve without mutation |

Uses Orchestration REST `POST /v2/incidents/search` and `POST /v2/incidents/{key}/resolution`. Requires a reachable cluster and OIDC credentials when not on a local full lab.

---

## trace

```bash
camunda trace 9007199254740993
camunda trace 9007199254740993 --env prod --json
camunda trace 9007199254740993 --follow --interval 2s --timeout 5m
camunda trace 9007199254740993 --follow --idle-stop 30s --max-events 10
```

| Flag | Meaning |
| ---- | ------- |
| `--env` | Environment profile |
| `--json` | Emit JSON timeline |
| `--follow` / `-f` | Poll until terminal state or timeout |
| `--interval` | Follow poll interval (default `2s`) |
| `--timeout` | Follow timeout (CLI default `5m`; API follow defaults to `30s`) |
| `--idle-stop` | Stop follow after idle period (**CLI-only**) |
| `--max-events` | Max changed timelines while following (`0` = domain default) |

---

## diff

```bash
camunda diff before.bpmn after.bpmn
camunda diff --from before.bpmn --to after.bpmn
camunda diff current.bpmn --against expected.bpmn
camunda diff bpmn/order.bpmn --base HEAD~1
camunda diff bpmn/order.bpmn --base main --json
```

The single-file `--base REF` form must be run inside a project with
`.camunda.yaml`. Its file argument is project-relative: the command compares
the working-tree BPMN file with the same repository path at `REF`. Absolute
paths, traversal, symlink escapes, directories, DMN, and form inputs are
rejected.

Semantic differences are printed and exit with status `1`. Invalid input,
project discovery, parsing, or Git failures exit with status `2`. No semantic
changes exit with status `0`.

---

## backup / restore

```bash
camunda backup
camunda backup -o ./lab-backup.tar.gz
camunda backup -o ./lab-backup-with-secrets.tar.gz --include-secrets

camunda restore ./lab-backup.tar.gz
camunda restore ./lab-backup.tar.gz --project /absolute/path/to/project
camunda restore ./lab-backup.tar.gz --yes
camunda restore ./lab-backup.tar.gz --yes --force
```

### What a backup contains

- `manifest.json`, including format version, creation time, lab version/profile, the exact payload list, and whether secrets are present.
- Lab `config.yaml`, when it exists.
- AI secret **key names only** in `ai.keys.json` by default. Secret values are excluded.
- `ai.env` values only with the explicit `--include-secrets` opt-in.
- From the detected current Camunda project: `.camunda.yaml` when present, plus regular files under the project's configured BPMN/DMN/form paths (defaults `bpmn/`, `dmn/`, `forms/` when unset). Project symlinks and other special files are rejected. Overlapping configured resource paths are rejected fail-closed.

Backups do **not** contain Docker volumes, databases, container images, downloaded Camunda version directories, logs, generated tests, workers, connectors, scripts, Helm files, arbitrary project files, or environment profiles. This is a lab configuration and model-resource backup, not a complete running-system or data backup.

The archive is created through a temporary file and published with permission `0600`. Treat an archive made with `--include-secrets` as a secret.

### Restore contract

Without `--yes` / `-y`, restore requires the exact, case-sensitive text `RESTORE`. `--yes` skips only that prompt; it does not relax archive validation.

The CLI checks the configured Compose project and refuses restore if any container is running. Stop it with `camunda down`. `--force` bypasses only this running-lab check; it does not bypass path, manifest, type, size, destination, or transactional safety checks.

`--project DIR` selects the destination for archived `project/...` resources. Without it, the CLI uses the current project root when one can be detected. If there is no project destination, project entries are validated but skipped; lab `config.yaml` and optional `ai.env` still restore.

Restore accepts only a gzip tar archive with a version-1 manifest whose file list exactly matches its payload. It rejects absolute/traversing/backslash paths, duplicate entries or destinations, links and special files, unsupported destinations, and symbolic links or incompatible file types in destination paths. Default decompressed limits are:

- at most 10,000 archive entries;
- at most 64 MiB per entry;
- at most 512 MiB total.

The complete archive is validated before destination mutation, then extracted into `0700` staging directories and committed with rollback on failure. Restored directories are created as `0700`; `ai.env` is `0600`; other restored files are `0644`. Restore replaces files named by the archive but does not delete unrelated existing files, restore Docker volumes/databases, or start/restart the lab.

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
