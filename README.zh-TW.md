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

當你同時使用多個 AI 開發工具（Claude Code、OpenAI Codex、Antigravity…），每個工具都有自己的指令檔目錄：

- Claude Code 讀 `.claude/rules/`、`.claude/skills/`、`.claude/commands/`
- Codex 讀 `.codex/prompts/`
- Antigravity 讀 `.agents/`

同一份「coding 規範」「常用 skill」「工作流程」得複製好幾份、改一處要同步好幾處；而且你可能還想把「個人共用的一套」和「這個專案專屬的一套」疊在一起用。

**agsy（agent-sync）** 就是做這件事的合併＋掛載工具：

> 把多個來源（sources）的指令檔，**合併建置**成單一產物目錄（預設 `.agsy/`），再用**目錄連結**掛載進每個工具的讀取位置。

改動時只改來源，跑一次 `agsy apply`，所有工具同時更新。

<br>

### 開始前，先認識幾個名詞

後面的說明會一直用到這些詞。先大概有個印象，讀起來就不會卡；中途忘記了，隨時回來這張表查。

| 名詞 | 意思 |
|------|------|
| source（來源） | 你維護的原始指令檔庫，`agsy.yaml` 的 `sources` 陣列，可以有多個 |
| build out（產物） | `apply` 建置出的目錄，預設 `.agsy/`。裡面全是複本，整個可以砍掉重建 |
| mount（掛載） | 在各工具的目錄裡建立指向產物的「連結」 |
| 連結（symlink / junction） | 作業系統的「捷徑」：一個指向別處資料夾的入口，打開它等於打開目標資料夾，**不會多存一份檔案** |
| category（類別） | 指令檔的三種分類：rules（規則）、skills（技能）、workflows（工作流程） |
| bucket（桶子） | 產物裡 `workflows/` 底下按工具分的子資料夾（如 `claude`、`codex`），一個工具掛一桶 |
| routing（分流） | 建置時決定「每份 workflow 要進哪些桶」的過程，依據是檔案裡的 `target` 標記 |
| manifest | `.agsy/.agsy-manifest.json`，建置紀錄檔；agsy 靠它判斷「哪邊被改過」 |
| source tag（來源標籤） | 同名檔案改名保留時，附加在檔名上的來源識別，如 `-fromlib-all-ai-lib` |
| adapter（適配器） | 內建的「各家工具掛載範本」，`init` 時用來產生掛載設定 |
| behind | 來源已更新、產物還沒重建 → 該跑 `apply` |
| local changes（本機改動） | 產物端被改了、還沒寫回來源 → 該跑 `promote` |
| untracked | 產物端新增、manifest 不認識的檔案（`apply` 會刪掉；要保留請搬回來源） |
| orphan（孤兒連結） | 之前的 `apply` 建立、但現在設定已不再引用的連結 |

> **兩端與通道**：檔案內容只存在於兩端——「**來源端**」＝ sources 裡的原始檔、「**產物端**」＝ `.agsy/` 裡的複本。掛載是連結，工具目錄（`.claude/` 等）裡開啟的檔案即為產物端。連結本身稱「**掛載連結**」，只是通道、不儲存內容；通道的各種狀況見[情境全覽「掛載連結的情境」](https://ingsquared99.github.io/agent-sync/#/zh-TW/scenarios)。

<br>

### 三層架構

agsy 的世界只有三層、兩個動作。先看最簡化的版本：

```
 你維護的來源      ── ①建置（複製）──▶    產物目錄      ── ②掛載（連結）──▶   各 AI 工具的目錄
   sources                              .agsy/                            .claude/ .codex/ …
```

- **來源（sources）**：你真正維護、進版控的原始檔。可以有多個，順序代表優先權（越前面越優先）。
- **產物（build.out，預設 `.agsy/`）**：建置出來的成品，**整個目錄視為可重建的拋棄式產出**——`apply` 每次會清空重建，所以不要把手寫的東西直接放進去（除非你打算用 `promote` 寫回來源）。
- **掛載（mount）**：在各工具的目錄裡建立指向產物的連結。工具看到的是連結，實際內容都在 `.agsy/` 裡。

接下來把 ① 和 ② 兩個動作分開看清楚。

#### 動作①「建置」：把多個來源合併「複製」成一份

```
 ~/all-ai-lib/rules/python-style.md  ──┐
 ~/all-ai-lib/rules/git-commit.md    ──┤  複製、合併
 ./repo-ai-lib/rules/api-naming.md   ──┘
                                       ▼
                              .agsy/rules/python-style.md
                              .agsy/rules/git-commit.md
                              .agsy/rules/api-naming.md
```

這張圖的重點只有一個：**產物裡的檔案是「複本」**。來源可以散落在好幾個地方（個人共用庫、專案內的庫…），建置後全部集中成一份；也正因為是複本，整個 `.agsy/` 隨時可以砍掉重建，不心疼。

#### 動作②「掛載」：建一個「連結」，不是再複製一次

```
 .claude/rules   ────────── 連結（捷徑）──────────▶   .agsy/rules/
 （工具從這裡讀）                                    （檔案實際只存在這裡）
```

這張圖的重點也只有一個：`.claude/rules` **不是一個真的資料夾**，而是一個連結（macOS / Linux 用 symlink，Windows 用 junction）——像捷徑一樣，打開它看到的就是 `.agsy/rules/` 裡的內容。

所以：同一份檔案不會佔兩份空間；`.agsy/` 一更新，所有工具**立刻**看到新內容，不需要再做一次「同步」。

#### 把兩個動作串起來：完整樣貌

前面兩個動作都懂了之後，一個掛載完成的專案整體長這樣（看不懂的話，回頭看上面兩張小圖即可）：

```
┌────────────────────┐
│  來源 sources       │  ~/all-ai-lib/       （個人共用庫，跨專案）
│ （你維護的原始檔）    │  ./repo-ai-lib/      （專案內的庫）
└─────────┬──────────┘
          │  ① agsy apply：掃描 → 合併 → 複製
          ▼
┌────────────────────┐
│  產物 build.out     │  .agsy/rules/
│ （可整個重建的複本）  │  .agsy/skills/
│                    │  .agsy/workflows/<bucket>/
└─────────┬──────────┘
          │  ② agsy apply：建立目錄連結（symlink / junction）
          ▼
┌────────────────────┐
│  掛載 mount         │  .claude/rules  → ../.agsy/rules
│ （各 AI 工具實際     │  .claude/skills → ../.agsy/skills
│   讀取的位置）       │  .codex/prompts → ../.agsy/workflows/codex
└────────────────────┘
```

<br>

### 三種類別（categories）

指令檔依用途分成三種類別，先弄懂它們各自是什麼：

- **rules（規則）**：長期有效的規範或風格指引——coding style、命名慣例、commit 訊息格式……AI 工具會把它當成「隨時都要遵守的背景守則」讀進去。
- **skills（技能）**：打包好的「一項能力」——一個目錄裝著說明（`SKILL.md`）加上可能附帶的腳本、範本等素材。工具依 `SKILL.md` 裡的描述，在遇到相關任務時自動取用。
- **workflows（工作流程）**：一步一步的操作流程或常用指令——release 流程、code review SOP……在 Claude Code 裡會變成可以直接呼叫的斜線指令（`/release` 這種）。

來源目錄下就用三個同名子目錄存放，格式各有規定：

| 類別 | 來源子目錄（預設） | 格式 | 輸出位置 |
|------|--------------------|------|----------|
| rules | `rules/` | 單一 `.md` 檔 | `.agsy/rules/` |
| skills | `skills/` | **目錄**，內含 `SKILL.md` | `.agsy/skills/` |
| workflows | `workflows/` | 單一 `.md` 檔，開頭可標 `target` | `.agsy/workflows/<bucket>/` |

一個典型的來源長這樣：

```
~/all-ai-lib/
├── rules/
│   ├── python-style.md
│   └── git-commit.md
├── skills/
│   └── code-review/
│       ├── SKILL.md
│       └── scripts/…
└── workflows/
    └── release.md        （front-matter: target: claude）
```

<br>

### 目前支援的工具與對應資料夾

掛載之後，三種類別會出現在各工具的哪個位置？內建適配器目前支援三個工具，對應關係如下（細節與自訂方式見[適配器說明](https://ingsquared99.github.io/agent-sync/#/zh-TW/adapters)）：

| 工具 | 掛載目錄 | rules | skills | workflows |
|------|----------|-------|--------|-----------|
| Claude Code | `.claude/` | `.claude/rules` | `.claude/skills` | `.claude/commands`（成為斜線指令） |
| OpenAI Codex | `.codex/` | —（不掛載） | —（不掛載） | `.codex/prompts` |
| Antigravity | `.agents/` | `.agents/rules` | `.agents/skills` | `.agents/workflows` |

表格裡的每一格都是一個**連結**，實際內容都在 `.agsy/` 裡；Codex 依其慣例只讀 prompts，所以 rules 和 skills 沒有掛載位置（`init` 時若只勾 Codex 會特別警告這件事）。

<br>

### 來源目錄必須照規範命名

不管是個人共用庫（`~/all-ai-lib`）還是專案內的庫（`./repo-ai-lib`），**裡面的子目錄都必須叫 `rules/`、`skills/`、`workflows/`**（複數）——agsy 掃描時只認這三個名字，拼錯或用別的名字（如 `rule/`、`my-rules/`）的目錄會被直接跳過，檔案就「神祕消失」了。

```
✔ 正確                          ✘ 掃不到
~/all-ai-lib/rules/…            ~/all-ai-lib/rule/…
~/all-ai-lib/skills/…           ~/all-ai-lib/Skill/…
~/all-ai-lib/workflows/…        ~/all-ai-lib/wf/…
```

> 如果你的既有目錄真的叫別的名字，不必搬家：可以在 `agsy.yaml` 用 `build.categories.<類別>.from` 改掉掃描的子目錄名，詳見[設定檔說明](https://ingsquared99.github.io/agent-sync/#/zh-TW/config)。不確定有沒有掃到時，跑 `agsy doctor` 立刻見分曉。

<br>

### workflows 的分流：bucket 與 routing

rules 和 skills 建置後**所有工具讀同一份**；workflows 不一樣——每個工具的「指令／prompt」格式與用途不一定通用，所以產物裡的 `workflows/` 又按工具分成子資料夾：

```
.agsy/workflows/
├── claude/        ←  Claude Code 專用（.claude/commands 掛的就是這裡）
└── codex/         ←  Codex 專用（.codex/prompts 掛的就是這裡）
```

先把兩個名詞接起來：

- 這些子資料夾就叫 **bucket（桶子）**：一個工具一個桶，各工具只掛載自己的桶。
- 建置時 agsy 決定「每份 workflow 該丟進哪些桶」的過程，就叫 **routing（分流）**。

分流的依據，是 workflow 檔案開頭 front-matter 的 `target` 標記：

```
 workflows/release.md （target: claude）          ──▶  只進 claude 桶 → 只有 Claude Code 看得到
 workflows/deploy.md  （target: [claude, codex]） ──▶  兩個桶各放一份 → 兩個工具都看得到
 workflows/note.md    （沒寫 target）             ──▶  依 route.default 的設定決定去向
```

`target` 的寫法就是在檔案最上方：

```markdown
---
target: claude          # 只給 Claude Code
# 或 target: [claude, codex]  # 兩者都要
---
Release 流程說明…
```

**沒寫 `target` 會怎樣？** 落入設定檔 `route.default` 指定的桶。建議把 default 設成「全部的桶」——一份檔案「到處都出現」比「神祕消失」容易理解得多。細節見[設定檔說明](https://ingsquared99.github.io/agent-sync/#/zh-TW/config)與[適配器說明](https://ingsquared99.github.io/agent-sync/#/zh-TW/adapters)。

<br>

### 雙向資料流：apply 與 promote

**apply（正向）：來源 → 產物。** 平常的改動都走這條——改來源、跑 apply，所有工具同步更新：

```
 sources（你在這裡改檔案）
    │
    │  agsy apply（重新建置＋掛載）
    ▼
 .agsy/ ──連結──▶  所有工具立刻讀到新內容
```

**promote（反向）：產物 → 來源。** 當你（或 AI 工具）直接改了產物端的檔案——因為掛載是連結，實際被改到的是 `.agsy/` 裡的複本——就用 promote 把改動收回來源：

```
 .claude/skills/…（AI 工具直接改了這裡的檔案）
    ‖  掛載是連結，所以實際改到的是產物端（.agsy/ 裡的複本）
    │
    │  agsy promote（把改動寫回）
    ▼
 sources（改動被保存——下次 apply 重建時才不會被蓋掉）
```

agsy 用 `.agsy/.agsy-manifest.json`（建置紀錄檔）記錄每個項目在建置當下的來源與產物雜湊值，因此 `status` 能精準判斷：哪些是「來源更新了還沒重建」（behind）、哪些是「產物端被改了還沒寫回」（local changes）、兩邊是否同時被改（需要人工合併）。

<br>

### 設計上的安全底線

讀文件時會一直看到這些原則，先列在這裡：

1. **不碰真實檔案**：掛載點若已存在「真實的目錄或檔案」（不是 agsy 建的連結），agsy 一律報錯請你手動處理，絕不代為刪除。
2. **同名策略必須由你明說**：`on_conflict` 每個類別都必填，沒有隱含預設。
3. **刪除前必先確認**：`apply` 會清空產物目錄，若偵測到未寫回的改動一定先問；非互動環境沒有 `--yes` 就取消，絕不硬做。
4. **產物目錄位置有防呆**：`build.out` 只能是專案內的專用目錄，指到家目錄、來源目錄、專案根都會被設定驗證直接擋下。
5. **不收符號連結**：來源裡的 symlink 一律不收（含 skill 目錄內部），避免透過連結把來源以外的檔案夾帶進產物。

<br>

---

## B. 安裝說明

依你的作業系統選一種方式，都是一行指令：

| 方式 | 平台 | 指令 | 事前需要 |
|------|------|------|----------|
| 方式一：Homebrew | macOS | `brew install ingsquared99/tap/agsy` | 已裝 Homebrew |
| 方式二：winget | Windows 10 / 11 | `winget install IngSquared99.agsy` | 不用，系統內建 |
| 方式三：Go 原始碼 | 全平台（Linux 請走這條） | `go install …`（見下方「從原始碼建置」） | 已裝 Go |

**安全性說明**：方式一、二安裝的是 GitHub Release 上的預編譯執行檔——由公開的 CI 流程從公開原始碼自動編譯，且 brew 的 cask 與 winget 的 manifest 都寫死了對應檔案的 SHA-256 校驗碼，下載內容可驗證、可稽核。方式三則是直接抓原始碼在你自己的電腦上編譯，完全不經過預編譯檔。agsy 本身**零第三方相依套件**（只用 Go 標準函式庫）。

<br>

### 方式一：Homebrew（macOS）

```sh
brew install ingsquared99/tap/agsy
```

- brew 會從 GitHub Release 下載對應你機器（Apple Silicon / Intel）的執行檔並校驗。
- 安裝過程已處理 macOS 的隔離屬性，第一次執行**不會**跳「無法驗證開發者」的警告。
- 還沒裝過 Homebrew？到官網 <https://brew.sh> 照首頁指示安裝（macOS 開發者的標準配備，裝一次終身受用）。

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

沒有其他框架或函式庫需求——不用 npm、不用 pip，`go build` 一行就是全部。

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

## C. 快速上手：四步完成第一次同步

整條路只有四步，走完一遍大約 10 分鐘：

```
 Step 0        Step 1         Step 2         Step 3
 準備來源  ──▶  init 設定  ──▶  plan 預覽  ──▶  apply 建置＋掛載  ──▶ 🎉 所有工具同步完成
 （擺檔案）     （問答一次）     （唯讀確認）     （正式執行）
```

> 以下的終端機畫面都是**示意**（依實際版本可能略有差異），幫助你預期每一步會看到什麼。

<br>

### 兩種操作方式

agsy 有雙入口：不帶參數是互動選單，帶參數直接執行。不想背指令，打 `agsy` 就對了：

```text
$ agsy
agsy v1.2.3

  狀態：落後 0 │ 新增 2 │ 本地改動 1 │ 未追蹤 0 │ 產物缺失 0 │ 掛載異常 0

要做什麼?
  apply    重建產物並掛載      ⚠ 1 項本地改動，需先確認
  plan     預覽變更，不寫入
  promote  回寫本地改動（1 項）
> status   檢視詳細狀態
  doctor   環境健檢
  init     設定（已有設定檔則進入編輯模式）
  clean    移除產物與掛載
  離開
```

第一次在專案裡執行、還沒有設定檔時，選單會直接引導你進入 `init`。

<br>

### 指令速查表

| 指令 | 做什麼 | 會不會寫入 |
|------|--------|-----------|
| `agsy init [sources...]` | 問答式產生 `agsy.yaml`（已存在則進入編輯模式） | 寫 `agsy.yaml` |
| `agsy doctor` | 環境健檢：設定、來源、掛載點、連結能力 | 唯讀 |
| `agsy plan` | 完整預覽 build + mount 的結果 | 唯讀 |
| `agsy apply` | 前置檢查 → 確認 → 清空重建產物 → 掛載 | 寫 `.agsy/` 與連結 |
| `agsy status` | 比對來源／產物／掛載三方，報告落差 | 唯讀（結尾有行動選單） |
| `agsy promote` | 把產物端的改動寫回來源 | 寫來源 |
| `agsy clean` | 移除連結與 `.agsy/`（反安裝，保留 `agsy.yaml`） | 刪除產物 |
| `agsy version` / `agsy help` | 版本 / 說明 | 唯讀 |

全域旗標：`--yes`（縮寫 `-y`）＝所有確認一律回答「是」，給 CI、腳本、git hook 等非互動環境用。**沒有 `--yes` 時，非互動環境遇到需要確認的動作會直接取消，絕不硬做。**

<br>

### Step 0：準備來源目錄

準備至少一個來源，子目錄**必須**叫 `rules/`、`skills/`、`workflows/`（複數，[命名規範](https://ingsquared99.github.io/agent-sync/#/zh-TW/overview)）。常見組合是「個人共用庫＋專案內庫」：

```sh
mkdir -p ~/all-ai-lib/rules ~/all-ai-lib/skills ~/all-ai-lib/workflows
mkdir -p ./repo-ai-lib/rules ./repo-ai-lib/skills ./repo-ai-lib/workflows
```

建好之後，把你的指令檔照類別放進去，長這樣就對了：

```
~/all-ai-lib/                     ./repo-ai-lib/
├── rules/                        ├── rules/
│   └── python-style.md           │   └── api-naming.md
├── skills/                       ├── skills/
│   └── code-review/              │   （還沒有也沒關係，留空即可）
│       └── SKILL.md              └── workflows/
└── workflows/                        └── deploy.md
    └── release.md
```

三個子目錄的格式，一行記住一個：

- `rules/` — 單一 `.md` 檔
- `skills/` — **目錄**＋必備 `SKILL.md`（內部不可含符號連結）
- `workflows/` — 單一 `.md` 檔，front-matter 可標 `target:` 分流

<br>

### Step 1：`agsy init` — 回答五個問題

在專案根目錄執行 `agsy init`，整個過程就是一次問答。畫面走起來像這樣：

```text
$ agsy init
開始設定 agsy（Enter 採用預設值）

來源路徑，依優先級排序（~ 開頭=共用庫、./ 開頭=專案內）
一行一個，直接按 Enter 結束輸入（例：~/all-ai-lib、./repo-ai-lib）
  來源 1: ~/all-ai-lib
  來源 2: ./repo-ai-lib
  來源 3: ⏎

要掛載哪些工具?
  [x] Claude Code（.claude/）
  [ ] OpenAI Codex（.codex/）
  [x] Antigravity（.agents/）

rules 同名時如何處理?（建議 rename：常有「全域基礎+專案補充」的並存需求）
> rename   兩份都保留，檔名加來源標記
  error    停止並列出衝突，由你手動處理（最保守）
  first    只留優先來源的那份，其餘丟棄

skills 同名時如何處理?（建議 error）…
workflows 同名時如何處理?（建議 rename）…

建置產物目錄 [.agsy] ⏎

workflow 沒標示 target 時放到哪?
> 全部 bucket（建議：檔案出現在所有地方，比神秘消失好理解）
  不放，並在 plan / apply 時警告

✔ 已寫入 agsy.yaml

.agsy/ 是可重建的產物，要加進 .gitignore 嗎? y
  下一步：agsy plan 預覽 → agsy apply 執行
```

五個問題的答法一張表看完：

| # | 問題 | 建議答案 | 為什麼 |
|---|------|----------|--------|
| 1 | 來源路徑？ | 共用庫在前、專案庫在後 | 順序＝優先權，越前面越優先 |
| 2 | 掛載哪些工具？ | 勾你實際在用的 | 之後隨時可重跑 init 增減 |
| 3 | 同名衝突怎麼辦？（三類各問一次） | rules=`rename`、skills=`error`、workflows=`rename` | rename 兩份共存、error 最保守、first 只留優先那份 |
| 4 | 產物目錄？ | 直接 Enter（`.agsy`） | 必須是專案內的專用目錄 |
| 5 | 沒標 target 的 workflow 去哪？ | 全部 bucket | 「到處出現」比「神祕消失」好除錯 |

收尾兩個小提醒：

- `.gitignore` 那題答 **y**——產物不進版控；`agsy.yaml` 本身則**要**進版控。
- 非互動環境（CI）用 `agsy init --yes ~/all-ai-lib ./repo-ai-lib`：來源用參數給，`--yes`＝接受建議預設策略。

<br>

### Step 2：`agsy plan` — 唯讀預覽，看三個地方

`plan` 把 apply 會做的每件事演練一遍，**保證不寫入任何檔案**：

```text
$ agsy plan
讀取 agsy.yaml ✔
來源（依優先級）:
  [1] ~/all-ai-lib    ✔   標記: @all-ai-lib
  [2] ./repo-ai-lib   ✔   標記: @repo-ai-lib

═══ build 預覽 ═══

rules → .agsy/rules/（策略: rename）
  git-commit.md                        ← [1] all-ai-lib
  python-style-fromlib-all-ai-lib.md   ← [1] all-ai-lib    ⚠ 同名，已加來源標記
  python-style-fromlib-repo-ai-lib.md  ← [2] repo-ai-lib   ⚠ 同名，已加來源標記

skills → .agsy/skills/（策略: error）
  code-review                          ← [1] all-ai-lib

workflows → .agsy/workflows/（策略: rename）
  release.md                           ← [1] all-ai-lib   → claude

═══ mount 預覽 ═══

.claude/
  commands   → workflows/claude   （不存在，將建立）
  rules      → rules              （不存在，將建立）
  skills     → skills             （不存在，將建立）

═══ 摘要 ═══
5 個項目 │ 2 個改名 │ 0 組衝突 │ 0 組撞名 │ 0 個丟棄(first)│ 0 個不收錄 │ 6 條連結 │ 0 個掛載異常 │ 0 個掛載衝突

未寫入任何檔案。確認無誤後執行 agsy apply。
```

這張畫面只需要看三個地方：

1. **每一行的 `← [n] 來源標記`**：確認每個檔案來自你預期的來源；`⚠` 表示同名被改名（兩份都在）。
2. **有沒有 `✘` 區塊**：同名衝突、撞名、路由錯誤——出現任何一種，apply 都會拒絕，先照清單修。
3. **摘要那一行**：衝突／撞名／掛載異常都是 0，就可以放心進下一步。

<br>

### Step 3：`agsy apply` — 正式建置與掛載

```text
$ agsy apply
✔ build 完成：5 個項目 → .agsy/
✔ mount 完成：6 條連結
```

兩行綠色就是完成了。此時打開 `.claude/`，`rules`、`skills`、`commands` 都已是指向 `.agsy/` 的連結，工具直接讀得到。

若中途停下，都是保護機制在動作：來源路徑缺失、掛載點被真實目錄佔用、或偵測到未寫回的本機改動（會列清單問你要不要捨棄）。照訊息處理完重跑即可，細節見[指令說明](https://ingsquared99.github.io/agent-sync/#/zh-TW/commands)。

<br>

### 日常循環：三句話

```
 平常改動：  改來源檔  ──▶  agsy apply                    （工具全部同步）
 產物端被改： agsy promote（寫回來源）──▶  agsy apply      （兩邊重新一致）
 不確定時：  agsy status                                  （它會告訴你該跑哪個）
```

**心智模型只有一條**：來源是唯一的真相來源（source of truth）。正常改動改來源＋`apply`；不小心（或刻意讓 AI）改到產物端，就用 `promote` 收回來。

<br>

### 在 CI / git hook 裡使用

`status` 的離開碼設計給自動化用：`0`＝完全同步、`1`＝有落差。

```sh
# pre-commit hook 範例：來源改了卻忘記 apply 就擋下 commit
agsy status || { echo "agsy 不同步，請先執行 agsy apply"; exit 1; }

# CI 裡重建（非互動，接受所有確認）
agsy apply --yes
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
| [情境全覽](https://ingsquared99.github.io/agent-sync/#/zh-TW/scenarios) | apply / promote 在每種情境下的行為 |
| [Q&A 常見問題](https://ingsquared99.github.io/agent-sync/#/zh-TW/faq) | 以使用者角度整理的常見問題 |
