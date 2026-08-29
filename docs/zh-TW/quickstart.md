# 快速上手

四步完成第一次同步，全部在專案目錄內進行。

## 第 0 步：準備一個來源庫

來源是一個至多含三個子目錄的資料夾，任何子集皆可：

```
~/all-ai-lib/
├── rules/
│   └── python-style.md          # 單純的 markdown 檔
├── skills/
│   └── code-review/
│       └── SKILL.md             # 內含 SKILL.md 的目錄
└── workflows/
    └── deploy.md                # 單純的 markdown 檔，可選 target: front matter
```

可以指向一個庫或多個；常見配置是個人共用庫（`~/all-ai-lib`）加專案內的庫（`./repo-ai-lib`）。

## 第 1 步：`agsy init`——產生設定檔

```
$ cd your-project
$ agsy init
開始設定 agsy（按 Enter 採用預設值）

來源路徑,依優先序排列（~ 開頭=共用庫,./ 開頭=專案內）
  來源 1: ~/all-ai-lib
  來源 2: ./repo-ai-lib
  來源 3: ⏎

要服務哪些工具?（空白分隔多個編號,a = 全部,Enter = 全部）
    1) Claude Code (.claude/)
    2) OpenAI Codex (.agents/)
    3) Antigravity (.agents/)
    4) Cursor (.agents/)
請輸入: a

rules 的同名衝突怎麼處理?（建議 rename…）        ❯ rename
skills 的同名衝突怎麼處理?（建議 error…）         ❯ error
workflows 的同名衝突怎麼處理?                     ❯ rename

建置產物目錄（預設: .agsy）: ⏎

✔ 已寫入 agsy.yaml

以下由 agsy 產生的路徑皆可重建,通常應加入 .gitignore:
要把哪些項目加進 .gitignore?（a = 全部）a
  ✔ 已將 6 個項目加入 .gitignore
  下一步:agsy plan 預覽 → agsy apply 執行
```

`.gitignore` 那題除非團隊刻意把連結進版控，答 **a**（全部）即可；`agsy.yaml` 本身**應該** commit。

腳本用的非互動形式：`agsy init --yes ~/all-ai-lib ./repo-ai-lib`。

## 第 2 步：`agsy plan`——預覽、不寫入

```
$ agsy plan
```

預覽逐類別列出建置會收的一切：哪些 rules 被衝突策略改名、每個 workflow 產生哪些形態（`skill skills/deploy`／`轉接頭 workflows/deploy.md`）、轉換產物 `AGENTS.md` 一行、每個被排除的檔案與原因、每條掛載連結會發生什麼。不寫入任何檔案；視需要調整後重新執行 plan。

## 第 3 步：`agsy apply`——建置並掛載

```
$ agsy apply
✔ build 完成:12 個項目 → .agsy/
✔ mount 完成:6 條連結
```

完成後的專案結構：

```
your-project/
├── AGENTS.md          → .agsy/AGENTS.md         （全部 rules 串接）
├── .claude/
│   ├── rules          → .agsy/rules
│   └── skills         → .agsy/skills
├── .agents/
│   ├── skills         → .agsy/skills
│   └── workflows      → .agsy/workflows
└── .agsy/             建置產物
```

每個工具都從自己的原生位置讀到同一批內容。在 Claude Code 或 Cursor 輸入 `/deploy` 會執行該 workflow 的 skill 形態；在 Antigravity 則執行轉接頭，由它載入該 skill。

## 第 4 步：日常循環

```
編輯來源  ──▶  agsy apply  ──▶  所有工具都是最新的
                 ▲
status 檢查落差 ──┘（有任何不同步時 exit code 為 1）
```

AI 工具透過掛載寫入內容（新規則、改過的 skill）時，`agsy status` 會附指引列出；把要保留的搬進來源，再 apply。詳見[指令參考](commands.md)與[情境指南](scenarios.md)。

## 指令速查

```
agsy            附狀態摘要的選單
agsy doctor     環境健檢
agsy plan       預覽（唯讀）
agsy apply      建置＋掛載（先確認要捨棄的東西）
agsy status     兩張落差清單＋掛載健康（唯讀,exit code 適合 CI）
agsy clean      從此專案反安裝
```

→ 下一章：[設定檔](config.md)
