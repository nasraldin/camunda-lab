# Monitoring (Prometheus + Grafana)

An optional add-on that wires **Prometheus** and **Grafana** into your lab with
dashboards for Zeebe/orchestration, Elasticsearch, and the connectors runtime
already provisioned. It's off by default and, like the AI and ElasticVue
add-ons, layers on with a Compose overlay — no change to the base stack.

Local lab only. Grafana signs in with `admin` / `admin`.

## Enable / disable

```bash
camunda monitoring enable     # starts Prometheus + Grafana
camunda open grafana          # http://localhost:3000 (admin/admin)
camunda monitoring status     # enablement + Grafana health probe
camunda monitoring disable    # stops and removes the monitoring containers
```

You can also toggle it from the **Monitoring** page in `camunda ui`, and the
**Apps** page shows Grafana/Prometheus cards once it's on.

## What you get

| Component | Host URL | Notes |
| --- | --- | --- |
| Grafana | `http://localhost:3000` | `admin` / `admin`, dashboards preloaded |
| Prometheus | `http://localhost:9490` | scraper UI (`9090` is the Lab UI, so it's remapped) |

Bundled starter dashboards (folder **Camunda Lab** in Grafana):

- **Zeebe / Orchestration** — JVM, stream-processor throughput, exported events, CPU
- **Elasticsearch** — cluster health, docs, JVM heap, disk (via `elasticsearch-exporter`)
- **Connectors** — JVM, HTTP request rate, threads
- **Optimize** — best-effort placeholder (metrics vary by version)

## How it scrapes

Camunda 8 exposes Spring Boot Actuator metrics on the management port `9600` at
`/actuator/prometheus`. Service names differ by minor — 8.8+ publish the
consolidated cluster as the `orchestration` service; 8.5–8.7 use `zeebe`. The
shipped `prometheus.yml` targets both, plus `connectors` and the ES exporter.

Scrape targets are **best-effort per Camunda minor**: unknown targets simply show
as *down* in Prometheus and their panels stay empty — harmless for a lab. Tune
them by editing:

```
~/.camunda-lab/overlays/monitoring/prometheus.yml
```

Restart to pick up changes: `camunda restart` (or re-run `camunda monitoring enable`).

## Notes

- The ES exporter only reports when the lab publishes Elasticsearch on the
  Compose network (host-ES profiles); otherwise its panels stay empty.
- Dashboards are editable in Grafana; provisioned copies are re-applied on
  restart, so save a copy under a new name if you want to keep custom edits.
- Prometheus retention is short (2h) — this is a lab, not long-term storage.
