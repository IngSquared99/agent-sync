# 設定檔：agsy.yaml 完整參考

`agsy.yaml` 是 agsy 唯一的設定檔，放在專案根目錄，**應該進版控**（產物目錄 `.agsy/` 則不應該）。
它通常由 `agsy init` 問答產生；本章解釋每個欄位，讓你能放心手動編輯。

## 完整範例

```yaml
# agsy 設定檔（agent-sync）
# 路徑語法：~ 開頭 = 展開家目錄；相對路徑 = 以本檔所在目錄為基準；絕對路徑 = 原樣使用
version: 2

sources:                      # 有序陣列，前者優先
  - ~/all-ai-lib
  - ./repo-ai-lib

build:
  out: .agsy                  # 必須在專案內（apply 會整個清空它）

  categories:                 # 來源子目錄 → 產物子目錄（from 與 to 各自不得重複）
    rules:     { from: rules, to: rules }
    skills:    { from: skills, to: skills }
    workflows: { from: workflows, to: workflows }
    hooks:     { from: hooks, to: hooks }

  on_conflict:                # 同名處理：first / rename / error（逐類別必填）
    rules:     rename
    skills:    error
    workflows: rename
    hooks:     error

  tools: [claude, codex, antigravity, cursor]   # workflow 的 target front matter 可用的工具名

mount:
  - dir: .                    # 專案根：AGENTS.md 供 Codex / Cursor / Antigravity 讀取
    links:
      AGENTS.md: AGENTS.md
  - dir: .claude              # Claude Code
    links:
      rules:  rules
      skills: skills
    merge:                    # hooks 合併進 settings.json 的 "hooks" 鍵（其他鍵不動）
      settings.json: hooks.claude.json
  - dir: .agents              # Codex + Antigravity + Cursor
    links:
      skills:     skills
      workflows:  workflows
      hooks.json: hooks.antigravity.json
  - dir: .codex               # Codex 的 hooks
    links:
      hooks.json: hooks.codex.json
  - dir: .cursor              # Cursor 的 hooks
    links:
      hooks.json: hooks.cursor.json
```

## 路徑語法（全檔通用）

| 形式 | 解讀 | 適合 |
|------|------|------|
| `~/xxx` | 展開為家目錄 | 跨專案共用庫 |
| `./xxx`、`xxx` | 以**agsy.yaml 所在目錄**為基準（不是你目前的工作目錄） | 專案內的庫 |
| `/abs/path` | 原樣使用 | 特殊配置 |

`~` 只展開為**你自己的**家目錄;shell 的 `~user` 形式(別的使用者的家目錄)不支援,會直接回報錯誤,而不是被誤讀成 `$HOME/user`。

因為相對路徑以設定檔位置為基準，從專案**子目錄**執行 agsy 也一切正常——agsy 會像 git 一樣往上尋找 `agsy.yaml`（唯一例外是 `init`，它只看目前目錄，設定檔要建在哪由你站的位置決定）。

## `version`

設定檔格式版本，目前為 `2`（v0.2.0 起）。省略＝視為目前版本；`version: 1` 的檔案照常載入（欄位只增不減），但必須補上 `on_conflict.hooks`（見升級說明）。版本高於執行中 agsy 支援上限時會要求升級——含 `merge` 的設定不該被不懂 merge 的舊版執行。

## `sources`（必填，至少一個）

- 有序陣列——**順序即優先序**：`on_conflict: first` 時前面來源的同名項目存活；串接的 `AGENTS.md` 也依此順序。
- 常見組合：`[~/共用庫, ./專案庫]`——基底內容放共用庫，專案專屬的放專案庫。
- 來源缺某個類別子目錄（例如沒有 `workflows/`）是正常的，不是錯誤。
- 整個來源路徑不存在時：`plan` 仍可預覽（標記不完整），但 `apply` **拒絕執行**——絕不以殘缺的來源清單重建，否則缺席來源的項目會被清掉。
- 來源彼此必須**互不重複、互不巢狀**：同一路徑列兩次（不論寫法）、或一個來源位於另一個之內，驗證時都會拒絕——否則其中的檔案會被收集兩次。

### 來源標記

每個來源會得到一個自動產生的標記（rename 用、`plan` 顯示）：路徑最後一段去掉開頭的點（`~/all-ai-lib` → `all-ai-lib`、`./.flow` → `flow`）。兩個來源標記相同時，agsy 逐層併入上層目錄名，仍相同才附加數字——標記保證唯一。

## `build.out`（產物目錄）

預設 `.agsy`。**`apply` 每次執行都會清空它、`clean` 會整個刪除它**，所以驗證會直接拒絕危險的值：

- 專案根本身或其上層（會清掉整個專案）；
- 專案**之外**的任何位置（不相干的目錄不是可拋棄的產物）；
- 涵蓋家目錄的位置；
- 涵蓋任一來源、或位於任一來源之內（會刪到原稿）。

一句話：**只能是專案內的專用子目錄。**

**修改 `out` 有固定順序**：先在**舊**設定下執行 `agsy clean`，再改 `agsy.yaml`、執行 `agsy apply`。順序顛倒會留下無人追蹤的舊產物目錄。記得同步更新 `.gitignore`。

## `build.categories`

- `from`：掃描每個來源裡的哪個子目錄。
- `to`：`.agsy/` 裡的產物子目錄。
- 只設定一半時，另一半採預設值。
- **四個 `from` 值必須不同，四個 `to` 值也必須不同**；`to` 不可為 `AGENTS.md`（保留給串接產生的 rules 檔），也不可為 `hooks.claude.json`、`hooks.codex.json`、`hooks.antigravity.json`、`hooks.cursor.json`（保留給 hook 登記表）。
- **什麼時候改 `from`**：既有的庫若子目錄一直叫別的名字（比如一直是 `prompts/`），用 `from: prompts` 就能直接接上、不必搬檔案。`to` 很少需要改。

### 各類別的收錄規則

不符規則的檔案會**略過並回報**（`plan` 與 `doctor` 都會列出原因）——不會有東西無聲消失：

| 規則 | 說明 |
|------|------|
| 名稱以 `.` 開頭 | 一律略過（隱藏檔連回報都不回報，讓 `.DS_Store` 進不了產物） |
| 符號連結 | **一律不收**。build 複製的是檔案內容；連結可能把來源以外的檔案（例如私鑰）夾帶進掛載中的產物 |
| 非一般檔案（FIFO、socket、裝置檔） | **一律不收**。讀取具名管線（named pipe）會永久阻塞，被放進來源就能讓建置卡死；skill 內含任何非一般檔案即整個拒收 |
| rules / workflows | 只收單一 `.md` 檔；目錄拒收；超過 **5 MB** 的檔案會略過（這麼大的指令檔對 context 毫無用處） |
| skills | 只收**目錄**，且必須含 `SKILL.md`；skill 內含**任何 symlink 即整個拒收** |
| hooks | 只收**目錄**，且必須含 `hook.yaml`；同樣內含 symlink 或非一般檔案即整個拒收。目錄名即 hook 名，需為小寫 a–z、0–9 與連字號（不符者正規化並在 plan 註明） |

## `build.on_conflict`（逐類別必填）

多個來源出現同名項目時怎麼辦。**沒有隱含預設**——決定誰存活的選擇必須明確做出。

| 策略 | 行為 | 適合 |
|------|------|------|
| `rename` | 兩份都留，檔名附上來源標記：`python-style.md` → `python-style-fromlib-all-ai-lib.md` | rules（「全域基底＋專案增補」常需共存） |
| `error` | 停下並列出衝突，由人工處理（最保守） | skills（見下） |
| `first` | 只留優先序最高的一份，其餘捨棄（`plan` 會列出被捨棄者） | 確定「專案覆蓋全域」時 |

### skills 為什麼建議 error 而非 rename？

skills 依 `SKILL.md` description 的語意觸發——兩個同名 skill 共存時，工具挑哪個無法預測。skill 名稱也受 Agent Skills 規範約束（小寫 a–z、0–9、單一連字號、name 必須等於目錄名），所以 rename 除了改目錄名還得改寫 front matter 的 `name`（agsy 會自動處理）。同名 skill 通常代表兩份應該合併，所以預設建議人工處理。

### hooks 為什麼也建議 error？

同名的兩個 hook 通常代表同一件事的兩個版本，該合併成一個。rename 仍可用（目錄名附加來源標記，Antigravity 登記表的頂層 key 也跟著改）。

### 最終輸出路徑的碰撞

rename 之後、或命名巧合，兩個項目的**最終輸出路徑**仍可能相同——包括跨類別：叫 `deploy.md` 的 workflow 會轉出 `skills/deploy`，與真的叫 `deploy` 的 skill 相撞。agsy 一律**擋下建置**——絕不無聲覆蓋。

## `build.tools`（必填）

workflow 的 `target:` front matter 可引用的工具名封閉清單。`init` 依你勾選的 adapter 填入。`target:` 出現清單外的值即為錯誤：`plan` 全數列出、`apply` 拒絕建置——打錯字必須大聲失敗，而不是讓 workflow 無聲地哪裡都去不了。

驗證也會維持這份清單與 workflows 掛載設定的一致性：有掛載連結指向 workflows 產物時，這裡就必須列出 `antigravity`（唯一會讀那個目錄的工具）;反之列出 `antigravity` 就必須在某處掛載 workflows——否則會產生無人讀取的 stub，或掛載一個永遠是空的目錄。

### workflow 的 `target:` front matter

```yaml
---
target: [claude, codex]   # 或單一字串：target: claude
---
```

- 沒寫 → 給 `build.tools` 裡的所有工具。
- 有寫 → 只給列出的工具。兩個效果：列出的工具中至少一個不是 `antigravity` 時產生 skill 形態；列出 `antigravity` 時產生 `workflows/` 形態（有 skill 形態時是轉接頭，否則放完整內容）。
- skills 輸出是多工具共讀的一個目錄，排除因此是粗粒度的：只列部分工具時，凡掛載 skills 的工具仍都看得到，`plan` 會註明。
- `target:` 是 agsy 的欄位；build 會把它從所有產物的 front matter 移除。

## hooks 的 `hook.yaml`

每個 hook 目錄必含一份 `hook.yaml`。事件與 matcher 的結構採 Claude Code / Codex 的形狀，因為那是四家裡最通用的；build 再依各家方言翻譯。

```yaml
description: 擋下 rm -rf 之類的破壞性指令     # 選填；plan 顯示用
target: [claude, codex, antigravity, cursor]   # 選填；語意同 workflows：省略＝build.tools 全部
events:                                        # 必填；至少一個事件
  PreToolUse:                                  # 通用事件名（見下表）
    - matcher: Bash                            # 選填；預設採 Claude / Codex 的工具名
      hooks:                                   # 必填；至少一個 handler
        - type: command                        # 選填；預設 command
          command: ./block-rm.sh               # command 型必填；相對於本 hook 目錄
          timeout: 10                          # 選填；其他欄位原樣透傳
      overrides:                               # 選填；某家不同時個別指定
        antigravity: { matcher: run_command }
        cursor:      { matcher: Shell }
```

**通用事件名**（以 Claude Code / Codex 命名為基準；「—」代表該家官方沒有此掛勾點，翻譯時略過並在 plan 註明）：

| 通用名 | claude | codex | antigravity | cursor |
|---|---|---|---|---|
| `PreToolUse` | ✓ | ✓ | ✓ | `preToolUse` |
| `PostToolUse` | ✓ | ✓ | ✓ | `postToolUse` |
| `Stop` | ✓ | ✓ | ✓ | `stop` |
| `SessionStart` / `SessionEnd` | ✓ | ✓ | — | `sessionStart` / `sessionEnd` |
| `UserPromptSubmit` | ✓ | ✓ | — | `beforeSubmitPrompt` |
| `PermissionRequest` | ✓ | ✓ | — | — |
| `SubagentStart` / `SubagentStop` | ✓ | ✓ | — | `subagentStart` / `subagentStop` |
| `PreCompact` | ✓ | ✓ | — | `preCompact` |
| `PostCompact`、`Interrupt` | — | ✓ | — | — |
| `PostToolUseFailure` | ✓ | — | — | `postToolUseFailure` |
| `StopFailure` | ✓ | — | — | — |
| `PreInvocation` / `PostInvocation` | — | — | ✓ | — |

**handler type** 支援：claude 全部（command、http、mcp_tool、prompt、agent）；codex 只有 command、mcp_tool；antigravity 只有 command；cursor 只有 command、prompt。不支援的 handler 逐筆略過並回報。

**翻譯規則**：

1. `target` 沒列的工具不寫入該家登記表；plan 註明少了哪幾家。`target` 指到 `build.tools` 裡沒有登記表格式的工具（四家內建工具以外的名字）時 plan 也會註明：該 hook 不會送達。
2. 該家沒有的事件、不支援的 type 略過並在 plan 註明；某家一個事件都對不上時，登記表仍會產生（內容為空），連結不會斷。
3. `overrides.<tool>` 的 key 只要是 agsy 認識的工具即可（不在 `build.tools` 就忽略），讓共用庫帶著四家的 overrides 也能在只用一家的專案使用；完全未知的名字才是錯誤。`matcher` 覆寫群組的 matcher；`hooks[i]` 依索引覆寫第 i 個 handler 的欄位。override 把 handler 的 `type` 改成 command 以外的型別時，該家捨棄 `command` 欄位並註明。
4. **路徑改寫**：`command` 裡每個以 `./` 開頭、以空白分隔的 token 改寫為指向 `.agsy/hooks/<name>/…` 的**絕對路徑**；字串其餘部分原樣保留（`python3 ./check.py` 成立）。command 由 shell 執行，改寫後的路徑含空白或 shell 特殊字元時**加引號**（例如專案在 `My Projects/` 底下）；一般路徑不加。引用的檔案不存在 → apply 拒絕，主 handler 與 `overrides` 皆然。`.agsy/` 不進版控、每台機器各自 apply，絕對路徑在這裡沒有可攜性問題。
5. 其他 handler 欄位原樣透傳（三家同構）；Cursor 的攤平結構只保留 `timeout`、`failClosed`、`loop_limit`、`prompt`，其餘丟棄並回報。

**agsy 只翻譯登記層，不翻譯腳本。** 四家餵給腳本的 stdin JSON 欄位不同（Claude / Codex 是 `tool_input.command`，Cursor 是另一套，Antigravity 又是一套），跨四家的腳本要自己看 `hook_event_name` 或各家欄位判斷。

## `mount`（必填，至少一個）

- `dir`：連結建在哪；`.` 代表專案根。`links`：`連結名: 產物最上層名稱`。
- 連結可指向各類別的 `to` 值、`AGENTS.md`（串接產生的 rules 檔），或四份 hook 登記表 `hooks.<tool>.json`（該工具必須在 `build.tools`）。目標只能是最上層。
- 同 `dir` 的多個條目會合併（多個 adapter 共掛 `.agents`）；同名連結指向不同目標才是錯誤。
- 驗證也會拒絕：`dir` 在產物之內（apply 清空時連結會一起消失）、`dir` 在來源之內（連結會被當成來源內容掃進去）、連結名含路徑分隔符。
- **`dir` 位於專案目錄之外時，該掛載條目必須明確加上 `outside_project: true`。**這是一道安全閘：少了它，在 clone 下來的 repo 裡執行 `apply`，一份把掛載指向家目錄的 `agsy.yaml`（例如 `dir: ~/.claude`）就能無聲地把你的**全域**工具設定導向該 repo 的內容。只有你親自撰寫的設定才應該加上這個選項。
- 連結實作：macOS / Linux 一律**相對路徑 symlink**。Windows 目錄用 **junction**、根目錄 `AGENTS.md` 與 hook 登記表用 **hard link**——都不需要管理員權限。junction 存的是絕對路徑：搬移專案後請重跑 `agsy apply`。

### `merge`：Claude Code 專用

```yaml
  - dir: .claude
    merge:
      settings.json: hooks.claude.json
```

Claude Code 的 hooks 沒有獨立檔，只能寫在 `.claude/settings.json` 的 `hooks` 鍵，而那份檔還裝著 permissions、model 等你自己的設定，整檔連結會吃掉它們。`merge` 的語意是「**只擁有 `hooks` 鍵裡自己的那幾筆條目**」：

- **所有權判準**：`hooks` 鍵下某個 matcher 群組，只要有任一 handler 帶著純顯示欄位 `statusMessage: "agsy:<hook 名>"`（build 為 Claude Code 寫出的每個 handler 都補上，`hook.yaml` 自己有寫則保留），或 `command` 指向 `.agsy/hooks/`（目前的產物目錄，或 manifest 記錄的那個），就是 agsy 的。標記與路徑無關，所以搬移專案或改名 `build.out` 之後，先前的群組仍會被認出並取代。邏輯同 `IsManagedLink`（連結指向產物就是 agsy 的）。`hook.yaml` 自帶 `statusMessage` 的 handler 只靠路徑辨認；改名 `build.out` 之後，先前 apply 的這種群組視為別人的，需手動移除。
- **apply**：移除舊的 agsy 群組、放入登記表的群組；其他鍵與其他群組原樣保留、順序不動（整份以兩空格重新縮排）。apply 之前就存在的容器——空的事件陣列、`hooks: {}`——空著也保留；apply 自己帶進來的 `hooks` 鍵與事件陣列空掉時移除（記錄在 manifest）。檔案不存在時建立並記錄「由 agsy 建立」。沒有東西可 merge、檔案裡也沒有 agsy 的東西時（狀態「閒置」），檔案既不建立也不改寫。
- **status**：agsy 群組被改 → 列入產物端改動（apply 詢問後重建）；被刪 → 列入缺失（apply 復原）。你自己加的群組不算異常。manifest 記錄過、mount 設定已不再列出、且仍留有 agsy 群組的檔案，回報為**孤兒**：apply 不碰它，clean 清掉。
- **clean**：移除目前與孤兒目標裡的 agsy 群組；apply 帶進來的 `hooks` 鍵與事件陣列一起移除，其他保留；檔案是 agsy 建的且變空才刪檔。閒置的目標不會被動到。
- **拒絕**：目標是 symlink（agsy 絕不透過連結寫入）或不是 JSON 物件 → apply 前置檢查停下、clean 跳過並回報。
- `merge` 的 value 必須是登記表名且該工具在 `build.tools`；同一 `dir` 內 `merge` 與 `links` 不得同名；`dir` 在專案外同樣需要 `outside_project: true`。
- 只有一個 `merge`、沒有 `links` 的掛載條目是合法的。

你自己的 Claude hooks 請放 `.claude/settings.local.json` 或 `~/.claude/settings.json`——Claude Code 的多層設定是疊加而非覆蓋。其他三家同理：`~/.codex/hooks.json`、`~/.cursor/hooks.json`、`~/.gemini/config/`（Antigravity 官方未載明合併方式，請實測）。

### 反向檢查只是提示

掛了 `hooks.<tool>.json` 但 `tool` 不在 `build.tools` 是錯誤（檔案永遠是空的）。反過來，`build.tools` 有 `tool` 但沒掛它的登記表**不是錯誤**——既有專案升級後不該直接壞掉——只在來源真的有 hook 時由 `plan` / `doctor` 印 ⚠ 提示。

## 驗證錯誤速查

問題會在「設定檔驗證失敗」標頭下**一次全部列出**。常見訊息：

| 錯誤訊息 | 處理 |
|---------|------|
| `sources 未設定,至少需要一個來源路徑` | 至少加一個來源 |
| `build.on_conflict.rules 未設定…` | 四個類別都要設策略 |
| `build.on_conflict.hooks 未設定(v0.2.0 新增)…` | 從 v0.1 升級：加一行 `hooks: error`，或重跑 `agsy init` |
| `build.categories.x.to 不可為 "hooks.codex.json"…` | 該名稱保留給登記表，換一個 |
| `mount … merge.settings.json 指向 "z",但只有 hook 登記表可以合併` | merge 的目標只能是 `hooks.<tool>.json` |
| `mount … links.hooks.json 指向 "hooks.cursor.json",但 build.tools 未列出 "cursor"` | 把 `cursor` 加進 `build.tools`，或移除該連結 |
| `build.tools 未設定…` | 列出工具，例如 `[claude, codex, antigravity, cursor]` |
| `build.out(…)不在專案目錄(…)底下` | 改回專案內的專用目錄（例如 `.agsy`） |
| `build.categories.x.to 與 y 同為 "…"` | 給其中一個不同的 `to` |
| `mount … links.x 指向 "z",但產物裡沒有這一層` | 目標必須是某類別的 `to` 值、`AGENTS.md` 或某份登記表 |
| `sources 中的 … 解析後是同一個目錄` / `… 互相巢狀` | 每個來源只列一次；拆開巢狀 |
| `mount dir … 解析後位於專案目錄之外` | 確為本意？在該掛載條目加 `outside_project: true` |
| `有掛載連結指向 workflows 產物 …,但 build.tools 未列出 "antigravity"` | 把 `antigravity` 加進 `build.tools`，或移除 workflows 連結 |
| `version: N 超過本 agsy 支援的上限…` | 設定檔來自較新的 agsy——請升級 |

→ 下一章：[指令參考](commands.md)
