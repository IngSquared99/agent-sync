# agent-sync（agsy）說明文件

agsy 的完整指南，依「安裝 → 快速上手 → 深入 → 疑難排解」排列。
左側側欄可瀏覽章節；右上搜尋框涵蓋全文。在 GitHub 上閱讀時請用下表的檔案連結。

## 目錄

| 章節 | 內容 |
|------|------|
| [核心概念](overview.md) | agsy 是什麼、單向資料流、三層架構與三種類別 |
| [安裝](install.md) | Homebrew / winget / 原始碼安裝、介面語言 |
| [快速上手](quickstart.md) | 四步完成第一次同步、指令速查 |
| [設定檔](config.md) | agsy.yaml 每個欄位與安全規則 |
| [指令參考](commands.md) | 指令總覽與逐指令細節 |
| [Adapters](adapters.md) | 內建 adapter（Claude Code / Codex / Antigravity / Cursor）與自訂掛載 |
| [情境指南](scenarios.md) | 各種變更組合下 apply 的行為 |
| [FAQ](faq.md) | 以使用者視角整理的常見問題 |

## 建議閱讀路徑

- **第一次用**：核心概念 → 安裝 → 快速上手；跑一輪 `init → plan → apply`，再回頭看設定檔與指令參考。
- **想搞懂設定檔每一行**：設定檔。
- **某個指令的行為出乎意料**：對應的指令參考段落、情境指南、FAQ。
- **要接新的 AI 工具**：Adapters。
