#!/usr/bin/env bash
# Publish / update Formula/camunda-lab.rb in nasraldin/homebrew-tools for a given tag.
#
# Usage:
#   ./scripts/publish-homebrew.sh v0.1.0
#   TAP_REPO=nasraldin/homebrew-tools ./scripts/publish-homebrew.sh v0.1.0
#
# Env:
#   TAP_REPO   default: nasraldin/homebrew-tools
#   GH_TOKEN   optional; used by gh when set (CI uses HOMEBREW_TAP_TOKEN)
# shellcheck shell=bash
set -euo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TAG="${1:-}"
TAP_REPO="${TAP_REPO:-nasraldin/homebrew-tools}"
FORMULA_NAME="camunda-lab"
SOURCE_REPO="nasraldin/camunda-lab"

[[ -n "${TAG}" ]] || {
  echo "Usage: $0 <tag>   e.g. $0 v0.1.0" >&2
  exit 1
}
[[ "${TAG}" == v* ]] || {
  echo "ERROR: tag must start with v (got '${TAG}')" >&2
  exit 1
}

VERSION="${TAG#v}"
TARBALL_URL="https://github.com/${SOURCE_REPO}/archive/refs/tags/${TAG}.tar.gz"

command -v gh >/dev/null 2>&1 || {
  echo "ERROR: gh CLI required" >&2
  exit 1
}
command -v shasum >/dev/null 2>&1 || command -v sha256sum >/dev/null 2>&1 || {
  echo "ERROR: shasum or sha256sum required" >&2
  exit 1
}

echo "==> Fetching ${TARBALL_URL}"
TMP="$(mktemp)"
curl -fsSL "${TARBALL_URL}" -o "${TMP}"

if command -v shasum >/dev/null 2>&1; then
  SHA="$(shasum -a 256 "${TMP}" | awk '{print $1}')"
else
  SHA="$(sha256sum "${TMP}" | awk '{print $1}')"
fi
rm -f "${TMP}"
echo "==> sha256 ${SHA}"

# Fail early with a clear message when HOMEBREW_TAP_TOKEN can't write the tap.
# A missing/wrong-scoped PAT still authenticates as the user, then 403s on push
# ("Permission to … denied to <login>"), which looks like a script bug.
if [[ -n "${GH_TOKEN:-}" ]]; then
  LOGIN="$(gh api user -q .login 2>/dev/null || true)"
  CAN_PUSH="$(gh api "repos/${TAP_REPO}" -q .permissions.push 2>/dev/null || true)"
  if [[ "${CAN_PUSH}" != "true" ]]; then
    echo "ERROR: token cannot push to ${TAP_REPO} (authenticated as ${LOGIN:-unknown})." >&2
    echo "Create a fine-grained PAT with Contents: Read and write on ${TAP_REPO}," >&2
    echo "then update repo secret HOMEBREW_TAP_TOKEN. See docs/homebrew.md." >&2
    exit 1
  fi
  echo "==> Token OK (push access as ${LOGIN} → ${TAP_REPO})"
fi

WORKDIR="$(mktemp -d)"
cleanup() { rm -rf "${WORKDIR}"; }
trap cleanup EXIT

echo "==> Cloning ${TAP_REPO}"
# Clone without credentials; push uses GH_TOKEN in the URL below.
# Keep credential.helper empty so Actions does not inject GITHUB_TOKEN on push.
git -c credential.helper= clone --depth 1 \
  "https://github.com/${TAP_REPO}.git" "${WORKDIR}/tap"
mkdir -p "${WORKDIR}/tap/Formula"

TEMPLATE="${ROOT}/Formula/${FORMULA_NAME}.rb"
[[ -f "${TEMPLATE}" ]] || {
  echo "ERROR: missing ${TEMPLATE}" >&2
  exit 1
}

awk -v url="${TARBALL_URL}" -v sha="${SHA}" '
  /^  url "/ { print "  url \"" url "\""; next }
  /^  sha256 "/ { print "  sha256 \"" sha "\""; next }
  { print }
' "${TEMPLATE}" >"${WORKDIR}/tap/Formula/${FORMULA_NAME}.rb"

if [[ ! -f "${WORKDIR}/tap/README.md" ]]; then
  cat >"${WORKDIR}/tap/README.md" <<'EOF'
# nasraldin/homebrew-tools

Homebrew tap for Nasr Aldin tools.

```bash
brew tap nasraldin/tools
brew install docker-lab
brew install camunda-lab
```
EOF
fi

cd "${WORKDIR}/tap"
if [[ -n "${GITHUB_ACTIONS:-}" ]]; then
  git config user.name "github-actions[bot]"
  git config user.email "41898282+github-actions[bot]@users.noreply.github.com"
fi

if [[ -n "${GH_TOKEN:-}" ]]; then
  PUSH_URL="https://x-access-token:${GH_TOKEN}@github.com/${TAP_REPO}.git"
else
  PUSH_URL="origin"
fi

git add Formula/"${FORMULA_NAME}.rb" README.md
if git diff --cached --quiet; then
  echo "==> No changes (formula already up to date for ${TAG})"
  exit 0
fi

git commit -m "camunda-lab ${VERSION}"
# Push with token in URL; credential.helper= avoids Actions overriding the PAT.
if [[ "${PUSH_URL}" == "origin" ]]; then
  git -c credential.helper= push origin HEAD
else
  git -c credential.helper= push "${PUSH_URL}" HEAD:main
fi
echo "==> Published ${FORMULA_NAME} ${VERSION} → ${TAP_REPO}"
echo "==> Users: brew update && brew upgrade camunda-lab"
