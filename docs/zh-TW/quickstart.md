# 快速上手

四步完成第一次同步。全部在專案資料夾內操作。

```
 ① 準備來源  ──▶  ② agsy init  ──▶  ③ agsy plan  ──▶  ④ agsy apply
    放指令檔        產生設定檔         預覽（不寫入）      建置＋掛載
```

## 第 1 步：準備一個來源資料夾

來源是一個資料夾，裡面最多四個子資料夾，只放你需要的就好：

```
~/all-ai-lib/
├── rules/
│   └── python-style.md          # 一個 .md 檔
├── skills/
│   └── code-review/
│       └── SKILL.md             # 含 SKILL.md 的資料夾
├── workflows/
│   └── deploy.md                # 一個 .md 檔
└── hooks/
    └── block-rm/
        ├── hook.yaml            # 宣告：哪個時機、跑哪支腳本
        └── block-rm.sh          # 腳本（結束碼 2 = 擋下）
```

常見配置是兩個來源：個人共用庫（`~/all-ai-lib`）加專案內的庫（`./repo-ai-lib`）。

## 第 2 步：`agsy init` 產生設定檔

在專案資料夾執行，依提示回答。按 Enter 採用預設值。

```
$ cd your-project
$ agsy init

來源路徑,依優先序排列（~ 開頭=共用庫,./ 開頭=專案內）
  來源 1: ~/all-ai-lib
  來源 2: ./repo-ai-lib
  來源 3: ⏎

要服務哪些工具?（a = 全部）
    1) Antigravity (.agents/)
    2) Claude Code (.claude/)
    3) OpenAI Codex (.agents/, .codex/)
    4) Cursor (.agents/, .cursor/)
請輸入: a

rules 的同名衝突怎麼處理?        ❯ rename
skills 的同名衝突怎麼處理?       ❯ error
workflows 的同名衝突怎麼處理?    ❯ rename
hooks 的同名衝突怎麼處理?        ❯ error

建置產物目錄（預設: .agsy）: ⏎

✔ 已寫入 agsy.yaml

以下由 agsy 產生的路徑皆可重建,通常應加入 .gitignore:
要把哪些項目加進 .gitignore?（a = 全部）a
```

「同名衝突」是指兩個來源有同名的檔案時怎麼辦：`rename` 兩份都留（檔名加上來源名），`error` 停下來讓你處理，`first` 只留優先序高的那份。

最後一題是要不要把產物路徑加進 `.gitignore`，由你決定；`agsy.yaml` 本身建議進版控。

腳本或 CI 用的無互動寫法：`agsy init --yes ~/all-ai-lib ./repo-ai-lib`。

## 第 3 步：`agsy plan` 預覽

```
$ agsy plan
```

列出 apply 會做的每一件事：收哪些檔、誰被改名、每個 workflow 產出什麼、每個 hook 到達哪幾家、每條連結會怎樣。不寫入任何東西。有不對的地方，改完再跑一次。

## 第 4 步：`agsy apply` 建置並掛載

```
$ agsy apply
✔ build 完成:13 個項目 → .agsy/
✔ mount 完成:8 條連結
✔ merge 完成:.claude/settings.json ← hooks.claude.json
```

完成後專案長這樣（`→` 是連結，`⇐` 是 merge）：

```
your-project/
├── AGENTS.md          → .agsy/AGENTS.md         （全部 rules 併成一檔）
├── .claude/
│   ├── rules          → .agsy/rules
│   ├── skills         → .agsy/skills
│   └── settings.json  ⇐ .agsy/hooks.claude.json （只寫進 "hooks" 欄位）
├── .agents/
│   ├── skills         → .agsy/skills
│   ├── workflows      → .agsy/workflows
│   └── hooks.json     → .agsy/hooks.antigravity.json
├── .codex/
│   └── hooks.json     → .agsy/hooks.codex.json
├── .cursor/
│   └── hooks.json     → .agsy/hooks.cursor.json
└── .agsy/             產物
```

四家工具都從自己的位置讀到同一批內容。在 Claude Code 或 Cursor 輸入 `/deploy` 會執行那個 workflow；在 Antigravity 也是 `/deploy`。四家在執行 shell 指令前都會先跑 `block-rm.sh`，它回傳結束碼 2 的話指令就不會執行。

## 之後的日常

```
 改來源  ──▶  agsy apply  ──▶  四家都是最新的
                 ▲
 agsy status ────┘  看哪裡不同步（有落差時結束碼為 1）
```

AI 工具透過連結改了產物時（例如加了新規則），`agsy status` 會列出來並說明要搬去哪個來源。想保留就搬過去，再 apply。

## 指令速查

```
agsy            選單（附狀態摘要）
agsy doctor     環境健檢（唯讀）
agsy plan       預覽（唯讀）
agsy apply      建置＋掛載（會先列出要捨棄的東西並詢問）
agsy status     兩張落差清單＋連結狀態（唯讀）
agsy clean      從這個專案移除 agsy 建立的東西
```

→ 下一章：[設定檔](config.md)
