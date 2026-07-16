<p align="center">
  <img src="docs/assets/logo-camunda-black.svg" alt="Camunda" width="220">
</p>

<h1 align="center">Camunda Lab</h1>

<p align="center">
  <a href="https://github.com/nasraldin/camunda-lab/actions/workflows/ci.yml"><img src="https://github.com/nasraldin/camunda-lab/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/nasraldin/camunda-lab/actions/workflows/docs.yml"><img src="https://github.com/nasraldin/camunda-lab/actions/workflows/docs.yml/badge.svg" alt="Docs"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://nasraldin.github.io/camunda-lab/"><img src="https://img.shields.io/badge/docs-GitHub%20Pages-indigo" alt="Docs site"></a>
</p>

Unofficial local Camunda 8 lab. Not affiliated with Camunda GmbH.

Camunda already ships solid Docker Compose files. What’s missing is the day-to-day glue: fetch the right zip, pick light vs full, wait until Keycloak wakes up, remember which port is Operate, switch 8.8 → 8.9 without leaving a mess. That’s what **`camunda`** does — without asking you to stand up Kubernetes.

Docs: [https://nasraldin.github.io/camunda-lab/](https://nasraldin.github.io/camunda-lab/)

---

## Why bother?

| | Official zip | Helm on Kind | Camunda Lab |
| --- | --- | --- | --- |
| Real Camunda stack | Yes | Yes | Yes (same zips) |
| Need local k8s | No | Yes | No |
| Change minor | Manual | Chart work | `camunda switch` |
| Doctor / URLs | DIY | DIY | Built in |

---

## Install

### Homebrew

```bash
brew tap nasraldin/tools
brew install camunda-lab
camunda about
```

### One-liner (checksum-verified release binary)

```bash
curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash
```

### From source

```bash
git clone https://github.com/nasraldin/camunda-lab.git
cd camunda-lab
make check
make install   # ~/.local/bin/camunda
```

You need Docker + Compose v2. On Apple Silicon, [docker-lab](https://github.com/nasraldin/docker-lab) is an easy Engine if you don’t want Desktop.

---

## Quick start

```bash
camunda install --version 8.9 --profile light --resources small --yes
camunda wait
camunda urls
camunda open operate
camunda open elasticvue   # ES browser UI when the profile has Elasticsearch
```

Interactive:

```bash
camunda install
```

Full stack (Identity, Keycloak, Optimize, Console, Web Modeler):

```bash
camunda install --version 8.9 --profile full --yes
```

**AI Agent + MCP** (8.9+, no local LLM required):

```bash
camunda ai enable --openai-key "$OPENAI_API_KEY"
camunda ai config    # Cursor / Claude MCP JSON
# or: camunda install --version 8.9 --profile light --yes --ai --openai-key "$OPENAI_API_KEY"
```

Default app login from Camunda’s files: **demo** / **demo**.

Ports differ by Camunda minor — run `camunda urls` (see [profiles](https://nasraldin.github.io/camunda-lab/profiles/)).

---

## Handy commands

| Command | Meaning |
| --- | --- |
| `camunda install` | Fetch zip, configure, start |
| `camunda about` | Project + runtime info card |
| `camunda wait` / `doctor` / `smoke` | Health |
| `camunda urls` / `open` | Where the UIs live (incl. `open elasticvue` when ES is on) |
| `camunda ai enable` / `config` | MCP + AI Agent connector secrets (8.9+) |
| `camunda switch 8.9 --wipe` | Another minor, clean volumes |
| `camunda profile light\|full\|modeler` | Compose profile |
| `camunda resources small\|balanced\|power` | Heap settings |
| `camunda tools c8ctl install` | Official deploy/debug CLI |
| `camunda nuke` | Wipe `~/.camunda-lab` |

More detail: [CLI reference](https://nasraldin.github.io/camunda-lab/cli-reference/) · [AI and MCP](https://nasraldin.github.io/camunda-lab/ai-mcp/).

---

## Docs

| Page | About |
| --- | --- |
| [Home](https://nasraldin.github.io/camunda-lab/) | Overview |
| [Installation](https://nasraldin.github.io/camunda-lab/installation/) | First boot |
| [AI and MCP](https://nasraldin.github.io/camunda-lab/ai-mcp/) | Cursor MCP + AI Agent secrets |
| [Why Camunda Lab](https://nasraldin.github.io/camunda-lab/comparison/) | vs zip / Helm / 8 Run |
| [Architecture](https://nasraldin.github.io/camunda-lab/architecture/) | How it fits |
| [App screenshots](https://nasraldin.github.io/camunda-lab/screenshots/) | What Operate / Tasklist / Admin look like |
| [Troubleshooting](https://nasraldin.github.io/camunda-lab/troubleshooting/) | When it breaks |

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).

## Disclaimer

Local development only. Production → [Camunda Helm](https://docs.camunda.io/docs/self-managed/setup/install/).

## License

MIT — see [LICENSE](LICENSE).
