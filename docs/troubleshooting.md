# Troubleshooting

## `docker compose` not found

You need Compose **v2** as a Docker plugin:

```bash
docker compose version
```

If that fails, fix Docker first. On Apple Silicon without Desktop, try [docker-lab](https://github.com/nasraldin/docker-lab).

`camunda doctor` should call this out clearly.

## Install hangs or never becomes healthy

Full profile waits on Keycloak + Elasticsearch. Give it time:

```bash
camunda wait --timeout 15m
camunda logs -f keycloak
camunda logs -f elasticsearch
```

Light profile is much kinder for a first try.

## Port already allocated

Something else owns the ports for your minor — see [Ports and URLs](profiles.md#ports-and-urls).

Common conflicts:

| Era | Busy hosts |
| --- | --- |
| 8.7 | `8081` / `8082` / `8088` / `9200` / `18080` |
| 8.8 | `8088` / `8086` / `9200` / `18080` |
| 8.9+ | `8080` / `8086` / `9200` / `18080` |

```bash
camunda urls    # what this lab expects
camunda status
camunda down
# stop the other compose project or Desktop stack fighting for ports
camunda up
```

## Switch to another minor broke everything

Expected without a wipe. Elasticsearch indices and Keycloak DB don’t always travel between minors.

```bash
camunda switch 8.9 --wipe
```

## 8.10 full profile and Elasticsearch

Camunda doesn’t ship ES in the 8.10 compose the same way. We drop in `overlays/elasticsearch-8.10.yaml`. If it still fails:

```bash
camunda logs -f elasticsearch
camunda doctor
```

## ElasticVue cannot connect to the cluster

ElasticVue runs at `http://localhost:9800` and talks to `http://localhost:9200` from your browser (CORS is enabled via our overlay).

```bash
camunda urls          # confirm elasticvue + elasticsearch rows
curl -s http://localhost:9200  # ES must answer on the host
camunda down && camunda up     # recreate so CORS + ElasticVue overlays apply
camunda open elasticvue
```

Not listed for **modeler**, **8.9+ light**, or any profile without host Elasticsearch.

## `camunda tools c8ctl install` fails

Needs Node/npm on your PATH. Install Node, or install `@camunda8/cli` yourself, then:

```bash
camunda tools c8ctl status
```

## Wrong Operate URL after profile change

Light and full use different published ports on some minors. Trust:

```bash
camunda urls
```

…after every profile/version change.

## Nuclear option

```bash
camunda nuke --yes
camunda install --version 8.8 --profile light --resources small --yes
```

Still stuck? Open an issue with `camunda doctor` output and your OS / Docker version: [github.com/nasraldin/camunda-lab/issues](https://github.com/nasraldin/camunda-lab/issues).
