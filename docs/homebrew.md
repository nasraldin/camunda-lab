# Homebrew

The CLI binary is **`camunda`**. On Homebrew the formula is **`camunda-lab`**.

```bash
brew tap nasraldin/tools
brew install camunda-lab
camunda version
camunda about
```

Upgrade later with:

```bash
brew update && brew upgrade camunda-lab
```

Tap repo: [`nasraldin/homebrew-tools`](https://github.com/nasraldin/homebrew-tools)

---

## For maintainers

On each GitHub **Release** (tag `v*`), the **Homebrew** workflow:

1. Downloads the source tarball for that tag
2. Computes sha256
3. Updates `Formula/camunda-lab.rb` in `nasraldin/homebrew-tools`
4. Pushes with `HOMEBREW_TAP_TOKEN`

You can also run the workflow by hand (**Actions → Homebrew → Run workflow**) and pass a tag.

Local publish:

```bash
./scripts/publish-homebrew.sh v0.4.0
```

### Secret: `HOMEBREW_TAP_TOKEN`

Create a fine-grained PAT (or classic) and store it as repo secret `HOMEBREW_TAP_TOKEN`.

| Target | Permission |
| --- | --- |
| `nasraldin/homebrew-tools` | **Contents: Read and write** |
| (optional) `nasraldin/camunda-lab` | Metadata read — only if the token is org-restricted |

A token that can read but not push fails with **403** on `git push`. Fix the PAT scopes or publish locally with the script above.

| Piece | Path |
| --- | --- |
| Formula template | `Formula/camunda-lab.rb` |
| Publish script | `scripts/publish-homebrew.sh` |
| Workflow | `.github/workflows/homebrew.yml` |
