# FAQ

| 區段 | 問題 | 什麼時候看 |
|------|------|-----------|
| [概念與日常使用](#概念與日常使用) | Q1–Q6 | 剛上手、對日常流程有疑問 |
| [我的檔案怎麼了](#我的檔案怎麼了) | Q7–Q12 | 檔案沒被收錄、被改名、可能被刪 |
| [工具與格式](#工具與格式) | Q13–Q17 | 各 AI 工具怎麼看到產物 |
| [平台與環境](#平台與環境) | Q18–Q24 | Windows、CI、移除、安全性 |

---

## 概念與日常使用

### Q1：我直接改了 `.claude/rules/` 裡的檔案——下次 apply 會蓋掉嗎？

`.claude/rules` 是指向 `.agsy/rules` 的連結；你改到的是產物副本，重建確實會重新產生它。但在那之前，`apply` 會偵測到改動並**列出、要求確認**——絕不無聲覆蓋。想保留：把改動合併進來源檔（status 會指名），再 `apply`。

### Q2：`.agsy/` 要進版控嗎？agsy.yaml 呢？

- `.agsy/`、各掛載連結、根目錄 `AGENTS.md`——**不要**：全是可重建的產物；`init` 會主動提議加進 `.gitignore`。
- `agsy.yaml`——**要**：它是專案的同步設定。

### Q3：改了來源之後，工具什麼時候看得到新內容？

`agsy apply` 跑完之後。agsy 不是常駐程式、不監看檔案——改完記得 apply（git hook 可以幫你提醒：`agsy status` 的非零 exit code）。

### Q4：`plan`、`status`、`doctor` 差在哪？

- `doctor`：**環境**健康——設定有效嗎、來源存在嗎、建得了連結嗎？
- `plan`：**這次建置**的完整彩排——會收什麼、轉什麼、誰被改名、誰被略過、每條連結會怎樣。
- `status`：**現況對基準**——哪些來源變了（清單 A）、掛載端被改了什麼（清單 B）、連結健康嗎？

三個都是唯讀，隨時可跑。

### Q5：可以從專案子目錄執行 agsy 嗎？

可以。除了 `init`，每個指令都像 git 一樣向上找 `agsy.yaml`。`init` 刻意只看目前目錄——設定檔建在哪由你站的位置決定。

### Q6：同一台機器的多個專案能共用一個來源庫嗎？

可以——這正是核心設計。每個專案有自己的 `agsy.yaml`，sources 指向同一個 `~/all-ai-lib`。

---

## 我的檔案怎麼了

### Q7：我的 skill 為什麼沒被收錄？

對照收錄規則（`agsy doctor` 或 `agsy plan` 會直接說明原因）：skill 必須是**目錄**；目錄裡必須有 `SKILL.md`；目錄內不得有**任何符號連結**（安全規則；整個 skill 略過）；名稱不能以 `.` 開頭。

### Q8：rules 目錄裡的檔案為什麼沒被收錄？

rules / workflows 只收**單一 `.md` 檔**：目錄、非 `.md` 副檔名、點開頭的名稱、symlink 一律拒收。每個原因都會出現在 `plan` 的排除清單。

### Q9：status 回報產物端改動、apply 會刪掉它們——怎麼辦？

那些是（通常是 AI 工具）透過掛載修改或新增的檔案。它們住在可重建的產物裡，下次 apply 會捨棄。**想保留：把它搬入或合併進 status 指名的來源，再跑 `apply`。** 這個手動步驟就是 AI 產出內容的審核關卡。

### Q10：檔名裡的 `-fromlib-xxx` 是什麼？

`rename` 衝突策略附加的**來源標記**：`python-style.md` 同時存在於兩個來源時，產出是 `python-style-fromlib-all-ai-lib.md` 與 `python-style-fromlib-repo-ai-lib.md`——兩份都留、來源看檔名即知。skill 連 front matter 的 `name` 也會一併改寫。

### Q11：我的 workflow 為什麼沒出現在某個工具？

用 `plan` 檢查：它的 `target:` 可能列了不含該工具的清單；`target:` 打錯字是會擋下建置的錯誤；該工具也要有掛對應的輸出（Claude / Codex / Cursor 掛 `skills`、Antigravity 掛 `workflows`）。

### Q12：我不小心刪了 `.agsy/` 裡的東西（或整個目錄）？

產物設計上即可重建：一次 `agsy apply` 全部復原。若 manifest 也壞了，apply 會多問一次確認再重建。

---

## 工具與格式

### Q13：workflow 為什麼會變成 skill？

Claude Code、Codex、Cursor 都透過 skills 機制讀取流程——`SKILL.md` 目錄加上決定誰能觸發的 front matter。build 把每個單檔 workflow 打包成那個形態並設定 `disable-model-invocation: true`，在遵守此欄位的工具裡只有人能以 `/名稱` 執行。你的來源維持單純的 markdown 檔。

### Q14：`.agents/workflows/` 裡的轉接頭是什麼？

Antigravity 從那個目錄以 `/名稱` 觸發 workflow。轉接頭讓 `/名稱` 繼續有效，而內容只存在一份（skill 形態）：它指示 agent 載入對應 skill 並照著執行。`target:` 只有 `antigravity` 的 workflow 沒有 skill 形態，該位置改放完整內容（`target:` 欄位移除）。

### Q15：為什麼 `.claude/rules/` 和根目錄 `AGENTS.md` 有同一批 rules？

不同工具讀不同形態。Claude Code 讀逐檔的 `.claude/rules/`、不讀 `AGENTS.md`；Codex、Cursor、Antigravity 讀串接的單檔 `AGENTS.md`。兩者由同一批來源產生，永不分歧——而且都是唯讀。

### Q16：我專案本來就有自己的 AGENTS.md——會怎樣？

agsy 絕不合併或覆蓋真實檔案：`init` 立即提醒、`apply` 拒絕執行，直到你做出決定——把內容搬進某個來源的 `rules/`（之後所有工具都讀得到），或改名保存、讓它留在 agsy 管理之外。

### Q17：AI 工具會不會自己觸發部署 workflow？

在 Claude Code 與 Cursor 不會：skill 形態帶著 `disable-model-invocation: true`，只有人打 `/名稱` 才會執行。Codex 與 Antigravity 忽略該欄位，模型可能自行決定執行——有副作用的流程請自行留意。

---

## 平台與環境

### Q18：Windows 需要管理員權限或開發人員模式嗎？

不需要。agsy 目錄掛載用 **junction**、根目錄 `AGENTS.md` 用 **hard link**，一般帳號都建得了。唯一注意：junction 存**絕對路徑**——搬移專案後請重跑 `agsy apply`（macOS / Linux 用相對 symlink，不受影響）。

### Q19：新機器（或共用庫尚未 clone）時 status 出現大量警告？

整個來源路徑不存在時，status 會把「**來源路徑不存在**」與「來源檔被刪」明確分開：前者通常是共用 repo 沒 clone、外接碟沒掛、或路徑打錯。此狀態下 `apply` 也會拒絕重建。修好路徑再繼續。

### Q20：能用在 CI 或 git hook 嗎？

可以，本來就是這樣設計的：

- `agsy status`：exit `0`＝一致、`1`＝有落差——直接當檢查用。
- 動作型指令加 `--yes`（例如 `agsy apply --yes`）；未加時，非互動環境的確認一律**取消**，不會有東西被意外破壞。
- `agsy init --yes <sources...>`：非互動初始化。

### Q21：怎麼切換介面語言？

`export AGSY_LANG=zh-TW`（任何 `zh` 開頭的值）＝繁體中文；`AGSY_LANG=en`＝英文。沒設 `AGSY_LANG` 時看系統的 `LC_ALL` / `LANG`。注意**所有** `zh` 開頭的值——包含 `zh-CN`、`zh-Hans`——目前一律對應繁體中文（zh-TW）介面；尚未提供簡體中文翻譯。

### Q22：怎麼徹底移除 agsy？

1. 在每個專案跑 `agsy clean`（移除連結、根目錄 `AGENTS.md` 連結與 `.agsy/`；只刪 agsy 建立的東西——實體檔案會略過並回報）。
2. 不要 `agsy.yaml` 的話手動刪除。
3. 依安裝方式移除執行檔：`brew uninstall agsy` / `winget uninstall IngSquared99.agsy` / Go 安裝的刪 `~/go/bin/agsy`。

### Q23：status 回報「孤兒連結」——那是什麼？

先前 apply 建立、之後你把該工具移出 mount 設定的連結。工具仍透過它讀舊內容，所以 status 持續提醒。`apply` 不會刪它們；手動移除，或 `agsy clean` 連同其他東西一起清。

### Q24：agsy 會不會把來源裡符號連結指到的檔案（例如私鑰）複製出去？

不會。掃描一律不收符號連結（含 skill 目錄內部——含連結的 skill 整個略過），複製階段還有第二道防線直接拒絕。產物會被掛載給所有工具讀，連結絕不能夾帶來源以外的檔案。

→ 回到：[核心概念](overview.md)
