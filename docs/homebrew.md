# Homebrew

The CLI binary is **`camunda`**. On Homebrew the formula is **`camunda-lab`**.

```bash
brew tap nasraldin/tools
brew install camunda-lab
```

Homebrew **starts the Lab UI in the background** and prints:

```text
Lab UI is running — open in your browser:
  http://localhost:9090
```

Open that link to install and manage Camunda from the browser (no CLI required). Prefer the terminal? Run `camunda install` as usual.

Upgrade later with:

```bash
brew update && brew upgrade camunda-lab
```

Tap repo: [`nasraldin/homebrew-tools`](https://github.com/nasraldin/homebrew-tools)

---

## For maintainers

On each GitHub **Release** (tag `v*`), the **Release** workflow:

1. Runs GoReleaser (binaries + GitHub Release)
2. Publishes `Formula/camunda-lab.rb` to `nasraldin/homebrew-tools` using `HOMEBREW_TAP_TOKEN`

GoReleaser creates the release via `GITHUB_TOKEN`, which **does not** trigger other workflows, so tap publish runs in the same Release workflow—not via `release: published`.

Backfill an old tag manually: **Actions → Homebrew → Run workflow** and pass the tag.

Local publish:

```bash
./scripts/publish-homebrew.sh v0.5.0
```

### Secret: `HOMEBREW_TAP_TOKEN`

Create a fine-grained PAT (or classic) and store it as repo secret `HOMEBREW_TAP_TOKEN`.

| Target                             | Permission                                          |
| ---------------------------------- | --------------------------------------------------- |
| `nasraldin/homebrew-tools`         | **Contents: Read and write**                        |
| (optional) `nasraldin/camunda-lab` | Metadata read — only if the token is org-restricted |

A token that can read but not push fails with **403** on `git push`. Fix the PAT scopes or publish locally with the script above.

| Piece                          | Path                                                     |
| ------------------------------ | -------------------------------------------------------- |
| Formula template               | `Formula/camunda-lab.rb`                                 |
| Publish script                 | `scripts/publish-homebrew.sh`                            |
| Release workflow (tap publish) | `.github/workflows/release.yml` → `publish-homebrew` job |
| Backfill workflow              | `.github/workflows/homebrew.yml` (manual only)           |
