# 指令參考

| 指令 | 做什麼 | 會寫入嗎？ |
|---|---|---|
| `agsy` | 互動選單（無參數執行時） | 視選擇 |
| `agsy init` | 問答式產生 `agsy.yaml`；已存在則進入**修改模式** | 只寫設定檔 |
| `agsy doctor` | 環境健檢——只檢查，不動任何東西 | 否 |
| `agsy plan` | 預覽 build / mount 會做的事 | **否，保證不寫** |
| `agsy apply` | 重建產物並掛載（會先確認未回寫的改動） | 是 |
| `agsy status` | 比對來源／產物／掛載，報告落差（CI 可用：exit 0 = 同步） | 否 |
| `agsy promote` | 把產物層的改動**寫回**來源 | 寫入來源 |
| `agsy clean` | 反安裝：移除連結與產物，只留 `agsy.yaml` | 刪除工具所建物 |
| `agsy version` | 版本、commit、建置時間、平台 | 否 |
| `agsy help` | 指令說明 | 否 |

## 全域旗標

**`--yes` / `-y`**：把每個確認都當作「y」——給 CI、git hook、腳本用。
沒加時如果沒人能回答提問（非互動環境），需要確認的動作**一律取消**，絕不硬做。

## 執行位置

指令可以在專案的**任何子目錄**執行，agsy 會像 git 一樣向上尋找 `agsy.yaml`。
唯一例外是 `agsy init`——它只看當前目錄（設定檔要建在你執行它的地方）。

## 各指令備忘

### agsy init

- 第一次：問答式設定來源、掛載工具、同名策略，產生帶註解的 `agsy.yaml`
- 已有設定檔：進入修改模式，Enter 保留現值，寫入前顯示 diff 要你確認
- 非互動環境必須加 `--yes` 並用參數帶來源：`agsy init --yes ~/all-ai-lib ./repo-ai-lib`
- 注意：修改模式會用範本重新產生整份設定——手動改過 `categories`、`route.field`、
  部分 `route.default` 的專案，重跑 init 會被還原成預設，請看 diff 再確認

### agsy plan

看四件事：每個檔案的去向與來源標記、同名處理結果、workflow 分流、
被略過的檔案（附原因）。摘要各數字對不上時，逐區往上找，每一筆都有列。

### agsy apply

流程固定：前置檢查（來源齊全？掛載點沒被實體目錄佔用？）→ 確認未回寫改動 →
清空產物 → build → mount。任何一關失敗都在破壞性動作之前停下。

### agsy promote

```bash
agsy promote                          # 互動多選
agsy promote <類別/名稱>              # 單項，寫回原家
agsy promote <類別/名稱> --to <來源>  # 單項，改寫到指定來源（搬家）
agsy promote --all                    # 全部，各回各家（先列清單再確認）
```

`--all --to` 不支援（批次＋轉向會讓來源和記帳本脫鉤）。
`--to` 請只指向 `agsy.yaml` 裡已設定的來源。

### agsy clean

只刪工具建立的東西：連結、產物目錄、因此空掉的掛載目錄。
你自己的真實檔案一律跳過並列出。`agsy.yaml` 不動——之後 `agsy apply` 一行還原全部。

## 環境變數

| 變數 | 作用 |
|---|---|
| `AGSY_LANG=zh` / `en` | 強制介面語言（預設跟隨系統 `LANG`，zh 開頭顯示繁中，其他顯示英文） |
