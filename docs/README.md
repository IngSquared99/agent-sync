# agent-sync 是什麼？

**一句話：把你散落各處的 AI 指令文件合併成一份，同步餵給所有 AI 開發工具。**

## 你可能遇過這個痛

如果你同時用超過一個 AI 開發工具，一定碰過這件事——每個工具都規定自己要從哪裡讀指令：

- Claude Code 讀 `.claude/`（rules、skills、commands）
- Antigravity 讀 `.agents/`（rules、skills、workflows）
- 你可能還有一個跨專案共用的個人指令庫（例如 `~/all-ai-lib`）

於是同一份「Python 風格規範」得複製到好幾個地方。改一次要改 N 份，而且它們**一定**會慢慢長得不一樣。

## agent-sync 的解法

你把指令文件放在「來源」裡（共用庫、專案內目錄，要幾個有幾個），然後 `agsy` 做兩件事：

```
【來源層】原稿，你唯一要編輯的地方
    ~/all-ai-lib/      ← 跨專案共用庫
    ./repo-ai-lib/     ← 專案自己的庫
         │
         │  build =「複製 + 合併」，來源永遠不會被工具直接碰
         ▼
【產物層】合併結果
    .agsy/             ← 隨時可以砍掉重建的副本
         │
         │  mount =「建立目錄連結」
         ▼
【掛載層】工具實際讀取的地方（裡面是連結，不是真檔案）
    .claude/rules  ──→ 指向 .agsy/rules
    .claude/skills ──→ 指向 .agsy/skills
    .agents/rules  ──→ 指向 .agsy/rules
    .agents/workflows → 指向 .agsy/workflows/agents
```

之後的日常只有三招：

| 情況 | 指令 |
|---|---|
| 我改了來源 | `agsy apply` |
| AI 工具改了掛載端的檔案 | `agsy promote` 寫回（完成後來源與產物即一致） |
| 搞不清楚現在什麼狀態 | `agsy status` |

## 為什麼不是「又一個複製腳本」？

- **真正的多來源合併**：`sources` 是有序陣列，共用庫、團隊庫、專案庫可以疊著用，順序就是優先級；同名衝突由你選策略（兩份都留、只留高優先、或直接報錯），絕不默默幫你決定。
- **中間多一層，來源永遠乾淨**：AI 工具改到掛載檔案時，髒的是可拋棄的 `.agsy/` 產物層，你的原稿結構上不可能被污染。想留的改動用 `promote` 接回家。
- **逐檔分流**：workflow 檔可以標記「只給 Claude」「只給 Antigravity」或「兩邊都要」，build 時自動分派。
- **看得見的狀態**：`agsy status` 把「哪邊落後、哪邊被改」分方向講清楚，還能當 CI 檢查（exit 0 = 同步）。

## 下一步

1. [安裝](install.md) —— 一行指令裝好
2. [五分鐘上手](quickstart.md) —— 從零到第一次同步
3. [三層架構，一次看懂](concepts.md) —— 想理解它為什麼這樣設計

> 名稱對照：專案叫 **agent-sync**，指令叫 **`agsy`**，設定檔叫 **`agsy.yaml`**，建置產物在 **`.agsy/`**。
