# agent-sync(agsy)

[English](README.md) | **繁體中文**

**AI 指令文件的單一來源同步工具** —— 把散落多處的 rules / skills / workflows 合併成一份建置產物,掛載到 Claude Code、Antigravity 等 AI 開發工具要求的位置。**改一份來源,所有工具同步生效。**

> 名稱對照:專案叫 **agent-sync**,指令叫 **`agsy`**,設定檔叫 **`agsy.yaml`**,建置產物在 **`.agsy/`**。

---

## 這是什麼?解決什麼問題?

同時使用多個 AI 開發工具,你會遇到這件麻煩事:每家工具都規定自己的讀取位置——

- Claude Code 讀 `.claude/`(rules、skills、commands)
- Antigravity 讀 `.agents/`(rules、skills、workflows)
- 你可能還有一份跨專案共用的個人指令庫(例如 `~/all-ai-lib`)

同一份「Python 風格規範」得複製到好幾個地方,改一次要改 N 份,遲早不同步。

**agsy 的做法**:指令文件放在「來源」(共用庫、專案內目錄,幾個都行),agsy 負責兩件事:

```
   來源們(正本)            build(複製合併)            mount(目錄連結)

  ~/all-ai-lib/  ──┐                                    .claude/rules   ──→ .agsy/rules
               ├──合併──→   .agsy/(單一產物)  ←──連結──  .claude/skills  ──→ .agsy/skills
  ./repo-ai-lib/  ──┘                                     .agents/rules   ──→ .agsy/rules
                                                    .agents/workflows … 依此類推
```

- **build**:把所有來源**複製**合併成 `.agsy/` 一份產物(來源永遠不被直接動到)
- **mount**:在 `.claude/`、`.agents/` 建立**目錄連結**指向產物(Windows 用 junction,免系統設定)

---

## 快速開始(3 步)

1. 安裝 `agsy`:打開終端機貼一行指令即可(見下方[安裝](#安裝))
2. 終端機切到你的專案
3. 輸入 `agsy`,跟著畫面走

```
$ cd 我的專案
$ agsy

⚠ 找不到 agsy.yaml,看起來是第一次在此專案使用
? 要現在建立設定嗎?
❯ 是,開始設定(init)
```

### 日常循環(一張圖)

```
              apply(從來源重建產物並掛載)
        ┌────────────────────────────────────┐
        │                                    ▼
   來源(正本)                       .agsy/(產物)◄─── .claude/ .agents/(連結)
        ▲                                    │              AI 工具在這裡讀、
        │                                    │              也可能在這裡改
        └────────────────────────────────────┘
              promote(把產物裡的改動回寫來源)
```

日常就三個動作:

| 情況 | 指令 |
|---|---|
| 我改了來源 | `agsy apply` |
| AI 幫我改了掛載中的檔案 | `agsy promote` 回寫(完成後來源與產物即一致) |
| 不確定現在什麼狀態 | `agsy status` |

---

## 指令一覽

| 指令 | 作用 |
|---|---|
| `agsy` | 互動選單(不帶參數時) |
| `agsy init` | 問答式產生 `agsy.yaml`;已存在時進入**修改模式**(現值當預設、Enter 保留、寫入前列出變更 diff 要你確認) |
| `agsy doctor` | 環境健檢,只檢查不動作 |
| `agsy plan` | 預覽 build / mount 會發生什麼,**不寫入任何東西** |
| `agsy apply` | 重建產物並掛載(會先檢查你有沒有未回寫的改動) |
| `agsy status` | 比對來源 / 產物 / 掛載,回報落差(可接 CI:exit 0 = 同步) |
| `agsy promote` | 把產物裡的改動**回寫**到來源(AI 幫你改了掛載中的檔案時用) |
| `agsy clean` | 反安裝:移除連結與產物,只剩 `agsy.yaml` |
| `agsy version` | 版本、commit、建置時間與平台 |
| `agsy help` | 指令說明 |

全域旗標 `--yes` / `-y`:把所有確認一律視為 y,給 CI、git hook、腳本用。不加時,只要沒有人能回答問題(非互動環境),需要確認的動作一律取消而不是硬做。

指令可以在專案的任何子目錄執行,agsy 會往上尋找 `agsy.yaml`(慣例同 git);只有 `agsy init` 例外——在哪執行就在哪建立設定。

---

## 支援的工具(adapters)

`adapters/` 內建各工具的掛載預設集,`agsy init` 問「要掛載哪些工具?」時的選項就來自這裡:

| 工具 | 掛載目錄 | 連結內容 |
|---|---|---|
| Claude Code | `.claude/` | rules、skills、commands |
| Antigravity | `.agents/` | rules、skills、workflows |
| OpenAI Codex | `.codex/` | prompts(掛載慣例驗證中) |

**想掛表上沒有的工具?** adapter 只是 init 的出廠範本,不是執行期相依——自己在 `agsy.yaml` 的 `mount` 加一段就行,**不用改程式、不用等新版本**:

```yaml
mount:
  - dir: .某工具        # 該工具規定的讀取目錄
    links:
      rules: rules      # 工具的子目錄 → 產物的層級
```

## 設定檔長什麼樣

`agsy init` 會產生帶註解的 `agsy.yaml`,之後直接用編輯器改即可(改完 `agsy apply` 生效):

```yaml
version: 1

sources:            # 有序陣列,前者優先
  - ~/all-ai-lib        # 跨專案共用庫
  - ./repo-ai-lib         # 專案內來源

build:
  out: .agsy
  on_conflict:      # 同名處理:first / rename / error(逐類別必填)
    rules:     rename
    skills:    error
    workflows: rename
  route:            # workflow 依 front-matter 的 target 欄位分流
    field: target
    default: [agents, claude]
    buckets: [agents, claude]

mount:
  - dir: .claude    # Claude Code
    links:
      rules:    rules
      skills:   skills
      commands: workflows/claude
  - dir: .agents    # Antigravity
    links:
      rules:     rules
      skills:    skills
      workflows: workflows/agents
```

## 哪些檔案會被收進來

| 類別 | 來源子目錄 | 收錄單位 |
|---|---|---|
| rules | `rule/` | 單一 `.md` 檔 |
| skills | `skill/` | 目錄(內含 `SKILL.md`) |
| workflows | `workflow/` | 單一 `.md` 檔 |

不符合的檔案一律不收,但也**不會靜默消失**——`agsy plan` 與 `agsy doctor` 都會列出被略過的檔案與原因。

---

## 為什麼不是又一個複製腳本?

同類工具大多是「單一來源目錄 → 複製或連結到各家位置」的單層流程。agsy 在三個地方走了不一樣的路:

### 特色 1:多來源真合併,同名衝突由你決定

多數做法只有一個來源目錄,或者全域/專案「二選一」的 fallback。agsy 的 `sources` 是**有序陣列**——共用庫、團隊庫、專案庫可以**整批疊加**,順序即優先級,撞名時按你選的策略處理:

```
  ~/all-all-ai-lib/rule/python-style.md ──┐                 rename 策略(兩份都保留、都標出處)
                                  ├── 同名!──→     python-style@all-ai-lib.md
  ./repo-all-ai-lib/rule/python-style.md ───┘                 python-style@repo-ai-lib.md

                                                    first:只留優先來源的一份
                                                    error:停下來,列出清單讓你處理
```

策略逐類別設定(rules / skills / workflows 各自獨立),而且是**必填**——同名處理的後果因專案而異,agsy 不替你偷偷決定。

### 特色 2:中間建置層——來源永遠乾淨

單層同步有個隱患:AI 工具在掛載位置改了檔案,改動會直接穿透回你的正本,污染所有用到它的專案。agsy 刻意多墊一層:

```
   來源(正本)────────── 永遠不被工具直接碰到
      │  build = 複製
      ▼
   .agsy/(可拋棄的沙盒)◄──── AI 的改動落在這裡,僅止於這裡
      │  mount = 連結              │
      ▼                            │ 想保留 → promote 回寫正本
   .claude/  .agents/              │ 不想要 → 下次 apply 自然消失
```

這一層帶來的性質是連動的:因為有它,「來源永遠乾淨」有了結構性保證;分流(見特色 3)能在 build 階段做完,掛載層永遠保持「整個目錄一條連結」的單純形式;產物隨時可重建,`clean` 一下就能完整反安裝。

### 特色 3:逐檔分流——這個檔案只給誰

不是每份 workflow 都該給每個工具。在檔案的 front-matter 標一個 `target`,build 時自動分流:

```markdown
---
target: [claude]
---
```

```
  deploy.md        (target: claude)  ──→  只進 .claude/commands/
  standup.md       (target: agents)  ──→  只進 .agents/workflows/
  hotfix.md        (target: 兩者)    ──→  兩邊都進
  release-note.md  (沒標)            ──→  依你設定的預設(出廠:全部)
```

沒有「全部工具一律複製一份」的浪費,也沒有「檔案神秘消失」——`agsy plan` 會逐檔標明分流去向,連「為什麼兩邊都放」都註明是套用預設。

### 基本盤:雙向同步與漂移偵測

build 時每個檔案都記入 manifest(sha256 指紋 + 來源血緣),所以 `agsy status` 能一眼分清**兩個方向**的落差:

```
  來源動了(該 apply)              產物被改(該 promote)
  ├─ 落後:來源已更新               ├─ 本地改動:AI 改了掛載中的檔案,
  ├─ 新增:來源有新檔案             │   並能指出「原家」在哪個來源
  └─ 來源已刪除(特別標示)         └─ 目錄型項目會列出改了哪幾個檔案
```

`promote` 回寫時**東西從哪來就回哪去**(改過名的檔案自動還原原名);`status` 非互動時以 exit code 回報(0 = 同步),可直接放進 CI 或 git hook。

### 安全底線

- **工具永遠不刪除自己沒建立的東西**——連結是 agsy 建的,放心刪除重建;你的實體目錄與檔案,一律停下來報錯,絕不代刪
- `build.out` 有守門:指到專案外、家目錄、來源路徑,設定驗證階段直接擋下
- `apply` 前強制檢查未回寫的改動**與掛載端新增的未追蹤檔案**,不會默默蓋掉或刪除(`status` 有獨立的「未追蹤」分區)
- `promote` 只會寫進 `agsy.yaml` 所設定的來源(manifest 位於掛載層,絕不盲信);來源中的符號連結一律不收錄
- `plan` 永遠零寫入,先看清楚再動手

### 下載即用

單一執行檔、**零外部相依**,不需要任何 runtime。Windows 用 junction 掛載,**不需要開發人員模式、不需要系統管理員**。macOS / Linux / Windows(x64 與 arm64)全支援。

---

## 常見問題

**Q:`.claude/` 裡已經有我原本的檔案,會被刪掉嗎?**
不會。agsy 只刪「自己建的連結」;遇到實體目錄或檔案一律報錯請你手動處理,絕不代刪。

**Q:Windows 需要開發人員模式或系統管理員嗎?**
不用。agsy 在 Windows 用 junction(目錄接合點)掛載,一般帳號即可。

**Q:專案搬到別的路徑後連結失效?**
重跑一次 `agsy apply` 即可(Windows 的 junction 存絕對路徑,搬家後需要重建)。

**Q:`.agsy/` 要進版本控制嗎?**
不用,它是可重建的產物。`agsy init` 結束時會問你要不要自動加進 `.gitignore`;`agsy.yaml` 則建議進版控——團隊成員 clone 下來直接 `agsy apply` 就能長出一模一樣的掛載。

**Q:AI 工具直接改了掛載中的檔案,會弄髒我的來源嗎?**
不會。改動實際落在 `.agsy/` 產物層,來源不受影響。想保留就 `agsy promote` 回寫;不想要,下次 `agsy apply` 重建就消失。

**Q:AI 工具在掛載目錄「新建」了一個檔案,它會活下來嗎?**
`status` 會把它列為**未追蹤**,`apply` 也會先問過你才刪。新檔案沒有原家,`promote` 回寫不了——想保留就把它搬進某個來源目錄(例如 `rule/`)。

**Q:同名檔案怎麼辦(共用庫和專案都有 `python-style.md`)?**
由你在 init 時決定策略:兩份都留並加來源標記(rename)、只留優先的一份(first),或直接報錯要你手動處理(error)。skills 建議用 error——skill 靠 description 語意觸發,兩份相近的並存會讓觸發不可控。

**Q:我自己從網頁下載了執行檔,Mac 跳出「無法驗證開發者」?**
瀏覽器下載的檔案會被 macOS 標記隔離(指令下載不會)。按「完成」(不要丟到垃圾桶),再執行 `xattr -d com.apple.quarantine 檔案路徑` 放行;這是未付費簽章的正常現象,不是中毒。

**Q:介面能顯示英文嗎?**
會自動跟著系統語言走(終端機 `LANG` 為中文就顯示中文,其他顯示英文);要強制切換,執行指令前加 `AGSY_LANG=en` 或 `AGSY_LANG=zh`。

**Q:`status` 說掛載正常,但工具還是讀不到指令?**
先確認 `.claude/` 之類的目錄裡不是你自己手動建的實體目錄。agsy 會檢查每條連結「是不是連結」以及「指向對不對」,異常都會標成 ✘,重跑 `agsy apply` 即可修復。

---

## 安裝

### 一行指令安裝(推薦)

**Mac / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.sh | sh
```

**Windows(PowerShell)**

```powershell
irm https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.ps1 | iex
```

腳本會偵測系統與晶片、從 [Releases](https://github.com/IngSquared99/agent-sync/releases) 下載對應版本、驗證 SHA-256 檢查碼後放進 PATH。內容僅數十行,可先開啟檢視:[install.sh](install.sh) / [install.ps1](install.ps1)。

### Homebrew(Mac)

```bash
brew install IngSquared99/tap/agsy
```

### Go(開發者)

```bash
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

裝完打 `agsy version` 確認。

<details>
<summary>手動下載(不跑腳本)</summary>

```bash
# Mac(Apple 晶片,M 系列)
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_apple_silicon.tar.gz | tar xz && sudo mv agsy /usr/local/bin/

# Mac(Intel 舊機型)
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_intel.tar.gz | tar xz && sudo mv agsy /usr/local/bin/

# Linux(x64;Arm 機器把 x64 換成 arm64)
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_linux_x64.tar.gz | tar xz && sudo mv agsy /usr/local/bin/
```

```powershell
# Windows(PowerShell)
iwr https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_windows_x64.zip -OutFile "$env:TEMP\agsy.zip"; Expand-Archive "$env:TEMP\agsy.zip" "$env:LOCALAPPDATA\Programs\agsy" -Force; [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path","User") + ";$env:LOCALAPPDATA\Programs\agsy", "User")
# 貼完重開一個終端機視窗讓 PATH 生效;Arm 筆電把 x64 換成 arm64
```

Mac / Linux 會要求輸入一次電腦密碼(放進 /usr/local/bin 需要);用指令下載不會被 macOS 標上隔離標記,不會出現「無法驗證開發者」的警告視窗。

</details>

### 從原始碼建置(開發者)

需要 [Go](https://go.dev/dl/) 1.22+:

```
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go build -o agsy ./cmd/agsy    # Windows: go build -o agsy.exe ./cmd/agsy
go test ./...             # 零外部相依,不必先 go mod download
```
