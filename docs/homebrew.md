# Homebrew

The CLI binary is **`camunda`**. On Homebrew the formula is **`camunda-lab`** so the name stays clear next to other tools in the tap.

```bash
brew tap nasraldin/tools
brew install camunda-lab
camunda version
```

Tap repo: [`nasraldin/homebrew-tools`](https://github.com/nasraldin/homebrew-tools)

## Status

| Piece | Status |
| --- | --- |
| Formula in this repo | `Formula/camunda-lab.rb` |
| sha256 values | Fill in on first release |
| Auto-publish to tap | Same pattern as docker-lab (optional) |

Until a release exists with real checksums, install from source (`make build && make install`).

## Local tap checkout

Same layout as docker-lab:

```text
~/homelab/
  camunda-lab/
  taps/
    homebrew-tools/
```

```bash
cp ~/homelab/camunda-lab/Formula/camunda-lab.rb \
   ~/homelab/taps/homebrew-tools/Formula/camunda-lab.rb
```

Commit and push in the tap repo after you update URLs and sha256s.
