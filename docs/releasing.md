# Releasing

Maintainer checklist for cutting a release.

## Checklist

1. `make check` and `make ui-check` are green on `main`
2. Docs still match the release (especially [roadmap](roadmap.md) and install paths)
3. `Formula/camunda-lab.rb` template is current (tap is updated automatically on tag)
4. Tag and push:

```bash
git tag -a v0.6.0 -m "v0.6.0"
git push origin v0.6.0
```

5. **Release** workflow runs GoReleaser (binaries + `checksums.txt`) and publishes the Homebrew tap
6. If tap publish failed, backfill manually or via **Actions → Homebrew → Run workflow**:

```bash
./scripts/publish-homebrew.sh v0.6.0
```

7. Smoke on a clean machine:

```bash
brew update && brew upgrade camunda-lab
# or: curl -fsSL …/install.sh | bash
camunda about
camunda version
```

## Versioning

SemVer tags: `vMAJOR.MINOR.PATCH`.

- **Patch** — fixes, docs, CI hygiene
- **Minor** — CLI features, new Camunda minors, UX
- **Major** — breaking CLI / state layout under `~/.camunda-lab`

`camunda version` / `camunda about` show the release version via GoReleaser ldflags (`-X main.version=`).

## Channels

| Channel        | How                                                             |
| -------------- | --------------------------------------------------------------- |
| GitHub Release | tag `v*` → GoReleaser                                           |
| `install.sh`   | downloads release tarball + verifies `checksums.txt`            |
| Homebrew       | `nasraldin/tools` formula `camunda-lab` (auto-published on tag) |
| Docs site      | MkDocs → GitHub Pages on `main`                                 |

## Repo hygiene (maintainers)

- Protect `main`: require CI (`Test & build`) and PR reviews before merge
- `HOMEBREW_TAP_TOKEN` must be set — release **fails** if missing (tap would stay stale)
- Commit `internal/ui/web/dist` whenever UI source changes (`make ui-check` in CI)
