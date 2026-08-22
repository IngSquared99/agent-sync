# B. 快速上手：Command Line 指引

這一章帶你在 5–10 分鐘內從零走完一輪：準備來源 → 初始化 → 預覽 → 建置掛載 → 日常同步。

## B-1. 兩種操作方式

agsy 有雙入口，兩種都可以用：

```sh
agsy            # 不帶參數：進入互動選單（會顯示目前同步狀態摘要）
agsy <指令>      # 帶參數：直接執行該指令（適合熟手與腳本）
```

第一次在專案裡執行 `agsy` 而還沒有設定檔時，選單會直接引導你進入 `init`。

## B-2. 指令速查表

| 指令 | 做什麼 | 會不會寫入 |
|------|--------|-----------|
| `agsy init [sources...]` | 問答式產生 `agsy.yaml`（已存在則進入編輯模式） | 寫 `agsy.yaml` |
| `agsy doctor` | 環境健檢：設定、來源、掛載點、連結能力 | 唯讀 |
| `agsy plan` | 完整預覽 build + mount 的結果 | 唯讀 |
| `agsy apply` | 前置檢查 → 確認 → 清空重建產物 → 掛載 | 寫 `.agsy/` 與連結 |
| `agsy status` | 比對來源／產物／掛載三方，報告落差 | 唯讀（結尾有行動選單） |
| `agsy promote` | 把掛載側的改動寫回來源 | 寫來源 |
| `agsy clean` | 移除連結與 `.agsy/`（反安裝，保留 `agsy.yaml`） | 刪除產物 |
| `agsy version` / `agsy help` | 版本 / 說明 | 唯讀 |

全域旗標：`--yes`（縮寫 `-y`）＝所有確認一律回答「是」，給 CI、腳本、git hook 等非互動環境用。**沒有 `--yes` 時，非互動環境遇到需要確認的動作會直接取消，絕不硬做。**

## B-3. Step 0：準備來源目錄

先準備至少一個來源。常見的組合是「個人共用庫 + 專案內庫」：

```sh
mkdir -p ~/all-ai-lib/rule ~/all-ai-lib/skill ~/all-ai-lib/workflow
mkdir -p ./repo-ai-lib/rule ./repo-ai-lib/skill ./repo-ai-lib/workflow
```

放一點內容進去，例如：

```sh
cat > ~/all-ai-lib/rule/python-style.md <<'EOF'
# Python 風格
- 用 ruff 排版
- 函式一律加 type hints
EOF
```

三個子目錄的格式要求（不合規的檔案會被略過，`plan` / `doctor` 會列出原因）：

- `rule/`：單一 `.md` 檔。
- `skill/`：**目錄**，裡面必須有 `SKILL.md`；目錄內不能含符號連結。
- `workflow/`：單一 `.md` 檔，front-matter 可標 `target:` 決定分流到哪些工具。

## B-4. Step 1：初始化 `agsy init`

在專案根目錄執行：

```sh
cd my-project
agsy init
```

問答內容依序是：

1. **來源路徑**（依優先權排序，一行一個；`~` 開頭＝共用庫、`./` 開頭＝專案內）。
2. **要掛載哪些工具**（多選：Claude Code / OpenAI Codex / Antigravity…）。
3. **三個類別的同名衝突策略**（rules / skills / workflows 各問一次，必答）：
   - `rename`：兩份都留，檔名附上來源標籤（rules 建議值）。
   - `error`：停下並列出衝突，由你手動處理（skills 建議值，最保守）。
   - `first`：只留優先權較高來源的那份，其餘捨棄。
4. **產物目錄**（預設 `.agsy`）。
5. **沒有標 target 的 workflow 要去哪**（建議「所有 bucket」）。

完成後會寫出 `agsy.yaml`，並詢問是否把產物目錄加入 `.gitignore`（建議加；`agsy.yaml` 本身則**應該**進版控）。

> 非互動環境（CI）要用 `agsy init --yes ~/all-ai-lib ./repo-ai-lib` 的形式：來源用參數給，`--yes` 表示接受建議的預設策略。

## B-5. Step 2：預覽 `agsy plan`

```sh
agsy plan
```

`plan` 把 `apply` 會做的每件事**演練一遍但完全不寫入**：

- 每個來源的存在狀態與來源標籤；
- 三個類別各會收進哪些項目、誰被改名、workflow 分流到哪些 bucket；
- 被略過的檔案與原因、同名衝突、路徑異常；
- 每個掛載連結會建立／重建／或被真實目錄擋住。

看到結尾 `No files were written`（未寫入任何檔案）字樣，確認內容符合預期再進下一步。

## B-6. Step 3：建置與掛載 `agsy apply`

```sh
agsy apply
```

流程：前置檢查（來源都在、掛載點沒被真實目錄佔用、沒有路由／衝突錯誤）→ 若偵測到未寫回的本機改動會**先列出並要求確認** → 清空 `.agsy/` → 重新建置 → 建立掛載連結。

成功後：

```
✔ build done: 12 items → .agsy/
✔ mount done: 5 links
```

此時打開 `.claude/` 就能看到 `rules`、`skills`、`commands` 都是指向 `.agsy/` 的連結，工具可以直接讀取。

## B-7. 日常循環

```sh
# 改了來源檔之後：
agsy apply                 # 重建 + 掛載，所有工具同步更新

# 不確定現在狀態：
agsy status                # behind？local changes？掛載正常嗎？

# AI 工具（或你）直接改了掛載側的檔案：
agsy promote               # 互動選擇要寫回來源的項目
agsy apply                 # 寫回後再重建，兩邊一致

# 想暫時移除 agsy 的所有產出：
agsy clean
```

**心智模型只有一條**：來源是唯一的真相來源（source of truth）。正常改動改來源＋`apply`；不小心（或刻意讓 AI）改到掛載側，就用 `promote` 收回來。

## B-8. 在 CI / git hook 裡使用

`status` 的離開碼設計給自動化用：`0`＝完全同步、`1`＝有落差。

```sh
# pre-commit hook 範例：來源改了卻忘記 apply 就擋下 commit
agsy status || { echo "agsy 不同步，請先執行 agsy apply"; exit 1; }

# CI 裡重建（非互動，接受所有確認）
agsy apply --yes
```

→ 下一章：[個人設定檔 agsy.yaml](03-config.md)
