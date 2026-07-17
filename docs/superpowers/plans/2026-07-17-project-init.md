# Project scaffolding (`camunda init`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans. Checkbox steps for tracking. Commits only when the user explicitly asks.

**Goal:** `camunda init [dir]` scaffolds a Camunda application project (folders, `.camunda.yaml`, README) so teams stop creating layout by hand — like `cargo new` / `npm create`, not a second Compose stack.

**Architecture:** New `internal/project` package owns `.camunda.yaml` schema, validation, and scaffold writers. Thin Cobra command in `internal/cli/init.go`. Does **not** start the lab, deploy processes, or generate worker code beyond empty dirs / stubs.

**Tech Stack:** Go, Cobra, YAML (`gopkg.in/yaml.v3` or existing project deps), `internal/prompt` for interactive defaults.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Binary / docs always say **`camunda init`** (never camctl/c8 branding)
- Do not vendor a full Camunda Compose; stub `docker-compose.yml` points users to `camunda install` / Lab
- No remote cluster credentials in `.camunda.yaml` v1
- Refuse to overwrite existing non-empty target unless `--force`
- `--yes` / `-y` skips prompts; interactive only when stdin is a TTY
- Commits only when the user explicitly asks

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/project/config.go` | `.camunda.yaml` structs, Load/Save/Validate |
| `internal/project/config_test.go` | Schema + validation tests |
| `internal/project/scaffold.go` | Create dirs, files, README template |
| `internal/project/scaffold_test.go` | Temp-dir scaffold tests |
| `internal/cli/init.go` | `camunda init` command |
| `internal/cli/root.go` | Register `init` |
| `docs/cli-reference.md` | Document `init` |
| `docs/roadmap.md` | Mark init shipped when done (follow-up) |

---

## Scaffold layout (locked)

```text
<dir>/
├── bpmn/
├── dmn/
├── forms/
├── workers/
├── connectors/
├── scripts/
├── tests/
├── environments/          # placeholder for Phase 3 env files
│   └── .gitkeep
├── helm/
│   └── README.md          # “production path = Helm; Lab is local”
├── docker-compose.yml     # stub: comment-only or minimal note → use camunda-lab
├── .camunda.yaml
└── README.md
```

### `.camunda.yaml` v1

```yaml
name: my-project
camundaVersion: "8.9"          # hint only
paths:
  bpmn: bpmn
  dmn: dmn
  forms: forms
lab:
  profile: light               # hint for humans / future tooling
  resources: balanced
```

---

### Task 1: Config types + validation

**Files:**
- Create: `internal/project/config.go`
- Create: `internal/project/config_test.go`

**Interfaces:**
- `type Config struct { Name string; CamundaVersion string; Paths Paths; Lab LabHints }`
- `func Load(path string) (Config, error)`
- `func (c Config) Validate() error`
- `func Save(path string, c Config) error`

- [ ] **Step 1: Write failing tests** for empty name, missing paths, round-trip YAML
- [ ] **Step 2: Implement** Load/Save/Validate with defaults for paths
- [ ] **Step 3: Run** `go test ./internal/project/ -run TestConfig -count=1`

---

### Task 2: Scaffold writer

**Files:**
- Create: `internal/project/scaffold.go`
- Create: `internal/project/scaffold_test.go`

**Interfaces:**
- `type ScaffoldOpts struct { Dir string; Name string; Version string; Profile string; Resources string; Force bool }`
- `func Scaffold(opts ScaffoldOpts) error`

- [ ] **Step 1: Failing test** — Scaffold creates all dirs + `.camunda.yaml` + README
- [ ] **Step 2: Failing test** — non-empty dir without Force returns error
- [ ] **Step 3: Implement** Scaffold (mkdir, write files, `.gitkeep` in environments)
- [ ] **Step 4: README template** mentions `camunda install`, `camunda ui`, and that deploy uses official tooling
- [ ] **Step 5: `go test ./internal/project/ -count=1`**

---

### Task 3: CLI command

**Files:**
- Create: `internal/cli/init.go`
- Modify: `internal/cli/root.go`

```bash
camunda init
camunda init ./order-service
camunda init ./order-service --name orders --version 8.9 --yes
camunda init ./order-service --force
```

| Flag | Meaning |
|------|---------|
| `--name` | Project name (default: basename of dir) |
| `--version` | `camundaVersion` hint (default: active lab version if config exists, else `8.9`) |
| `--profile` | lab.profile hint |
| `--resources` | lab.resources hint |
| `--yes` / `-y` | Non-interactive |
| `--force` | Allow non-empty target |

- [ ] **Step 1: Implement** `newInitCmd` with flags; prompt when TTY and flags missing
- [ ] **Step 2: Register** on root
- [ ] **Step 3: Manual** `go run ./cmd/camunda init /tmp/camunda-init-test --yes` and inspect tree
- [ ] **Step 4: Update** `docs/cli-reference.md` with `init` section

---

### Task 4: About / features line (optional small)

- [ ] Add `init` to features list in `camunda about` when scaffolding ships

---

## Out of scope

- Starting Docker / calling `camunda install`
- Generating Java/JS worker boilerplate (Phase 2 `test generate` / later)
- Writing real Helm charts
- Remote env credentials

## Success criteria

- `camunda init --yes` creates the locked tree and valid `.camunda.yaml`
- Re-run without `--force` fails safely
- Unit tests cover config + scaffold
- CLI reference documents the command
