# Backup & restore (`camunda backup` / `restore`) — Implementation Plan

> **For agentic workers:** Use superpowers:subagent-driven-development or executing-plans. Checkbox tracking. Commits only when asked. Lab-oriented MVP first.

**Goal:** Snapshot lab-linked config and project resources for restore/dev cloning — not a full enterprise Identity disaster-recovery product on day one.

**Architecture:** `internal/backup` writes a dated archive (directory or `.tar.gz`) containing manifest + files. Restore validates manifest and copies back with confirmation.

**Tech Stack:** Go, archive/tar + gzip, Cobra, lab paths, project paths.

**Vision:** [../specs/2026-07-17-platform-toolkit-vision.md](../specs/2026-07-17-platform-toolkit-vision.md)

## Global Constraints

### MVP includes

- Lab `config.yaml` (no raw secrets)
- List of AI secret **key names** present in `ai.env` (values only with `--include-secrets`)
- Copy of project `bpmn/` `dmn/` `forms/` when run from a project (or `--project path`)
- Compose project name / version / profile metadata in `manifest.json`

### MVP excludes (stretch — document in plan, do not build in first PR)

- Full Keycloak / Identity user export
- Elasticsearch snapshot
- Multi-tenant Camunda SaaS export
- Hot backup of running Zeebe partitions

### Safety

- `restore` requires `--yes` or typed confirm
- `--include-secrets` prints loud warning; archive should be chmod 600
- Never upload archives anywhere

---

## File map

| File                             | Responsibility                |
| -------------------------------- | ----------------------------- |
| `internal/backup/backup.go`      | Create archive                |
| `internal/backup/restore.go`     | Restore with checks           |
| `internal/backup/manifest.go`    | Manifest schema               |
| `internal/backup/backup_test.go` | Temp lab home round-trip      |
| `internal/cli/backup.go`         | `backup` + `restore` commands |
| `internal/cli/root.go`           | Register                      |
| `docs/cli-reference.md`          | Docs + warnings               |

---

### Task 1: Manifest + backup

```json
{
  "version": 1,
  "createdAt": "...",
  "lab": { "version": "8.9", "profile": "light" },
  "includesSecrets": false,
  "files": ["config.yaml", "project/bpmn/order.bpmn"]
}
```

- [ ] Round-trip test without secrets
- [ ] Test `--include-secrets` packs masked note that values were included

### Task 2: Restore

```bash
camunda backup -o ./lab-backup.tar.gz
camunda backup --include-secrets -o /tmp/lab-secrets.tar.gz
camunda restore ./lab-backup.tar.gz --yes
```

- [ ] Refuse restore if lab running unless `--force` (down first hint)
- [ ] Restore config + project files; secrets file only if archive had them
- [ ] Unit tests

### Task 3: Docs

- [ ] cli-reference admonitions
- [ ] troubleshooting: “backup is not ES snapshot”

---

## Stretch (later tasks in same plan file when prioritizing)

- [ ] Optional `docker compose` volume export helper (document risk)
- [ ] Remote env export of deployed BPMN XML via read APIs into archive

## Success criteria

- MVP backup/restore of config + project resources works in tests
- Secret inclusion is opt-in and warned
