# Camunda Lab

![Camunda](assets/logo-camunda-black.svg){: .brand-logo }

Want Camunda 8 on your machine without Kind, Helm, or a day of YAML archaeology? This lab wraps Camunda’s **official** Docker Compose distributions and gives you a small CLI named **`camunda`**.

Pick a minor (8.7–8.10), pick light or full, start it, open Operate. When you’re done testing an upgrade path, switch versions or wipe the lab clean.

!!! warning "Unofficial"
    Community project. Not affiliated with Camunda GmbH. Great for local tryouts — not a production install. For production, use [Camunda’s Helm charts](https://docs.camunda.io/docs/self-managed/setup/install/).

## Install

=== "From source"

    ```bash
    git clone https://github.com/nasraldin/camunda-lab.git
    cd camunda-lab
    make build
    make install   # puts camunda in ~/.local/bin
    ```

=== "One-liner"

    After the first GitHub Release exists:

    ```bash
    curl -fsSL https://raw.githubusercontent.com/nasraldin/camunda-lab/main/install.sh | bash
    ```

=== "Homebrew"

    Once the formula is published to the tap:

    ```bash
    brew tap nasraldin/tools
    brew install camunda-lab
    ```

    Formula name is `camunda-lab`; the binary is still `camunda`.

You need Docker with Compose v2 (`docker compose version`). On an Apple Silicon Mac, [docker-lab](https://github.com/nasraldin/docker-lab) is a solid way to get that without Desktop.

## First run

```bash
camunda install --version 8.8 --profile light --resources small --yes
camunda wait
camunda urls
camunda open operate
```

Or skip the flags and answer the prompts:

```bash
camunda install
```

Default UI login from Camunda’s compose files: **demo** / **demo**.

## Why bother?

| | Official zip | Helm on Kind | Camunda Lab |
| --- | --- | --- | --- |
| Real Camunda stack | Yes | Yes | Yes (same zips) |
| Need local Kubernetes | No | Yes | No |
| Change 8.8 → 8.9 | Manual | Chart dance | `camunda switch` |
| “Where’s Operate?” | Dig through README | Port-forward | `camunda urls` |
| Doctor / smoke | You write it | You write it | Built in |

## Commands you’ll use most

| Command | Meaning |
| --- | --- |
| `camunda install` | Download the official zip, configure, start |
| `camunda wait` | Sit until the stack looks healthy |
| `camunda urls` / `open` | Ports without guessing |
| `camunda switch 8.9 --wipe` | Try another minor cleanly |
| `camunda doctor` | Docker, compose, config sanity |
| `camunda tools c8ctl install` | Get Camunda’s `c8ctl` for deploy/debug |
| `camunda nuke` | Delete `~/.camunda-lab` and volumes |

## Where to go next

- [Installation](installation.md) — prerequisites, profiles, first boot
- [Why Camunda Lab](comparison.md) — vs zip, Helm, Camunda 8 Run
- [CLI reference](cli-reference.md) — every command
- [Troubleshooting](troubleshooting.md) — when Keycloak won’t wake up

## Source

[github.com/nasraldin/camunda-lab](https://github.com/nasraldin/camunda-lab)
