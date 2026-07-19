# Environment profiles (`camunda env`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked. Implement **after** Phase 2 DX unless unblocked early for plan/drift.

**Goal:** Named environment profiles so Phase 3 commands target lab or remote clusters without export soup — like kubectl context / AWS profiles, for Camunda Lab.

**Architecture:** `internal/env` loads/stores profiles. Global active pointer in `~/.camunda-lab/config.yaml` (or `activeEnv` file). Project overlays in `environments/*.yaml`. Secrets referenced by **env var names only**.

**Tech Stack:** Go, YAML, Cobra.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Kinds: `lab` | `remote`
- Default profile is implicit **lab** (current `~/.camunda-lab` compose stack) even before any `env add`
- Never store raw client secrets/passwords in yaml
- Remote profiles used by plan/drift/incidents/trace — not by `install`/`up` (those stay lab lifecycle)
- Complementarity: env selects API endpoints; deploy still official tooling

---

## Schema

```yaml
# ~/.camunda-lab/envs/prod.yaml  OR  ./environments/prod.yaml
name: prod
kind: remote
endpoints:
  orchestration: https://camunda.example.com
  operate: https://camunda.example.com/operate # optional if consolidated
auth:
  # values read from process environment at runtime
  clientIdEnv: CAMUNDA_CLIENT_ID
  clientSecretEnv: CAMUNDA_CLIENT_SECRET
  tokenUrlEnv: CAMUNDA_TOKEN_URL # optional
```

Lab kind may omit endpoints and resolve via `internal/urls` + lab config.

---

## File map

| File                       | Responsibility                       |
| -------------------------- | ------------------------------------ |
| `internal/env/profile.go`  | Types, Load, Save, List, Validate    |
| `internal/env/active.go`   | Get/Set active profile name          |
| `internal/env/resolve.go`  | Resolve endpoints + auth for callers |
| `internal/env/env_test.go` | Validation + resolve tests           |
| `internal/paths/paths.go`  | `EnvsDir()` under lab home           |
| `internal/cli/env.go`      | `add                                 | list | use | show | remove` |
| `internal/cli/root.go`     | Register                             |
| `docs/cli-reference.md`    | Docs                                 |

---

### Task 1: Storage + validation

- [ ] **Step 1:** Tests — reject profile with inline `clientSecret: "..."`
- [ ] **Step 2:** Tests — lab kind validates; remote requires orchestration URL + auth env names
- [ ] **Step 3:** Implement load from lab home envs dir + project `environments/`
- [ ] **Step 4:** Name collision: project overrides global or vice versa — **document: project wins when cwd has `.camunda.yaml`**

### Task 2: Active pointer

- [ ] Persist `activeEnv` in lab `config.yaml`
- [ ] `Resolve()` returns lab URLs when active empty or `lab`

### Task 3: CLI

```bash
camunda env list
camunda env add prod --kind remote --orchestration https://...
camunda env use prod
camunda env show
camunda env remove prod
```

- [ ] Interactive `add` prompts for endpoint + env var **names**
- [ ] `use` switches active; print confirmation
- [ ] cli-reference + architecture note

---

## Out of scope

- Implementing OAuth token fetch beyond sharing a small `internal/env/token.go` helper for Phase 3 clients
- Multi-lab named compose instances (`--name`) — separate epic

## Success criteria

- `env use` changes what a stub `Resolve()` returns in tests
- No secret values written to disk by `env add`
