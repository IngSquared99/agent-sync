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

## 開始前，先認識幾個名詞

後面的說明會一直用到這些詞。先大概有個印象，讀起來就不會卡；中途忘記了，隨時回來這張表查。

| 名詞 | 意思 |
|------|------|
| source（來源） | 你維護的原始指令檔庫，`agsy.yaml` 的 `sources` 陣列，可以有多個 |
| build out（產物） | `apply` 建置出的目錄，預設 `.agsy/`。裡面全是複本，整個可以砍掉重建 |
| mount（掛載） | 在各工具的目錄裡建立指向產物的「連結」 |
| 連結（symlink / junction） | 作業系統的「捷徑」：一個指向別處資料夾的入口，打開它等於打開目標資料夾，**不會多存一份檔案** |
| category（類別） | 指令檔的三種分類：rules（規則）、skills（技能）、workflows（工作流程） |
| bucket（桶子） | 產物裡 `workflows/` 底下按工具分的子資料夾（如 `claude`、`codex`），一個工具掛一桶 |
| routing（分流） | 建置時決定「每份 workflow 要進哪些桶」的過程，依據是檔案裡的 `target` 標記 |
| manifest | `.agsy/.agsy-manifest.json`，建置紀錄檔；agsy 靠它判斷「哪邊被改過」 |
| source tag（來源標籤） | 同名檔案改名保留時，附加在檔名上的來源識別，如 `-fromlib-all-ai-lib` |
| adapter（適配器） | 內建的「各家工具掛載範本」，`init` 時用來產生掛載設定 |
| behind | 來源已更新、產物還沒重建 → 該跑 `apply` |
| local changes（本機改動） | 掛載側被改了、還沒寫回來源 → 該跑 `promote` |
| untracked | 掛載側新增、manifest 不認識的檔案（`apply` 會刪掉；要保留請搬回來源） |
| orphan（孤兒連結） | 之前的 `apply` 建立、但現在設定已不再引用的連結 |

## 三層架構

agsy 的世界只有三層、兩個動作。先看最簡化的版本：

```
 你維護的來源      ── ①建置（複製）──▶    產物目錄      ── ②掛載（連結）──▶   各 AI 工具的目錄
   sources                              .agsy/                            .claude/ .codex/ …
```

- **來源（sources）**：你真正維護、進版控的原始檔。可以有多個，順序代表優先權（越前面越優先）。
- **產物（build.out，預設 `.agsy/`）**：建置出來的成品，**整個目錄視為可重建的拋棄式產出**——`apply` 每次會清空重建，所以不要把手寫的東西直接放進去（除非你打算用 `promote` 寫回來源）。
- **掛載（mount）**：在各工具的目錄裡建立指向產物的連結。工具看到的是連結，實際內容都在 `.agsy/` 裡。

接下來把 ① 和 ② 兩個動作分開看清楚。

### 動作①「建置」：把多個來源合併「複製」成一份

```
 ~/all-ai-lib/rules/python-style.md  ──┐
 ~/all-ai-lib/rules/git-commit.md    ──┤  複製、合併
 ./repo-ai-lib/rules/api-naming.md   ──┘
                                       ▼
                              .agsy/rules/python-style.md
                              .agsy/rules/git-commit.md
                              .agsy/rules/api-naming.md
```

這張圖的重點只有一個：**產物裡的檔案是「複本」**。來源可以散落在好幾個地方（個人共用庫、專案內的庫…），建置後全部集中成一份；也正因為是複本，整個 `.agsy/` 隨時可以砍掉重建，不心疼。

### 動作②「掛載」：建一個「連結」，不是再複製一次

```
 .claude/rules   ────────── 連結（捷徑）──────────▶   .agsy/rules/
 （工具從這裡讀）                                    （檔案實際只存在這裡）
```

這張圖的重點也只有一個：`.claude/rules` **不是一個真的資料夾**，而是一個連結（macOS / Linux 用 symlink，Windows 用 junction）——像捷徑一樣，打開它看到的就是 `.agsy/rules/` 裡的內容。

所以：同一份檔案不會佔兩份空間；`.agsy/` 一更新，所有工具**立刻**看到新內容，不需要再做一次「同步」。

### 把兩個動作串起來：完整樣貌

前面兩個動作都懂了之後，一個掛載完成的專案整體長這樣（看不懂的話，回頭看上面兩張小圖即可）：

```
┌────────────────────┐
│  來源 sources       │  ~/all-ai-lib/       （個人共用庫，跨專案）
│ （你維護的原始檔）    │  ./repo-ai-lib/      （專案內的庫）
└─────────┬──────────┘
          │  ① agsy apply：掃描 → 合併 → 複製
          ▼
┌────────────────────┐
│  產物 build.out     │  .agsy/rules/
│ （可整個重建的複本）  │  .agsy/skills/
│                    │  .agsy/workflows/<bucket>/
└─────────┬──────────┘
          │  ② agsy apply：建立目錄連結（symlink / junction）
          ▼
┌────────────────────┐
│  掛載 mount         │  .claude/rules  → ../.agsy/rules
│ （各 AI 工具實際     │  .claude/skills → ../.agsy/skills
│   讀取的位置）       │  .codex/prompts → ../.agsy/workflows/codex
└────────────────────┘
```

## 三種類別（categories）

指令檔依用途分成三種類別，先弄懂它們各自是什麼：

- **rules（規則）**：長期有效的規範或風格指引——coding style、命名慣例、commit 訊息格式……AI 工具會把它當成「隨時都要遵守的背景守則」讀進去。
- **skills（技能）**：打包好的「一項能力」——一個目錄裝著說明（`SKILL.md`）加上可能附帶的腳本、範本等素材。工具依 `SKILL.md` 裡的描述，在遇到相關任務時自動取用。
- **workflows（工作流程）**：一步一步的操作流程或常用指令——release 流程、code review SOP……在 Claude Code 裡會變成可以直接呼叫的斜線指令（`/release` 這種）。

來源目錄下就用三個同名子目錄存放，格式各有規定：

| 類別 | 來源子目錄（預設） | 格式 | 輸出位置 |
|------|--------------------|------|----------|
| rules | `rules/` | 單一 `.md` 檔 | `.agsy/rules/` |
| skills | `skills/` | **目錄**，內含 `SKILL.md` | `.agsy/skills/` |
| workflows | `workflows/` | 單一 `.md` 檔，開頭可標 `target` | `.agsy/workflows/<bucket>/` |

一個典型的來源長這樣：

```
~/all-ai-lib/
├── rules/
│   ├── python-style.md
│   └── git-commit.md
├── skills/
│   └── code-review/
│       ├── SKILL.md
│       └── scripts/…
└── workflows/
    └── release.md        （front-matter: target: claude）
```

## 目前支援的工具與對應資料夾

掛載之後，三種類別會出現在各工具的哪個位置？內建適配器目前支援三個工具，對應關係如下（細節與自訂方式見[適配器說明](05-adapters.md)）：

| 工具 | 掛載目錄 | rules | skills | workflows |
|------|----------|-------|--------|-----------|
| Claude Code | `.claude/` | `.claude/rules` | `.claude/skills` | `.claude/commands`（成為斜線指令） |
| OpenAI Codex | `.codex/` | —（不掛載） | —（不掛載） | `.codex/prompts` |
| Antigravity | `.agents/` | `.agents/rules` | `.agents/skills` | `.agents/workflows` |

表格裡的每一格都是一個**連結**，實際內容都在 `.agsy/` 裡；Codex 依其慣例只讀 prompts，所以 rules 和 skills 沒有掛載位置（`init` 時若只勾 Codex 會特別警告這件事）。

## 來源目錄必須照規範命名

不管是個人共用庫（`~/all-ai-lib`）還是專案內的庫（`./repo-ai-lib`），**裡面的子目錄都必須叫 `rules/`、`skills/`、`workflows/`**（複數）——agsy 掃描時只認這三個名字，拼錯或用別的名字（如 `rule/`、`my-rules/`）的目錄會被直接跳過，檔案就「神祕消失」了。

```
✔ 正確                          ✘ 掃不到
~/all-ai-lib/rules/…            ~/all-ai-lib/rule/…
~/all-ai-lib/skills/…           ~/all-ai-lib/Skill/…
~/all-ai-lib/workflows/…        ~/all-ai-lib/wf/…
```

> 如果你的既有目錄真的叫別的名字，不必搬家：可以在 `agsy.yaml` 用 `build.categories.<類別>.from` 改掉掃描的子目錄名，詳見[設定檔說明](03-config.md)。不確定有沒有掃到時，跑 `agsy doctor` 立刻見分曉。

## workflows 的分流：bucket 與 routing

rules 和 skills 建置後**所有工具讀同一份**；workflows 不一樣——每個工具的「指令／prompt」格式與用途不一定通用，所以產物裡的 `workflows/` 又按工具分成子資料夾：

```
.agsy/workflows/
├── claude/        ←  Claude Code 專用（.claude/commands 掛的就是這裡）
└── codex/         ←  Codex 專用（.codex/prompts 掛的就是這裡）
```

先把兩個名詞接起來：

- 這些子資料夾就叫 **bucket（桶子）**：一個工具一個桶，各工具只掛載自己的桶。
- 建置時 agsy 決定「每份 workflow 該丟進哪些桶」的過程，就叫 **routing（分流）**。

分流的依據，是 workflow 檔案開頭 front-matter 的 `target` 標記：

```
 workflows/release.md （target: claude）          ──▶  只進 claude 桶 → 只有 Claude Code 看得到
 workflows/deploy.md  （target: [claude, codex]） ──▶  兩個桶各放一份 → 兩個工具都看得到
 workflows/note.md    （沒寫 target）             ──▶  依 route.default 的設定決定去向
```

`target` 的寫法就是在檔案最上方：

```markdown
---
target: claude          # 只給 Claude Code
# 或 target: [claude, codex]  # 兩者都要
---
Release 流程說明…
```

**沒寫 `target` 會怎樣？** 落入設定檔 `route.default` 指定的桶。建議把 default 設成「全部的桶」——一份檔案「到處都出現」比「神祕消失」容易理解得多。細節見[設定檔說明](03-config.md)與[適配器說明](05-adapters.md)。

## 雙向資料流：apply 與 promote

**apply（正向）：來源 → 產物。** 平常的改動都走這條——改來源、跑 apply，所有工具同步更新：

```
 sources（你在這裡改檔案）
    │
    │  agsy apply（重新建置＋掛載）
    ▼
 .agsy/ ──連結──▶  所有工具立刻讀到新內容
```

**promote（反向）：產物 → 來源。** 當你（或 AI 工具）直接改了掛載側的檔案——因為掛載是連結，實際被改到的是 `.agsy/` 裡的複本——就用 promote 把改動收回來源：

```
 .claude/skills/…（AI 工具直接改了這裡的檔案）
    ‖  掛載是連結，所以實際改到的是 .agsy/ 裡的複本
    │
    │  agsy promote（把改動寫回）
    ▼
 sources（改動被保存——下次 apply 重建時才不會被蓋掉）
```

agsy 用 `.agsy/.agsy-manifest.json`（建置紀錄檔）記錄每個項目在建置當下的來源與產物雜湊值，因此 `status` 能精準判斷：哪些是「來源更新了還沒重建」（behind）、哪些是「掛載側被改了還沒寫回」（local changes）、兩邊是否同時被改（需要人工合併）。

## 設計上的安全底線

讀文件時會一直看到這些原則，先列在這裡：

1. **不碰真實檔案**：掛載點若已存在「真實的目錄或檔案」（不是 agsy 建的連結），agsy 一律報錯請你手動處理，絕不代為刪除。
2. **同名策略必須由你明說**：`on_conflict` 每個類別都必填，沒有隱含預設。
3. **刪除前必先確認**：`apply` 會清空產物目錄，若偵測到未寫回的改動一定先問；非互動環境沒有 `--yes` 就取消，絕不硬做。
4. **產物目錄位置有防呆**：`build.out` 只能是專案內的專用目錄，指到家目錄、來源目錄、專案根都會被設定驗證直接擋下。
5. **不收符號連結**：來源裡的 symlink 一律不收（含 skill 目錄內部），避免透過連結把來源以外的檔案夾帶進產物。

→ 下一章：[安裝說明](01-install.md)
