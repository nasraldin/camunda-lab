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

### From source

```bash
git clone https://github.com/nasraldin/camunda-lab.git
cd camunda-lab
make build
make install   # ~/.local/bin/camunda
```

### One-liner (after first release)

```bash
curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash
```

### Homebrew (after formula publish)

```bash
brew tap nasraldin/tools
brew install camunda-lab
```

You need Docker + Compose v2. On Apple Silicon, [docker-lab](https://github.com/nasraldin/docker-lab) is an easy Engine if you don’t want Desktop.

---

## Quick start

```bash
camunda install --version 8.8 --profile light --resources small --yes
camunda wait
camunda urls
camunda open operate
```

Interactive:

```bash
camunda install
```

Full stack (Identity, Keycloak, Optimize, Console, Web Modeler):

```bash
camunda install --version 8.8 --profile full --yes
```

Default app login from Camunda’s files: **demo** / **demo**.

---

## Handy commands

| Command | Meaning |
| --- | --- |
| `camunda install` | Fetch zip, configure, start |
| `camunda wait` / `doctor` / `smoke` | Health |
| `camunda urls` / `open` | Where the UIs live |
| `camunda switch 8.9 --wipe` | Another minor, clean volumes |
| `camunda profile light\|full\|modeler` | Compose profile |
| `camunda resources small\|balanced\|power` | Heap settings |
| `camunda tools c8ctl install` | Official deploy/debug CLI |
| `camunda nuke` | Wipe `~/.camunda-lab` |

More detail: [CLI reference](https://nasraldin.github.io/camunda-lab/cli-reference/).

---

## Docs

| Page | About |
| --- | --- |
| [Home](https://nasraldin.github.io/camunda-lab/) | Overview |
| [Installation](https://nasraldin.github.io/camunda-lab/installation/) | First boot |
| [Why Camunda Lab](https://nasraldin.github.io/camunda-lab/comparison/) | vs zip / Helm / 8 Run |
| [Architecture](https://nasraldin.github.io/camunda-lab/architecture/) | How it fits |
| [Troubleshooting](https://nasraldin.github.io/camunda-lab/troubleshooting/) | When it breaks |

---

## Disclaimer

Local development only. Production → [Camunda Helm](https://docs.camunda.io/docs/self-managed/setup/install/).

## License

MIT — see [LICENSE](LICENSE).
