# Homebrew

The CLI binary is **`camunda`**. On Homebrew the formula is **`camunda-lab`**.

```bash
brew tap nasraldin/tools
brew install camunda-lab
camunda version
camunda about
```

Tap repo: [`nasraldin/homebrew-tools`](https://github.com/nasraldin/homebrew-tools)

## How publishing works

On each GitHub **Release** (tag `v*`), the **Homebrew** workflow:

1. Downloads the source tarball for that tag
2. Computes sha256
3. Updates `Formula/camunda-lab.rb` in `nasraldin/homebrew-tools`
4. Pushes with `HOMEBREW_TAP_TOKEN`

You can also run the workflow by hand (**Actions → Homebrew → Run workflow**) and pass a tag.

Local publish (uses your `gh` / git credentials):

```bash
./scripts/publish-homebrew.sh v0.2.0
```

## Secret: `HOMEBREW_TAP_TOKEN`

Create a **fine-grained PAT** (or classic) and store it as repo secret `HOMEBREW_TAP_TOKEN`.

Required access:

| Target | Permission |
| --- | --- |
| `nasraldin/homebrew-tools` | **Contents: Read and write** |
| (optional) `nasraldin/camunda-lab` | Metadata read — only if the token is org-restricted |

A token that can read but not push will fail with **403** on `git push`. If CI fails that way, fix the PAT scopes or publish locally with the script above.

Same pattern as [docker-lab’s Homebrew docs](https://nasraldin.github.io/docker-lab/homebrew/).

## Status

| Piece | Status |
| --- | --- |
| Formula in this repo | `Formula/camunda-lab.rb` |
| Publish script | `scripts/publish-homebrew.sh` |
| Workflow | `.github/workflows/homebrew.yml` |
| Auto-publish on release | yes (when token has write access) |
