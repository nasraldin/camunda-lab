# Homebrew

The CLI binary is **`camunda`**. On Homebrew the formula is **`camunda-lab`**.

```bash
brew tap nasraldin/tools
brew install camunda-lab
camunda version
```

Tap repo: [`nasraldin/homebrew-tools`](https://github.com/nasraldin/homebrew-tools)

## How publishing works

On each GitHub **Release** (tag `v*`), the **Homebrew** workflow:

1. Downloads the source tarball for that tag
2. Computes sha256
3. Updates `Formula/camunda-lab.rb` in `nasraldin/homebrew-tools`
4. Pushes with `HOMEBREW_TAP_TOKEN`

You can also run the workflow by hand (**Actions → Homebrew → Run workflow**) and pass a tag.

Local dry-run (needs `gh` auth that can push the tap):

```bash
./scripts/publish-homebrew.sh v0.1.0
```

## Secret

Repo secret **`HOMEBREW_TAP_TOKEN`**: fine-grained PAT (or classic) that can push to `nasraldin/homebrew-tools` (Contents: read/write).

Same pattern as [docker-lab’s Homebrew docs](https://nasraldin.github.io/docker-lab/homebrew/).

## Status

| Piece | Status |
| --- | --- |
| Formula in this repo | `Formula/camunda-lab.rb` |
| Publish script | `scripts/publish-homebrew.sh` |
| Workflow | `.github/workflows/homebrew.yml` |
| Auto-publish on release | yes (uses `HOMEBREW_TAP_TOKEN`) |

Until the first `v*` release exists, install from source:

```bash
make build && make install
```
