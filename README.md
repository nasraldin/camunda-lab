# Camunda Lab

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

**Unofficial** local Camunda 8 platform lab. Not affiliated with Camunda GmbH.

Run the official Camunda Docker Compose stack (light / full / Web Modeler) with one CLI — switch minors (8.7–8.10), doctor the stack, and glue developer tools — without standing up Kubernetes.

## Why

| | Official Compose zip | Helm / Kind | **camunda-lab** |
| --- | --- | --- | --- |
| Full / light stack | Yes | Yes | Yes (official zips) |
| Version switch | Manual | Chart version | `camunda switch 8.9` |
| Install DX | Download + extract | k8s required | `camunda install` |
| Doctor / URLs | DIY | DIY | Built-in |

## Prerequisites

- macOS or Linux
- Docker Engine + Compose **v2** (`docker compose version`)

On Apple Silicon, [docker-lab](https://github.com/nasraldin/docker-lab) is one way to get a light Linux Docker host.

## Install

### From source

```bash
git clone https://github.com/nasraldin/camunda-lab.git
cd camunda-lab
make build
./bin/camunda cli-install  # optional: symlink — or: make install
# for now:
sudo cp bin/camunda /usr/local/bin/camunda   # or ~/.local/bin
```

### One-liner (after first release)

```bash
curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash
```

### Homebrew (after tap publish)

```bash
brew tap nasraldin/tools
brew install camunda-lab   # installs binary named camunda
```

## Quick start

```bash
camunda install --version 8.8 --profile light --resources small --yes
camunda wait
camunda urls
camunda open operate
```

Interactive install (prompts for version / profile / resources):

```bash
camunda install
```

Full stack (Identity, Keycloak, Optimize, Console, Web Modeler):

```bash
camunda install --version 8.8 --profile full --yes
```

## Handy commands

| Command | Meaning |
| --- | --- |
| `camunda install` | Fetch official zip, configure, start |
| `camunda up` / `down` / `restart` | Lifecycle |
| `camunda switch 8.9 [--wipe]` | Change minor |
| `camunda profile light\|full\|modeler` | Change compose profile |
| `camunda resources small\|balanced\|power` | Heap / resource env |
| `camunda status` / `urls` / `open` | What’s running |
| `camunda doctor` / `wait` / `smoke` | Health |
| `camunda tools c8ctl install` | Bootstrap official `c8ctl` |
| `camunda tools modeler profile` | Desktop Modeler connection |
| `camunda nuke` | Wipe `~/.camunda-lab` |

## Docs

- [Architecture](docs/architecture.md)
- [Installation](docs/installation.md)
- [CLI reference](docs/cli-reference.md)
- [Troubleshooting](docs/troubleshooting.md)
- [Design spec](docs/superpowers/specs/2026-07-16-camunda-lab-design.md)

## Disclaimer

This is a community project for **local development**. Camunda Docker images may be used in production; the Compose files and this lab are not. Production: [Camunda Helm](https://docs.camunda.io/docs/self-managed/setup/install/).

## License

MIT — see [LICENSE](LICENSE).
