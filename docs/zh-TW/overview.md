# 核心概念：agsy 是什麼？

這一章不講操作，只講「它在解決什麼問題、用什麼方式解決」。看完這章再看安裝與快速上手，後面每一章的名詞都在這裡定義。

## 一、問題：同一份規範要放四個地方

現在的 AI 開發工具（Claude Code、OpenAI Codex、Google Antigravity、Cursor）都會讀「給 AI 看的指令檔」，最常見的就是編碼規範。問題是每家讀的位置和格式都不一樣：

```
 同一份「編碼規範」

   Claude Code  ──▶ .claude/rules/       （一條規範一個檔）
   Codex        ──▶ AGENTS.md            （全部規範併成一個檔）
   Antigravity  ──▶ AGENTS.md
   Cursor       ──▶ AGENTS.md
```

技能（skills）、流程（workflows）也一樣各有各的位置。只要同時用兩家以上，或者手上有好幾個專案，每份指令就要維護好幾份副本，改一次要同步好幾次。

## 二、agsy 做的事：只維護一份，其他都是產出來的

![一份正本，多個專案](../assets/one-source-many-projects.zh-TW.gif)

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

前三種都是「給 AI 讀的文字」，第四種 hooks 不太一樣，下一節單獨說明。

## 五、rules 與 hooks 的差別

![rules 教、hooks 擋](../assets/rules-teach-hooks-block.zh-TW.gif)

**rules 是文字，hooks 是程式。**

rules 寫在 `.md` 檔裡，AI 讀了之後照做；但 AI 是機率性的，對話一長、跟任務目標衝突時，可能就不照做。hook 則是掛在 AI 工作流程固定時機上的一支小程式：例如「執行任何指令之前」（`PreToolUse`），AI 走到那裡會先暫停，把它打算做的事交給你的腳本檢查；腳本回傳結束碼 2 就擋下，AI 不能選擇不遵守。

```
 rules             AI 讀到「不可執行 rm -rf」  ──▶  通常照做，偶爾不照做
 hooks   AI 要執行指令前 ──▶ 你的腳本檢查 ──▶ exit 2 擋下 ／ exit 0 放行
```

怎麼分：能寫成「如果…就不准」的規範（禁止碰的檔案、必跑的檢查、不准停的條件）用 hooks；風格與偏好這類無法用程式判斷的用 rules。兩層並用。

在 agsy 裡，hook 也是來源的一種：一個資料夾放 `hook.yaml`（宣告在哪個時機、跑哪支腳本）和腳本本身。四家工具的 hook 機制相同，差別在登記的位置和格式，這一段由 agsy 翻譯：寫一次 `hook.yaml`，`apply` 之後四家的登記表都有它。

還沒用到 hooks 也沒關係：來源裡沒有 `hooks/` 資料夾時，這一層什麼都不會發生。

## 六、每種類別產出什麼

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

## 七、每家工具最後讀到什麼

![同一份規範，四家工具](../assets/four-tools-one-apply.zh-TW.gif)

| 工具 | rules | skills | workflows | hooks |
|------|-------|--------|-----------|-------|
| Claude Code | `.claude/rules/` | `.claude/skills/` | `.claude/skills/` 裡的 skill，用 `/名稱` | `.claude/settings.json` 的 `hooks` 欄位（merge） |
| Codex | 根目錄 `AGENTS.md` | `.agents/skills/` | 同左 | `.codex/hooks.json` |
| Antigravity | 根目錄 `AGENTS.md` | `.agents/skills/` | `.agents/workflows/` 的轉接頭 | `.agents/hooks.json` |
| Cursor | 根目錄 `AGENTS.md` | `.agents/skills/` | 同 Claude | `.cursor/hooks.json` |

`.agents/skills/` 是 Codex、Antigravity、Cursor 三家共同讀的資料夾，一條連結服務三家。Claude Code 不讀 `AGENTS.md`，所以它另外掛 `.claude/rules/`。

## 八、方向只有一個

```
 來源 ──▶ apply ──▶ 產物 ──▶ 連結 ──▶ 工具
```

來源是唯一的正本。產物和工具讀到的一切都是可重建的副本，沒有「從副本寫回正本」這回事。如果 AI 工具透過連結改了產物（例如替你加了一條規則），`agsy status` 會列出來；想保留就自己搬回來源，再 apply。這個手動步驟就是審核關卡：AI 產出的內容先經過人，才進入正本。

## 九、agsy 只動 repo 裡的東西

- 來源可以在任何地方（例如家目錄的共用庫 `~/all-ai-lib`），agsy 只讀它。
- 產物和連結都放在專案資料夾內。
- 各工具的個人層設定（`~/.claude`、`~/.codex` 之類）不是 agsy 的寫入目標，那裡放你自己的東西。

## 十、來源資料夾的命名

agsy 預設在每個來源裡找 `rules/`、`skills/`、`workflows/`、`hooks/` 四個子資料夾，缺哪個都沒關係。既有的庫用別的名字（例如 `prompts/`）時，設定檔的 `build.categories.<類別>.from` 可以直接指過去，不必搬檔案，見[設定檔](config.md)。

→ 下一章：[安裝](install.md)
