# Adapters

adapter 是單一 AI 工具的內建掛載預設：它讀哪個目錄、該看到產物的哪些部分。`agsy init` 會把你勾選的 adapter 轉成 `agsy.yaml` 的 `mount` 區段（以及 `build.tools` 清單）。adapter 是出廠模板、不是執行期相依——產生出來的設定隨你編輯。

## 內建 adapters

| Adapter | 掛載 | rules 走 | hooks 走 | 備註 |
|---------|------|----------|----------|------|
| `claude`（Claude Code） | `.claude/rules → rules`、`.claude/skills → skills`、`.claude/settings.json ⇐ hooks.claude.json`（merge） | 逐檔的 `.claude/rules/` | `settings.json` 的 `hooks` 鍵 | Claude Code 不讀 `AGENTS.md`；hooks 沒有獨立檔，所以用 merge；workflow 以 skill 形態抵達，用 `/名稱` 觸發 |
| `codex`（OpenAI Codex） | `.agents/skills → skills`、`.codex/hooks.json → hooks.codex.json` | 根目錄 `AGENTS.md` | `.codex/hooks.json` | 兩個 mount 目錄：skills 走共用的 `.agents`，hooks 走 Codex 自己的 `.codex` |
| `antigravity`（Google Antigravity） | `.agents/skills → skills`、`.agents/workflows → workflows`、`.agents/hooks.json → hooks.antigravity.json` | 根目錄 `AGENTS.md` | `.agents/hooks.json` | 不掛 `.agents/rules`——Antigravity 讀 `AGENTS.md`，再掛 rules 目錄會讓每條規則重複兩次。`/名稱` 執行轉接頭，由它載入 skill；hooks 就在已掛的 `.agents` 裡 |
| `cursor`（Cursor） | `.agents/skills → skills`、`.cursor/hooks.json → hooks.cursor.json` | 根目錄 `AGENTS.md` | `.cursor/hooks.json` | Cursor 原生讀 `.agents/skills/`，與其他工具共用 `.agents` 掛載；hooks 走 `.cursor` |

勾選 `codex`、`antigravity`、`cursor` 任一個，init 就會加上根目錄掛載：

```yaml
  - dir: .
    links:
      AGENTS.md: AGENTS.md
```

共用同一目錄的 adapter 會合併成一個 mount 條目——同時勾 Codex、Antigravity、Cursor 只會產生一個 `.agents` 區塊（links 與 merge 都會合併）。一個 adapter 可以有多個 mount 目錄：Codex 與 Cursor 各自多一個放 hooks 的目錄。

## hooks 的四種登記位置

四家的 hook 機制相同（掛在 agent 生命週期的檢查點上執行你的腳本），差別全在登記表的位置與格式。三家有獨立檔，走檔案連結；只有 Claude Code 把 hooks 放在還裝著其他設定的 `settings.json`，所以走 merge。build 會依各家方言翻譯：Claude / Codex 同構、Antigravity 外包一層 hook 名、Cursor 事件名不同且結構攤平。細節見[設定檔](config.md)的 `hook.yaml` 一節。

## 共用的 .agents 目錄

`.agents/skills/` 是正在成形的跨工具慣例：Codex、Antigravity、Cursor 都原生讀取。agsy 順勢利用——一條連結服務三個工具，每個 workflow 的 skill 形態三家都拿得到。

兩個值得知道的工具差異：

- 每個 workflow 的 skill 形態都帶 `disable-model-invocation: true`，Claude Code 與 Cursor 會遵守：只有人能觸發流程。Codex 與 Antigravity 忽略此欄位——在那兩個工具裡，模型可能自行決定執行 workflow，有副作用的流程請自行留意。
- 在 Antigravity 裡，`/名稱` 執行的是轉接頭，由它指示 agent 載入對應 skill。多了一層間接；極少數情況 agent 可能不照做。

## 自訂掛載

清單外的工具可以手寫 mount 條目：

```yaml
mount:
  - dir: .someothertool
    links:
      skills: skills
```

`init` 的編輯模式會原樣保留自訂條目。連結可指向各類別的 `to` 值、`AGENTS.md` 或某份 hook 登記表；驗證規則見[設定檔](config.md)。自訂工具目前拿不到 hook 登記表：方言表在 agsy 內部（`internal/build/hooks.go`），登記表只為四家內建工具產生。

## 新增內建 adapter

adapter 位於 agsy 原始碼的 `adapters/` 目錄，一個工具一個 YAML 檔：

```yaml
name: newtool
display: New Tool
needs_agents_md: true      # 該工具讀根目錄 AGENTS.md 時設定
mounts:                    # 可以有多個目錄
  - dir: .newtool
    links:
      skills: skills
```

放入檔案、重新建置，`init` 就會出現這個新工具。要讓它也拿到 hook 登記表，還需要在 `internal/build/hooks.go` 的方言表加一筆（事件對照、支援的 handler type、登記表形狀），並在 `internal/config` 的 `HookTools` / `HookRegistryFiles` 登記檔名。

→ 下一章：[情境指南](scenarios.md)
