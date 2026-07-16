#!/usr/bin/env bash
# LIVE smoke: light profile 8.8 — requires Docker + significant RAM/time
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export CAMUNDA_LAB_HOME="${CAMUNDA_LAB_HOME:-$(mktemp -d)}"
echo "CAMUNDA_LAB_HOME=$CAMUNDA_LAB_HOME"
go build -o bin/camunda ./cmd/camunda
./bin/camunda install --version 8.8 --profile light --resources small --yes
./bin/camunda wait --timeout 15m
./bin/camunda smoke
./bin/camunda urls
./bin/camunda down
echo "LIVE smoke OK"
