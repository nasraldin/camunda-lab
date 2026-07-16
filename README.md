<p align="center">
  <img src="docs/assets/logo-camunda-black.svg" alt="Camunda" width="220">
</p>

<h1 align="center">Camunda Lab</h1>

<p align="center">
  <a href="https://github.com/nasraldin/camunda-lab/actions/workflows/ci.yml"><img src="https://github.com/nasraldin/camunda-lab/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/nasraldin/camunda-lab/actions/workflows/docs.yml"><img src="https://github.com/nasraldin/camunda-lab/actions/workflows/docs.yml/badge.svg" alt="Docs"></a>
  <a href="https://github.com/nasraldin/camunda-lab/releases/tag/v0.5.0"><img src="https://img.shields.io/badge/release-v0.5.0-blue" alt="v0.5.0"></a>
  <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-yellow.svg" alt="License: MIT"></a>
  <a href="https://nasraldin.github.io/camunda-lab/"><img src="https://img.shields.io/badge/docs-GitHub%20Pages-indigo" alt="Docs site"></a>
</p>

Unofficial local Camunda 8 lab. Not affiliated with Camunda GmbH.

Camunda already ships solid Docker Compose files. What’s missing is the day-to-day glue: fetch the right zip, pick light vs full, wait until Keycloak is up, remember which port is Operate, switch 8.8 → 8.9 without leaving a mess. That’s what **`camunda`** does — no Kubernetes required.

**Docs:** [https://nasraldin.github.io/camunda-lab/](https://nasraldin.github.io/camunda-lab/) · **Latest:** [v0.5.0](https://github.com/nasraldin/camunda-lab/releases/tag/v0.5.0)

<p align="center">
  <img src="docs/assets/screenshots/lab-ui-home.png" alt="Camunda Lab UI — Home" width="920">
</p>

<p align="center"><em>Lab UI — local browser control panel (<code>camunda ui</code>)</em></p>

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

Needs Docker + Compose v2. On Apple Silicon, [docker-lab](https://github.com/nasraldin/docker-lab) is an easy Engine if you don’t want Desktop.

---

## Quick start

```bash
camunda install --version 8.9 --profile light --resources small --yes
camunda wait
camunda urls
camunda open operate
camunda open elasticvue   # when the profile publishes Elasticsearch
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
# or at install:
# camunda install --version 8.9 --profile light --yes --ai --openai-key "$OPENAI_API_KEY"
```

**Lab UI** (embedded control panel, no auth, localhost only):

```bash
camunda ui
# http://localhost:9090
```

Walkthrough of every page (Apps auto sign-in, Services, Logs, …): [Lab UI docs](https://nasraldin.github.io/camunda-lab/lab-ui/).

Default app login: **demo** / **demo**.

Ports differ by Camunda minor — run `camunda urls` (see [profiles](https://nasraldin.github.io/camunda-lab/profiles/)).

---

## Handy commands

| Command | Meaning |
| --- | --- |
| `camunda install` | Fetch zip, configure, start |
| `camunda about` | Project + runtime info |
| `camunda wait` / `doctor` / `smoke` | Health |
| `camunda urls` / `open` | Where the UIs live |
| `camunda ui` | Local Lab UI (http://localhost:9090) |
| `camunda ai enable` / `config` | MCP + AI Agent secrets (8.9+) |
| `camunda switch 8.9 --wipe` | Another minor, clean volumes |
| `camunda profile light\|full\|modeler` | Compose profile |
| `camunda resources small\|balanced\|power` | Heap settings |
| `camunda tools c8ctl install` | Official deploy/debug CLI |
| `camunda nuke` | Wipe `~/.camunda-lab` |

More: [CLI reference](https://nasraldin.github.io/camunda-lab/cli-reference/) · [AI and MCP](https://nasraldin.github.io/camunda-lab/ai-mcp/) · [Roadmap](https://nasraldin.github.io/camunda-lab/roadmap/).

---

## Docs

| Page | About |
| --- | --- |
| [Home](https://nasraldin.github.io/camunda-lab/) | Overview |
| [Installation](https://nasraldin.github.io/camunda-lab/installation/) | First boot |
| [Lab UI](https://nasraldin.github.io/camunda-lab/lab-ui/) | Browser control panel |
| [Why Camunda Lab](https://nasraldin.github.io/camunda-lab/comparison/) | vs zip / Helm / 8 Run |
| [Profiles](https://nasraldin.github.io/camunda-lab/profiles/) | Versions, ports, ElasticVue |
| [AI and MCP](https://nasraldin.github.io/camunda-lab/ai-mcp/) | Cursor MCP + connector secrets |
| [Roadmap](https://nasraldin.github.io/camunda-lab/roadmap/) | Shipped and next |
| [Troubleshooting](https://nasraldin.github.io/camunda-lab/troubleshooting/) | When it breaks |

---

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md), [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md), and [SECURITY.md](SECURITY.md).

## Disclaimer

Local development only. Production → [Camunda Helm](https://docs.camunda.io/docs/self-managed/setup/install/).

## License

MIT — see [LICENSE](LICENSE).
