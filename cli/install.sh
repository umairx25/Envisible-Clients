#!/usr/bin/env bash
set -euo pipefail

REPO="umairx25/Envisible-Clients"
BIN="envis"
INSTALL_DIR="${ENVIS_INSTALL_DIR:-$HOME/.local/bin}"

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
case "$OS" in
  linux|darwin) ;;
  *)
    echo "Unsupported OS: $OS"
    echo "Please download a release manually from GitHub Releases."
    exit 1
    ;;
 esac

ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH="amd64" ;;
  arm64|aarch64) ARCH="arm64" ;;
  *)
    echo "Unsupported architecture: $ARCH"
    exit 1
    ;;
 esac

API_URL="https://api.github.com/repos/${REPO}/releases/latest"
URL=$(curl -fsSL "$API_URL" | grep -oE "https://[^\"]+/${BIN}_[0-9.]+_${OS}_${ARCH}\.tar\.gz" | head -n1 || true)

if [ -z "$URL" ]; then
  echo "Could not find a matching release asset for ${OS}/${ARCH}."
  echo "Check the latest GitHub Release assets for ${REPO}."
  exit 1
fi

TMP_DIR="$(mktemp -d)"
cleanup() { rm -rf "$TMP_DIR"; }
trap cleanup EXIT

curl -fsSL "$URL" -o "$TMP_DIR/${BIN}.tar.gz"
tar -xzf "$TMP_DIR/${BIN}.tar.gz" -C "$TMP_DIR"

mkdir -p "$INSTALL_DIR"
if command -v install >/dev/null 2>&1; then
  install -m 0755 "$TMP_DIR/$BIN" "$INSTALL_DIR/$BIN"
else
  chmod +x "$TMP_DIR/$BIN"
  mv "$TMP_DIR/$BIN" "$INSTALL_DIR/$BIN"
fi

echo "Installed $BIN to $INSTALL_DIR/$BIN"
echo "Ensure $INSTALL_DIR is on your PATH."
