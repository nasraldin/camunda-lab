#!/usr/bin/env bash
# install.sh — install camunda CLI from GitHub Releases (checksum verified)
set -euo pipefail

REPO="${REPO:-nasraldin/camunda-lab}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"
SKIP_CHECKSUM="${SKIP_CHECKSUM:-0}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  darwin|linux) ;;
  *) echo "unsupported OS: $os (macOS/Linux only)" >&2; exit 1 ;;
esac

sha256_file() {
  local f="$1"
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$f" | awk '{print $1}'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$f" | awk '{print $1}'
  else
    echo "ERROR: need shasum or sha256sum to verify the download" >&2
    exit 1
  fi
}

if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
fi
if [[ -z "$VERSION" ]]; then
  echo "No GitHub release found yet — build from source: make build && make install" >&2
  exit 1
fi

asset="camunda-lab_${VERSION#v}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
sums_url="https://github.com/${REPO}/releases/download/${VERSION}/checksums.txt"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "Downloading $url"
curl -fsSL "$url" -o "$tmp/camunda.tgz"

if [[ "$SKIP_CHECKSUM" != "1" ]]; then
  echo "Verifying checksum from $sums_url"
  curl -fsSL "$sums_url" -o "$tmp/checksums.txt"
  expected="$(awk -v a="$asset" '$2 == a { print $1; exit }' "$tmp/checksums.txt")"
  if [[ -z "$expected" ]]; then
    echo "ERROR: $asset not found in checksums.txt for ${VERSION}" >&2
    exit 1
  fi
  actual="$(sha256_file "$tmp/camunda.tgz")"
  if [[ "$actual" != "$expected" ]]; then
    echo "ERROR: checksum mismatch for $asset" >&2
    echo "  expected: $expected" >&2
    echo "  actual:   $actual" >&2
    exit 1
  fi
  echo "Checksum OK"
fi

tar -xzf "$tmp/camunda.tgz" -C "$tmp"
if [[ ! -f "$tmp/camunda" ]]; then
  # goreleaser may nest the binary; find it
  bin_path="$(find "$tmp" -type f -name camunda | head -1)"
  if [[ -z "$bin_path" ]]; then
    echo "ERROR: camunda binary not found in archive" >&2
    exit 1
  fi
else
  bin_path="$tmp/camunda"
fi

mkdir -p "$INSTALL_DIR"
install -m 755 "$bin_path" "$INSTALL_DIR/camunda"
echo "Installed $INSTALL_DIR/camunda ($VERSION)"
echo "Ensure $INSTALL_DIR is on your PATH"

echo ""
echo "Starting Lab UI in the background..."
if "$INSTALL_DIR/camunda" ui --no-open 2>/dev/null; then
  cat <<'EOF'

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  Camunda Lab UI is running

  Open in your browser: http://localhost:9090

  Install and manage Camunda from the UI — no terminal required.
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
EOF
else
  echo "Start the UI manually: camunda ui"
fi
