# B. 快速上手：四步完成第一次同步

整條路只有四步，走完一遍大約 10 分鐘：

```
 Step 0        Step 1         Step 2         Step 3
 準備來源  ──▶  init 設定  ──▶  plan 預覽  ──▶  apply 建置＋掛載  ──▶ 🎉 所有工具同步完成
 （擺檔案）     （問答一次）     （唯讀確認）     （正式執行）
```

> 以下的終端機畫面都是**示意**（依實際版本可能略有差異），幫助你預期每一步會看到什麼。

## B-1. 兩種操作方式

agsy 有雙入口：不帶參數是互動選單，帶參數直接執行。不想背指令，打 `agsy` 就對了：

```text
$ agsy
agsy v1.2.3

  狀態：落後 0 │ 新增 2 │ 本地改動 1 │ 未追蹤 0 │ 產物缺失 0 │ 掛載異常 0

要做什麼?
  apply    重建產物並掛載      ⚠ 1 項本地改動，需先確認
  plan     預覽變更，不寫入
  promote  回寫本地改動（1 項）
> status   檢視詳細狀態
  doctor   環境健檢
  init     設定（已有設定檔則進入編輯模式）
  clean    移除產物與掛載
  離開
```

第一次在專案裡執行、還沒有設定檔時，選單會直接引導你進入 `init`。

## B-2. 指令速查表

| 指令 | 做什麼 | 會不會寫入 |
|------|--------|-----------|
| `agsy init [sources...]` | 問答式產生 `agsy.yaml`（已存在則進入編輯模式） | 寫 `agsy.yaml` |
| `agsy doctor` | 環境健檢：設定、來源、掛載點、連結能力 | 唯讀 |
| `agsy plan` | 完整預覽 build + mount 的結果 | 唯讀 |
| `agsy apply` | 前置檢查 → 確認 → 清空重建產物 → 掛載 | 寫 `.agsy/` 與連結 |
| `agsy status` | 比對來源／產物／掛載三方，報告落差 | 唯讀（結尾有行動選單） |
| `agsy promote` | 把產物端的改動寫回來源 | 寫來源 |
| `agsy clean` | 移除連結與 `.agsy/`（反安裝，保留 `agsy.yaml`） | 刪除產物 |
| `agsy version` / `agsy help` | 版本 / 說明 | 唯讀 |

全域旗標：`--yes`（縮寫 `-y`）＝所有確認一律回答「是」，給 CI、腳本、git hook 等非互動環境用。**沒有 `--yes` 時，非互動環境遇到需要確認的動作會直接取消，絕不硬做。**

## B-3. Step 0：準備來源目錄

準備至少一個來源，子目錄**必須**叫 `rules/`、`skills/`、`workflows/`（複數，[命名規範](00-overview.md#來源目錄必須照規範命名)）。常見組合是「個人共用庫＋專案內庫」：

```sh
mkdir -p ~/all-ai-lib/rules ~/all-ai-lib/skills ~/all-ai-lib/workflows
mkdir -p ./repo-ai-lib/rules ./repo-ai-lib/skills ./repo-ai-lib/workflows
```

建好之後，把你的指令檔照類別放進去，長這樣就對了：

```
~/all-ai-lib/                     ./repo-ai-lib/
├── rules/                        ├── rules/
│   └── python-style.md           │   └── api-naming.md
├── skills/                       ├── skills/
│   └── code-review/              │   （還沒有也沒關係，留空即可）
│       └── SKILL.md              └── workflows/
└── workflows/                        └── deploy.md
    └── release.md
```

三個子目錄的格式，一行記住一個：

- `rules/` — 單一 `.md` 檔
- `skills/` — **目錄**＋必備 `SKILL.md`（內部不可含符號連結）
- `workflows/` — 單一 `.md` 檔，front-matter 可標 `target:` 分流

## B-4. Step 1：`agsy init` — 回答五個問題

在專案根目錄執行 `agsy init`，整個過程就是一次問答。畫面走起來像這樣：

```text
$ agsy init
開始設定 agsy（Enter 採用預設值）

來源路徑，依優先級排序（~ 開頭=共用庫、./ 開頭=專案內）
一行一個，直接按 Enter 結束輸入（例：~/all-ai-lib、./repo-ai-lib）
  來源 1: ~/all-ai-lib
  來源 2: ./repo-ai-lib
  來源 3: ⏎

要掛載哪些工具?
  [x] Claude Code（.claude/）
  [ ] OpenAI Codex（.codex/）
  [x] Antigravity（.agents/）

rules 同名時如何處理?（建議 rename：常有「全域基礎+專案補充」的並存需求）
> rename   兩份都保留，檔名加來源標記
  error    停止並列出衝突，由你手動處理（最保守）
  first    只留優先來源的那份，其餘丟棄

skills 同名時如何處理?（建議 error）…
workflows 同名時如何處理?（建議 rename）…

建置產物目錄 [.agsy] ⏎

workflow 沒標示 target 時放到哪?
> 全部 bucket（建議：檔案出現在所有地方，比神秘消失好理解）
  不放，並在 plan / apply 時警告

✔ 已寫入 agsy.yaml

.agsy/ 是可重建的產物，要加進 .gitignore 嗎? y
  下一步：agsy plan 預覽 → agsy apply 執行
```

五個問題的答法一張表看完：

| # | 問題 | 建議答案 | 為什麼 |
|---|------|----------|--------|
| 1 | 來源路徑？ | 共用庫在前、專案庫在後 | 順序＝優先權，越前面越優先 |
| 2 | 掛載哪些工具？ | 勾你實際在用的 | 之後隨時可重跑 init 增減 |
| 3 | 同名衝突怎麼辦？（三類各問一次） | rules=`rename`、skills=`error`、workflows=`rename` | rename 兩份共存、error 最保守、first 只留優先那份 |
| 4 | 產物目錄？ | 直接 Enter（`.agsy`） | 必須是專案內的專用目錄 |
| 5 | 沒標 target 的 workflow 去哪？ | 全部 bucket | 「到處出現」比「神祕消失」好除錯 |

收尾兩個小提醒：

- `.gitignore` 那題答 **y**——產物不進版控；`agsy.yaml` 本身則**要**進版控。
- 非互動環境（CI）用 `agsy init --yes ~/all-ai-lib ./repo-ai-lib`：來源用參數給，`--yes`＝接受建議預設策略。

## B-5. Step 2：`agsy plan` — 唯讀預覽，看三個地方

`plan` 把 apply 會做的每件事演練一遍，**保證不寫入任何檔案**：

```text
$ agsy plan
讀取 agsy.yaml ✔
來源（依優先級）:
  [1] ~/all-ai-lib    ✔   標記: @all-ai-lib
  [2] ./repo-ai-lib   ✔   標記: @repo-ai-lib

═══ build 預覽 ═══

rules → .agsy/rules/（策略: rename）
  git-commit.md                        ← [1] all-ai-lib
  python-style-fromlib-all-ai-lib.md   ← [1] all-ai-lib    ⚠ 同名，已加來源標記
  python-style-fromlib-repo-ai-lib.md  ← [2] repo-ai-lib   ⚠ 同名，已加來源標記

skills → .agsy/skills/（策略: error）
  code-review                          ← [1] all-ai-lib

workflows → .agsy/workflows/（策略: rename）
  release.md                           ← [1] all-ai-lib   → claude

═══ mount 預覽 ═══

.claude/
  commands   → workflows/claude   （不存在，將建立）
  rules      → rules              （不存在，將建立）
  skills     → skills             （不存在，將建立）

═══ 摘要 ═══
5 個項目 │ 2 個改名 │ 0 組衝突 │ 0 組撞名 │ 0 個丟棄(first)│ 0 個不收錄 │ 6 條連結 │ 0 個掛載異常 │ 0 個掛載衝突

未寫入任何檔案。確認無誤後執行 agsy apply。
```

這張畫面只需要看三個地方：

1. **每一行的 `← [n] 來源標記`**：確認每個檔案來自你預期的來源；`⚠` 表示同名被改名（兩份都在）。
2. **有沒有 `✘` 區塊**：同名衝突、撞名、路由錯誤——出現任何一種，apply 都會拒絕，先照清單修。
3. **摘要那一行**：衝突／撞名／掛載異常都是 0，就可以放心進下一步。

## B-6. Step 3：`agsy apply` — 正式建置與掛載

```text
$ agsy apply
✔ build 完成：5 個項目 → .agsy/
✔ mount 完成：6 條連結
```

兩行綠色就是完成了。此時打開 `.claude/`，`rules`、`skills`、`commands` 都已是指向 `.agsy/` 的連結，工具直接讀得到。

若中途停下，都是保護機制在動作：來源路徑缺失、掛載點被真實目錄佔用、或偵測到未寫回的本機改動（會列清單問你要不要捨棄）。照訊息處理完重跑即可，細節見[指令說明](04-commands.md#c-2-6-agsy-apply)。

## B-7. 日常循環：三句話

```
 平常改動：  改來源檔  ──▶  agsy apply                    （工具全部同步）
 產物端被改： agsy promote（寫回來源）──▶  agsy apply      （兩邊重新一致）
 不確定時：  agsy status                                  （它會告訴你該跑哪個）
```

**心智模型只有一條**：來源是唯一的真相來源（source of truth）。正常改動改來源＋`apply`；不小心（或刻意讓 AI）改到產物端，就用 `promote` 收回來。

## B-8. 在 CI / git hook 裡使用

`status` 的離開碼設計給自動化用：`0`＝完全同步、`1`＝有落差。

```sh
# pre-commit hook 範例：來源改了卻忘記 apply 就擋下 commit
agsy status || { echo "agsy 不同步，請先執行 agsy apply"; exit 1; }

# CI 裡重建（非互動，接受所有確認）
agsy apply --yes
```

→ 下一章：[個人設定檔 agsy.yaml](03-config.md)
