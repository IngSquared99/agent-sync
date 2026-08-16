# 安裝

agsy 是**單一執行檔、零外部依賴**——不用裝 Node、Python 或任何執行環境。
裝一次就是全機可用（跟 git 一樣），不是裝進某個專案裡。

## 一行安裝（推薦）

**Mac / Linux**

```bash
curl -fsSL https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.sh | sh
```

**Windows（PowerShell）**

```powershell
irm https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.ps1 | iex
```

腳本做的事很單純：偵測你的系統與晶片 → 從
[Releases](https://github.com/IngSquared99/agent-sync/releases) 下載對應版本 → 放進 PATH。
內容就幾十行，不放心可以先點開看：
[install.sh](https://github.com/IngSquared99/agent-sync/blob/main/install.sh) /
[install.ps1](https://github.com/IngSquared99/agent-sync/blob/main/install.ps1)。

## Homebrew（Mac 使用者）

```bash
brew install IngSquared99/tap/agsy
```

之後升級跟其他 brew 套件一樣：`brew upgrade agsy`。

## Go（開發者）

已裝 Go 1.22+ 的話：

```bash
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

## 確認裝好了

```bash
agsy version
```

看到版本號就完成了。往下走 → [五分鐘上手](quickstart.md)

?> Mac / Linux 的安裝腳本過程中會要你輸入一次密碼——把檔案搬進 `/usr/local/bin` 需要它，屬正常現象。

## 手動下載（不跑腳本）

<details>
<summary>展開手動安裝指令</summary>

```bash
# Mac（Apple 晶片，M 系列）
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_apple_silicon.tar.gz | tar xz && sudo mv agsy /usr/local/bin/

# Mac（Intel）
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_intel.tar.gz | tar xz && sudo mv agsy /usr/local/bin/

# Linux（x64；Arm 機器把 x64 換成 arm64）
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_linux_x64.tar.gz | tar xz && sudo mv agsy /usr/local/bin/
```

```powershell
# Windows（PowerShell）
iwr https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_windows_x64.zip -OutFile "$env:TEMP\agsy.zip"; Expand-Archive "$env:TEMP\agsy.zip" "$env:LOCALAPPDATA\Programs\agsy" -Force; [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path","User") + ";$env:LOCALAPPDATA\Programs\agsy", "User")
```

</details>

## 常見安裝問題

**Q：我自己從網頁下載了執行檔，Mac 說「無法驗證開發者」？**
瀏覽器下載會被 macOS 加上隔離標記（指令列下載與 Homebrew 安裝不會）。
點「完成」（不要點「丟到垃圾桶」），然後執行 `xattr -d com.apple.quarantine <檔案路徑>` 清掉即可。
這是未付費簽章的正常結果，不是惡意軟體。

**Q：Windows 需要開發人員模式或系統管理員權限嗎？**
不用。安裝與之後的掛載（junction）都只需要一般帳號。

**Q：想移除 agsy？**
刪掉執行檔即可（Mac/Linux：`sudo rm /usr/local/bin/agsy`；Homebrew：`brew uninstall agsy`；
Windows：刪除 `%LOCALAPPDATA%\Programs\agsy`）。專案裡的掛載用 `agsy clean` 移除。
