#!/usr/bin/env bash
# install.sh — install camunda CLI from GitHub Releases
set -euo pipefail

REPO="${REPO:-nasraldin/camunda-lab}"
INSTALL_DIR="${INSTALL_DIR:-$HOME/.local/bin}"
VERSION="${VERSION:-latest}"

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

if [[ "$VERSION" == "latest" ]]; then
  VERSION="$(curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)"
fi
if [[ -z "$VERSION" ]]; then
  echo "No GitHub release found yet — build from source: make build" >&2
  exit 1
fi

asset="camunda-lab_${VERSION#v}_${os}_${arch}.tar.gz"
url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
echo "Downloading $url"
curl -fsSL "$url" -o "$tmp/camunda.tgz"
tar -xzf "$tmp/camunda.tgz" -C "$tmp"
mkdir -p "$INSTALL_DIR"
install -m 755 "$tmp/camunda" "$INSTALL_DIR/camunda"
echo "Installed $INSTALL_DIR/camunda ($VERSION)"
echo "Ensure $INSTALL_DIR is on your PATH"
