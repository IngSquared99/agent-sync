# Adapters

adapter 是一家工具的內建掛載設定：它讀哪個資料夾、要看到產物的哪些部分。`agsy init` 把你勾選的 adapter 轉成 `agsy.yaml` 的 `mount` 區段和 `build.tools`。adapter 是範本，產生出來的設定隨你改。

## 四家內建 adapter

| Adapter | 掛載 | rules 從哪讀 | hooks 從哪讀 |
|---------|------|-------------|-------------|
| `claude`（Claude Code） | `.claude/rules → rules`、`.claude/skills → skills`、`.claude/settings.json ⇐ hooks.claude.json`（merge） | `.claude/rules/` 逐檔 | `settings.json` 的 `hooks` 欄位 |
| `codex`（OpenAI Codex） | `.agents/skills → skills`、`.codex/hooks.json → hooks.codex.json` | 根目錄 `AGENTS.md` | `.codex/hooks.json` |
| `antigravity`（Google Antigravity） | `.agents/skills → skills`、`.agents/workflows → workflows`、`.agents/hooks.json → hooks.antigravity.json` | 根目錄 `AGENTS.md` | `.agents/hooks.json` |
| `cursor`（Cursor） | `.agents/skills → skills`、`.cursor/hooks.json → hooks.cursor.json` | 根目錄 `AGENTS.md` | `.cursor/hooks.json` |

幾個細節：

- Claude Code 不讀 `AGENTS.md`，所以另外掛逐檔的 `.claude/rules/`。它的 hooks 沒有獨立檔，用 merge。workflow 以 skill 的形態到達，用 `/名稱` 觸發。
- Antigravity 不掛 `.agents/rules`：它讀 `AGENTS.md`，再掛一份會讓每條規則出現兩次。`/名稱` 執行的是轉接頭，由它載入 skill。
- Codex 與 Cursor 各有兩個資料夾：skills 走共用的 `.agents`，hooks 走自己的 `.codex` / `.cursor`。

勾了 `codex`、`antigravity`、`cursor` 任一個，init 會多加根目錄的掛載：

```yaml
  - dir: .
    links:
      AGENTS.md: AGENTS.md
```

共用同一個資料夾的 adapter 會合併成一個 mount 條目：同時勾三家只會產生一個 `.agents` 區塊。

## 共用的 `.agents` 資料夾

```
 .agents/skills/  ◀── Codex
                  ◀── Antigravity
                  ◀── Cursor
```

三家都原生讀 `.agents/skills/`，一條連結服務三家。每個 workflow 的 skill 形態三家都拿得到。

兩個工具差異：

- workflow 的 skill 形態帶 `disable-model-invocation: true`。Claude Code 與 Cursor 遵守：只有人能觸發。Codex 與 Antigravity 忽略此欄位，模型可能自行執行，有副作用的流程請留意。
- Antigravity 的 `/名稱` 執行的是轉接頭，多一層間接；極少數情況 AI 可能不照做。

## hooks 的四種登記位置

四家的 hook 機制相同（在 AI 動作的固定時機執行你的腳本），差別在登記表的位置與格式：

| 工具 | 登記表 | 掛載方式 | 格式 |
|------|--------|---------|------|
| Claude Code | `.claude/settings.json` 的 `hooks` 欄位 | merge | 事件 → 群組 → handler |
| Codex | `.codex/hooks.json` | 連結 | 與 Claude 相同 |
| Antigravity | `.agents/hooks.json` | 連結 | 外面多包一層 hook 名 |
| Cursor | `.cursor/hooks.json` | 連結 | 事件名不同、結構攤平 |

翻譯細節見[設定檔](config.md)的 `hook.yaml` 一節。

## 自訂掛載

四家以外的工具可以手寫 mount 條目：

```yaml
mount:
  - dir: .someothertool
    links:
      skills: skills
```

`init` 的編輯模式會保留自訂條目。連結可指向各類別的 `to`、`AGENTS.md` 或某份登記表。自訂工具拿不到自己的 hook 登記表：登記表只為四家內建工具產生。

## 新增內建 adapter（給開發者）

adapter 在原始碼的 `adapters/` 資料夾，一家一個 YAML：

```yaml
name: newtool
display: New Tool
needs_agents_md: true      # 該工具讀根目錄 AGENTS.md 時設定
mounts:
  - dir: .newtool
    links:
      skills: skills
```

放入檔案、重新建置，`init` 就會出現這家工具。要讓它也有 hook 登記表，還要在 `internal/build/hooks.go` 的方言表加一筆（事件對照、支援的 handler 型別、登記表格式），並在 `internal/config` 登記檔名。

→ 下一章：[情境指南](scenarios.md)
