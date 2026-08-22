# 安裝說明

依你的作業系統選一種方式，都是一行指令：

| 方式 | 平台 | 指令 | 事前需要 |
|------|------|------|----------|
| 方式一：Homebrew | macOS | `brew install ingsquared99/tap/agsy` | 已裝 Homebrew |
| 方式二：winget | Windows 10 / 11 | `winget install IngSquared99.agsy` | 不用，系統內建 |
| 方式三：Go 原始碼 | 全平台（Linux 請走這條） | `go install …`（見下方「從原始碼建置」） | 已裝 Go |

**安全性說明**：方式一、二安裝的是 GitHub Release 上的預編譯執行檔——由公開的 CI 流程從公開原始碼自動編譯，且 brew 的 cask 與 winget 的 manifest 都寫死了對應檔案的 SHA-256 校驗碼，下載內容可驗證、可稽核。方式三則是直接抓原始碼在你自己的電腦上編譯，完全不經過預編譯檔。agsy 本身**零第三方相依套件**（只用 Go 標準函式庫）。

## 方式一：Homebrew（macOS）

```sh
brew install ingsquared99/tap/agsy
```

- brew 會從 GitHub Release 下載對應你機器（Apple Silicon / Intel）的執行檔並校驗。
- 安裝過程已處理 macOS 的隔離屬性，第一次執行**不會**跳「無法驗證開發者」的警告。
- 還沒裝過 Homebrew？到官網 <https://brew.sh> 照首頁指示安裝（macOS 開發者的標準配備，裝一次終身受用）。

## 方式二：winget（Windows）

```powershell
winget install IngSquared99.agsy
```

- winget 是 Windows 10 / 11 **內建**的官方套件管理器，不用先裝任何東西，開終端機（PowerShell 或 cmd）直接打即可。
- 裝完重開一個新的終端機視窗，再執行 `agsy version` 確認。

## 方式三：從原始碼建置（全平台；Linux 請走這條）

需要 **Go 1.22 以上**（建議最新穩定版）。還沒有 Go：macOS `brew install go`、Windows `winget install GoLang.Go`、Linux 用發行版套件（如 `apt install golang-go`）或官網 <https://go.dev/dl/>。

**快速版**——一行指令，Go 工具鏈自動抓原始碼、本機編譯、裝進 `~/go/bin/`：

```sh
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

裝完若終端機找不到 `agsy`，是 `~/go/bin` 不在 PATH（PATH＝終端機尋找指令的目錄清單）：

```sh
# macOS（預設 zsh）：加入設定檔後重開終端機；Linux（bash）改寫進 ~/.bashrc
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

**完整版**——適合想先檢視程式碼、或打算修改程式的人（另需 Git）：

```sh
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go test ./...                # （可選）先跑測試確認環境正常
go build -o agsy ./cmd/agsy  # 產出 agsy 執行檔（Windows 為 agsy.exe）
mv agsy ~/go/bin/            # 放進任一在 PATH 裡的目錄
```

沒有其他框架或函式庫需求——不用 npm、不用 pip，`go build` 一行就是全部。

## 驗證安裝

```sh
agsy version
# 例：agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

有印出版本資訊就是裝好了。接著可以在任一專案跑一次環境健檢（唯讀、不會做任何動作）：

```sh
agsy doctor
```

## 介面語言：中文／英文怎麼決定

agsy 內建繁體中文與英文兩種介面，**不用設定就會自動判斷**。它啟動時依序檢查三個「環境變數」（環境變數＝作業系統層級的設定值，終端機裡的程式都讀得到），找到第一個有值的就用它：

```
 AGSY_LANG 有值嗎？ ──有──▶ 用它判斷
     │ 沒有
     ▼
 LC_ALL 有值嗎？    ──有──▶ 用它判斷
     │ 沒有
     ▼
 LANG 有值嗎？      ──有──▶ 用它判斷
     │ 沒有
     ▼
   英文
```

判斷規則只有一條：**值以 `zh` 開頭（例如 `zh_TW.UTF-8`、`zh-TW`）→ 繁體中文；其他任何值 → 英文。**

三個變數的分工：

- `LC_ALL`、`LANG`：**作業系統本來就有的**語言設定，不是 agsy 的東西。台灣的 macOS / Linux 通常已經是 `zh_TW.UTF-8`，所以什麼都不用做，agsy 一開就是中文。
- `AGSY_LANG`：**agsy 專屬的開關**，優先權最高，用來蓋過系統設定（例如系統是英文但你想看中文介面）。

想手動指定語言：

```sh
export AGSY_LANG=zh-TW    # 這個終端機視窗內，強制中文
export AGSY_LANG=en       # 強制英文
```

`export` 只對目前這個終端機視窗有效；想永久生效，把那一行加進 shell 設定檔（macOS 預設 zsh → `~/.zshrc`），重開終端機後生效。

## 升級與移除

| | 方式一 Homebrew | 方式二 winget | 方式三 Go |
|---|---|---|---|
| 升級 | `brew upgrade agsy` | `winget upgrade IngSquared99.agsy` | 重跑一次 `go install …@latest` |
| 移除執行檔 | `brew uninstall agsy` | `winget uninstall IngSquared99.agsy` | 刪 `~/go/bin/agsy` |

移除前記得先在每個用過 agsy 的專案裡跑 `agsy clean`（移除掛載連結與 `.agsy/` 產物；`agsy.yaml` 會保留，不需要的話手動刪除）。

→ 下一章：[快速上手](quickstart.md)
