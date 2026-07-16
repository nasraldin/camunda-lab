# Installation

## What you need

- **macOS or Linux** (Windows isn’t supported yet)
- **Docker Engine** with **Compose v2** — check with:

```bash
docker compose version
```

- Disk and RAM for Elasticsearch (light) or the full stack (heavier). On a 16 GB laptop prefer `light` + `small` resources.

Apple Silicon tip: if you don’t want Docker Desktop, use [docker-lab](https://github.com/nasraldin/docker-lab) / `ducker` for a Lima-based Engine.

## Install the CLI

### From source (works today)

```bash
git clone https://github.com/nasraldin/camunda-lab.git ~/homelab/camunda-lab
cd ~/homelab/camunda-lab
make build
make install
camunda version
```

`make install` copies the binary to `~/.local/bin/camunda`. Put that directory on your `PATH` if it isn’t already.

### One-liner

After a release is published on GitHub:

```bash
curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash
```

### Homebrew

```bash
brew tap nasraldin/tools
brew install camunda-lab
```

The tap formula is named `camunda-lab` so it doesn’t clash with anything else; you still run `camunda`.

## Start a lab

### Non-interactive (scripts / CI)

```bash
camunda install --version 8.8 --profile light --resources small --yes
camunda wait
camunda urls
```

### Interactive

```bash
camunda install
```

You’ll get asked for:

1. **Version** — 8.7, 8.8, 8.9, or 8.10 (marked preview)
2. **Profile** — light (default), full, or Web Modeler only
3. **Resources** — small / balanced / power (tunes `JAVA_TOOL_OPTIONS`)

## After it comes up

```bash
camunda status
camunda doctor
camunda open operate
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
  config.yaml          # active version, profile, resources
  versions/8.8/        # extracted official zip
  overlays/            # our thin helpers (e.g. 8.10 Elasticsearch)
  resources.env        # heap settings for the resource profile
```

## Tear down

```bash
camunda down          # stop containers, keep volumes
camunda nuke          # wipe the lab home (asks for confirmation)
```

Next: [profiles and versions](profiles.md) · [CLI reference](cli-reference.md)
