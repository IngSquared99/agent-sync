# A. 安裝說明

agsy 是單一執行檔（Go 編譯），沒有其他相依套件。支援 macOS（Intel / Apple Silicon）、Linux（x64 / arm64）、Windows（x64 / arm64）。

## A-1. macOS / Linux：一行安裝（建議）

```sh
curl -fsSL https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.sh | sh
```

安裝腳本會做的事：

1. 偵測作業系統與 CPU 架構，決定要下載哪個 release 壓縮檔。
2. 從 GitHub Releases 下載壓縮檔與 `checksums.txt`。
3. **驗證 SHA-256 檢查碼**（防止下載過程損毀或被竄改）。
4. 把 `agsy` 執行檔安裝到 `/usr/local/bin`（若該目錄不可寫，會透過 `sudo` 安裝，此時可能要求輸入密碼）。

### 可用的環境變數

| 變數 | 作用 |
|------|------|
| `AGSY_INSTALL_DIR` | 改變安裝目錄（預設 `/usr/local/bin`）。例：`AGSY_INSTALL_DIR=~/.local/bin curl -fsSL … \| sh` |
| `AGSY_DRYRUN=1` | 只印出「會下載什麼、會裝到哪」，不實際下載安裝。想先確認行為時很好用 |

## A-2. Windows：PowerShell 一行安裝

```powershell
irm https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.ps1 | iex
```

- 安裝到 `%LOCALAPPDATA%\Programs\agsy`，並自動加入**使用者層級**的 PATH。
- **不需要系統管理員權限**，全程只動使用者範圍的設定。
- 一樣會驗證 SHA-256 檢查碼。
- 安裝完請**開一個新的終端機視窗**再執行 `agsy version`（PATH 變更對已開啟的視窗不生效）。

## A-3. 手動下載

到 GitHub Releases 頁面（`https://github.com/IngSquared99/agent-sync/releases/latest`）下載對應平台的檔案：

| 平台 | 檔名 |
|------|------|
| macOS Apple Silicon（M 系列） | `agsy_mac_apple_silicon.tar.gz` |
| macOS Intel | `agsy_mac_intel.tar.gz` |
| Linux x64 | `agsy_linux_x64.tar.gz` |
| Linux arm64 | `agsy_linux_arm64.tar.gz` |
| Windows x64 | `agsy_windows_x64.zip` |
| Windows arm64 | `agsy_windows_arm64.zip` |

解壓縮後把 `agsy`（Windows 為 `agsy.exe`）放進任一在 PATH 裡的目錄即可。建議同時下載 `checksums.txt` 核對檢查碼。

## A-4. 從原始碼建置

需要 Go 1.22 以上：

```sh
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go build -o agsy ./cmd/agsy
```

或直接：

```sh
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

> 自行建置的版本 `agsy version` 會顯示 `dev`（正式版號是 release 流程注入的），功能不受影響。

## A-5. 驗證安裝

```sh
agsy version
# 例：agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

接著可以在任一專案跑一次環境檢查（唯讀、不會做任何動作）：

```sh
agsy doctor
```

## A-6. 介面語言

agsy 內建繁體中文介面，依環境變數自動偵測：

- 判斷順序：`AGSY_LANG` → `LC_ALL` → `LANG`；值以 `zh` 開頭就用繁體中文，其餘為英文。
- 想強制中文：`export AGSY_LANG=zh-TW`；想強制英文：`export AGSY_LANG=en`。

## A-7. 升級

重跑一次安裝指令即可（腳本永遠抓 `releases/latest`，會直接覆蓋舊執行檔）。

## A-8. 移除

1. 在每個用過 agsy 的專案裡先跑 `agsy clean`（移除掛載連結與 `.agsy/` 產物；`agsy.yaml` 會保留，不需要的話手動刪除）。
2. 刪掉執行檔：
   - macOS / Linux：`sudo rm /usr/local/bin/agsy`（或你自訂的安裝目錄）
   - Windows：刪除 `%LOCALAPPDATA%\Programs\agsy` 資料夾，並自 PATH 移除該項目。

→ 下一章：[快速上手](02-quickstart.md)
