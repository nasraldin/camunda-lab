# Test generator (`camunda test generate`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked.

**Goal:** From a BPMN file, generate starter test skeletons (Camunda Process Test / Java and optional JS worker stubs) under `tests/`, driven by IR job types and process id.

**Architecture:** `internal/testgen` reads IR, applies Go templates, writes files. Respects `.camunda.yaml` paths. No network.

**Tech Stack:** Go `text/template`, shared `internal/bpmn`, Cobra.

**Depends on:** `internal/bpmn`, preferably `internal/project` for output dirs.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Generators produce **skeletons** marked TODO — not full coverage claims
- `--lang java|js` (MVP); default `java` for Process Test
- Refuse overwrite unless `--force`
- Do not call LLMs in MVP (templates only)
- Do not deploy processes as part of generate

---

## File map

| File | Responsibility |
|------|----------------|
| `internal/testgen/generate.go` | Entry + lang switch |
| `internal/testgen/java.go` | Camunda Process Test class template |
| `internal/testgen/js.go` | Jest-style worker stub template |
| `internal/testgen/templates/*.tmpl` | Embedded templates |
| `internal/testgen/generate_test.go` | Assert files + key symbols |
| `internal/cli/test.go` | `camunda test generate` |
| `internal/cli/root.go` | Register `test` parent |
| `docs/cli-reference.md` | Docs |

---

### Task 1: Java Process Test skeleton

Emitted shape (illustrative):

```java
@CamundaSpringProcessTest
class OrderProcessTest {
  // @Autowired CamundaClient ...
  // @Test void happyPath() { /* TODO deploy + create instance */ }
}
```

- [ ] **Step 1:** Template embeds process id + list of service-task job types as commented stubs
- [ ] **Step 2:** Test writes to temp `tests/java/...` and checks process id string present
- [ ] **Step 3:** Implement generator

### Task 2: JS worker stubs

- [ ] **Step 1:** One Jest describe per job type with `it.todo` or failing placeholder
- [ ] **Step 2:** Unit test

### Task 3: CLI

```bash
camunda test generate bpmn/order.bpmn
camunda test generate bpmn/order.bpmn --lang java
camunda test generate bpmn/order.bpmn --lang js --force
```

- [ ] **Step 1:** Default out dir `tests/` or `.camunda.yaml` override if added (`paths.tests`)
- [ ] **Step 2:** Parent command `camunda test` with subcommand `generate` (room for future `test run`)
- [ ] **Step 3:** cli-reference

---

## Out of scope

- Playwright E2E Tasklist flows (mention as future lang)
- Running tests (`mvn test` / `npm test`)
- AI-generated assertions

## Success criteria

- Generate from fixture creates non-empty compilable-looking skeleton (Java) without network
- `--force` required to clobber
