# ElasticVue auto-connect for Camunda Lab

**Date:** 2026-07-17  
**Status:** Draft — awaiting user review  
**Repo:** camunda-lab

## Goal

Give every lab profile that exposes Elasticsearch a zero-setup [ElasticVue](https://github.com/cars10/elasticvue) UI so users can open a browser and see the Camunda ES cluster without adding a cluster manually.

## Decisions (approved)

| Decision        | Choice                                                                                                                     |
| --------------- | -------------------------------------------------------------------------------------------------------------------------- |
| When to include | Auto whenever the profile exposes Elasticsearch (light ≤8.9, full including 8.10 ES overlay; not modeler / not 8.10 light) |
| Host port       | `9800` → `http://localhost:9800`                                                                                           |
| CLI surface     | Full: `urls`, `status`, `open elasticvue`, smoke optional (warn-only)                                                      |
| Approach        | Thin Compose overlay (same pattern as 8.10 ES), not a separate project                                                     |

## Non-goals

- Rewriting Camunda’s official compose files
- Desktop ElasticVue app / browser extension packaging
- Auth against secured ES (Camunda local stacks use open HTTP on `:9200`)
- Making ElasticVue available for modeler-only or 8.10 light (no host ES)

## How ElasticVue + Docker works

1. ElasticVue is a browser SPA served from the container (`cars10/elasticvue`).
2. The browser talks to Elasticsearch **from the host**, so the preconfigured URI must be `http://localhost:9200` (not the Docker-internal hostname).
3. Predefined clusters via env:

   ```bash
   ELASTICVUE_CLUSTERS='[{"name":"camunda-lab","uri":"http://localhost:9200"}]'
   ```

4. Elasticsearch must enable CORS for the ElasticVue origin (`http://localhost:9800` or a permissive `/.*/` for local labs).

## Design

### 1. Compose overlays (checked in + embedded)

Two thin overlays under `overlays/` (mirrored in `internal/overlay/embed/`):

**`elasticsearch-cors.yaml`** — patches existing `elasticsearch` service only:

```yaml
services:
  elasticsearch:
    environment:
      http.cors.enabled: 'true'
      http.cors.allow-origin: '/.*/'
      http.cors.allow-headers: 'X-Requested-With,Content-Type,Content-Length,Authorization'
```

(Exact YAML style to match Compose merge rules used by the 8.10 ES overlay.)

**`elasticvue.yaml`** — adds the UI service:

```yaml
services:
  elasticvue:
    image: cars10/elasticvue:<pinned-tag>
    container_name: camunda-lab-elasticvue
    ports:
      - '9800:8080'
    environment:
      ELASTICVUE_CLUSTERS: '[{"name":"camunda-lab","uri":"http://localhost:9200"}]'
    restart: unless-stopped
```

- Pin a concrete image tag (not `latest`) for reproducible labs; record the tag in this overlay and release notes.
- No dependency edge on `elasticsearch` health is required for bring-up (ElasticVue is useful even while ES is still starting); optional `depends_on` is fine if it does not fail when service names differ across minors.
- Join the default Compose project network automatically when applied with `-f` (same project as Camunda).

Also update **`elasticsearch-8.10.yaml`** so the sidecar ES we invent for 8.10 full includes the same CORS env (so CORS is present whether ES is upstream or our overlay).

### 2. Selection logic

Add `versions.HasHostElasticsearch(minor, profile) bool` (name can vary) aligned with `urls.List` today:

| Profile / minor      | Host ES (`:9200`)              | ElasticVue overlays |
| -------------------- | ------------------------------ | ------------------- |
| modeler (any)        | no                             | no                  |
| light 8.9 / 8.10     | no (official light dropped ES) | no                  |
| light 8.7–8.8        | yes                            | yes                 |
| full (any supported) | yes (8.10 via ES overlay)      | yes                 |

`overlay.ComposeOverrideFiles` returns, in order when applicable:

1. `elasticsearch-8.10.yaml` (existing, if `NeedsElasticsearchOverlay`)
2. `elasticsearch-cors.yaml` (if `HasHostElasticsearch` and ES is **not** solely from our 8.10 file — or always apply CORS file; merge is additive)
3. `elasticvue.yaml` (if `HasHostElasticsearch`)

Simplest rule: if `HasHostElasticsearch` → always sync + append `elasticsearch-cors.yaml` + `elasticvue.yaml`. For 8.10 full, also keep existing ES overlay; put CORS settings in both places or only in the dedicated CORS overlay (Compose merges env onto the same service). Prefer **one CORS overlay** applied whenever host ES exists, plus bake CORS into the 8.10 sidecar for clarity/self-containment.

### 3. URLs / status / open / smoke

- `urls.List`: add `{Name: "elasticvue", URL: "http://<host>:9800"}` whenever `HasHostElasticsearch`.
- Group under **Web apps** (or APIs — prefer Web apps).
- `status` Apps section: include `elasticvue` in the wanted list.
- `camunda open elasticvue` works via existing `urls.Find`.
- Smoke: treat `elasticvue` like connectors — **optional** (`warn` on failure, does not fail smoke). Required apps remain Operate/Tasklist/etc.

### 4. Docs

Update briefly:

- `docs/profiles.md` — ElasticVue URL table row
- `docs/cli-reference.md` / troubleshooting — CORS / port `9800`
- `README.md` — one-line mention under URLs / features

### 5. Tests

- `versions`: `HasHostElasticsearch` table tests
- `overlay`: files returned for light 8.9, full 8.10, modeler, light 8.10
- `urls`: elasticvue present/absent matching the table
- Overlay embed ↔ `overlays/` byte parity (same pattern as ES 8.10)

## User experience

```text
camunda install --version 8.9 --profile light -y
camunda wait
camunda open elasticvue
# Browser opens http://localhost:9800 with cluster "camunda-lab" already listed
```

`camunda urls` shows:

```text
  - elasticvue -> http://localhost:9800
  - elasticsearch -> http://localhost:9200
```

## Risks / mitigations

| Risk                                        | Mitigation                                                                                                                          |
| ------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------- |
| Upstream Camunda ES service name differs    | Stick to `elasticsearch` (official compose name); verify on 8.7–8.9 light/full in live smoke                                        |
| CORS merge ignored by some Compose versions | Prefer list-style `environment:` keys that merge; document `camunda doctor` note if cluster import works but connection fails       |
| Port 9800 conflict                          | Document; no CLI flag in v1 (can add later)                                                                                         |
| Light profile without published `:9200`     | Align URL map with reality; if a minor does not publish ES, exclude from `HasHostElasticsearch` (do not start a useless ElasticVue) |
| Browser still prompts to import             | Pin ElasticVue version that auto-imports `ELASTICVUE_CLUSTERS` on open (per upstream README / issue #254 behavior)                  |

## Implementation outline (for later plan)

1. Add `HasHostElasticsearch` + tests
2. Author overlays + embed + sync in `ComposeOverrideFiles`
3. Wire URLs / status / smoke
4. Docs + optional LIVE verify on light 8.9

## Success criteria

- [ ] `camunda up` on light 8.9 and full 8.9 starts ElasticVue on `:9800`
- [ ] Opening ElasticVue shows predefined cluster `camunda-lab` → `http://localhost:9200` without manual setup
- [ ] Browser can query the cluster (CORS works)
- [ ] `camunda urls` / `open elasticvue` / status Apps list include it
- [ ] Smoke does not fail solely because ElasticVue or ES infra is flaky
- [ ] Modeler and 8.10 light do **not** start ElasticVue
