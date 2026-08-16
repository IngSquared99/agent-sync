#!/bin/sh
# agsy installer for macOS / Linux.
# Usage: curl -fsSL https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.sh | sh
# Steps: detect OS/arch -> download the matching release archive and checksums.txt
#        -> verify SHA-256 -> install the binary into INSTALL_DIR.
# Environment overrides:
#   AGSY_INSTALL_DIR  install directory (default: /usr/local/bin)
#   AGSY_DRYRUN=1     print planned actions without downloading or installing
set -eu

REPO="IngSquared99/agent-sync"
INSTALL_DIR="${AGSY_INSTALL_DIR:-/usr/local/bin}"
BASE="https://github.com/$REPO/releases/latest/download"

os="$(uname -s)"
arch="$(uname -m)"

# Map platform to release asset names (must match name_template in .goreleaser.yaml).
case "$os" in
  Darwin)
    case "$arch" in
      arm64)  file="agsy_mac_apple_silicon.tar.gz" ;;
      x86_64) file="agsy_mac_intel.tar.gz" ;;
      *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
    esac ;;
  Linux)
    case "$arch" in
      aarch64|arm64) file="agsy_linux_arm64.tar.gz" ;;
      x86_64)        file="agsy_linux_x64.tar.gz" ;;
      *) echo "unsupported architecture: $arch" >&2; exit 1 ;;
    esac ;;
  *)
    echo "unsupported OS: $os (on Windows, use install.ps1)" >&2
    exit 1 ;;
esac

if [ "${AGSY_DRYRUN:-}" = "1" ]; then
  echo "would download: $BASE/$file"
  echo "would verify:   $BASE/checksums.txt"
  echo "would install:  $INSTALL_DIR/agsy"
  exit 0
fi

# Download and extract inside a temp directory; cleaned up on exit.
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $file ..."
curl -fsSL "$BASE/$file" -o "$tmp/$file"
curl -fsSL "$BASE/checksums.txt" -o "$tmp/checksums.txt"

# Verify SHA-256 against the checksum file published with the release.
# This defends against corrupted or tampered downloads in transit; it does not
# (and cannot) defend against a compromised release itself.
expected="$(awk -v f="$file" '$2 == f { print $1 }' "$tmp/checksums.txt")"
if [ -z "$expected" ]; then
  echo "checksum entry for $file not found in checksums.txt" >&2
  exit 1
fi
if command -v sha256sum >/dev/null 2>&1; then
  actual="$(sha256sum "$tmp/$file" | awk '{ print $1 }')"
else
  actual="$(shasum -a 256 "$tmp/$file" | awk '{ print $1 }')"
fi
if [ "$actual" != "$expected" ]; then
  echo "checksum mismatch for $file (expected $expected, got $actual)" >&2
  exit 1
fi
echo "checksum OK"

tar -xzf "$tmp/$file" -C "$tmp"

echo "installing to $INSTALL_DIR (your password may be asked) ..."
if [ -w "$INSTALL_DIR" ]; then
  mv "$tmp/agsy" "$INSTALL_DIR/agsy"
else
  sudo mv "$tmp/agsy" "$INSTALL_DIR/agsy"
fi

echo "done: $("$INSTALL_DIR/agsy" version)"
