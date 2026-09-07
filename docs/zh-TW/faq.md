# FAQ

| 區段 | 問題 | 什麼時候看 |
|------|------|-----------|
| [概念與日常使用](#概念與日常使用) | Q1–Q6 | 剛上手 |
| [我的檔案怎麼了](#我的檔案怎麼了) | Q7–Q12 | 檔案沒被收、被改名、可能被刪 |
| [工具與格式](#工具與格式) | Q13–Q17 | 各家工具怎麼看到產物 |
| [平台與環境](#平台與環境) | Q18–Q24 | Windows、CI、移除、安全 |
| [hooks](#hooks) | Q25–Q32 | 守衛腳本 |

## 概念與日常使用

### Q1：我直接改了 `.claude/rules/` 裡的檔案，下次 apply 會蓋掉嗎？

會，但會先問。`.claude/rules` 是指向 `.agsy/rules` 的連結，你改到的是產物副本。`apply` 會列出這筆改動並要求確認。想保留：把改動合併進來源檔（status 會指名），再 apply。

### Q2：哪些東西要進版控？

`agsy.yaml` 是專案的同步設定，建議進版控。`.agsy/`、`.agsy.lock`、各連結、根目錄 `AGENTS.md` 都是產生出來的，要不要進版控由你決定；`init` 會問你要不要把它們加進 `.gitignore`。

### Q3：改了來源之後，工具什麼時候看得到？

跑 `agsy apply` 之後。agsy 不是常駐程式，不會監看檔案。

### Q4：`plan`、`status`、`doctor` 差在哪？

- `doctor`：環境健康。設定有效嗎、來源存在嗎、建得了連結嗎。
- `plan`：這次建置的預覽。會收什麼、轉什麼、誰被改名、誰被略過、每條連結會怎樣。
- `status`：現況與上次 apply 的差異。哪些來源變了（清單 A）、產物端被改了什麼（清單 B）、連結正常嗎。

三個都是唯讀。

### Q5：可以從專案的子資料夾執行嗎？

可以。除了 `init`，每個指令都會往上找 `agsy.yaml`。`init` 只看目前資料夾，設定檔建在你站的位置。

### Q6：多個專案能共用一個來源庫嗎？

可以。每個專案有自己的 `agsy.yaml`，`sources` 指向同一個 `~/all-ai-lib`。

## 我的檔案怎麼了

### Q7：我的 skill 為什麼沒被收？

`agsy doctor` 或 `agsy plan` 會說明原因。規則：skill 必須是資料夾；裡面要有 `SKILL.md`；資料夾內不可有任何符號連結；名稱不可以 `.` 開頭。

### Q8：rules 資料夾裡的檔案為什麼沒被收？

rules 和 workflows 只收單一 `.md` 檔：資料夾、其他副檔名、`.` 開頭的名稱、符號連結都不收。原因會出現在 `plan` 的排除清單。

### Q9：status 說有產物端改動、apply 會刪掉它們，怎麼辦？

那些是透過連結修改或新增的檔案（通常是 AI 工具做的）。它們住在可重建的產物裡，下次 apply 會捨棄。想保留：搬進或合併進 status 指名的來源，再 apply。這個手動步驟就是審核 AI 產出內容的關卡。

### Q10：檔名裡的 `-fromlib-xxx` 是什麼？

`rename` 策略加上的來源標記。`python-style.md` 同時存在於兩個來源時，產物是 `python-style-fromlib-all-ai-lib.md` 與 `python-style-fromlib-repo-ai-lib.md`，兩份都留，看檔名就知道來自哪裡。skill 的 `SKILL.md` 裡的 `name` 也會跟著改。

### Q11：我的 workflow 為什麼沒出現在某家工具？

用 `plan` 檢查：`target:` 可能沒列那家；`target:` 寫錯字會擋下建置；那家也要掛對應的產物（Claude / Codex / Cursor 掛 `skills`，Antigravity 掛 `workflows`）。

### Q12：我刪了 `.agsy/` 裡的東西（或整個資料夾）？

產物本來就是可重建的，跑一次 `agsy apply` 全部復原。manifest 也壞了的話，apply 會多問一次確認。

## 工具與格式

### Q13：workflow 為什麼變成 skill？

Claude Code、Codex、Cursor 都是用 skills 機制讀流程：`SKILL.md` 資料夾加上決定誰能觸發的檔頭欄位。agsy 把每個單檔 workflow 打包成那個形態並設 `disable-model-invocation: true`，在遵守此欄位的工具裡只有人能用 `/名稱` 執行。來源維持一個簡單的 `.md`。

### Q14：`.agents/workflows/` 裡的轉接頭是什麼？

Antigravity 從那個資料夾用 `/名稱` 觸發 workflow。轉接頭讓 `/名稱` 有效，內容只存在一份（skill 形態）：它指示 AI 載入對應的 skill 並照著做。`target:` 只有 `antigravity` 的 workflow 沒有 skill 形態，那個位置放完整內容。

### Q15：為什麼 `.claude/rules/` 和根目錄 `AGENTS.md` 有同一批規則？

不同工具讀不同形態。Claude Code 讀逐檔的 `.claude/rules/`，不讀 `AGENTS.md`；Codex、Cursor、Antigravity 讀併成一檔的 `AGENTS.md`。兩者由同一批來源產生。

### Q16：我專案本來就有自己的 `AGENTS.md`，會怎樣？

agsy 不會合併或覆蓋真實檔案：`init` 提醒、`apply` 拒絕，直到你決定：把內容搬進某個來源的 `rules/`（之後所有工具都讀得到），或改名保存。

### Q17：AI 會不會自己觸發部署 workflow？

在 Claude Code 與 Cursor 不會：skill 帶著 `disable-model-invocation: true`，只有人打 `/名稱` 才會執行。Codex 與 Antigravity 忽略這個欄位，模型可能自行執行，有副作用的流程請留意。

## 平台與環境

### Q18：Windows 需要管理員權限嗎？

不需要。資料夾用 junction、檔案用 hard link，一般帳號都建得了。junction 記的是絕對路徑，搬移專案後重跑 `agsy apply`。

### Q19：新機器上 status 出現大量警告？

整個來源路徑不存在時，status 會標示「來源路徑不存在」，通常是共用庫沒 clone、外接碟沒掛、或路徑打錯。此時 `apply` 也會拒絕。修好路徑再繼續。

### Q20：能用在 CI 或 git hook 嗎？

可以：

- `agsy status`：結束碼 `0` = 一致、`1` = 有落差，直接當檢查用。
- 動作型指令加 `--yes`（例如 `agsy apply --yes`）；沒加時，無互動環境的確認一律取消。
- `agsy init --yes <sources...>`：無互動初始化。

### Q21：怎麼切換介面語言？

`export AGSY_LANG=zh-TW`（任何 `zh` 開頭的值）= 繁體中文；`AGSY_LANG=en` = 英文。沒設時看系統的 `LC_ALL` / `LANG`。所有 `zh` 開頭的值（含 `zh-CN`）都對應繁體中文介面。

### Q22：怎麼徹底移除 agsy？

1. 在每個專案跑 `agsy clean`。
2. 不要 `agsy.yaml` 的話手動刪。
3. 依安裝方式移除執行檔：`brew uninstall agsy`、`winget uninstall IngSquared99.agsy`、或刪 `~/go/bin/agsy`。

### Q23：status 說有「孤兒連結」？

先前 apply 建立、之後你把那家工具移出 mount 設定的連結。工具仍透過它讀舊內容，所以 status 持續提醒。`apply` 不刪它；手動移除，或 `agsy clean` 一起清。從設定移除的 merge 目標（Claude Code 的 `settings.json`）回報方式相同。

### Q24：agsy 會不會把來源裡符號連結指到的檔案（例如私鑰）複製出去？

不會。掃描不收符號連結（含 skill 與 hook 資料夾內部），複製階段再擋一次。

## hooks

### Q25：hook 是什麼？是 webhook 嗎？

不是。AI 工具在你送出一句話之後會跑一個迴圈：思考 → 執行工具 → 結果回到模型 → …。hook 是掛在這個迴圈固定位置的檢查點（例如「工具執行前」`PreToolUse`、「準備停下前」`Stop`）：AI 走到那裡會暫停，在本機執行你的腳本，把現況以 JSON 餵給它；腳本結束碼 2 就擋下、0 就放行。全程在本機。webhook 是「某服務發生事件時對你的網址發 HTTP 請求」，只是名字裡都有 hook。

### Q26：rules 已經寫了「禁止 rm -rf」，為什麼還要 hook？

rules 是給模型讀的文字，遵不遵守有機率。hook 是程式，擋下就是擋下。能寫成「如果…就不准」的用 hook；風格與偏好用 rules。

### Q27：為什麼 Claude Code 用 merge，其他三家用連結？

Codex、Antigravity、Cursor 都有獨立的 `hooks.json`，整檔用連結最乾淨。Claude Code 的 hooks 寫在 `.claude/settings.json` 的 `hooks` 欄位，那份檔案還裝著你的 permissions、model 等設定，所以 agsy 只把自己的條目合併進去。辨識方式有兩條：handler 的 `statusMessage` 以 `agsy:` 開頭，或 command 指向 `.agsy/hooks/`。merge 目標必須在專案內，`~/.claude/settings.json` 留給你自己的 hooks。

### Q28：我自己的 hooks 要放哪？

各家的個人層檔案，與專案層疊加：Claude Code 放 `.claude/settings.local.json` 或 `~/.claude/settings.json`；Codex 放 `~/.codex/hooks.json` 或 `config.toml`；Cursor 放 `~/.cursor/hooks.json`；Antigravity 放 `~/.gemini/config/`（官方未載明合併方式，請實測）。

### Q29：一支腳本能四家通用嗎？

登記表可以（agsy 翻譯事件名、結構、路徑），腳本內容要自己處理差異：四家餵給腳本的 JSON 欄位不同，matcher 裡「shell 工具」的名字也不同（Bash / Bash / run_command / Shell，用 `overrides` 指定）。腳本先看 `hook_event_name` 或各家欄位再判斷。

### Q30：hook 在某家沒生效？

`agsy plan` 在每個 hook 下面列出 `claude ✓  codex ✓  antigravity —  cursor ✓` 與原因：該家沒有這個時機（Antigravity 只有五個事件）、不支援這種 handler 型別（Codex 沒有 `http`）、或 `target:` 沒列它。另外有些工具要求專案層 hooks 先被「信任」（工作區信任對話框），第一次啟動時記得同意。

### Q31：Windows 上腳本跑不起來？

登記表裡的路徑是絕對路徑、需要時有加引號，但 `.sh` 在 Windows 不能直接執行。在 `hook.yaml` 用 `command: python3 ./check.py` 指定直譯器，或用 `overrides.codex.hooks[0].commandWindows` 之類各家自己的欄位。

### Q32：apply 說 `build.on_conflict.hooks 未設定`？

hooks 的衝突策略和其他類別一樣必填。在 `agsy.yaml` 的 `on_conflict` 加一行 `hooks: error`。要讓四家都收到 hooks，重跑 `agsy init` 讓 adapter 補上 `.codex`、`.cursor`、`.agents/hooks.json` 與 `.claude` 的 `merge`，或照[設定檔](config.md)手動加。

→ 回到：[核心概念](overview.md)
