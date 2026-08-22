# C-3. 適配器（Adapters）說明

## C-3-1. 適配器是什麼？

適配器是**內建的「各家工具掛載範本」**：記錄某個 AI 工具的設定目錄在哪、該把 rules / skills / workflows 連到哪個名字底下、以及它專屬的 workflow bucket 名稱。

要點：

- 適配器只在 `agsy init` 時發揮作用——你勾選工具，init 就把對應的 `mount` 段落與 bucket 寫進 `agsy.yaml`。
- **執行期完全不依賴適配器**：`plan` / `apply` / `status` 只看 `agsy.yaml`。也就是說 init 產生設定後，你可以任意手改 mount 段落，不受範本限制。

## C-3-2. 內建適配器一覽

> 只想快速查「哪個類別掛在哪個資料夾」，看[核心概念的總表](00-overview.md#目前支援的工具與對應資料夾)一張就夠；下面是逐工具的細節。

### Claude Code（`claude`）

| 項目 | 值 |
|------|-----|
| 掛載目錄 | `.claude/` |
| bucket | `claude` |

| 連結 | 指向產物 | Claude Code 的用途 |
|------|----------|---------------------|
| `.claude/rules` | `rules` | 規則檔 |
| `.claude/skills` | `skills` | Agent Skills |
| `.claude/commands` | `workflows/claude` | 斜線指令（slash commands） |

三個類別全部有掛載，是覆蓋最完整的適配器。標了 `target: claude` 的 workflow 會出現在 `.claude/commands/`，在 Claude Code 裡就是 `/<檔名>` 斜線指令。

### OpenAI Codex（`codex`）

| 項目 | 值 |
|------|-----|
| 掛載目錄 | `.codex/` |
| bucket | `codex` |

| 連結 | 指向產物 | 用途 |
|------|----------|------|
| `.codex/prompts` | `workflows/codex` | 自訂 prompts |

**只掛 workflows（prompts）**，遵循 Codex CLI 的 custom prompts 慣例（原始碼註記：專案層級支援仍待實測驗證）。因此若你**只**勾選 Codex，`init` 結束時會警告：rules 和 skills 會被建置、但沒有任何工具讀它們——需要的話可自行在 `agsy.yaml` 補 links。

### Antigravity（`antigravity`）

| 項目 | 值 |
|------|-----|
| 掛載目錄 | `.agents/` |
| bucket | `agents` |

| 連結 | 指向產物 | 用途 |
|------|----------|------|
| `.agents/rules` | `rules` | 規則檔 |
| `.agents/skills` | `skills` | Skills |
| `.agents/workflows` | `workflows/agents` | Workflows |

## C-3-3. bucket 與適配器的關係

每個適配器宣告一個 bucket 名稱。`init` 時：

- `route.buckets` ＝ 你勾選的適配器 bucket 的聯集（＋你手動加過的、＋自訂 mount 用到的）。
- workflow 用 front-matter `target:` 指名要去哪些工具：

```markdown
---
target: claude            # 只出現在 .claude/commands/
---
```

```markdown
---
target: [claude, codex]   # 兩邊都出現（各複製一份）
---
```

- 沒標 `target` → 依 `route.default`（init 建議設為全部 bucket）。

## C-3-4. 自訂掛載（接入不在內建清單的工具）

任何「從固定目錄讀 Markdown」的工具都能手動接上。直接在 `agsy.yaml` 的 `mount` 加一段即可：

```yaml
build:
  route:
    buckets: [claude, codex, mytool]   # 若要有專屬 bucket，記得加進來
    default: [claude, codex, mytool]

mount:
  - dir: .mytool
    links:
      rules:   rules                # 共用 rules
      prompts: workflows/mytool     # 專屬 workflow bucket
```

規則回顧（詳見[設定檔說明](03-config.md)）：

- links 目標第一層必須是某類別的 `to` 值；只有 workflows 可加第二層 bucket，且 bucket 要在 `route.buckets` 裡。
- `dir` 不可與其他 mount 重複、不可位於產物目錄或來源目錄內。

**自訂項目不會被 init 洗掉**：之後再跑 `agsy init` 編輯設定，非內建適配器的 mount 段落、以及自訂 bucket 都會原樣保留，並在畫面上標明「custom mount，維持原樣」。

## C-3-5. （進階）為 agsy 專案新增內建適配器

這一節給想貢獻上游的人。內建適配器是 repo 裡 `adapters/` 目錄下的 YAML 檔，經 `go:embed` 打包進執行檔。新增一個工具只要放一個 `<name>.yaml`：

```yaml
# adapters/mytool.yaml
name: mytool          # 內部識別名
display: My Tool      # init 選單顯示名
bucket: mytool        # 專屬 workflow bucket
mount:
  dir: .mytool
  links:
    rules:   rules
    prompts: workflows/mytool
```

重新編譯後，init 的工具多選清單就會出現它（清單依 name 排序）。

→ 下一章：[Q&A 常見問題](06-faq.md)
