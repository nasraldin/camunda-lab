# Troubleshooting

## docker compose not found

Install Docker Compose v2. On Apple Silicon, [docker-lab](https://github.com/nasraldin/docker-lab) can provide a Lima-based Engine.

## Ports in use

`camunda doctor` and `camunda status`. Stop other Camunda/Compose projects or change host port mappings in the extracted compose (advanced).

## Switch between minors fails

Use `camunda switch 8.9 --wipe` to discard volumes. Cross-minor data is often incompatible.

## 8.10 full profile

Elasticsearch is not bundled upstream. camunda-lab applies `overlays/elasticsearch-8.10.yaml`. If start fails, check `camunda logs elasticsearch`.

## Full stack slow to become healthy

Keycloak and Elasticsearch need time. Prefer `camunda wait --timeout 15m`.
