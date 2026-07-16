# Installation

## What you need

- **macOS or Linux** (Windows isn’t supported yet)
- **Docker Engine** with **Compose v2**:

```bash
docker compose version
```

- Enough disk and RAM for Elasticsearch (light) or the full stack. On a 16 GB laptop, start with `light` + `small`.

Apple Silicon tip: if you don’t want Docker Desktop, use [docker-lab](https://github.com/nasraldin/docker-lab) for a Lima-based Engine.

## Install the CLI

### Homebrew

```bash
brew tap nasraldin/tools
brew install camunda-lab
camunda about
```

The formula is named `camunda-lab`; you still run `camunda`. See [Homebrew](homebrew.md).

### One-liner

Downloads the GitHub Release binary and verifies `checksums.txt`:

```bash
curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash
```

### From source

```bash
git clone https://github.com/nasraldin/camunda-lab.git
cd camunda-lab
make check
make install
camunda version
```

`make install` copies the binary to `~/.local/bin/camunda`. Put that directory on your `PATH` if it isn’t already.

## Start a lab

### Non-interactive

```bash
camunda install --version 8.9 --profile light --resources small --yes
camunda wait
camunda urls
```

With AI Agent connector secrets + MCP URLs (8.9+ only):

```bash
camunda install --version 8.9 --profile light --resources small --yes \
  --ai --openai-key "$OPENAI_API_KEY"
```

### Interactive

```bash
camunda install
```

You’ll be asked for:

1. **Version** — 8.7, 8.8, 8.9, or 8.10 (preview)
2. **Profile** — light (default), full, or Web Modeler only
3. **Resources** — small / balanced / power (tunes `JAVA_TOOL_OPTIONS`)

## After it comes up

```bash
camunda status
camunda doctor
camunda open operate
camunda open elasticvue   # when host Elasticsearch is published
```

Enable MCP + AI Agent secrets on a running lab:

```bash
camunda ai enable --openai-key "$OPENAI_API_KEY"
camunda ai config
```

See [AI and MCP](ai-mcp.md).

Prefer a browser? After the CLI is installed:

```bash
camunda ui
# http://127.0.0.1:9090 — Overview, Setup, Apps, Logs, AI, …
```

Login defaults from Camunda’s compose files:

| Who | User | Password |
| --- | --- | --- |
| Operate / Tasklist / apps | `demo` | `demo` |
| Keycloak (full profile) | `admin` | `admin` |

## Where state lives

Everything under `~/.camunda-lab/` unless you set `CAMUNDA_LAB_HOME`:

```text
~/.camunda-lab/
  config.yaml          # active version, profile, resources, ai.enabled
  ai.env               # SECRET_* for AI Agent connectors (mode 0600)
  versions/8.9/        # extracted official zip
  overlays/            # ElasticVue, AI secrets, 8.10 ES, …
  resources.env        # heap settings for the resource profile
```

## Tear down

```bash
camunda down          # stop containers, keep volumes
camunda nuke          # wipe the lab home (asks for confirmation)
```

Next: [profiles and versions](profiles.md) · [CLI reference](cli-reference.md)
