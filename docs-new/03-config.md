# C-1. 個人設定檔：agsy.yaml 完整說明

`agsy.yaml` 是 agsy 唯一的設定檔，放在專案根目錄，**建議進版控**（產物目錄 `.agsy/` 則不要）。
通常由 `agsy init` 問答產生；本章說明每個欄位，讓你能放心手動編輯。

## C-1-1. 完整範例

```yaml
# agsy 設定檔（agent-sync）
# 路徑語法：~ 開頭＝家目錄展開；相對路徑＝以本檔所在目錄為基準；絕對路徑＝原樣使用
version: 1

sources:                      # 有序陣列，越前面優先權越高
  - ~/all-ai-lib
  - ./repo-ai-lib

build:
  out: .agsy                  # 必須在專案目錄內（apply 會整個清空重建）

  categories:                 # 來源子目錄 → 產物子目錄（三個 to 不可相同）
    rules:     { from: rules, to: rules }
    skills:    { from: skills, to: skills }
    workflows: { from: workflows, to: workflows }

  on_conflict:                # 同名處理：first / rename / error（每類別必填）
    rules:     rename
    skills:    error
    workflows: rename

  route:                      # workflow 分流
    field: target             # 讀哪個 front-matter 欄位（agsy 自訂欄位）
    default: [claude, codex]  # 沒標 target 的 workflow 去哪些 bucket
    buckets: [claude, codex]  # 所有合法 bucket

mount:
  - dir: .claude              # Claude Code
    links:
      rules:    rules
      skills:   skills
      commands: workflows/claude
  - dir: .codex               # OpenAI Codex
    links:
      prompts:  workflows/codex
```

## C-1-2. 路徑語法（全設定檔通用）

| 寫法 | 解讀方式 | 適合 |
|------|----------|------|
| `~/xxx` | 展開為使用者家目錄 | 跨專案的個人共用庫 |
| `./xxx`、`xxx` | 以 **agsy.yaml 所在目錄**為基準（不是你執行指令的目錄） | 專案內的庫 |
| `/abs/path` | 原樣使用 | 特殊佈局 |

因為相對路徑以設定檔位置為基準，所以從專案的**子目錄**執行 agsy 也完全正常——agsy 會像 git 一樣往上層尋找 `agsy.yaml`（唯一例外是 `init`，它只看目前目錄，讓你能精準決定設定檔要建在哪）。

## C-1-3. `version`

設定檔格式版本，目前為 `1`。省略時視為 1；若檔案版本高於執行中的 agsy 支援上限，會被要求升級 agsy（避免舊程式用舊規則誤讀新格式）。

## C-1-4. `sources`：來源清單（必填，至少一個）

- 有序陣列，**順序就是優先權**：`on_conflict: first` 時，同名項目保留排在前面的來源那份。
- 常見組合：`[~/共用庫, ./專案庫]`——基底放共用庫、專案特化放專案庫。
- 來源缺少某個類別子目錄（例如沒有 `workflows/`）是正常的，不會報錯。
- 整個來源路徑不存在時：`plan` 仍可預覽（會標明結果不完整），但 `apply` 會**拒絕執行**——不從殘缺的來源清單重建，避免把還在的內容清掉。

### 來源標籤（source tag）

每個來源會有一個自動產生的標籤（用於 `rename` 改名與 `plan` 顯示）：取路徑最後一段、去掉開頭的點（`~/all-ai-lib` → `all-ai-lib`、`./.flow` → `flow`）。若兩個來源標籤撞名，agsy 會自動往上併入父目錄名稱（`b-flow`），仍相同才附加數字，確保標籤絕不重複。

## C-1-5. `build.out`：產物目錄

預設 `.agsy`。**`apply` 每次會整個清空這個目錄、`clean` 會整個刪除**，所以有嚴格的防呆驗證，以下設定會直接被擋下：

- 指向專案根目錄本身、或專案的上層（會把整個專案清掉）；
- 指向專案目錄**之外**（例如 `~/Documents`——別人的資料夾不是拋棄式產出）;
- 包含家目錄；
- 與任一來源互相包含（會把原始檔一起刪掉）。

一句話：**只能是專案內的一個專用子目錄**。改了 `out` 之後記得也更新 `.gitignore`。

## C-1-6. `build.categories`：類別目錄對應

```yaml
categories:
  rules:     { from: rules, to: rules }
  skills:    { from: skills, to: skills }
  workflows: { from: workflows, to: workflows }
```

- `from`：來源目錄下要掃描的子目錄名。
- `to`：產物目錄下的輸出子目錄名。
- 只寫其中一半時，另一半補預設值。
- **三個 `to` 必須互不相同**（否則 rules 的單檔和 skills 的目錄會混在同一層，同名互蓋卻檢查不到）。

### 各類別的收錄規則

掃描時不符合規則的檔案會被**略過並回報**（`plan` 與 `doctor` 都會列出原因），不會默默消失：

| 規則 | 說明 |
|------|------|
| `.` 開頭的檔案 | 一律略過（隱藏檔不回報，避免 `.DS_Store` 洗版） |
| 符號連結 | **一律不收**。建置是複製「內容」，連結可能把來源以外的檔案（如私鑰）夾帶進被掛載的產物 |
| rules / workflows | 只收單一 `.md` 檔；目錄不收 |
| skills | 只收**目錄**；目錄裡必須有 `SKILL.md`（沒有的視為素材／草稿）；目錄內部**含任何符號連結則整個 skill 不收** |

## C-1-7. `build.on_conflict`：同名衝突策略（每類別必填）

多個來源出現同名項目時怎麼辦。**沒有隱含預設值**——這是 agsy 的刻意設計：會影響「哪份內容存活」的決策必須由你明確選擇。

| 策略 | 行為 | 適合 |
|------|------|------|
| `rename` | 兩份都保留，檔名各附上來源標籤：`python-style.md` → `python-style-fromlib-all-ai-lib.md` | rules（「全域基底＋專案補充」常常需要並存） |
| `error` | 停止並列出衝突清單，由你手動改名或刪除（最保守） | skills（見下方說明） |
| `first` | 只保留優先權最高來源的那份，其餘捨棄（`plan` 會列出被捨棄者） | 確定「專案覆蓋全域」的情境 |

### 為什麼 skills 建議 `error` 而不是 `rename`？

skill 是靠 `SKILL.md` 裡的 description 語意觸發的——兩個同名 skill 併存時，工具選哪個很難預測。而且 skill 名稱受 Agent Skills 規範限制（小寫 a-z、0-9、單一連字號、與目錄同名），`rename` 除了改目錄名還得改寫 front-matter 的 `name` 欄位（agsy 會自動做，promote 寫回時也會自動還原），流程較複雜。同名 skill 通常代表真的該整併，所以預設請人工處理。

### 改名後仍撞名（collision）

`rename` 之後或名稱巧合仍可能導致**最終輸出名**相同（例如 `God-Lib` 與 `god-lib` 淨化後相同）。這種情況 agsy 一律**擋下建置**，不會默默互蓋。

## C-1-8. `build.route`：workflow 分流

```yaml
route:
  field: target             # 讀 workflow front-matter 的哪個欄位
  default: [claude, codex]  # 沒標 target → 去這些 bucket；設 [] 則不放並警告
  buckets: [claude, codex]  # 所有合法 bucket 清單（必填）
```

- workflow 檔案 front-matter 寫 `target: claude`（單一）或 `target: [claude, codex]`（多個），建置時就複製到 `workflows/<bucket>/` 對應目錄。
- `target` 指到 `buckets` 沒有的值＝**路由錯誤**，`plan` 會列出全部、`apply` 拒絕建置。
- `default: []` 時，沒標 target 的 workflow **不會放進任何 bucket**，`plan`/`apply` 會警告。建議 default 涵蓋所有 bucket：「到處都出現」比「神祕消失」容易理解。
- `buckets` 通常由 `init` 依你勾選的適配器自動組出；手動加過的 bucket 在 `init` 編輯模式會被保留。

## C-1-9. `mount`：掛載設定（必填，至少一個）

```yaml
mount:
  - dir: .claude            # 掛載目錄（通常是某工具的設定目錄）
    links:
      rules:    rules              # .claude/rules   → .agsy/rules
      skills:   skills             # .claude/skills  → .agsy/skills
      commands: workflows/claude   # .claude/commands → .agsy/workflows/claude
```

- `dir`：要放連結的目錄；`links`：`連結名稱: 產物內的相對目標`。
- 目標的第一層必須是某個類別的 `to` 值；**只有 workflows 可以有第二層**（bucket，且必須在 `route.buckets` 裡）；不能更深。
- 驗證也會擋：同一個 `dir` 重複出現、`dir` 落在產物目錄內（apply 清空時連結會一起被砍）、`dir` 落在來源內（連結會被當成來源內容掃描）。
- 連結實作：macOS / Linux 用**相對路徑 symlink**（專案整個搬移仍有效）；Windows 用 **junction**（不需管理員權限，但只能存絕對路徑——搬移專案後要重跑 `agsy apply` 重建）。

內建適配器提供的掛載組合見[適配器說明](05-adapters.md)；不在內建清單的工具可以自己加一個 mount 項目，`init` 編輯模式會把自訂項目原樣保留。

## C-1-10. 驗證錯誤速查

設定載入時所有問題會**一次列出**（不是逐條擋）。常見訊息與處理：

| 錯誤訊息（意譯） | 處理 |
|------------------|------|
| `sources is not set` | 至少填一個來源 |
| `build.on_conflict.rules is not set …` | 三個類別的策略都要明確填 `first`/`rename`/`error` |
| `build.out … apply would wipe …` | 產物目錄指到危險位置，改回專案內專用目錄（如 `.agsy`） |
| `build.categories.x.to and y are both …` | 兩個類別輸出到同一子目錄，把 `to` 改成不同值 |
| `mount … links.x points to "z", but the output has no such top level` | 連結目標第一層要是 rules/skills/workflows（依你的 `to` 設定） |
| `mount … points to bucket "b", but it is not in build.route.buckets` | 把該 bucket 加進 `route.buckets`，或改連結目標 |
| `version: N exceeds the maximum …` | 設定檔來自較新的 agsy，請升級 agsy |

→ 下一章：[指令說明](04-commands.md)
