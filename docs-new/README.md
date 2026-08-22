# agent-sync（agsy）說明文件

這份資料夾是 agsy 的完整使用說明，依「安裝 → 快速上手 → 深入教學 → 疑難排解」的順序編排。
可以直接依序閱讀，也可以用瀏覽器開啟 [index.html](index.html) 以網頁版閱讀全部內容。

## 目錄

| 章節 | 檔案 | 內容 |
|------|------|------|
| 0. 核心概念 | [00-overview.md](00-overview.md) | agsy 是什麼、解決什麼問題、三層架構與資料流 |
| A. 安裝說明 | [01-install.md](01-install.md) | macOS / Linux / Windows 安裝、驗證、升級、移除 |
| B. 快速上手 | [02-quickstart.md](02-quickstart.md) | 四步完成第一次同步（附 CLI 畫面示意）、指令速查表 |
| C-1. 設定檔 | [03-config.md](03-config.md) | agsy.yaml 每個欄位的完整說明與安全規則 |
| C-2. 指令說明 | [04-commands.md](04-commands.md) | 指令總覽 + 各指令細節與使用情境 |
| C-3. 適配器 | [05-adapters.md](05-adapters.md) | 內建適配器（Claude Code / Codex / Antigravity）與自訂掛載 |
| D. Q&A | [06-faq.md](06-faq.md) | 以使用者角度整理的常見問題 |

## 建議閱讀路徑

- **第一次使用**：00 → 01 → 02，跑完 `init → plan → apply` 後再回頭看 03、04。
- **想搞懂設定檔每一行**：03。
- **某個指令的行為跟預期不同**：04 對應指令的小節，以及 06 的 Q&A。
- **想接新的 AI 工具**：05。
