# 設定檔 agsy.yaml

`agsy init` 會產生一份帶註解的 `agsy.yaml`；之後直接編輯它即可（改完跑 `agsy apply` 生效）。
這一頁逐段解釋每個欄位。

## 完整範例

```yaml
version: 1

sources:                      # 有序陣列，前者優先
  - ~/all-ai-lib              # 跨專案共用庫
  - ./repo-ai-lib             # 專案內的庫

build:
  out: .agsy                  # 產物目錄（必須在專案內，apply 會整個清空重建）

  categories:                 # 來源子目錄 → 產物子目錄（三個 to 必須互異）
    rules:     { from: rule,     to: rules }
    skills:    { from: skill,    to: skills }
    workflows: { from: workflow, to: workflows }

  on_conflict:                # 同名處理：first / rename / error（逐類別必填）
    rules:     rename         # 兩份都留，檔名加來源標記
    skills:    error          # 撞名就停，列清單給你處理
    workflows: rename

  route:                      # workflow 分流
    field: target             # 讀 front-matter 的哪個欄位
    default: [agents, claude] # 沒標 target 的檔案去哪（空陣列 = 不放並警告）
    buckets: [agents, claude] # 所有合法的分流桶

mount:
  - dir: .claude              # Claude Code
    links:
      rules:    rules
      skills:   skills
      commands: workflows/claude
  - dir: .agents              # Antigravity
    links:
      rules:     rules
      skills:    skills
      workflows: workflows/agents
```

## 逐段解釋

### sources：來源，順序就是優先級

路徑三種寫法：`~` 開頭（家目錄展開）、相對路徑（**以 agsy.yaml 所在目錄為基準**，
不是你執行指令的位置）、絕對路徑。

排前面的來源優先——同名衝突用 `first` 策略時，留的是前面那份。

### build.out：產物目錄

`apply` 會**整個清空**這個目錄再重建、`clean` 會整個刪除，所以它有嚴格防呆：
指到專案外、專案根、家目錄、涵蓋任何一個來源、或**位於任何來源底下**，都會在讀設定時直接報錯。
只允許專案內的專用目錄，預設 `.agsy` 就好。

### build.categories：子目錄對應

你的指令庫如果用不同的子目錄名（例如 `rules/` 而不是 `rule/`），改 `from` 即可。
三個 `to` 必須互異——不同類別混在同一層會讓同名偵測失效，所以直接禁止。

### build.on_conflict：同名策略（必填）

| 策略 | 行為 | 適合 |
|---|---|---|
| `rename` | 兩份都留，檔名各加來源標記（`python-style-fromlib-all-ai-lib.md`） | rules：全域基準＋專案例外常需共存 |
| `error` | 停下來列出衝突，請你手動處理（最保守） | skills：兩個相似 skill 並存會讓觸發不可預測 |
| `first` | 只留優先級高的那份，其餘丟棄（plan 會列出丟了什麼） | 明確想要「專案覆蓋全域」的情況 |

這三個欄位**沒有預設值、必須明填**——撞名的後果每個專案不同，agsy 不替你默默決定。

### build.route：workflow 分流

在 workflow 檔的 front-matter 標 `target`，build 時自動分派：

```markdown
---
target: [claude]
---
```

- `field`：讀哪個 front-matter 欄位（預設 `target`）
- `buckets`：所有合法的桶；標了不存在的桶會直接報錯
- `default`：沒標 target 的檔案去哪。設 `[]` 表示「不放」，plan / apply 會對每個這樣的檔案發警告

### mount：掛載對應

每個 `dir` 是一個工具的讀取目錄，`links` 是「工具的子目錄 → 產物裡的哪一層」。
內建 adapter（init 的選單）提供 Claude Code、Antigravity、OpenAI Codex 的預設值；
**清單上沒有的工具不用等更新**，自己加一段就行：

```yaml
mount:
  - dir: .sometool        # 該工具讀取的目錄
    links:
      rules: rules        # 工具子目錄 → 產物層
```

`links` 指向的層必須真的存在於產物（`rules` / `skills` / `workflows/<bucket>`），
設定驗證會交叉檢查，指錯會在讀設定時報錯。

## 版控建議

- `agsy.yaml` **進版控**——隊友 clone 後跑 `agsy apply` 就長出相同掛載
- `.agsy/` **不進版控**——它是可重建產物（init 會主動幫你加進 `.gitignore`）
