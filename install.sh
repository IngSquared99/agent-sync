#!/bin/sh
# agsy one-line installer (macOS / Linux)
# 用法:curl -fsSL https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.sh | sh
# 行為:偵測平台 → 從 GitHub Releases 下載對應版本 → 解壓 → 安裝到 /usr/local/bin
# 環境變數:
#   AGSY_INSTALL_DIR  覆寫安裝目錄(預設 /usr/local/bin)
#   AGSY_DRYRUN=1     只印出將執行的動作,不下載不安裝(供測試)
set -eu

REPO="IngSquared99/agent-sync"
INSTALL_DIR="${AGSY_INSTALL_DIR:-/usr/local/bin}"

os="$(uname -s)"
arch="$(uname -m)"

# 平台 → Releases 檔名對照(與 .goreleaser.yaml 的 name_template 一致)
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

url="https://github.com/$REPO/releases/latest/download/$file"

if [ "${AGSY_DRYRUN:-}" = "1" ]; then
  echo "would download: $url"
  echo "would install to: $INSTALL_DIR/agsy"
  exit 0
fi

# 下載與解壓都在暫存目錄進行,結束時自動清理
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

echo "downloading $file ..."
curl -fsSL "$url" -o "$tmp/agsy.tar.gz"
tar -xzf "$tmp/agsy.tar.gz" -C "$tmp"

echo "installing to $INSTALL_DIR (your password may be asked) ..."
if [ -w "$INSTALL_DIR" ]; then
  mv "$tmp/agsy" "$INSTALL_DIR/agsy"
else
  sudo mv "$tmp/agsy" "$INSTALL_DIR/agsy"
fi

echo "done: $("$INSTALL_DIR/agsy" version)"
