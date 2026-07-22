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

This secret pushes into a **different** repo (`nasraldin/homebrew-tools`). The
workflow’s built-in `GITHUB_TOKEN` only covers `camunda-lab`, so a PAT is required.

If Release fails with:

```text
remote: Permission to nasraldin/homebrew-tools.git denied to nasraldin.
```

the script/auth path is fine — the PAT authenticates as you but **cannot write**
the tap (wrong repo selected, Contents not Read and write, or expired). Local
`gh` can still push because it uses your interactive login, not this secret.

#### Create / rotate the PAT

1. Open [Fine-grained tokens](https://github.com/settings/personal-access-tokens/new)
2. **Resource owner:** your user (`nasraldin`)
3. **Repository access:** Only select repositories → **`homebrew-tools`**
   (must include the tap; a token scoped only to `camunda-lab` will 403)
4. **Permissions → Repository → Contents:** **Read and write**
5. Generate, copy the token
6. In `camunda-lab` → Settings → Secrets and variables → Actions → update
   **`HOMEBREW_TAP_TOKEN`**
7. Verify: **Actions → Homebrew → Run workflow** (tag `v0.7.0` or latest)

Classic PAT alternative: `repo` (or `public_repo` if the tap stays public).

| Target                             | Permission                                          |
| ---------------------------------- | --------------------------------------------------- |
| `nasraldin/homebrew-tools`         | **Contents: Read and write**                        |
| (optional) `nasraldin/camunda-lab` | Metadata read — only if the token is org-restricted |

`scripts/publish-homebrew.sh` checks push access via the API before `git push`
and prints this diagnosis when the secret is wrong.

| Piece                          | Path                                                     |
| ------------------------------ | -------------------------------------------------------- |
| Formula template               | `Formula/camunda-lab.rb`                                 |
| Publish script                 | `scripts/publish-homebrew.sh`                            |
| Release workflow (tap publish) | `.github/workflows/release.yml` → `publish-homebrew` job |
| Backfill workflow              | `.github/workflows/homebrew.yml` (manual only)           |
