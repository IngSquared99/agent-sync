# 進階：同名衝突、分流與回寫

這一頁給想把 agsy 用深的人：多來源同名時發生什麼事、workflow 怎麼分流、
promote 的完整規則，以及幾個要知道的邊界。

## 同名衝突的完整行為

兩個來源都有 `rule/python-style.md` 時，依 `on_conflict` 策略：

```
  ~/all-ai-lib/rule/python-style.md ──┐        rename：兩份都留、都加標記
                                      ├─ 撞名 ─→   python-style@all-ai-lib.md
  ./repo-ai-lib/rule/python-style.md ─┘            python-style@repo-ai-lib.md

                                          first：只留優先級高的那份（plan 列出丟棄清單）
                                          error：停下來列清單，請你手動處理
```

細節：

- **rename 是雙方都改名**，不是只改後來者——看到帶標記的檔名就知道發生過合併。
- 來源標記取自來源路徑的最後一段；兩個來源同名時自動往上取父目錄組合，保證標記唯一。
- 被 rename 的 **skill** 連 `SKILL.md` front-matter 裡的 `name` 都會同步改寫
  （promote 回寫時再自動還原原名，帶標記的名字不會漏回來源）。
- rename 之後如果**最終檔名還是撞**（極端巧合），一律擋下不建置——絕不無聲覆蓋。

## workflow 分流（route）

```
  deploy.md        (target: claude)        ──→ 只進 .agsy/workflows/claude/
  standup.md       (target: agents)        ──→ 只進 .agsy/workflows/agents/
  hotfix.md        (target: [claude,agents]) → 兩邊都進
  release-note.md  (沒標)                  ──→ 進 route.default 指定的桶
```

- 標了 `buckets` 裡不存在的桶 → build 直接報錯（打錯字不會默默消失）
- 沒標 target 且 `default: []` → 該檔不放進任何桶，plan / apply 每次都警告
- 沒標 target 而套用 default 的檔案，plan 會特別註明「未標示 target，套用預設」
- 一個檔案進多個桶時是**多份獨立副本**——這影響 promote，見下段

## promote 的完整規則

| 情況 | 行為 |
|---|---|
| 一般改動 | 寫回 manifest 記錄的原家 |
| 改動的是被 rename 的檔案 | 以**原名**寫回（標記名不外洩到來源） |
| 原家在專案外（共用庫） | 先警告「影響所有使用它的專案」再確認 |
| 來源同時也變了（兩邊都動） | **拒絕**，請你手動比對（硬蓋會毀掉來源的新內容） |
| workflow 多副本、只改了其中一份 | 回寫後其他副本同步更新成相同內容 |
| workflow 多副本、改成了不同內容 | **拒絕**，請你手動處理 |
| `--to` 搬家 | 只搬內容，舊來源檔案還在——提醒你下次 build 會同名兩份，確認後手動刪舊 |

`--to` 只接受 `agsy.yaml` 裡已設定的來源——指到其他路徑會被直接拒絕。
另外，所有回寫目的地（不只 `--to`）都必須落在已設定的來源底下：
manifest 放在掛載工具寫得到的產物層，不能盲信；`agsy.yaml` 由你控制、
不在掛載層，才是回寫目的地的信任根。

## 已知邊界（使用前該知道的）

**掛載端「新增」的檔案不受追蹤。** manifest 只記 build 放進產物的項目；
AI 在 `.claude/` 裡**新建**的檔案（不是修改既有檔案）status 看不見、promote 認不得，
下次 apply 會直接消失。AI 幫你新寫的東西想留，請手動搬進來源資料夾。

**init 修改模式會正規化整份設定。** 手動改過 `categories`、`route.field`、
部分 `route.default` 的專案，重跑 init 會被範本預設值取代——寫入前的 diff 是最後防線,
所以不要在這類專案上用 `agsy init --yes`(會跳過 diff)。

**來源中的符號連結（symlink）一律不收錄。** build 複製的是檔案內容，
跟隨連結等於允許共用庫把來源外的任意檔案（例如私鑰）偷渡進掛載層，
所以連結本身、以及內部藏有連結的 skill 目錄，都會被列入「不符收錄規則」並拒收。

**Windows junction 存絕對路徑。** 專案整個搬目錄後連結會失效——`agsy status` 會報掛載異常，
重跑 `agsy apply` 即修復。（macOS / Linux 用相對 symlink，搬目錄不受影響。）

## 把 status 當 CI 檢查

```yaml
# .github/workflows/check-sync.yml 片段
- name: Check agsy sync
  run: agsy status   # exit 0 = 同步；1 = 有落差，讓 CI 紅燈
```

配合 git hook 也一樣：`agsy status` 非互動時只印報告、以退出碼表態，不會卡在提問上。
