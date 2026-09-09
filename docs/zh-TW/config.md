# 設定檔 agsy.yaml

`agsy.yaml` 是 agsy 唯一的設定檔，放在專案根目錄，通常由 `agsy init` 產生。這一章逐欄位說明，讓你能放心手動編輯。看不懂某個名詞時回[核心概念](overview.md)。

## 完整範例

```yaml
# 路徑寫法：~ 開頭 = 家目錄；相對路徑 = 以本檔所在資料夾為基準；絕對路徑 = 原樣使用
version: 2

sources:                      # 來源，有順序：前面的優先
  - ~/all-ai-lib
  - ./repo-ai-lib

build:
  out: .agsy                  # 產物資料夾，必須在專案內（apply 會整個清空它）

  categories:                 # 來源子資料夾 → 產物子資料夾
    rules:     { from: rules, to: rules }
    skills:    { from: skills, to: skills }
    workflows: { from: workflows, to: workflows }
    hooks:     { from: hooks, to: hooks }

  on_conflict:                # 同名檔案怎麼辦：first / rename / error（四個都要填）
    rules:     rename
    skills:    error
    workflows: rename
    hooks:     error

  tools: [claude, codex, antigravity, cursor]   # 要服務的工具

mount:
  - dir: .                    # 專案根：AGENTS.md 給 Codex / Cursor / Antigravity 讀
    links:
      AGENTS.md: AGENTS.md
  - dir: .claude              # Claude Code
    links:
      rules:  rules
      skills: skills
    merge:                    # hooks 合併進 settings.json 的 "hooks" 欄位
      settings.json: hooks.claude.json
  - dir: .agents              # Codex + Antigravity + Cursor 共用
    links:
      skills:     skills
      workflows:  workflows
      hooks.json: hooks.antigravity.json
  - dir: .codex
    links:
      hooks.json: hooks.codex.json
  - dir: .cursor
    links:
      hooks.json: hooks.cursor.json
```

## 路徑怎麼寫

| 寫法 | 意思 | 適合 |
|------|------|------|
| `~/xxx` | 家目錄底下 | 跨專案共用的來源庫 |
| `./xxx` 或 `xxx` | 以 **agsy.yaml 所在資料夾**為基準（不是你目前所在的資料夾） | 專案內的來源庫 |
| `/abs/path` | 原樣使用 | 特殊配置 |

`~` 只代表你自己的家目錄；`~someone` 這種寫法會直接報錯。

因為相對路徑以設定檔位置為基準，從專案的子資料夾執行 agsy 也一樣：除了 `init`，每個指令都會往上層找 `agsy.yaml`。

## 逐欄位說明

### `version`

設定檔格式的版本，目前是 `2`。省略等於目前版本。數字大於執行中的 agsy 所支援的上限時，會要求升級 agsy。

### `sources`（必填，至少一個）

來源資料夾的清單。

- 順序就是優先順序：`on_conflict: first` 時前面來源的同名檔案保留；`AGENTS.md` 併檔時也依這個順序。
- 常見組合：`[~/共用庫, ./專案庫]`。
- 來源缺某個子資料夾（例如沒有 `workflows/`）是正常的。
- 整個來源路徑不存在時，`plan` 仍可預覽，但 `apply` 拒絕執行：缺了一個來源就重建，等於把那個來源的東西全部刪掉。
- 同一路徑不可列兩次，一個來源也不可位於另一個來源之內。

每個來源會得到一個自動的**來源標記**：路徑最後一段去掉開頭的點（`~/all-ai-lib` → `all-ai-lib`）。兩個來源標記相同時，往上一層併入資料夾名，仍相同才加數字。

### `build.out`（產物資料夾）

預設 `.agsy`。`apply` 每次會清空它、`clean` 會整個刪掉它，所以只接受**專案內的專用子資料夾**。以下都會被拒絕：專案根本身或它的上層、專案外的位置、家目錄、任何來源的所在位置或內部。

要改 `out` 的順序：先在**舊**設定下跑 `agsy clean`，再改 `agsy.yaml`，再 `agsy apply`。

### `build.categories`

- `from`：每個來源裡要掃描的子資料夾名。
- `to`：產物裡的子資料夾名。
- 只填一邊時另一邊用預設值。
- 四個 `from` 不可重複，四個 `to` 也不可重複；`to` 不可叫 `AGENTS.md` 或 `hooks.<工具>.json`（保留給轉換產物）。
- 既有的庫子資料夾叫別的名字（例如 `prompts/`），用 `from: prompts` 接上即可，不必搬檔案。

哪些檔案會被收進來：

| 規則 | 說明 |
|------|------|
| 名稱以 `.` 開頭 | 一律略過，也不回報（`.DS_Store` 這類） |
| 符號連結 | 一律不收。連結可能把來源以外的檔案（例如私鑰）帶進產物 |
| 非一般檔案（FIFO、socket、裝置檔） | 一律不收。skill 或 hook 資料夾內含這類檔案時整個資料夾不收 |
| rules / workflows | 只收單一 `.md` 檔；超過 5 MB 略過 |
| skills | 只收資料夾，且必須有 `SKILL.md`；內含任何符號連結整個不收 |
| hooks | 只收資料夾，且必須有 `hook.yaml`；規則同 skills。資料夾名就是 hook 名，限小寫 a–z、0–9、連字號，不符時自動轉換並在 plan 註明 |

不符規則的檔案會在 `plan` 與 `doctor` 列出原因。

### `build.on_conflict`（四個類別都要填）

兩個來源出現同名檔案時怎麼辦。沒有預設值，必須明確選。

| 策略 | 行為 | 適合 |
|------|------|------|
| `rename` | 兩份都留，檔名加上來源標記：`python-style.md` → `python-style-fromlib-all-ai-lib.md` | rules |
| `error` | 停下並列出衝突，由人處理 | skills、hooks |
| `first` | 只留優先序最高的一份，其餘捨棄（plan 會列出被捨棄的） | 確定「專案蓋過共用庫」時 |

skills 建議 `error`：兩個同名 skill 共存時，工具挑哪個無法預測，通常代表該合併成一個。hooks 同理。

rename 之後仍可能兩個項目的**最終輸出路徑**相同，跨類別也算：叫 `deploy.md` 的 workflow 會產出 `skills/deploy`，與叫 `deploy` 的 skill 相撞。這種情況一律擋下建置。

### `build.tools`（必填）

要服務的工具名單。`init` 依你勾選的工具填入。workflow 和 hook 的 `target:` 只能寫這裡有的名字，寫錯會擋下建置。

有連結指向 workflows 產物時，這裡必須有 `antigravity`（只有它讀那個資料夾）；反過來列了 `antigravity` 就必須在某處掛 workflows。

### workflow 的 `target:`

在 workflow 的 `.md` 檔頭（front matter）：

```yaml
---
target: [claude, codex]   # 或單一個：target: claude
---
```

- 沒寫：給 `build.tools` 裡的所有工具。
- 有寫：只給列出的工具。列出的工具中有非 `antigravity` 的就產生 skill；列出 `antigravity` 就產生 `workflows/` 裡的轉接頭。
- skills 資料夾是多家共讀的，所以排除是粗粒度的：只列部分工具時，凡是掛了 skills 的工具仍看得到，plan 會註明。
- `target:` 是 agsy 的欄位，產物裡會被移除。

## hooks 的 `hook.yaml`

每個 hook 資料夾必須有一份 `hook.yaml`。

### 範例

```yaml
description: 擋下 rm -rf                     # 選填；plan 顯示用
target: [claude, codex, antigravity, cursor]  # 選填；同 workflow：省略 = build.tools 全部
events:                                       # 必填；至少一個事件
  PreToolUse:                                 # 事件名（見下表）
    - matcher: Bash                           # 選填；哪個工具觸發時才跑
      hooks:                                  # 必填；至少一個 handler
        - type: command                       # 選填；預設 command
          command: ./block-rm.sh              # 相對於本 hook 資料夾
          timeout: 10                         # 選填；其他欄位原樣帶過去
      overrides:                              # 選填；某一家要不一樣時
        antigravity: { matcher: run_command }
        cursor:      { matcher: Shell }
```

一個 hook 的結構：

```
 events
 └── 事件（什麼時機）
     └── 群組（可加 matcher：哪個工具觸發時）
         ├── hooks：一到多個 handler（跑什麼）
         └── overrides：某一家的例外
```

### 事件名

事件名用 Claude Code / Codex 的命名，其他兩家由 agsy 翻譯。「—」代表該家沒有這個時機，翻譯時略過並在 plan 註明。

這張表收的是四家共有的部分，不是每家的全部：Claude Code 自己還有很多事件（`PermissionDenied`、`PostToolBatch`、`Elicitation`……），`hook.yaml` 寫表外的名字會被拒絕。只有一家能跑的 hook 放在那家自己的設定檔；若某個事件有好幾家都支援卻不在表上，請開 issue。

| 事件 | claude | codex | antigravity | cursor |
|---|---|---|---|---|
| `PreToolUse` | ✓ | ✓ | ✓ | `preToolUse` |
| `PostToolUse` | ✓ | ✓ | ✓ | `postToolUse` |
| `Stop` | ✓ | ✓ | ✓ | `stop` |
| `SessionStart` / `SessionEnd` | ✓ | ✓ | — | `sessionStart` / `sessionEnd` |
| `UserPromptSubmit` | ✓ | ✓ | — | `beforeSubmitPrompt` |
| `PermissionRequest` | ✓ | ✓ | — | — |
| `SubagentStart` / `SubagentStop` | ✓ | ✓ | — | `subagentStart` / `subagentStop` |
| `PreCompact` | ✓ | ✓ | — | `preCompact` |
| `PostCompact` | ✓ | ✓ | — | — |
| `Interrupt` | — | ✓ | — | — |
| `PostToolUseFailure` | ✓ | — | — | `postToolUseFailure` |
| `StopFailure` | ✓ | — | — | — |
| `PreInvocation` / `PostInvocation` | — | — | ✓ | — |

### handler 型別

| 工具 | 支援的 `type` |
|------|--------------|
| claude | command、http、mcp_tool、prompt、agent |
| codex | command、mcp_tool |
| antigravity | command |
| cursor | command、prompt |

不支援的 handler 在該家略過並回報。

### 翻譯規則

1. `target` 沒列的工具不寫入該家登記表，plan 註明少了哪幾家。
2. 該家沒有的事件、不支援的型別略過並註明。Antigravity 只在 `PreToolUse` / `PostToolUse` 收 matcher，其他事件的 matcher 丟棄並註明。某家一個事件都對不上時，登記表仍會產生（內容為空）。
3. `overrides.<工具>`：`matcher` 覆蓋群組的 matcher；`hooks[i]` 依序覆蓋第 i 個 handler 的欄位。索引超出群組的 handler 數、或覆蓋後 command 型 handler 沒有 `command`，都是錯誤。override 把 `type` 改成 command 以外的型別時，該家丟掉 `command` 欄位並註明。`overrides` 的工具名只要是 agsy 認識的四家即可，不在 `build.tools` 的會忽略；完全未知的名字才是錯誤。
4. 其他 handler 欄位原樣帶過去。Cursor 的結構不同，只保留 `timeout`、`failClosed`、`loop_limit`、`prompt`，其餘丟棄並註明。

### 路徑改寫

`command` 裡每個以 `./` 開頭、以空白分隔的字串，會改寫成指向 `.agsy/hooks/<名稱>/…` 的絕對路徑；其他部分原樣保留（`python3 ./check.py` 可以）。

```
 hook.yaml     command: python3 ./check.py
                                 │
                                 ▼ apply
 登記表        command: python3 /Users/me/proj/.agsy/hooks/block-rm/check.py
```

- 改寫後的路徑含空白或特殊字元時自動加引號。來源裡**不要自己加引號**（`"./x.sh"` 會被拒絕），也不要把 `./` 路徑放進引號字串裡（`sh -c 'echo ./x.sh'` 同樣被拒絕：改寫會弄壞引號）。
- 引用的檔案必須存在於 hook 資料夾內，否則 apply 拒絕；override 裡的 command 也一樣。
- 登記表裡的是這台機器的絕對路徑，每台機器各自 apply。

### 腳本要自己處理各家差異

agsy 只翻譯登記表，不翻譯腳本。四家餵給腳本的 JSON 欄位不同（Claude / Codex 是 `tool_input.command`，Cursor、Antigravity 各有一套）。跨四家的腳本要自己看 `hook_event_name` 或各家欄位。

## `mount`（必填，至少一個）

每個條目：`dir` 是連結要建在哪個資料夾（`.` 是專案根），`links` 是「連結名: 產物最上層的名稱」。

- 連結可以指向各類別的 `to` 值、`AGENTS.md`，或四份登記表 `hooks.<工具>.json`（該工具必須在 `build.tools`）。只能指向產物的最上層。
- 同一個 `dir` 的多個條目會合併；同名連結指向不同目標才是錯誤。
- `dir` 不可在產物內（apply 清空時連結會一起消失）、不可在來源內（連結會被當成來源掃進去）；連結名不可含路徑分隔符。
- `dir` 在專案資料夾之外時，該條目必須加 `outside_project: true`。這是安全閘：clone 下來的 repo 裡的 `agsy.yaml` 不能在你不知情的情況下改到家目錄的設定。只有你親自寫的設定才該加。
- 連結的實作：macOS / Linux 用相對路徑的 symlink。Windows 資料夾用 junction、檔案用 hard link，都不需要管理員權限；junction 記的是絕對路徑，搬移專案後重跑 `agsy apply`。

## `merge`：Claude Code 專用

```yaml
  - dir: .claude
    merge:
      settings.json: hooks.claude.json
```

### 為什麼不用連結

Claude Code 的 hooks 寫在 `.claude/settings.json` 的 `hooks` 欄位，那份檔案還裝著 permissions、model 等你自己的設定。整檔用連結會蓋掉它們，所以 agsy 只把自己的幾筆條目合併進去。

```
 .claude/settings.json（你的檔案）
 {
   "permissions": {…},        ← 不動
   "hooks": {
     "Stop": [ 你自己的 ],     ← 不動
     "PreToolUse": [ agsy 的 ] ← 只換這裡
   }
 }
```

### 怎麼分辨哪幾筆是 agsy 的

`hooks` 底下的一個群組，只要有任一 handler 符合其中一項，就算 agsy 的：

- `statusMessage` 以 `agsy:` 開頭。agsy 寫出的每一筆都帶 `agsy:<hook 名>`；`hook.yaml` 自己寫的 `statusMessage` 接在後面（`agsy:block-rm · linting`）。
- `command` 的路徑指向 `.agsy/hooks/`（目前的產物資料夾，或 manifest 記錄的那個；記錄的資料夾只在專案內才算數）。

記號與路徑無關，所以專案搬移、`build.out` 改名、command 不含 `./` 的群組都能認出來。

### 各指令對 merge 目標做什麼

| 指令 | 做什麼 |
|------|-------|
| apply | 移除舊的 agsy 群組、放入登記表的群組；其他欄位與群組原樣保留、順序不變（整檔以兩格空白重新縮排）。檔案不存在時建立並記錄「由 agsy 建立」。登記表沒東西且檔案裡沒有 agsy 群組時，檔案不建立也不改寫 |
| status | agsy 群組被改 → 列入產物端改動；被刪 → 列入缺失。你自己的群組不算異常。設定裡已移除但檔案裡還有 agsy 群組 → 回報為孤兒 |
| clean | 移除 agsy 群組；apply 帶進來的 `hooks` 欄位與事件陣列一起移除，原本就有的空容器保留；檔案是 agsy 建的且變空才刪檔 |

目標檔是符號連結、或不是 JSON 物件時，apply 在建置前就停下，clean 跳過並回報。

### 限制

- `merge` 的值只能是 `hooks.claude.json` 或 `hooks.codex.json`（merge 只讀得回這兩家的格式），且該工具在 `build.tools`。
- 同一個 `dir` 內 `merge` 與 `links` 不可同名；同一份登記表不可同時被連結與合併。
- 帶 merge 的 `dir` 必須在專案內，`outside_project: true` 對 merge 無效。agsy 只擁有專案層的檔案。
- 只有 `merge`、沒有 `links` 的條目是合法的。

### 你自己的 hooks 放哪

各家的個人層檔案，四家都是與專案層疊加：Claude Code 放 `.claude/settings.local.json` 或 `~/.claude/settings.json`；Codex 放 `~/.codex/hooks.json` 或 `config.toml`；Cursor 放 `~/.cursor/hooks.json`；Antigravity 放 `~/.gemini/config/`（官方未載明合併方式，請實測）。

### 反向檢查只是提示

掛了 `hooks.<工具>.json` 但該工具不在 `build.tools` 是錯誤（檔案永遠是空的）。反過來，`build.tools` 有列但沒掛它的登記表不是錯誤，只在來源真的有 hook 時由 `plan` / `doctor` 提示。

## 錯誤速查

驗證失敗時所有問題會一次列出。常見訊息：

| 訊息 | 處理 |
|------|------|
| `sources 未設定,至少需要一個來源路徑` | 至少加一個來源 |
| `build.on_conflict.rules 未設定…` | 四個類別都要填策略 |
| `build.on_conflict.hooks 未設定…` | 在 `on_conflict` 補一行 `hooks: error`，或重跑 `agsy init` |
| `build.categories.x.to 不可為 "hooks.codex.json"…` | 該名稱保留給登記表，換一個 |
| `mount … merge.settings.json 指向 "z",但只有 claude / codex 形狀的 hook 登記表可以合併` | merge 只能指向 `hooks.claude.json` 或 `hooks.codex.json` |
| `mount dir … 解析後位於專案目錄之外,卻帶有 merge 條目` | merge 目標必須在專案內 |
| `mount … 的 merge.x 與連結 y 都用到 "z"` | 同一份登記表只能連結或合併擇一 |
| `mount … links.hooks.json 指向 "hooks.cursor.json",但 build.tools 未列出 "cursor"` | 把 `cursor` 加進 `build.tools`，或移除該連結 |
| `build.tools 未設定…` | 列出工具，例如 `[claude, codex, antigravity, cursor]` |
| `build.out(…)不在專案目錄(…)底下` | 改回專案內的專用資料夾 |
| `build.categories.x.to 與 y 同為 "…"` | 給其中一個不同的 `to` |
| `mount … links.x 指向 "z",但產物裡沒有這一層` | 目標必須是某類別的 `to`、`AGENTS.md` 或某份登記表 |
| `sources 中的 … 解析後是同一個目錄` / `… 互相巢狀` | 每個來源只列一次；拆開巢狀 |
| `mount dir … 解析後位於專案目錄之外` | 確定是本意的話，該條目加 `outside_project: true` |
| `有掛載連結指向 workflows 產物 …,但 build.tools 未列出 "antigravity"` | 加進 `build.tools`，或移除 workflows 連結 |
| `version: N 超過本 agsy 支援的上限…` | 升級 agsy |

→ 下一章：[指令參考](commands.md)
