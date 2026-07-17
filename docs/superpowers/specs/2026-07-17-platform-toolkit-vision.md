# Design: Camunda Lab — Platform Toolkit Vision

**Status:** Approved  
**Date:** 2026-07-17  
**CLI binary:** `camunda`  
**Project:** `camunda-lab`

This document is the product-direction contract for roadmap and implementation plans. Prefer it over chat history when scoping features.

---

## Vision

> **Camunda Lab starts as the easiest way to run and manage local Camunda environments, then evolves into a productivity toolkit for developers and platform engineers. It complements official Camunda cluster CLIs rather than replacing them.**

| Horizon | Value proposition |
| --- | --- |
| **Today (Phase 1)** | Best local Camunda developer experience — official Compose, one CLI, Lab UI, AI/MCP glue |
| **Tomorrow (Phase 2–3)** | Developer and platform toolkit — analysis, diagnostics, GitOps-style preview, ops helpers |

**Naming:** Always brand as **Camunda Lab** / command **`camunda`**. Do not brand features as `camctl`, `c8`, or other third-party CLI names. Official tools may be named only when stating complementarity boundaries.

---

## Locked decisions

| Topic | Choice |
| --- | --- |
| Product direction | **B** — lab-first, grow into toolkit |
| Binary | `camunda` |
| Lab home | `~/.camunda-lab` (or `CAMUNDA_LAB_HOME`) |
| Project config | `.camunda.yaml` at repo root (new in Phase 1 `init`) |
| Cluster resource management | Official tooling owns deploy / start instance / generic resource CRUD |
| Our lane | Lab lifecycle, DX on project files, ops/diagnostics, env context, thin kubectl helpers |
| Phase 2 UI | CLI-first; Lab UI surfaces are optional follow-ons |
| Phase 3 cluster APIs | Orchestration / Operate-compatible HTTP against **active env** (lab by default) |
| Secrets in config | Env var **names** only in yaml; never raw secrets in `.camunda.yaml` or env profiles |
| AI | Optional; deterministic/offline path first; no paid LLM calls in CI |

---

## Complementarity rules

**Official cluster CLIs** talk *to* a cluster: deploy BPMN, watch files, inspect/start instances, manage jobs.

**Camunda Lab** gets the cluster **up**, then adds:

- Project scaffolding and file analysis (`init`, `diff`, `lint`, `review`, `explain`, `scan`, `test generate`)
- Deeper lab health (`doctor --deep`)
- Environment profiles, deployment **preview**, drift, backup/restore (lab-oriented MVP)
- Incident list / process trace helpers and `k8s` wrappers that add context — not a second Operate or a Helm replacement

### Hard non-goals (all phases)

- Recreating official deploy / start-instance / watch UX as our primary product surface
- Replacing Helm / Kubernetes as the production install path
- Full Identity / Optimize / Console rebuild inside Lab UI
- Storing raw OAuth client secrets or passwords in checked-in yaml
- Process replay engine, full C7→C8 migrator, or executive HTML `report` as Phase 1–3 commitments

---

## Phases

### Phase 1 — Lab core (mostly shipped)

**Shipped in v0.6.x:** install / switch / profile / resources, Lab UI, AI/MCP, basic `doctor`, smoke/wait, tools glue, overlays.

**Remaining Phase 1:** `camunda init` — project scaffolding + `.camunda.yaml`.

Plan: [../plans/2026-07-17-project-init.md](../plans/2026-07-17-project-init.md)

### Phase 2 — High-value developer experience

| Command | Role |
| --- | --- |
| `camunda diff` | Semantic BPMN/DMN/form diff |
| `camunda lint` | Deterministic BPMN rules |
| `camunda review` | Lint + optional AI review |
| `camunda explain` | Business/technical summary |
| `camunda test generate` | Test skeletons from BPMN |
| `camunda scan` | Secrets / credential scan |
| `camunda doctor --deep` | Component-level lab health |

Shared foundation: `internal/bpmn` IR used by lint/diff/explain/review/testgen.

### Phase 3 — Platform engineering

| Command | Role |
| --- | --- |
| `camunda env` | Named lab / remote profiles |
| `camunda plan` | Deployment preview (does not deploy) |
| `camunda drift` | Git vs cluster drift |
| `camunda backup` / `restore` | Lab-oriented snapshot MVP |
| `camunda incidents` | Incident explorer helpers |
| `camunda trace` | Live process instance timeline |
| `camunda k8s` | Thin kubectl helpers for Camunda Helm labels |

**Implementation order after docs:** `init` → BPMN IR → lint → diff → scan → test generate → explain/review → doctor --deep → env → plan/drift → incidents/trace → backup → k8s.

---

## Package map (planned)

```text
internal/project/     # .camunda.yaml + scaffold
internal/bpmn/        # XML → IR
internal/diff/        # semantic diff
internal/lint/        # deterministic rules
internal/review/      # lint + optional AI
internal/explain/     # summaries
internal/testgen/     # test skeletons
internal/scan/        # secrets scanner
internal/env/         # env profiles (Phase 3)
internal/plan/        # deploy preview (Phase 3)
internal/drift/       # git vs cluster (Phase 3)
internal/backup/      # backup/restore (Phase 3)
internal/incidents/   # incident list/helpers (Phase 3)
internal/trace/       # instance timeline (Phase 3)
internal/k8s/         # kubectl wrappers (Phase 3)
```

Existing packages (`lab`, `compose`, `doctor`, `smoke`, `ai`, `ui`, …) remain the lab control plane.

---

## Success criteria

- Public roadmap states this vision and phases honestly (no fake ETAs)
- Every Phase 1–3 feature above has a detailed plan under `docs/superpowers/plans/`
- Agents implementing features respect complementarity and naming rules
- Phase 2 does not block on Lab UI; Phase 3 does not block on a full Console rebuild
