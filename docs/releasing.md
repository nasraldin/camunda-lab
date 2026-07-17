# Releasing

Maintainer checklist for cutting a release.

## Checklist

1. `make check` is green on `main`
2. Docs still match the release (especially [roadmap](roadmap.md) and install paths)
3. Keep `Formula/camunda-lab.rb` in sync after the tag (or let `scripts/publish-homebrew.sh` update the tap)
4. Tag and push:

```bash
git tag -a v0.4.0 -m "v0.4.0"
git push origin v0.4.0
```

5. GoReleaser creates the GitHub Release + archives + `checksums.txt` (`.github/workflows/release.yml`)
6. Same Release workflow publishes the Homebrew tap (`HOMEBREW_TAP_TOKEN`), or publish locally / via **Actions → Homebrew**:

```bash
./scripts/publish-homebrew.sh v0.5.0
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

| Channel | How |
| --- | --- |
| GitHub Release | tag `v*` → GoReleaser |
| `install.sh` | downloads release tarball + verifies `checksums.txt` |
| Homebrew | `nasraldin/tools` formula `camunda-lab` |
| Docs site | MkDocs → GitHub Pages on `main` |
