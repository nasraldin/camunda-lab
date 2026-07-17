# Contributing

Thanks for helping improve Camunda Lab. This is an unofficial community project — not affiliated with Camunda GmbH.

## Before you start

- Read [SECURITY.md](SECURITY.md) for vulnerability reporting (do not file public issues for security bugs).
- Follow the [Code of Conduct](CODE_OF_CONDUCT.md).
- Prefer small PRs with a clear problem statement.

## Dev setup

```bash
git clone https://github.com/nasraldin/camunda-lab.git
cd camunda-lab
make check          # fmt + tidy + vet + test
make ui-check       # after UI source changes — rebuild dist and verify committed
make build
make install        # ~/.local/bin/camunda
camunda about
```

Requirements: Go (see `go.mod`), Docker Compose v2 for LIVE tests.

## What to change carefully

| Area | Guidance |
| --- | --- |
| Compose topology | Do **not** rewrite Camunda’s official files — thin overlays only |
| Ports / URLs | Must match `camunda/camunda-distributions` per minor (`internal/urls`) |
| Auth / OIDC | Leave Keycloak wiring to upstream Compose |
| Homebrew | Formula template lives in `Formula/`; tap publish via `scripts/publish-homebrew.sh` |

## Tests

```bash
make test
LIVE=1 ./scripts/live-smoke.sh   # needs Docker + enough RAM
```

## Docs

User docs live under `docs/` and publish to GitHub Pages via MkDocs.

```bash
pip install -r requirements-docs.txt
mkdocs serve
```

Historical design notes (not on the site): [`docs/design/`](docs/design/).

## Pull requests

Use the PR template. CI must be green (`fmt`, `tidy`, `vet`, `govulncheck`, `test`, embedded UI dist check, ShellCheck, docs build when docs change).

**Recommended:** enable branch protection on `main` (require `Test & build` status check, block force-push).

## Releases

Maintainers: see [docs/releasing.md](docs/releasing.md).
