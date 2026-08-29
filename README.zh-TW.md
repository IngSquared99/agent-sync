<!-- 本檔由 scripts/genreadme 自動組裝，內容來源為 docs/zh-TW/ — 請勿手動編輯。 -->

# agsy（agent-sync）

[![Release](https://img.shields.io/github/v/release/IngSquared99/agent-sync)](https://github.com/IngSquared99/agent-sync/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> 多來源 AI 指令檔的合併與掛載工具：一份來源，同步 Claude Code、Codex、Antigravity 等多個 AI 工具。

**[📘 完整說明文件](https://ingsquared99.github.io/agent-sync/#/zh-TW/)** ｜ [English README](README.md)

---

## A. 核心概念：agsy 是什麼？

<br>

### 它解決的問題

當你同時使用多個 AI 開發工具（Claude Code、OpenAI Codex、Google Antigravity、Cursor…），每個工具讀取的位置與格式都不一樣：

- Claude Code 從 `.claude/rules/` 讀 rules、從 `.claude/skills/` 讀 skills
- Codex 讀專案根目錄的 `AGENTS.md`，並從 `.agents/skills/` 讀 skills
- Antigravity 讀根目錄 `AGENTS.md`、從 `.agents/skills/` 讀 skills、從 `.agents/workflows/` 讀 `/名稱` workflows
- Cursor 讀根目錄 `AGENTS.md`，並從 `.agents/skills/` 讀 skills

同一套編碼規範、技能與流程被複製成好幾份、好幾種格式，每次修改都要同步到每一份。此外你通常還想要「個人共用庫＋專案專屬庫」的分層。

**agsy（agent-sync）** 正是為此而生：

> 它把多個來源的指令檔**合併**進單一建置輸出目錄（預設 `.agsy/`），**轉換**成各工具的原生格式，再以連結**掛載**到各工具的讀取位置。

你只編輯來源，執行一次 `agsy apply`，所有工具同時更新。

<br>

### 先認識這些名詞

之後的文件會一直用到這些詞。

| 名詞 | 意思 |
|------|------|
| 來源（source） | 你維護的原始指令庫（`sources` 陣列），可以有多個 |
| 產物（output / artifacts） | `apply` 建置出來的目錄，預設 `.agsy/`。裡面全部是產生出來的，整個目錄可重建 |
| 掛載（mount） | 在各工具的讀取位置建立指向產物的「連結」 |
| 連結（symlink / junction / hard link） | 作業系統層級的指標，指向另一個目錄或檔案——**內容不會有第二份** |
| 類別（category） | 三種指令檔：rules、skills、workflows |
| 轉換產物（derived form） | 由轉換而非原樣複製產生的輸出：串接的 `AGENTS.md`、workflow 的 skill 形態、workflow 的轉接頭 |
| manifest | `.agsy/.agsy-manifest.json`，建置紀錄；agsy 用它判斷哪一端變了什麼 |
| 來源標記（source tag） | 同名項目以 rename 保留時附加在檔名上的來源識別，如 `-fromlib-all-ai-lib` |
| adapter | 各工具的內建掛載預設，`init` 用它產生掛載設定 |
| tools | `build.tools` 清單；workflow 的 `target:` 只能引用這裡列出的名稱 |

<br>

### 單向資料流

agsy 的資料流嚴格單向：

```
 你維護的來源 ──▶ build（複製＋轉換）──▶ .agsy/ ──▶ mount（連結）──▶ 各工具
```

**來源是唯一真相。`.agsy/` 裡的一切——也就是工具透過掛載讀到的一切——都是唯讀、可重建的產物。** 沒有回寫機制：掛載中的檔案被修改時，`status` 會回報、`apply` 會列出並詢問確認後重建覆蓋。保留改動的方式是手動搬進來源（status 會指出目的地），因此 AI 產出的內容在進入庫之前必經人工審視。

<br>

### 三層架構

```
┌─────────────────────┐
│  來源                │  ~/all-ai-lib/       （個人共用庫）
│ （你維護的原稿）      │  ./repo-ai-lib/      （專案內的庫）
└─────────┬───────────┘
          │  ① agsy apply：掃描 → 合併 → 複製 → 轉換
          ▼
┌─────────────────────┐
│  產物                │  .agsy/rules/        rules 原樣
│ （可重建、唯讀）      │  .agsy/AGENTS.md     rules 串接（轉換產物）
│                      │  .agsy/skills/       skills＋workflow 的 skill 形態
│                      │  .agsy/workflows/    workflow 轉接頭
└─────────┬───────────┘
          │  ② agsy apply：建立連結
          ▼
┌─────────────────────┐
│  掛載                │  AGENTS.md        → .agsy/AGENTS.md
│ （工具實際讀取處）    │  .claude/rules    → .agsy/rules
│                      │  .claude/skills   → .agsy/skills
│                      │  .agents/skills   → .agsy/skills
│                      │  .agents/workflows→ .agsy/workflows
└─────────────────────┘
```

- **來源**：你維護並進版控的原稿。順序＝優先序（前者優先）。
- **產物**（`build.out`，預設 `.agsy/`）：建置成品。`apply` 每次整個清空重建。
- **掛載**：各工具讀取位置的連結。目錄用 symlink（Windows 用 junction）；根目錄 `AGENTS.md` 用檔案 symlink（Windows 用 hard link）。工具看到的是連結；內容都在 `.agsy/`。

<br>

### 三種類別

指令檔依用途分三類。來源以三個子目錄存放（預設 `rules/`、`skills/`、`workflows/`），各有格式規則：

| 類別 | 來源格式 | 是什麼 | 輸出形態 |
|------|---------|--------|---------|
| rules | 單一 `.md` 檔 | 長期有效的規範與風格指南，常駐於 context | `rules/` 逐檔原樣（給 Claude Code）**加上**一份串接的 `AGENTS.md`（給 Codex / Cursor / Antigravity） |
| skills | 內含 `SKILL.md` 的**目錄** | 打包好的能力；工具在任務符合描述時自動取用 | `skills/` 原樣複製 |
| workflows | 單一 `.md` 檔 | 由人觸發的流程與 SOP，以 `/名稱` 執行 | `skills/` 裡的 **skill 形態**（front matter 補上 `disable-model-invocation: true`，支援此欄位的工具絕不自行執行）**加上** `workflows/` 裡給 Antigravity `/名稱` 用的**轉接頭** |

為什麼 workflow 要變成 skill：Claude Code、Codex、Cursor 都是透過 skills 機制讀取指令與流程，以 front matter（而非另一個目錄）決定誰能觸發。workflow 的來源維持單一檔案，由 build 打包成這些工具期望的 skill 形態。Antigravity 從 `workflows/` 目錄以 `/名稱` 觸發，build 在該處留一個簡短的轉接頭，指示 agent 載入對應的 skill（內容只存在一份，即 skill 形態）。

<br>

### 各工具最終讀到什麼

| 工具 | rules | skills | workflows |
|------|-------|--------|-----------|
| Claude Code | `.claude/rules/`（逐檔） | `.claude/skills/` | `.claude/skills/` 裡的 skill 形態，以 `/名稱` 觸發 |
| Codex | 根目錄 `AGENTS.md` | `.agents/skills/` | 同左（skill 形態） |
| Antigravity | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/workflows/` 的轉接頭 → `/名稱` 載入 skill |
| Cursor | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/skills/` 裡的 skill 形態，以 `/名稱` 觸發 |

`.agents/skills/` 是 Codex、Antigravity、Cursor 三家原生共讀的目錄——一條連結服務三個工具。Claude Code 不讀 `AGENTS.md`，所以只有它另外掛逐檔的 `.claude/rules/`。

<br>

### 來源子目錄需遵循命名慣例

agsy 預設掃描來源裡的 `rules/`、`skills/`、`workflows/`。既有的庫若一直使用別的名稱，可透過 `build.categories.<類別>.from` 直接接上、不必搬檔案——見[設定檔](https://ingsquared99.github.io/agent-sync/#/zh-TW/config)。

<br>

---

## B. 安裝說明

依你的作業系統選一種方式，都是一行指令：

| 方式 | 平台 | 指令 | 事前需要 |
|------|------|------|----------|
| 方式一：Homebrew | macOS | `brew install ingsquared99/tap/agsy` | 已裝 Homebrew |
| 方式二：winget | Windows 10 / 11 | `winget install IngSquared99.agsy` | 不用，系統內建 |
| 方式三：Go 原始碼 | 全平台（Linux 請走這條） | `go install …`（見下方「從原始碼建置」） | 已裝 Go |

**安全性說明**：方式一、二安裝的是 GitHub Release 上的預編譯執行檔——由公開的 CI 流程從公開原始碼自動編譯，且 brew 的 cask 與 winget 的 manifest 都寫死了對應檔案的 SHA-256 校驗碼，下載內容可驗證、可稽核。方式三則是直接抓原始碼在你自己的電腦上編譯，完全不經過預編譯檔。agsy **不依賴外部模組**：YAML 解析為內嵌（vendored）的 go-yaml 副本，其餘皆為 Go 標準函式庫。

<br>

### 方式一：Homebrew（macOS）

```sh
brew install ingsquared99/tap/agsy
```

- brew 會從 GitHub Release 下載對應你機器（Apple Silicon / Intel）的執行檔並校驗。
- 安裝過程已處理 macOS 的隔離屬性，第一次執行**不會**跳「無法驗證開發者」的警告。
- Homebrew 本身依 <https://brew.sh> 的指示安裝。

<br>

### 方式二：winget（Windows）

```powershell
winget install IngSquared99.agsy
```

- winget 是 Windows 10 / 11 **內建**的官方套件管理器，不用先裝任何東西，開終端機（PowerShell 或 cmd）直接打即可。
- 裝完重開一個新的終端機視窗，再執行 `agsy version` 確認。

<br>

### 方式三：從原始碼建置（全平台；Linux 請走這條）

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

不需其他框架或套件管理工具，`go build` 即完成建置。

<br>

### 驗證安裝

```sh
agsy version
# 例：agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

有印出版本資訊就是裝好了。接著可以在任一專案跑一次環境健檢（唯讀、不會做任何動作）：

```sh
agsy doctor
```

<br>

### 介面語言：中文／英文怎麼決定

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

<br>

### 升級與移除

| | 方式一 Homebrew | 方式二 winget | 方式三 Go |
|---|---|---|---|
| 升級 | `brew upgrade agsy` | `winget upgrade IngSquared99.agsy` | 重跑一次 `go install …@latest` |
| 移除執行檔 | `brew uninstall agsy` | `winget uninstall IngSquared99.agsy` | 刪 `~/go/bin/agsy` |

移除前記得先在每個用過 agsy 的專案裡跑 `agsy clean`（移除掛載連結與 `.agsy/` 產物；`agsy.yaml` 會保留，不需要的話手動刪除）。

<br>

---

## C. 快速上手

四步完成第一次同步，全部在專案目錄內進行。

<br>

### 第 0 步：準備一個來源庫

來源是一個至多含三個子目錄的資料夾，任何子集皆可：

```
~/all-ai-lib/
├── rules/
│   └── python-style.md          # 單純的 markdown 檔
├── skills/
│   └── code-review/
│       └── SKILL.md             # 內含 SKILL.md 的目錄
└── workflows/
    └── deploy.md                # 單純的 markdown 檔，可選 target: front matter
```

可以指向一個庫或多個；常見配置是個人共用庫（`~/all-ai-lib`）加專案內的庫（`./repo-ai-lib`）。

<br>

### 第 1 步：`agsy init`——產生設定檔

```
$ cd your-project
$ agsy init
開始設定 agsy（按 Enter 採用預設值）

來源路徑,依優先序排列（~ 開頭=共用庫,./ 開頭=專案內）
  來源 1: ~/all-ai-lib
  來源 2: ./repo-ai-lib
  來源 3: ⏎

要服務哪些工具?（空白分隔多個編號,a = 全部,Enter = 全部）
    1) Claude Code (.claude/)
    2) OpenAI Codex (.agents/)
    3) Antigravity (.agents/)
    4) Cursor (.agents/)
請輸入: a

rules 的同名衝突怎麼處理?（建議 rename…）        ❯ rename
skills 的同名衝突怎麼處理?（建議 error…）         ❯ error
workflows 的同名衝突怎麼處理?                     ❯ rename

建置產物目錄（預設: .agsy）: ⏎

✔ 已寫入 agsy.yaml

以下由 agsy 產生的路徑皆可重建,通常應加入 .gitignore:
要把哪些項目加進 .gitignore?（a = 全部）a
  ✔ 已將 6 個項目加入 .gitignore
  下一步:agsy plan 預覽 → agsy apply 執行
```

`.gitignore` 那題除非團隊刻意把連結進版控，答 **a**（全部）即可；`agsy.yaml` 本身**應該** commit。

腳本用的非互動形式：`agsy init --yes ~/all-ai-lib ./repo-ai-lib`。

<br>

### 第 2 步：`agsy plan`——預覽、不寫入

```
$ agsy plan
```

預覽逐類別列出建置會收的一切：哪些 rules 被衝突策略改名、每個 workflow 產生哪些形態（`skill skills/deploy`／`轉接頭 workflows/deploy.md`）、轉換產物 `AGENTS.md` 一行、每個被排除的檔案與原因、每條掛載連結會發生什麼。不寫入任何檔案；視需要調整後重新執行 plan。

<br>

### 第 3 步：`agsy apply`——建置並掛載

```
$ agsy apply
✔ build 完成:12 個項目 → .agsy/
✔ mount 完成:6 條連結
```

完成後的專案結構：

```
your-project/
├── AGENTS.md          → .agsy/AGENTS.md         （全部 rules 串接）
├── .claude/
│   ├── rules          → .agsy/rules
│   └── skills         → .agsy/skills
├── .agents/
│   ├── skills         → .agsy/skills
│   └── workflows      → .agsy/workflows
└── .agsy/             建置產物
```

每個工具都從自己的原生位置讀到同一批內容。在 Claude Code 或 Cursor 輸入 `/deploy` 會執行該 workflow 的 skill 形態；在 Antigravity 則執行轉接頭，由它載入該 skill。

<br>

### 第 4 步：日常循環

```
編輯來源  ──▶  agsy apply  ──▶  所有工具都是最新的
                 ▲
status 檢查落差 ──┘（有任何不同步時 exit code 為 1）
```

AI 工具透過掛載寫入內容（新規則、改過的 skill）時，`agsy status` 會附指引列出；把要保留的搬進來源，再 apply。詳見[指令參考](https://ingsquared99.github.io/agent-sync/#/zh-TW/commands)與[情境指南](https://ingsquared99.github.io/agent-sync/#/zh-TW/scenarios)。

<br>

### 指令速查

```
agsy            附狀態摘要的選單
agsy doctor     環境健檢
agsy plan       預覽（唯讀）
agsy apply      建置＋掛載（先確認要捨棄的東西）
agsy status     兩張落差清單＋掛載健康（唯讀,exit code 適合 CI）
agsy clean      從此專案反安裝
```

<br>

---

## D. 深入了解

安裝與快速上手之外的完整教學都在說明文件站：

| 主題 | 內容 |
|------|------|
| [設定檔 agsy.yaml](https://ingsquared99.github.io/agent-sync/#/zh-TW/config) | 每個欄位的完整說明與安全規則 |
| [指令說明](https://ingsquared99.github.io/agent-sync/#/zh-TW/commands) | 各指令細節與使用情境 |
| [適配器](https://ingsquared99.github.io/agent-sync/#/zh-TW/adapters) | 內建工具範本與自訂掛載 |
| [情境全覽](https://ingsquared99.github.io/agent-sync/#/zh-TW/scenarios) | apply 在每種情境下的行為 |
| [Q&A 常見問題](https://ingsquared99.github.io/agent-sync/#/zh-TW/faq) | 以使用者角度整理的常見問題 |
