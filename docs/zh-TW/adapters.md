# Adapters

adapter 是單一 AI 工具的內建掛載預設：它讀哪個目錄、該看到產物的哪些部分。`agsy init` 會把你勾選的 adapter 轉成 `agsy.yaml` 的 `mount` 區段（以及 `build.tools` 清單）。adapter 是出廠模板、不是執行期相依——產生出來的設定隨你編輯。

## 內建 adapters

| Adapter | 掛載 | rules 走 | 備註 |
|---------|------|----------|------|
| `claude`（Claude Code） | `.claude/rules → rules`、`.claude/skills → skills` | 逐檔的 `.claude/rules/` | Claude Code 不讀 `AGENTS.md`；workflow 以 skill 形態抵達，用 `/名稱` 觸發 |
| `codex`（OpenAI Codex） | `.agents/skills → skills` | 根目錄 `AGENTS.md` | Codex 專案層只讀 `AGENTS.md` 與 `.agents/skills/` |
| `antigravity`（Google Antigravity） | `.agents/skills → skills`、`.agents/workflows → workflows` | 根目錄 `AGENTS.md` | 不掛 `.agents/rules`——Antigravity 讀 `AGENTS.md`，再掛 rules 目錄會讓每條規則重複兩次。`/名稱` 執行轉接頭，由它載入 skill |
| `cursor`（Cursor） | `.agents/skills → skills` | 根目錄 `AGENTS.md` | Cursor 原生讀 `.agents/skills/`，與其他工具共用 `.agents` 掛載 |

勾選 `codex`、`antigravity`、`cursor` 任一個，init 就會加上根目錄掛載：123

```yaml
  - dir: .
    links:
      AGENTS.md: AGENTS.md
```

共用同一目錄的 adapter 會合併成一個 mount 條目——同時勾 Codex、Antigravity、Cursor 只會產生一個 `.agents` 區塊。

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

`init` 的編輯模式會原樣保留自訂條目。連結可指向各類別的 `to` 值或 `AGENTS.md`；驗證規則見[設定檔](config.md)。

## 新增內建 adapter

adapter 位於 agsy 原始碼的 `adapters/` 目錄，一個工具一個 YAML 檔：

```yaml
name: newtool
display: New Tool
needs_agents_md: true      # 該工具讀根目錄 AGENTS.md 時設定
mount:
  dir: .newtool
  links:
    skills: skills
```

放入檔案、重新建置，`init` 就會出現這個新工具。

→ 下一章：[情境指南](scenarios.md)
