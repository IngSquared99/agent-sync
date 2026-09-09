# 指令參考

## 總覽

```
agsy                        選單（附狀態摘要）
agsy init [sources...]      產生或編輯 agsy.yaml
agsy doctor                 環境健檢（唯讀）
agsy plan                   預覽 apply 會做的事（唯讀）
agsy apply                  檢查 → 確認 → 重建產物 → 掛載
agsy status                 回報落差（唯讀；結束碼 0 = 一致，1 = 有落差）
agsy clean                  移除連結與產物
agsy version                版本
agsy help                   用法
```

什麼時候用哪個：

```
 第一次用             init → doctor → plan → apply
 改了來源             apply（不放心先 plan）
 AI 改了掛載中的檔案   status → 把要留的搬進來源 → apply
 不確定現況           status
 不再用 agsy          clean
```

所有指令共通：

- 除了 `init`（只看目前資料夾），每個指令都會往上層找 `agsy.yaml`，從專案的子資料夾執行也可以。
- `--yes`（或 `-y`）：所有確認一律當作「是」。給 CI、腳本、git hook 用。沒加時，在無法回答的環境（沒有終端機）遇到需要確認的動作一律取消。
- 介面語言依 `AGSY_LANG` / `LC_ALL` / `LANG` 判斷，見[安裝](install.md)。
- Windows 上雙擊執行時，視窗關閉前會暫停，讓輸出留得住。
- `apply`、`clean`、`init` 執行期間會在 `agsy.yaml` 旁建一個 `.agsy.lock`，防止同一專案兩個指令同時跑；結束即移除。殘留超過 15 分鐘的鎖（程式當掉留下的）會自動接管。

## `agsy`（選單）

不帶參數執行：

- 找不到 `agsy.yaml`：引導進 `init`。
- 找到：第一行是狀態摘要（來源異動、產物端改動、產物缺失、掛載異常各幾個），接著是 apply / plan / status / doctor / init / clean 選項。

## `agsy init`

產生或編輯 `agsy.yaml`。

### 第一次

依序問四件事：

1. 來源路徑，一行一個。
2. 要服務的工具（多選）。
3. 每個類別的同名衝突策略（必答；建議 rules=rename、skills=error、workflows=rename、hooks=error）。
4. 產物資料夾（預設 `.agsy`）。

寫入後：

- 專案根已有真實的 `AGENTS.md` 時會提醒：agsy 會在那個位置放連結，不會合併或覆蓋真實檔案。把內容搬進某個來源的 `rules/`，或改名保存。
- 列出一份 `.gitignore` 建議清單（`.agsy/`、`.agsy.lock`、各連結、根目錄 `AGENTS.md`），勾選要加的；`settings.json` 是你的檔案，不在清單上。

### 編輯（agsy.yaml 已存在）

- 每一題預填現值，按 Enter 保留。
- 已服務的工具預先勾選；手動改過的 `build.tools`、`categories`、自訂掛載條目原樣沿用。
- 寫入前顯示逐行 diff 並確認；沒有變更就不寫入。
- yaml 裡的註解會被換成範本註解。

### 無互動（CI、腳本）

```sh
agsy init --yes ~/all-ai-lib ./repo-ai-lib
```

來源用參數帶入；`--yes` 表示接受建議的預設值。

## `agsy doctor`

唯讀健檢，依序檢查：

1. `agsy.yaml` 找得到且格式正確。
2. 每個來源路徑存在（不存在 = ✘）。
3. 每個來源的子資料夾：缺的只是 ⚠；存在的統計會收幾個檔，規則與 build 完全相同，略過的檔附原因。
4. 每個掛載點：不存在（可建立）／已是連結／指向別處或已斷（apply 可修）／被真實資料夾或檔案佔用（apply 會失敗，要手動處理）。每個 merge 目標：不存在（apply 建立）／是 JSON 物件（可合併）／是符號連結或不是 JSON 物件（apply 會失敗）。
5. hooks：腳本在 macOS / Linux 上有沒有執行權限（沒有時提示 `chmod +x`；交給直譯器執行的檔案不檢查）；`build.tools` 有列但沒掛登記表的工具（來源真的有 hook 時才提示）。
6. 連結能力：實際建立並移除一個暫時連結。

結尾印 `N 個錯誤,M 個警告`；有錯誤時結束碼 1。

## `agsy plan`

把 apply 會做的事完整列一遍，不寫入任何東西。分三段：

**build 預覽**：來源清單（優先序、存在性、標記）；逐類別列出要收的項目、誰被改名、每個 workflow 產出什麼（`skill skills/<名稱>`、`轉接頭 workflows/<名稱>.md`）；每個 hook 的說明、到達哪幾家（`claude ✓  codex ✓  antigravity —  cursor ✓`）與沒到達的原因；轉換產物各一行；被 `first` 捨棄的項目；不符規則的檔案與原因；所有會擋下建置的問題。

**mount 預覽**：每條連結會發生什麼（建立／重建／修復／被佔用）；每個 merge 目標會發生什麼（建立／合併／更新既有條目／條目被改過會先問／不是 JSON 物件會失敗）。

**摘要**：一行統計。

有衝突、碰撞、target 或 `hook.yaml` 錯誤時結束碼 1，可當 CI 閘門。

## `agsy apply`

實際執行：清空產物 → 重建 → 掛載。

### 1. 前置檢查

全部通過才會進到任何確認：

| 檢查 | 失敗時 |
|------|--------|
| 每個來源路徑存在 | ✘ 拒絕 |
| 沒有掛載點被真實資料夾或檔案佔用（包括根目錄真實的 `AGENTS.md`、真實的 `.codex/hooks.json` 等） | ✘ 拒絕並列出；不會代為刪除 |
| 每個 merge 目標不是符號連結、且是 JSON 物件（或不存在） | ✘ 拒絕並列出 |
| 沒有 target、`hook.yaml` 錯誤、同名衝突、輸出路徑碰撞 | ✘ 拒絕；`plan` 有完整清單 |

### 2. 兩張清單

- **清單 A：來源異動**（只列出）：這次會同步的更新、新增、移除。來源被刪的項目特別標示，它的所有產出會一併消失。
- **清單 B：產物端改動**（要確認）：上次建置以來透過連結被改的一切：修改過的檔、新增的檔、被改的轉換產物、`settings.json` 裡被改的 agsy 條目。每一筆附「想保留：搬去哪個來源」。繼續執行就全部捨棄；沒有終端機且沒加 `--yes` 時取消。
- 產物資料夾存在但 manifest 讀不到：內容無從得知，一律先問再清。

### 3. build → mount → merge

```
 清空 .agsy/
   → 先寫一份只含上次 merge 記錄的 manifest
   → 原樣複製 rules、skills、hooks
   → 轉換 workflows（skill ＋ 轉接頭）
   → 併出 AGENTS.md
   → 翻譯四份 hook 登記表
   → 寫 manifest
   → 建立連結
   → merge（hooks.claude.json → .claude/settings.json 的 hooks 欄位）
```

mount 或 merge 失敗時，建置結果保持完整，排除問題後重跑 `agsy apply` 即可。建置本身失敗時，最先寫的那份 manifest 仍保有 merge 記錄（哪個檔是 agsy 建的、agsy 加了哪些容器），之後 clean 還是分得清哪些是 agsy 的。

### 4. 孤兒回報

先前 apply 建立、目前設定已不再引用的連結或 merge 條目會列出提醒，只回報不刪除；手動移除或交給 `agsy clean`。

## `agsy status`

唯讀。印出與 apply 相同的兩張清單，加上掛載狀態：

- **清單 A**：來源 → 產物：內容變更、新增、刪除，以及產物缺失（產物副本被刪；apply 可重建）。「來源路徑不存在」與「檔案被刪」分開標示。
- **清單 B**：產物端：下次 apply 會捨棄的一切，各附保留指引。
- **掛載**：每條連結的狀態（正常／遺失／指錯或已斷／被佔用／孤兒）；每個 merge 目標的狀態（同步／沒有東西可合併／agsy 條目被改／條目不見／不是 JSON 物件／孤兒）。
- **摘要**：`來源異動 N │ 產物端改動 N │ 產物缺失 N │ 掛載異常 N` 與建議的下一步。

結束碼 `0` = 完全一致，`1` = 有任何落差。適合 CI 與 git hook。

## `agsy clean`

從這個專案移除 agsy 建立的東西，確認後依序：

1. 從 merge 目標移除 agsy 的 hook 條目（孤兒目標也包括）。只刪 agsy 的條目與 apply 帶進來的容器；檔案是 agsy 建的且裡面沒別的東西（這次清空的，或先前 apply 就清空的）才刪檔；不是 agsy 建的檔絕不刪，沒有 agsy 條目的檔也不改寫。
2. 移除掛載連結（含根目錄 `AGENTS.md` 與各 `hooks.json` 連結）與 manifest 記錄的孤兒連結；每條都先確認確實是指向產物的連結才動。真實的資料夾與檔案略過並回報。連結移除後空掉的資料夾也移除。
3. 刪除整個產物資料夾。

`agsy.yaml` 保留。之後一次 `agsy apply` 可全部重建。

## `agsy version` / `agsy help`

```sh
agsy version    # agsy v1.2.3（commit、建置時間、go 版本、平台）
agsy help       # 用法（同 --help / -h）
```

→ 下一章：[Adapters](adapters.md)
