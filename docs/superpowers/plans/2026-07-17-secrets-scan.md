# Secrets scanner (`camunda scan`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked.

**Goal:** `camunda scan` finds hardcoded client secrets, passwords, webhook tokens, and OAuth-looking credentials in a Camunda project tree for CI and local hygiene.

**Architecture:** Walk filesystem from project root (or `.`); respect `.gitignore` when possible; apply regex + simple entropy heuristics; report text/JSON. Exit `1` on findings at/above severity.

**Tech Stack:** Go, Cobra, optional `go-gitignore`, filepath walk.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

- Never print full secret values in default text mode — mask middle (`abcd…wxyz`)
- Skip `node_modules/`, `.git/`, binary files, `~/.camunda-lab` unless explicitly targeted
- Scan BPMN/DMN/forms/YAML/env/scripts/JSON by default
- No network
- SARIF reporter is follow-on; MVP = text + `--json`
- False positives: allow `# camunda-scan-ignore` line or `.camunda-scanignore`

---

## Detection categories (MVP)

| ID                    | Examples                                                    |
| --------------------- | ----------------------------------------------------------- |
| `secret.client`       | `client_secret`, `clientSecret`, `CLIENT_SECRET=`           |
| `secret.password`     | `password:`, `PASSWORD=`                                    |
| `secret.token`        | `webhook`, `bearer `, `api_key`, `apiKey`                   |
| `secret.oauth`        | `refresh_token`, long JWT-shaped strings in files           |
| `secret.high-entropy` | Assignment-like strings with high Shannon entropy (tunable) |

---

## File map

| File                         | Responsibility             |
| ---------------------------- | -------------------------- |
| `internal/scan/walk.go`      | Directory walk + ignore    |
| `internal/scan/rules.go`     | Patterns                   |
| `internal/scan/scan.go`      | Run + Finding type         |
| `internal/scan/report.go`    | Text/JSON masked           |
| `internal/scan/scan_test.go` | Fixtures with fake secrets |
| `testdata/scan/`             | Sample dirty/clean trees   |
| `internal/cli/scan.go`       | CLI                        |
| `internal/cli/root.go`       | Register                   |
| `docs/cli-reference.md`      | Docs                       |

---

### Task 1: Engine

- [ ] **Step 1:** Fixture file with `client_secret=supersecretvalue` → finding; masked output test
- [ ] **Step 2:** Clean fixture → no findings
- [ ] **Step 3:** Implement walk + rules + entropy gate
- [ ] **Step 4:** `go test ./internal/scan/ -count=1`

### Task 2: CLI

```bash
camunda scan
camunda scan ./connectors
camunda scan --json
camunda scan --fail-on medium
```

- [ ] **Step 1:** Default root = cwd; use `.camunda.yaml` presence as project hint only
- [ ] **Step 2:** Severity levels `low|medium|high`; default fail on `medium+`
- [ ] **Step 3:** cli-reference; SECURITY.md one-liner pointing to `camunda scan` when shipped

---

## Out of scope

- Uploading findings to a SaaS
- Rotating secrets automatically
- Scanning running container env dumps (Phase 3 backup may note separately)

## Success criteria

- CI-friendly exit codes; secrets masked in default output
- Unit tests never embed real third-party credentials
