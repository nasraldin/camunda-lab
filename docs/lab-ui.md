# Lab UI

The embedded **Camunda Lab Console** is a local browser control panel for the same workflows as the CLI — install, open apps, tail logs, AI helpers, and reset — without living in the terminal.

`camunda install` (and `camunda up`) **start the UI in the background** automatically and open your browser on first install. You can also manage it manually:

```bash
camunda ui              # ensure background UI + open browser
camunda ui --no-open    # background only
camunda ui --foreground # block in this terminal (Ctrl+C to stop)
camunda ui --stop       # stop background UI
camunda ui logs         # recent background log lines
camunda ui logs -f      # follow background logs
# http://localhost:9090  (loopback only, no auth)
```

Default bind is **`localhost`** (not `127.0.0.1`) so Keycloak SSO cookies match Camunda apps. Override with `--host` / `--port` if needed (`CAMUNDA_LAB_UI_PORT`).

!!! tip "Quick look"
![Lab UI Home](assets/screenshots/lab-ui-home.png)

---

## Start and layout

| Control       | Where          | What it does                                                                                                        |
| ------------- | -------------- | ------------------------------------------------------------------------------------------------------------------- |
| Sidebar       | Left           | Jump between Home, Get started, Apps, BPMN, Cluster, Project, Logins, Services, Logs, AI helpers, Extras, Reset lab |
| Light / Dark  | Sidebar footer | Theme preference (saved in the browser)                                                                             |
| Version       | Sidebar footer | CLI version currently serving the UI                                                                                |
| Project links | Sidebar footer | GitHub, help docs, Camunda Docs, releases                                                                           |

No authentication — only bind loopback addresses.

### Lab API request boundary

The local server enforces more than its loopback bind:

- The request `Host` must literally be `localhost`, `127.0.0.1`, or `[::1]`, optionally followed by a numeric port. This blocks DNS-rebinding and malformed-host requests.
- `GET`, `HEAD`, and `OPTIONS` need no CSRF header.
- Every mutation must have an `Origin` exactly equal to `http://` plus that request `Host`, including its port.
- Every mutation must also send the per-process token from `GET /api/v1/session` in `X-Camunda-Lab-CSRF`. The SPA adds it to JSON, form, multipart, and `DELETE` requests and refetches it once after an invalid-token response.

The token is a same-origin request defense, **not authentication**. Do not expose or proxy the Lab UI beyond loopback.

This boundary belongs to the **Lab API**. It is distinct from the Camunda Compose `csrf-disabled.yaml` overlay described under [Auto sign-in](#auto-sign-in): that local-only overlay disables application CSRF in Operate/Tasklist-style Camunda UIs to preserve new-tab sessions. It does not weaken the Lab API Host, Origin, or token checks.

---

## Home

Status strip (version, profile, resources), lab actions (Start / Stop / Restart), health checks (Doctor / Test apps), optional CLI update, and project links.

![Home](assets/screenshots/lab-ui-home.png)

**Useful actions**

- **Start lab / Stop lab / Restart lab** — same idea as `camunda up` / `down` / restart
- **Doctor** — Docker / compose / config sanity (`camunda doctor`)
- **Test apps** — quick HTTP probes (`camunda smoke`)
- **Update** — when a newer CLI release is available

---

## Get started

Install or switch Camunda minor, profile (`light` / `full` / `modeler`), and resource size (`small` / `balanced` / `power`). Optional AI enable fields for OpenAI-compatible keys.

![Get started](assets/screenshots/lab-ui-setup.png)

Use this when you want a guided install instead of `camunda install` prompts.

---

## Apps

Open Operate, Tasklist, Admin, Console, Identity, Optimize, Web Modeler, ElasticVue, and more — grouped by everyday use.

**Developer endpoints** (orchestration base, REST `/v2`, gRPC, connectors, MCP) are **not** web UIs. Each card explains the endpoint, links to official Camunda docs, and has **Test health** (Lab calls the official verify path — e.g. `:9600/actuator/health`, `/v2/topology`, TCP `26500`).

![Apps](assets/screenshots/lab-ui-apps.png)

### Auto sign-in

Auto sign-in is for labs that include **Keycloak** (full profile). On a **light** lab there is no Keycloak, so the switch is hidden and apps open with direct links.

| Setting                    | Behavior                                                                                                                                                                                                          |
| -------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **On (default, full lab)** | Lab warms the Keycloak session as `demo` / `demo`, then opens the app so you skip the login form                                                                                                                  |
| **Off (full lab)**         | Apps open directly; Lab also signs you out of Camunda so the next open shows the Keycloak login page                                                                                                              |
| **Light lab**              | Auto sign-in stays off. Log in once on Operate/Tasklist/Admin (`demo` / `demo`); they share one Camunda session cookie. Lab disables Camunda CSRF locally so opening apps in new tabs does not force login again. |

Preference is stored in the browser (`camunda-lab-auto-sso`) and only applies when Keycloak is present.

![Apps with auto sign-in off](assets/screenshots/lab-ui-apps-manual.png)

### Session tools

Shown only when Keycloak is in the lab:

- **Sign out of Camunda** — opens Keycloak logout
- **Fix broken session** — same logout path, for login loops / mixed `localhost` vs `127.0.0.1` cookies

Always available:

- **Show all addresses** — copy raw URLs for Desktop Modeler, clients, or AI tools

!!! note "localhost only"
Use `http://localhost:…` for Lab UI and Camunda apps. Mixing `127.0.0.1` breaks shared cookies (Keycloak SSO on full labs, or the Camunda session cookie on light labs).

---

## BPMN toolkit

Sidebar **BPMN** mirrors the CLI toolkit: lint, diff, explain, review, test generate, and secrets scan.

- Upload `.bpmn` files, or pass absolute paths on the machine running Lab UI
- Scan / plan-style flows use a **project directory** (must be under your home, `/tmp`, or lab home)
- Each action shows the equivalent `camunda …` command

## Cluster

Sidebar **Cluster**: incidents list/retry (with refresh), process instance trace (optional bounded follow), deployment **plan** and **drift**. Set an absolute **project path** so incidents/trace/plan/drift resolve the project-local active env (same as CLI). Plan and drift also need `.camunda.yaml` in that directory — create via **Project → Init**.

Uses the same Orchestration REST client as the CLI (OIDC on full labs).

## Project

Sidebar **Project**: `camunda init` scaffold, env profiles, and backup/restore for local Docker Compose labs.

### Backup and restore boundaries

Browser backup streams a gzip tar download (`POST /api/v1/backup/download`). Responses never include server temp paths. The archive includes lab `config.yaml`, AI key-name metadata without values, project `.camunda.yaml` when present, and regular files under the project's configured BPMN/DMN/form paths (or `bpmn/`/`dmn/`/`forms/` defaults). The browser does not expose the CLI's `--include-secrets` opt-in, so it does not put `ai.env` values into its backup. Docker volumes, databases, images, downloaded versions, logs, and other project directories are not backed up.

Authorized absolute output paths can still be written via `POST /api/v1/backup` with an `output` field; empty output is rejected (use the download route instead).

Browser restore requires selecting an archive and then typing exact `RESTORE` in the confirmation dialog. The selected project directory is the destination for `project/...` entries; if it is blank, those entries are validated but not written. The upload is capped by the API at 50 MiB compressed, and the shared restore engine additionally enforces the decompressed limits documented in [CLI backup / restore](cli-reference.md#backup--restore). It validates the complete manifest and payload, rejects unsafe paths and file types, stages privately, and rolls back a failed commit. It replaces archived files but does not remove unrelated files or restore Docker data.

Browser restore uses the same `backup.Service` running-lab gate as the CLI. Without **Force restore while lab is running**, restore refuses when Compose containers are up. Force bypasses only that gate; archive validation and transactional safety still apply.

### Browser confirmation gates

The browser opens an explicit confirmation dialog before incident retry, environment removal, service restart, lab stop/restart, and destructive version switch. Restore additionally requires typed `RESTORE`. Reset lab retains typed `DELETE` and adds a second explicit modal confirmation. These are browser interaction gates; direct API mutations are separately protected by the Lab API Origin/CSRF boundary and, where implemented, server-side confirmation fields. They do not turn backup/restore into a full data backup or make an operation reversible.

---

## Logins

Default credentials for apps and Keycloak, with copy buttons.

![Logins](assets/screenshots/lab-ui-logins.png)

Typical defaults from Camunda’s compose files:

| Who                       | User    | Password |
| ------------------------- | ------- | -------- |
| Operate / Tasklist / apps | `demo`  | `demo`   |
| Keycloak admin (full)     | `admin` | `admin`  |

---

## Services

Friendly names for Compose containers, status/metrics strip, filter chips, search, **Restart**, and **View logs**.

![Services](assets/screenshots/lab-ui-services.png)

---

## Logs

Pick a service, stream recent logs, then **Filter** (hide non-matching lines) or **Highlight** (keep context, mark matches). Near-bottom auto-scroll; link back to Services.

![Logs](assets/screenshots/lab-ui-logs.png)

---

## AI helpers

Enable / disable AI Agent connector secrets and view MCP endpoint hints for Cursor / Claude (8.9+). Same surface as `camunda ai enable` / `config`.

![AI helpers](assets/screenshots/lab-ui-ai.png)

Details: [AI and MCP](ai-mcp.md).

---

## Extras

Install or check **c8ctl**, Desktop Modeler profile helpers, and light deploy helpers.

![Extras](assets/screenshots/lab-ui-extras.png)

---

## Developer toolkit

The BPMN toolkit page exposes the same service-backed lint, semantic diff,
offline explain, optional/required AI review, test generation, and secret scan
contracts as the CLI.

- Lint/review show threshold and ignore controls.
- Diff supports two files, `path`/`against`, and project-relative Git `base`
  mode.
- Review selects optional or required AI enrichment plus provider/model; the UI
  never renders credentials or raw provider response bodies.
- Test generation defaults to an in-memory browser download. Writing requires
  the separate write switch and an authorized absolute output directory.
- Scan shows complete or partial status, findings, issues, and accounting
  statistics.
- Deep doctor presents structured checks; invalid configuration is returned as
  a stable fatal error rather than a partial success.

Every mutation still passes the Lab API Origin/CSRF boundary, and filesystem
paths still pass canonical authorization. These developer tools do not deploy
resources or add platform-operation parity.

HTTP route details: [Lab API reference](api-reference.md). Live cluster acceptance status: [acceptance record](acceptance/platform-toolkit-parity.md).

---

## Reset lab

Danger zone — wipe `~/.camunda-lab` and volumes (same idea as `camunda nuke`). Confirm carefully.

![Reset lab](assets/screenshots/lab-ui-reset.png)

---

## Reproduce these shots

```bash
camunda install --version 8.9 --profile full --resources small --yes
camunda wait
camunda ui --port 9091 --no-open
# open http://localhost:9091/ — Home, Apps, Services, …
```

Camunda app UIs (Operate, Tasklist, …) are documented separately: [App screenshots](screenshots.md).
