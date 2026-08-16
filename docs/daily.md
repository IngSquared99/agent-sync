# 日常使用與雙向同步

裝好、跑過第一次 `apply` 之後，你跟 agsy 的互動基本上只剩三個指令：
`status` 看狀態、`apply` 往產物推、`promote` 往來源收。這一頁把它們串起來。

## status：把「不同步」講清楚

```bash
agsy status
```

status 把落差拆成兩個方向加一個健康檢查：

```
方向一：來源變了 → 該 apply
    落後      = 來源被更新，產物還是舊的
    新增      = 來源多了新檔案，產物裡還沒有
    來源被刪  = 大聲警告（apply 後該項會跟著消失）

方向二：產物變了 → 該 promote
    本地改動  = AI 改了掛載端的檔案（status 會附上它的原家路徑）
    產物缺失  = 產物副本被手動刪掉（沒東西可回寫，apply 可重建）

第三區：掛載連結本身的健康狀態（遺失／指錯地方／被實體目錄佔位）
```

報告最後有一行摘要和建議動作，照著做就對了：

```
═══ 摘要 ═══
落後 1 │ 新增 1 │ 本地改動 1 │ 產物缺失 0 │ 掛載異常 1
建議：先 agsy promote 保留改動，再 agsy apply 重建
```

**為什麼順序是先 promote 再 apply？** 因為 apply 會重建產物——
還沒寫回的改動會被蓋掉（apply 也會先停下來問你）。先收後推，就不會掉東西。

## 情境走一遍：AI 改了你的檔案

假設 Claude Code 在對話中幫你補了一段 skill 的說明，它改的是 `.claude/skills/api-doc/SKILL.md`。

1. **改動實際落在產物層**（`.claude/` 只是連結）——你的來源原稿沒被動。
2. `agsy status` 看到：

   ```
   本地改動 1 項（產物與建置當時不同，可回寫）
     skills/api-doc    原來源: …/a-lib/skill/api-doc   改動 1 個檔案
   ```

   注意它直接告訴你這個檔案的**原家**在哪——這是 manifest 血緣在工作。
3. 想留這個改動：

   ```bash
   agsy promote skills/api-doc
   agsy apply
   ```

4. 不想留？什麼都不用做，下次 `agsy apply` 重建時自然消失。

## promote 的幾種用法

```bash
agsy promote                        # 互動式：列出所有改動讓你勾選
agsy promote skills/api-doc         # 只回寫這一項
agsy promote --all                  # 全部回寫（先列清單再確認）
agsy promote rules/x.md --to ./lib  # 回寫到另一個來源（搬家）
```

promote 的保護規則：

- 目標在**專案外**（共用庫）→ 先警告「回寫會影響所有使用它的專案」再確認
- **兩邊都改了**（來源也變、產物也變）→ 拒絕硬蓋，請你手動比對
- workflow 有**多份副本**且被改成不同內容 → 拒絕，請你手動處理
- 用 `--to` 搬家後，舊來源的檔案還在——會提醒你下次 build 將出現同名兩份，確認後手動刪舊的

## 在 CI 或 git hook 裡用

- `agsy status` 在非互動環境只印報告：exit 0 = 同步、1 = 有落差，可直接當檢查。
- 所有需要確認的指令（apply / promote --all / clean）在沒人能回答時**一律取消**，
  不會硬做。要無人值守執行，加上 `--yes`（等於你的明示同意）：

```bash
agsy apply --yes
```

## 團隊協作的節奏

1. `agsy.yaml` 進版控，`.agsy/` 不進（init 會幫你加 .gitignore）
2. 隊友 clone 後跑一次 `agsy apply` 長出掛載
3. 共用庫更新後，各專案跑 `agsy apply` 即同步
4. 想在 CI 確保沒有人忘了同步：跑 `agsy status` 當檢查
