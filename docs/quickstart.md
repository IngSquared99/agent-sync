# 五分鐘上手

目標：從零開始，讓你的指令文件第一次同步到 AI 工具。

## 第 0 步：準備你的指令庫

agsy 只認來源資料夾底下這三個固定名字的子目錄：

| 類別 | 來源子目錄 | 單位 |
|---|---|---|
| rules | `rule/` | 單一 `.md` 檔 |
| skills | `skill/` | 目錄（裡面必須有 `SKILL.md`） |
| workflows | `workflow/` | 單一 `.md` 檔 |

所以一個最小的指令庫長這樣：

```
~/all-ai-lib/
├── rule/
│   └── python-style.md
├── skill/
│   └── code-review/
│       └── SKILL.md
└── workflow/
    └── deploy.md
```

沒有的類別可以整個省略（例如只有 rule/ 也行）。
如果你現有的資料夾用了別的子目錄名，之後可以在設定檔的 `build.categories` 改 `from`。

## 第 1 步：在專案裡啟動設定

```bash
cd 你的專案
agsy
```

第一次執行會發現沒有設定檔，主動問你要不要開始設定：

```
⚠ agsy.yaml 不存在，看起來是本專案第一次使用
要現在建立設定嗎？
❯ 是，開始設定（init）
```

接著問答式帶你填三件事：

1. **來源路徑**（照優先順序）——例如 `~/all-ai-lib` 和 `./repo-ai-lib`
2. **要掛載哪些工具**——Claude Code / Antigravity / OpenAI Codex 勾選即可
3. **同名檔案怎麼處理**——三個類別各選一次（不確定就用建議值，[進階](advanced.md)有詳解）

完成後專案根目錄會多一個 `agsy.yaml`，全程你的檔案一個都沒被動過。

## 第 2 步：預覽（不寫入任何東西）

```bash
agsy plan
```

plan 會列出：每個檔案會進到產物的哪裡、同名衝突怎麼處理了、
workflow 分流到哪些工具、哪些檔案不符規則被略過（附原因）。
**它保證不寫入任何檔案**——看不對勁就回頭改，零風險。

## 第 3 步：執行

```bash
agsy apply
```

看到這兩行就成功了：

```
✔ build 完成：12 個項目 → .agsy/
✔ mount 完成：6 條連結
```

驗證一下（例如你掛了 Claude Code）：

```bash
ls .claude/rules/        # 你的 rules 出現在這裡了
cat .claude/rules/python-style.md
```

打開 Claude Code，它已經讀得到你的規則了。

## 第 4 步：之後的日常

```
改了來源？           → agsy apply
AI 改了掛載端檔案？   → agsy promote 寫回（完成後即一致）
狀態不明？           → agsy status
```

就這樣。想知道每個指令背後在做什麼、為什麼來源永遠不會被弄髒，
繼續看 [三層架構，一次看懂](concepts.md)。

## 給團隊用

`agsy.yaml` 應該進版控（`.agsy/` 不用，init 會主動幫你加進 `.gitignore`）。
隊友 clone 專案後只要跑 `agsy apply`，就會長出跟你一模一樣的掛載。
