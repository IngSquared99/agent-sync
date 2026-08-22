# 0. 核心概念：agsy 是什麼？

## 它解決的問題

當你同時使用多個 AI 開發工具（Claude Code、OpenAI Codex、Antigravity…），每個工具都有自己的指令檔目錄：

- Claude Code 讀 `.claude/rules/`、`.claude/skills/`、`.claude/commands/`
- Codex 讀 `.codex/prompts/`
- Antigravity 讀 `.agents/`

同一份「coding 規範」「常用 skill」「工作流程」得複製好幾份、改一處要同步好幾處；而且你可能還想把「個人共用的一套」和「這個專案專屬的一套」疊在一起用。

**agsy（agent-sync）** 就是做這件事的合併＋掛載工具：

> 把多個來源（sources）的指令檔，**合併建置**成單一產物目錄（預設 `.agsy/`），再用**目錄連結**掛載進每個工具的讀取位置。

改動時只改來源，跑一次 `agsy apply`，所有工具同時更新。

## 三層架構

```
┌────────────────────┐
│  來源 sources       │  ~/all-ai-lib/       （個人共用庫，跨專案）
│ （你維護的原始檔）    │  ./repo-ai-lib/      （專案內的庫）
└─────────┬──────────┘
          │  agsy apply（掃描 → 合併 → 複製）
          ▼
┌────────────────────┐
│  產物 build.out     │  .agsy/rules/
│ （工具建置的成品，    │  .agsy/skills/
│   可整個重建）       │  .agsy/workflows/<bucket>/
└─────────┬──────────┘
          │  agsy apply（建立目錄連結 symlink / junction）
          ▼
┌────────────────────┐
│  掛載 mount         │  .claude/rules  → ../.agsy/rules
│ （各 AI 工具實際     │  .claude/skills → ../.agsy/skills
│   讀取的位置）       │  .codex/prompts → ../.agsy/workflows/codex
└────────────────────┘
```

- **來源（sources）**：你真正維護、進版控的原始檔。可以有多個，順序代表優先權（越前面越優先）。
- **產物（build.out，預設 `.agsy/`）**：agsy 建置出來的成品，**整個目錄視為可重建的拋棄式產出**——`apply` 每次會清空重建，所以不要把手寫的東西直接放進去（除非你打算用 `promote` 寫回來源）。
- **掛載（mount）**：在各工具的目錄裡建立指向產物的連結。工具看到的是連結，實際內容都在 `.agsy/` 裡。

## 三種類別（categories）

來源目錄下用三個子目錄分類，格式各有規定：

| 類別 | 來源子目錄（預設） | 格式 | 輸出位置 |
|------|--------------------|------|----------|
| rules | `rule/` | 單一 `.md` 檔 | `.agsy/rules/` |
| skills | `skill/` | **目錄**，內含 `SKILL.md` | `.agsy/skills/` |
| workflows | `workflow/` | 單一 `.md` 檔，front-matter 可標 `target` | `.agsy/workflows/<bucket>/` |

一個典型的來源長這樣：

```
~/all-ai-lib/
├── rule/
│   ├── python-style.md
│   └── git-commit.md
├── skill/
│   └── code-review/
│       ├── SKILL.md
│       └── scripts/…
└── workflow/
    └── release.md        （front-matter: target: claude）
```

## workflows 的「分流」（routing 與 bucket）

rules 和 skills 是所有工具共用一份；workflows 則不同——每個工具的「指令／prompt」格式與用途不一定通用，所以 workflows 會依 front-matter 的 `target` 欄位**分流**到不同 bucket（子目錄）：

```markdown
---
target: claude          # 只給 Claude Code
# 或 target: [claude, codex]  # 兩者都要
---
Release 流程說明…
```

沒有標 `target` 的 workflow 會落入 `route.default` 設定的 bucket（預設是全部）。詳見[設定檔說明](03-config.md)與[適配器說明](05-adapters.md)。

## 雙向資料流：apply 與 promote

```
sources  ──── apply ───▶  .agsy/（掛載給工具）
sources  ◀── promote ───  .agsy/
```

- **apply（正向）**：來源改了 → 重建產物、更新掛載。
- **promote（反向）**：你（或 AI 工具）直接改了掛載側的檔案（例如 AI 在 `.claude/skills/` 裡幫你改了一個 skill）→ 用 `promote` 把改動**寫回來源**，改動才不會在下次 apply 被蓋掉。

agsy 用 `.agsy/.agsy-manifest.json`（建置紀錄檔）記錄每個項目在建置當下的來源與產物雜湊值，因此 `status` 能精準判斷：哪些是「來源更新了還沒重建」（behind）、哪些是「掛載側被改了還沒寫回」（local changes）、兩邊是否同時被改（需要人工合併）。

## 設計上的安全底線

讀文件時會一直看到這些原則，先列在這裡：

1. **不碰真實檔案**：掛載點若已存在「真實的目錄或檔案」（不是 agsy 建的連結），agsy 一律報錯請你手動處理，絕不代為刪除。
2. **同名策略必須由你明說**：`on_conflict` 每個類別都必填，沒有隱含預設。
3. **刪除前必先確認**：`apply` 會清空產物目錄，若偵測到未寫回的改動一定先問；非互動環境沒有 `--yes` 就取消，絕不硬做。
4. **產物目錄位置有防呆**：`build.out` 只能是專案內的專用目錄，指到家目錄、來源目錄、專案根都會被設定驗證直接擋下。
5. **不收符號連結**：來源裡的 symlink 一律不收（含 skill 目錄內部），避免透過連結把來源以外的檔案夾帶進產物。

## 名詞小抄

| 名詞 | 意思 |
|------|------|
| source（來源） | 你維護的原始指令檔庫，`agsy.yaml` 的 `sources` 陣列 |
| build out（產物） | `apply` 建置出的目錄，預設 `.agsy/`，整個可重建 |
| mount（掛載） | 在工具目錄建立指向產物的連結 |
| bucket | workflows 分流的目的地子目錄（如 `claude`、`codex`） |
| manifest | `.agsy/.agsy-manifest.json`，建置紀錄與變更偵測的基準 |
| source tag（來源標籤） | 同名改名時附加在檔名上的來源識別，如 `-fromlib-all-ai-lib` |
| adapter（適配器） | 內建的工具掛載範本，`init` 時用來產生 mount 設定 |
| behind | 來源已更新、產物還沒重建（該跑 apply） |
| local changes | 掛載側被改了、還沒寫回來源（該跑 promote） |
| untracked | 掛載側新增、manifest 不認識的檔案（apply 會刪掉，要保留請搬回來源） |
| orphan（孤兒連結） | 之前的 apply 建立、但現在設定已不再引用的連結 |

→ 下一章：[安裝說明](01-install.md)
