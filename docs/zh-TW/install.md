# 安裝

依作業系統選一種方式。裝完用 `agsy version` 確認。

## 一、選安裝方式

| 你的系統 | 用這個 | 事前需要 |
|---------|--------|---------|
| macOS | 方式一：Homebrew | 已裝 Homebrew |
| Windows 10 / 11 | 方式二：winget | 不用，系統內建 |
| Linux，或想自己編譯 | 方式三：Go 原始碼 | 已裝 Go 1.22 以上 |

方式一、二下載的是 GitHub Release 上的預編譯執行檔，由公開的 CI 從公開原始碼自動編譯，安裝設定裡寫死了檔案的 SHA-256 校驗碼。方式三是在你的電腦上從原始碼編譯。agsy 不依賴任何外部套件。

## 二、安裝

### 方式一：Homebrew（macOS）

```sh
brew install ingsquared99/tap/agsy
```

會自動抓對應 Apple Silicon 或 Intel 的版本。第一次執行不會出現「無法驗證開發者」的警告。還沒有 Homebrew：依 <https://brew.sh> 安裝。

### 方式二：winget（Windows）

```powershell
winget install IngSquared99.agsy
```

開 PowerShell 或 cmd 直接輸入即可。裝完**重開一個新的終端機視窗**再繼續。

### 方式三：從原始碼（全平台）

還沒有 Go：macOS `brew install go`、Windows `winget install GoLang.Go`、Linux 用發行版套件（例如 `apt install golang-go`）或 <https://go.dev/dl/>。

一行安裝：

```sh
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

執行檔會放在 `~/go/bin/`。裝完終端機找不到 `agsy` 的話，是這個資料夾不在 PATH（終端機找指令的資料夾清單）裡：

```sh
# macOS（zsh）：加進設定檔後重開終端機；Linux（bash）改寫進 ~/.bashrc
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

想先看程式碼或自己修改：

```sh
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go test ./...                # 可選：先跑測試
go build -o agsy ./cmd/agsy  # 產出 agsy（Windows 是 agsy.exe）
mv agsy ~/go/bin/            # 放進任一在 PATH 裡的資料夾
```

## 三、確認

```sh
agsy version
# 例：agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

有印出版本就是裝好了。可以再跑一次唯讀的健檢：

```sh
agsy doctor
```

## 四、介面語言

agsy 有繁體中文和英文兩種介面，自動判斷，不用設定。判斷順序：

```
 AGSY_LANG 有值？ ──有──▶ 用它
     │ 沒有
 LC_ALL 有值？    ──有──▶ 用它
     │ 沒有
 LANG 有值？      ──有──▶ 用它
     │ 沒有
   英文
```

規則只有一條：值以 `zh` 開頭（例如 `zh_TW.UTF-8`）→ 繁體中文；其他 → 英文。

- `LC_ALL`、`LANG` 是作業系統本來就有的語言設定。台灣的 macOS / Linux 通常已經是 `zh_TW.UTF-8`，什麼都不用做。
- `AGSY_LANG` 是 agsy 專用的開關，優先權最高，用來蓋過系統設定。

手動指定：

```sh
export AGSY_LANG=zh-TW    # 這個終端機視窗內用中文
export AGSY_LANG=en       # 用英文
```

`export` 只對目前的終端機視窗有效。要永久生效，把那一行加進 shell 設定檔（macOS 是 `~/.zshrc`）。

## 五、升級與移除

| | Homebrew | winget | Go |
|---|---|---|---|
| 升級 | `brew upgrade agsy` | `winget upgrade IngSquared99.agsy` | 重跑 `go install …@latest` |
| 移除 | `brew uninstall agsy` | `winget uninstall IngSquared99.agsy` | 刪 `~/go/bin/agsy` |

移除前，先在每個用過 agsy 的專案跑 `agsy clean`（移除連結與 `.agsy/`；`agsy.yaml` 會留下，不要的話自己刪）。

→ 下一章：[快速上手](quickstart.md)
