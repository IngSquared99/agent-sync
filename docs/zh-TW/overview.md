# 核心概念：agsy 是什麼？

這一章不講操作，只講「它在解決什麼問題、用什麼方式解決」。看完這章再看安裝與快速上手，後面每一章的名詞都在這裡定義。

## 一、問題：同一份規範要放四個地方

現在的 AI 開發工具（Claude Code、OpenAI Codex、Google Antigravity、Cursor）都能讀「給 AI 看的指令檔」：編碼規範、技能、流程、守衛腳本。問題是每家讀的位置和格式都不一樣：

```
 同一份「編碼規範」

   Claude Code  ──▶ .claude/rules/       （一條規範一個檔）
   Codex        ──▶ AGENTS.md            （全部規範併成一個檔）
   Antigravity  ──▶ AGENTS.md
   Cursor       ──▶ AGENTS.md

 同一份「擋下 rm -rf 的守衛」

   Claude Code  ──▶ .claude/settings.json 裡的一小段
   Codex        ──▶ .codex/hooks.json
   Antigravity  ──▶ .agents/hooks.json
   Cursor       ──▶ .cursor/hooks.json     （而且格式都不同）
```

只要同時用兩家以上，每份指令就要維護好幾份副本，改一次要同步好幾次。

## 二、agsy 做的事：只維護一份，其他都是產出來的

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

## 三、名詞

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

## 四、四種類別

指令檔依用途分四種，來源資料夾裡各放一個子資料夾。

| 類別 | 是什麼 | 來源長什麼樣 | 一句話 |
|------|--------|-------------|--------|
| rules | 長期有效的規範、風格要求；AI 讀了照做 | 一個 `.md` 檔 | 教 |
| skills | 打包好的能力；AI 判斷任務符合時自己拿來用 | 一個含 `SKILL.md` 的資料夾 | 會 |
| workflows | 由人觸發的流程（輸入 `/名稱`） | 一個 `.md` 檔 | 做 |
| hooks | 在 AI 動作前後執行的小程式；不合規就擋下，AI 不能選擇不遵守 | 一個含 `hook.yaml` 和腳本的資料夾 | 擋 |

**rules 教、hooks 擋。** rules 是給 AI 讀的文字，遵不遵守有機率；hooks 是程式，在 AI 外面執行，擋下就是擋下。能寫成「如果…就不准」的規範用 hooks，風格偏好這類無法用程式判斷的用 rules。

## 五、每種類別產出什麼

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

## 六、每家工具最後讀到什麼

| 工具 | rules | skills | workflows | hooks |
|------|-------|--------|-----------|-------|
| Claude Code | `.claude/rules/` | `.claude/skills/` | `.claude/skills/` 裡的 skill，用 `/名稱` | `.claude/settings.json` 的 `hooks` 欄位（merge） |
| Codex | 根目錄 `AGENTS.md` | `.agents/skills/` | 同左 | `.codex/hooks.json` |
| Antigravity | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/workflows/` 的轉接頭 | `.agents/hooks.json` |
| Cursor | 根目錄 `AGENTS.md` | `.agents/skills/` | 同 Claude | `.cursor/hooks.json` |

`.agents/skills/` 是 Codex、Antigravity、Cursor 三家共同讀的資料夾，一條連結服務三家。Claude Code 不讀 `AGENTS.md`，所以它另外掛 `.claude/rules/`。

## 七、方向只有一個

```
 來源 ──▶ apply ──▶ 產物 ──▶ 連結 ──▶ 工具
```

來源是唯一的正本。產物和工具讀到的一切都是可重建的副本，沒有「從副本寫回正本」這回事。如果 AI 工具透過連結改了產物（例如替你加了一條規則），`agsy status` 會列出來；想保留就自己搬回來源，再 apply。這個手動步驟就是審核關卡：AI 產出的內容先經過人，才進入正本。

## 八、agsy 只動 repo 裡的東西

- 來源可以在任何地方（例如家目錄的共用庫 `~/all-ai-lib`），agsy 只讀它。
- 產物和連結都放在專案資料夾內。
- 各工具的個人層設定（`~/.claude`、`~/.codex` 之類）不是 agsy 的寫入目標，那裡放你自己的東西。

## 九、來源資料夾的命名

agsy 預設在每個來源裡找 `rules/`、`skills/`、`workflows/`、`hooks/` 四個子資料夾，缺哪個都沒關係。既有的庫用別的名字（例如 `prompts/`）時，設定檔的 `build.categories.<類別>.from` 可以直接指過去，不必搬檔案，見[設定檔](config.md)。

→ 下一章：[安裝](install.md)
