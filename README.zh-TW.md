<!-- 本檔由 scripts/genreadme 自動組裝，內容來源為 docs/zh-TW/ — 請勿手動編輯。 -->

# agsy（agent-sync）

[![Release](https://img.shields.io/github/v/release/IngSquared99/agent-sync)](https://github.com/IngSquared99/agent-sync/releases)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

> 多來源 AI 指令檔的合併與掛載工具：一份來源，同步 Claude Code、Codex、Antigravity 等多個 AI 工具。

**[📘 完整說明文件](https://ingsquared99.github.io/agent-sync/#/zh-TW/)** ｜ [English README](README.md)

---

## A. 核心概念：agsy 是什麼？

這一章不講操作，只講「它在解決什麼問題、用什麼方式解決」。看完這章再看安裝與快速上手，後面每一章的名詞都在這裡定義。

<br>

### 一、問題：同一份規範要放四個地方

現在的 AI 開發工具（Claude Code、OpenAI Codex、Google Antigravity、Cursor）都會讀「給 AI 看的指令檔」，最常見的就是編碼規範。問題是每家讀的位置和格式都不一樣：

```
 同一份「編碼規範」

   Claude Code  ──▶ .claude/rules/       （一條規範一個檔）
   Codex        ──▶ AGENTS.md            （全部規範併成一個檔）
   Antigravity  ──▶ AGENTS.md
   Cursor       ──▶ AGENTS.md
```

技能（skills）、流程（workflows）也一樣各有各的位置。只要同時用兩家以上，或者手上有好幾個專案，每份指令就要維護好幾份副本，改一次要同步好幾次。

<br>

### 二、agsy 做的事：只維護一份，其他都是產出來的

![一份正本，多個專案](docs/assets/one-source-many-projects.zh-TW.gif)

agsy 是一個命令列工具。你只維護**一份**指令檔（叫「來源」），跑一次 `agsy apply`，它做三件事：

```
 ①「來源」          ②「產物」                  ③「掛載」
 你維護的那一份 ──▶ 複製＋轉成各家格式 ──▶ 在各工具的讀取位置放一條「連結」
                    放進 .agsy/                指向產物

 例：
 ~/all-ai-lib/rules/python-style.md
        │
        ▼ apply
 .agsy/rules/python-style.md          ◀── .claude/rules（連結）
 .agsy/AGENTS.md（所有 rules 併成一檔） ◀── AGENTS.md（連結）
```

「連結」是作業系統的功能：一個看起來像資料夾或檔案的東西，實際上指向另一個位置。工具打開 `.claude/rules/` 時，看到的就是 `.agsy/rules/` 的內容，不會有第二份複本。

改了來源之後再跑一次 `agsy apply`，四家工具同時更新。

<br>

### 三、名詞

後面的章節會一直用到這些詞。

| 名詞 | 意思 | 比喻 |
|------|------|------|
| 來源（source） | 你維護的指令檔資料夾，可以有好幾個 | 原稿 |
| 產物（output） | `apply` 產生的資料夾，預設叫 `.agsy/`。裡面全部是產生出來的，隨時可以整個重建 | 印出來的副本 |
| 掛載（mount） | 在各工具的讀取位置放連結，指向產物 | 把副本擺到各人桌上 |
| 連結（link） | 作業系統層級的「指標」，指向另一個資料夾或檔案；內容只有一份 | 捷徑 |
| 類別（category） | 指令檔的四種類型：rules、skills、workflows、hooks | 見下一節 |
| 轉換產物（derived） | 不是原樣複製、而是轉換出來的產物：併成一檔的 `AGENTS.md`、workflow 變成的 skill、各家的 hook 登記表 | 翻譯本 |
| 登記表（registry） | 每家工具一份的 hooks 清單檔 `.agsy/hooks.<工具>.json` | 各家格式的名冊 |
| merge | Claude Code 專用：把 hooks 登記表合併進 `.claude/settings.json` 的 `hooks` 欄位，其他內容不動 | 只在別人的本子上加自己那幾行 |
| manifest | `.agsy/.agsy-manifest.json`，上次 apply 的紀錄；`status` 用它判斷哪邊改了什麼 | 上次印刷的存根 |
| 來源標記（tag） | 同名檔案兩邊都保留時，附在檔名上的來源名稱 | 出處章 |
| adapter | 各工具的內建掛載設定，`init` 用它產生設定檔 | 出廠範本 |
| tools | 設定檔裡列出要服務的工具名單 | 收件人名單 |

<br>

### 四、四種類別

指令檔依用途分四種，來源資料夾裡各放一個子資料夾。

| 類別 | 是什麼 | 來源長什麼樣 | 一句話 |
|------|--------|-------------|--------|
| rules | 長期有效的規範、風格要求；AI 讀了照做 | 一個 `.md` 檔 | 教 |
| skills | 打包好的能力；AI 判斷任務符合時自己拿來用 | 一個含 `SKILL.md` 的資料夾 | 會 |
| workflows | 由人觸發的流程（輸入 `/名稱`） | 一個 `.md` 檔 | 做 |
| hooks | 在 AI 動作前後執行的小程式；不合規就擋下，AI 不能選擇不遵守 | 一個含 `hook.yaml` 和腳本的資料夾 | 擋 |

前三種都是「給 AI 讀的文字」，第四種 hooks 不太一樣，下一節單獨說明。

<br>

### 五、rules 與 hooks 的差別

![rules 教、hooks 擋](docs/assets/rules-teach-hooks-block.zh-TW.gif)

**rules 是文字，hooks 是程式。**

rules 寫在 `.md` 檔裡，AI 讀了之後照做；但 AI 是機率性的，對話一長、跟任務目標衝突時，可能就不照做。hook 則是掛在 AI 工作流程固定時機上的一支小程式：例如「執行任何指令之前」（`PreToolUse`），AI 走到那裡會先暫停，把它打算做的事交給你的腳本檢查；腳本回傳結束碼 2 就擋下，AI 不能選擇不遵守。

```
 rules             AI 讀到「不可執行 rm -rf」  ──▶  通常照做，偶爾不照做
 hooks   AI 要執行指令前 ──▶ 你的腳本檢查 ──▶ exit 2 擋下 ／ exit 0 放行
```

怎麼分：能寫成「如果…就不准」的規範（禁止碰的檔案、必跑的檢查、不准停的條件）用 hooks；風格與偏好這類無法用程式判斷的用 rules。兩層並用。

在 agsy 裡，hook 也是來源的一種：一個資料夾放 `hook.yaml`（宣告在哪個時機、跑哪支腳本）和腳本本身。四家工具的 hook 機制相同，差別在登記的位置和格式，這一段由 agsy 翻譯：寫一次 `hook.yaml`，`apply` 之後四家的登記表都有它。

還沒用到 hooks 也沒關係：來源裡沒有 `hooks/` 資料夾時，這一層什麼都不會發生。

<br>

### 六、每種類別產出什麼

```
 來源                      產物（.agsy/）
 ─────────────────────     ───────────────────────────────────────
 rules/a.md          ──▶   rules/a.md              原樣
                     ──▶   AGENTS.md               所有 rules 併成一檔
 skills/x/SKILL.md   ──▶   skills/x/SKILL.md       原樣
 workflows/deploy.md ──▶   skills/deploy/SKILL.md  變成 skill（給 Claude / Codex / Cursor）
                     ──▶   workflows/deploy.md     轉接頭（給 Antigravity 的 /deploy）
 hooks/block-rm/     ──▶   hooks/block-rm/         原樣（腳本＋hook.yaml）
                     ──▶   hooks.claude.json       ┐
                     ──▶   hooks.codex.json        │ 四家各一份登記表
                     ──▶   hooks.antigravity.json  │
                     ──▶   hooks.cursor.json       ┘
```

workflow 為什麼變成 skill：Claude Code、Codex、Cursor 都是用 skills 機制讀流程，用檔頭的欄位決定誰能觸發。來源維持一個簡單的 `.md`，轉換由 agsy 做。Antigravity 是從 `workflows/` 資料夾用 `/名稱` 觸發，所以那裡留一個短短的「轉接頭」，告訴 AI 去載入對應的 skill。

<br>

### 七、每家工具最後讀到什麼

![同一份規範，四家工具](docs/assets/four-tools-one-apply.zh-TW.gif)

| 工具 | rules | skills | workflows | hooks |
|------|-------|--------|-----------|-------|
| Claude Code | `.claude/rules/` | `.claude/skills/` | `.claude/skills/` 裡的 skill，用 `/名稱` | `.claude/settings.json` 的 `hooks` 欄位（merge） |
| Codex | 根目錄 `AGENTS.md` | `.agents/skills/` | 同左 | `.codex/hooks.json` |
| Antigravity | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/workflows/` 的轉接頭 | `.agents/hooks.json` |
| Cursor | 根目錄 `AGENTS.md` | `.agents/skills/` | 同 Claude | `.cursor/hooks.json` |

`.agents/skills/` 是 Codex、Antigravity、Cursor 三家共同讀的資料夾，一條連結服務三家。Claude Code 不讀 `AGENTS.md`，所以它另外掛 `.claude/rules/`。

<br>

### 八、方向只有一個

```
 來源 ──▶ apply ──▶ 產物 ──▶ 連結 ──▶ 工具
```

來源是唯一的正本。產物和工具讀到的一切都是可重建的副本，沒有「從副本寫回正本」這回事。如果 AI 工具透過連結改了產物（例如替你加了一條規則），`agsy status` 會列出來；想保留就自己搬回來源，再 apply。這個手動步驟就是審核關卡：AI 產出的內容先經過人，才進入正本。

<br>

### 九、agsy 只動 repo 裡的東西

- 來源可以在任何地方（例如家目錄的共用庫 `~/all-ai-lib`），agsy 只讀它。
- 產物和連結都放在專案資料夾內。
- 各工具的個人層設定（`~/.claude`、`~/.codex` 之類）不是 agsy 的寫入目標，那裡放你自己的東西。

<br>

### 十、來源資料夾的命名

agsy 預設在每個來源裡找 `rules/`、`skills/`、`workflows/`、`hooks/` 四個子資料夾，缺哪個都沒關係。既有的庫用別的名字（例如 `prompts/`）時，設定檔的 `build.categories.<類別>.from` 可以直接指過去，不必搬檔案，見[設定檔](https://ingsquared99.github.io/agent-sync/#/zh-TW/config)。

<br>

---

## B. 安裝

依作業系統選一種方式。裝完用 `agsy version` 確認。

<br>

### 一、選安裝方式

| 你的系統 | 用這個 | 事前需要 |
|---------|--------|---------|
| macOS | 方式一：Homebrew | 已裝 Homebrew |
| Windows 10 / 11、Linux，或想自己編譯 | 方式二：Go 原始碼 | 已裝 Go 1.22 以上 |

方式一下載的是 GitHub Release 上的預編譯執行檔，由公開的 CI 從公開原始碼自動編譯，安裝設定裡寫死了檔案的 SHA-256 校驗碼。方式二是在你的電腦上從原始碼編譯；Windows 目前沒有 winget 套件，請走方式二。agsy 不依賴任何外部套件。

<br>

### 二、安裝

#### 方式一：Homebrew（macOS）

```sh
brew install ingsquared99/tap/agsy
```

會自動抓對應 Apple Silicon 或 Intel 的版本。第一次執行不會出現「無法驗證開發者」的警告。還沒有 Homebrew：依 <https://brew.sh> 安裝。

#### 方式二：從原始碼（全平台）

還沒有 Go：macOS `brew install go`、Windows `winget install GoLang.Go`、Linux 用發行版套件（例如 `apt install golang-go`）或 <https://go.dev/dl/>。

一行安裝：

```sh
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

執行檔會放在 `~/go/bin/`（Windows 是 `%USERPROFILE%\go\bin\`）。裝完終端機找不到 `agsy` 的話，是這個資料夾不在 PATH（終端機找指令的資料夾清單）裡：

```sh
# macOS（zsh）：加進設定檔後重開終端機；Linux（bash）改寫進 ~/.bashrc
echo 'export PATH="$HOME/go/bin:$PATH"' >> ~/.zshrc
```

Windows 的 Go 安裝程式通常已把這個資料夾加進 PATH；沒有的話，在「編輯系統環境變數」把 `%USERPROFILE%\go\bin` 加進使用者的 Path，然後**重開一個新的終端機視窗**。

想先看程式碼或自己修改：

```sh
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go test ./...                # 可選：先跑測試
go build -o agsy ./cmd/agsy  # 產出 agsy（Windows 是 agsy.exe）
mv agsy ~/go/bin/            # 放進任一在 PATH 裡的資料夾
```

<br>

### 三、確認

```sh
agsy version
# 例：agsy v1.2.3 (commit abc1234, built 2026-…, go1.22.x, darwin/arm64)
```

有印出版本就是裝好了。可以再跑一次唯讀的健檢：

```sh
agsy doctor
```

<br>

### 四、介面語言

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

<br>

### 五、升級與移除

| | Homebrew | Go |
|---|---|---|
| 升級 | `brew upgrade agsy` | 重跑 `go install …@latest` |
| 移除 | `brew uninstall agsy` | 刪 `~/go/bin/agsy` |

移除前，先在每個用過 agsy 的專案跑 `agsy clean`（移除連結與 `.agsy/`；`agsy.yaml` 會留下，不要的話自己刪）。

<br>

---

## C. 快速上手

四步完成第一次同步。全部在專案資料夾內操作。

```
 ① 準備來源  ──▶  ② agsy init  ──▶  ③ agsy plan  ──▶  ④ agsy apply
    放指令檔        產生設定檔         預覽（不寫入）      建置＋掛載
```

<br>

### 第 1 步：準備一個來源資料夾

來源是一個資料夾，裡面最多四個子資料夾，只放你需要的就好：

```
~/all-ai-lib/
├── rules/
│   └── python-style.md          # 一個 .md 檔
├── skills/
│   └── code-review/
│       └── SKILL.md             # 含 SKILL.md 的資料夾
├── workflows/
│   └── deploy.md                # 一個 .md 檔
└── hooks/
    └── block-rm/
        ├── hook.yaml            # 宣告：哪個時機、跑哪支腳本
        └── block-rm.sh          # 腳本（結束碼 2 = 擋下）
```

常見配置是兩個來源：個人共用庫（`~/all-ai-lib`）加專案內的庫（`./repo-ai-lib`）。

<br>

### 第 2 步：`agsy init` 產生設定檔

在專案資料夾執行，依提示回答。按 Enter 採用預設值。

```
$ cd your-project
$ agsy init

來源路徑,依優先序排列（~ 開頭=共用庫,./ 開頭=專案內）
  來源 1: ~/all-ai-lib
  來源 2: ./repo-ai-lib
  來源 3: ⏎

要服務哪些工具?（a = 全部）
    1) Antigravity (.agents/)
    2) Claude Code (.claude/)
    3) OpenAI Codex (.agents/, .codex/)
    4) Cursor (.agents/, .cursor/)
請輸入: a

rules 的同名衝突怎麼處理?        ❯ rename
skills 的同名衝突怎麼處理?       ❯ error
workflows 的同名衝突怎麼處理?    ❯ rename
hooks 的同名衝突怎麼處理?        ❯ error

建置產物目錄（預設: .agsy）: ⏎

✔ 已寫入 agsy.yaml

以下由 agsy 產生的路徑皆可重建,通常應加入 .gitignore:
要把哪些項目加進 .gitignore?（a = 全部）a
```

「同名衝突」是指兩個來源有同名的檔案時怎麼辦：`rename` 兩份都留（檔名加上來源名），`error` 停下來讓你處理，`first` 只留優先序高的那份。

最後一題是要不要把產物路徑加進 `.gitignore`，由你決定；`agsy.yaml` 本身建議進版控。

腳本或 CI 用的無互動寫法：`agsy init --yes ~/all-ai-lib ./repo-ai-lib`。

<br>

### 第 3 步：`agsy plan` 預覽

```
$ agsy plan
```

列出 apply 會做的每一件事：收哪些檔、誰被改名、每個 workflow 產出什麼、每個 hook 到達哪幾家、每條連結會怎樣。不寫入任何東西。有不對的地方，改完再跑一次。

<br>

### 第 4 步：`agsy apply` 建置並掛載

```
$ agsy apply
✔ build 完成:13 個項目 → .agsy/
✔ mount 完成:8 條連結
✔ merge 完成:.claude/settings.json ← hooks.claude.json
```

完成後專案長這樣（`→` 是連結，`⇐` 是 merge）：

```
your-project/
├── AGENTS.md          → .agsy/AGENTS.md         （全部 rules 併成一檔）
├── .claude/
│   ├── rules          → .agsy/rules
│   ├── skills         → .agsy/skills
│   └── settings.json  ⇐ .agsy/hooks.claude.json （只寫進 "hooks" 欄位）
├── .agents/
│   ├── skills         → .agsy/skills
│   ├── workflows      → .agsy/workflows
│   └── hooks.json     → .agsy/hooks.antigravity.json
├── .codex/
│   └── hooks.json     → .agsy/hooks.codex.json
├── .cursor/
│   └── hooks.json     → .agsy/hooks.cursor.json
└── .agsy/             產物
```

四家工具都從自己的位置讀到同一批內容。在 Claude Code 或 Cursor 輸入 `/deploy` 會執行那個 workflow；在 Antigravity 也是 `/deploy`。四家在執行 shell 指令前都會先跑 `block-rm.sh`，它回傳結束碼 2 的話指令就不會執行。

<br>

### 之後的日常

```
 改來源  ──▶  agsy apply  ──▶  四家都是最新的
                 ▲
 agsy status ────┘  看哪裡不同步（有落差時結束碼為 1）
```

AI 工具透過連結改了產物時（例如加了新規則），`agsy status` 會列出來並說明要搬去哪個來源。想保留就搬過去，再 apply。

<br>

### 指令速查

```
agsy            選單（附狀態摘要）
agsy doctor     環境健檢（唯讀）
agsy plan       預覽（唯讀）
agsy apply      建置＋掛載（會先列出要捨棄的東西並詢問）
agsy status     兩張落差清單＋連結狀態（唯讀）
agsy clean      從這個專案移除 agsy 建立的東西
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
