# Installation

## Prerequisites

- macOS or Linux
- Docker Engine with Compose v2

```bash
docker compose version
```

## Build from source

```bash
git clone https://github.com/nasraldin/camunda-lab.git
cd camunda-lab
make build
cp bin/camunda ~/.local/bin/camunda
```

## First run

```bash
camunda install --version 8.8 --profile light --yes
camunda wait
camunda urls
```

State lives in `~/.camunda-lab/` (override with `CAMUNDA_LAB_HOME`).
