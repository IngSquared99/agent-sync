# 核心概念：agsy 是什麼？

## 它解決的問題

當你同時使用多個 AI 開發工具（Claude Code、OpenAI Codex、Google Antigravity、Cursor…），每個工具讀取的位置與格式都不一樣：

- Claude Code 從 `.claude/rules/` 讀 rules、從 `.claude/skills/` 讀 skills
- Codex 讀專案根目錄的 `AGENTS.md`，並從 `.agents/skills/` 讀 skills
- Antigravity 讀根目錄 `AGENTS.md`、從 `.agents/skills/` 讀 skills、從 `.agents/workflows/` 讀 `/名稱` workflows
- Cursor 讀根目錄 `AGENTS.md`，並從 `.agents/skills/` 讀 skills
- 四家的 hooks（agent 生命週期掛勾）又各有登記位置與格式：`.claude/settings.json`、`.codex/hooks.json`、`.agents/hooks.json`、`.cursor/hooks.json`

同一套編碼規範、技能、流程與守衛被複製成好幾份、好幾種格式，每次修改都要同步到每一份。此外你通常還想要「個人共用庫＋專案專屬庫」的分層。

**agsy（agent-sync）** 正是為此而生：

> 它把多個來源的指令檔**合併**進單一建置輸出目錄（預設 `.agsy/`），**轉換**成各工具的原生格式，再以連結**掛載**到各工具的讀取位置。

你只編輯來源，執行一次 `agsy apply`，所有工具同時更新。

## 先認識這些名詞

之後的文件會一直用到這些詞。

| 名詞 | 意思 |
|------|------|
| 來源（source） | 你維護的原始指令庫（`sources` 陣列），可以有多個 |
| 產物（output / artifacts） | `apply` 建置出來的目錄，預設 `.agsy/`。裡面全部是產生出來的，整個目錄可重建 |
| 掛載（mount） | 在各工具的讀取位置建立指向產物的「連結」 |
| 連結（symlink / junction / hard link） | 作業系統層級的指標，指向另一個目錄或檔案——**內容不會有第二份** |
| 類別（category） | 四種指令檔：rules、skills、workflows、hooks |
| 轉換產物（derived form） | 由轉換而非原樣複製產生的輸出：串接的 `AGENTS.md`、workflow 的 skill 形態、workflow 的轉接頭、各工具的 hook 登記表 |
| 登記表（hook registry） | `.agsy/hooks.<tool>.json`，每家工具一份，由所有 hook 的 `hook.yaml` 翻譯而成 |
| merge | Claude Code 專用的掛載方式：把登記表合併進 `.claude/settings.json` 的 `hooks` 鍵，其他內容不動 |
| manifest | `.agsy/.agsy-manifest.json`，建置紀錄；agsy 用它判斷哪一端變了什麼 |
| 來源標記（source tag） | 同名項目以 rename 保留時附加在檔名上的來源識別，如 `-fromlib-all-ai-lib` |
| adapter | 各工具的內建掛載預設，`init` 用它產生掛載設定 |
| tools | `build.tools` 清單；workflow 與 hook 的 `target:` 只能引用這裡列出的名稱 |

## 單向資料流

agsy 的資料流嚴格單向：

```
 你維護的來源 ──▶ build（複製＋轉換）──▶ .agsy/ ──▶ mount（連結／merge）──▶ 各工具
```

**來源是唯一真相。`.agsy/` 裡的一切——也就是工具透過掛載讀到的一切——都是唯讀、可重建的產物。** 沒有回寫機制：掛載中的檔案被修改時，`status` 會回報、`apply` 會列出並詢問確認後重建覆蓋。保留改動的方式是手動搬進來源（status 會指出目的地），因此 AI 產出的內容在進入庫之前必經人工審視。

## 三層架構

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
│                      │  .agsy/hooks/        hooks 原樣（腳本＋hook.yaml）
│                      │  .agsy/hooks.*.json  各工具的 hook 登記表（轉換產物）
└─────────┬───────────┘
          │  ② agsy apply：建立連結（Claude 的 hooks 改為 merge）
          ▼
┌─────────────────────┐
│  掛載                │  AGENTS.md          → .agsy/AGENTS.md
│ （工具實際讀取處）    │  .claude/rules      → .agsy/rules
│                      │  .claude/skills     → .agsy/skills
│                      │  .claude/settings.json ⇐ .agsy/hooks.claude.json（merge）
│                      │  .agents/skills     → .agsy/skills
│                      │  .agents/workflows  → .agsy/workflows
│                      │  .agents/hooks.json → .agsy/hooks.antigravity.json
│                      │  .codex/hooks.json  → .agsy/hooks.codex.json
│                      │  .cursor/hooks.json → .agsy/hooks.cursor.json
└─────────────────────┘
```

- **來源**：你維護並進版控的原稿。順序＝優先序（前者優先）。
- **產物**（`build.out`，預設 `.agsy/`）：建置成品。`apply` 每次整個清空重建。
- **掛載**：各工具讀取位置的連結。目錄用 symlink（Windows 用 junction）；根目錄 `AGENTS.md` 與三份 hook 登記表用檔案 symlink（Windows 用 hard link）。工具看到的是連結；內容都在 `.agsy/`。唯一的例外是 Claude Code 的 hooks：它沒有獨立檔，agsy 改用 **merge** 把登記表合併進 `.claude/settings.json` 的 `hooks` 鍵，只擁有自己寫進去的那幾筆條目。

## 四種類別

指令檔依用途分四類。來源以四個子目錄存放（預設 `rules/`、`skills/`、`workflows/`、`hooks/`），各有格式規則：

| 類別 | 來源格式 | 是什麼 | 輸出形態 |
|------|---------|--------|---------|
| rules | 單一 `.md` 檔 | 長期有效的規範與風格指南，常駐於 context | `rules/` 逐檔原樣（給 Claude Code）**加上**一份串接的 `AGENTS.md`（給 Codex / Cursor / Antigravity） |
| skills | 內含 `SKILL.md` 的**目錄** | 打包好的能力；工具在任務符合描述時自動取用 | `skills/` 原樣複製 |
| workflows | 單一 `.md` 檔 | 由人觸發的流程與 SOP，以 `/名稱` 執行 | `skills/` 裡的 **skill 形態**（front matter 補上 `disable-model-invocation: true`，支援此欄位的工具絕不自行執行）**加上** `workflows/` 裡給 Antigravity `/名稱` 用的**轉接頭** |
| hooks | 內含 `hook.yaml` 的**目錄**（腳本放同目錄） | 掛在 agent 生命週期上的程式：工具執行前擋下、停止前擋回去、事後檢查——模型不能選擇不遵守 | `hooks/` 原樣複製**加上**每家工具一份**登記表** `hooks.<tool>.json`（事件名、結構依各家翻譯；腳本路徑改寫成指向產物的絕對路徑） |

### rules 教、hooks 守

rules 是塞進模型 context 的文字，模型「讀到了」，但遵不遵守是機率性的。hooks 是在 agent 外面執行的程式：agent 走到掛勾點（例如 `PreToolUse`）會暫停、把現況以 JSON 交給你的腳本，腳本以 exit code 2 擋下就是擋下。凡是能寫成 if 的規範（禁止碰的檔案、必跑的檢查、不准停的條件）用 hooks；風格與偏好這類無法用程式判斷的仍用 rules。兩層並用，規範放哪一層由你決定。

為什麼 workflow 要變成 skill：Claude Code、Codex、Cursor 都是透過 skills 機制讀取指令與流程，以 front matter（而非另一個目錄）決定誰能觸發。workflow 的來源維持單一檔案，由 build 打包成這些工具期望的 skill 形態。Antigravity 從 `workflows/` 目錄以 `/名稱` 觸發，build 在該處留一個簡短的轉接頭，指示 agent 載入對應的 skill（內容只存在一份，即 skill 形態）。

## 各工具最終讀到什麼

| 工具 | rules | skills | workflows | hooks |
|------|-------|--------|-----------|-------|
| Claude Code | `.claude/rules/`（逐檔） | `.claude/skills/` | `.claude/skills/` 裡的 skill 形態，以 `/名稱` 觸發 | `.claude/settings.json` 的 `hooks` 鍵（merge） |
| Codex | 根目錄 `AGENTS.md` | `.agents/skills/` | 同左（skill 形態） | `.codex/hooks.json` |
| Antigravity | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/workflows/` 的轉接頭 → `/名稱` 載入 skill | `.agents/hooks.json` |
| Cursor | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/skills/` 裡的 skill 形態，以 `/名稱` 觸發 | `.cursor/hooks.json` |

`.agents/skills/` 是 Codex、Antigravity、Cursor 三家原生共讀的目錄——一條連結服務三個工具。Claude Code 不讀 `AGENTS.md`，所以只有它另外掛逐檔的 `.claude/rules/`。

## 來源子目錄需遵循命名慣例

agsy 預設掃描來源裡的 `rules/`、`skills/`、`workflows/`、`hooks/`。既有的庫若一直使用別的名稱，可透過 `build.categories.<類別>.from` 直接接上、不必搬檔案——見[設定檔](config.md)。

→ 下一章：[安裝](install.md)
