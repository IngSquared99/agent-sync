# 安裝

agsy 是**單一執行檔、零外部依賴**——不用裝 Node、Python 或任何執行環境。
裝一次就是全機可用（跟 git 一樣），不是裝進某個專案裡。

## 一行安裝（推薦）

打開終端機，貼上對應你機器的那一行：

**Mac（Apple Silicon，M 系列晶片）**

```bash
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_apple_silicon.tar.gz | tar xz && sudo mv agsy /usr/local/bin/
```

**Mac（Intel）**

```bash
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_mac_intel.tar.gz | tar xz && sudo mv agsy /usr/local/bin/
```

**Linux（x64；Arm 機器把 x64 換成 arm64）**

```bash
curl -sL https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_linux_x64.tar.gz | tar xz && sudo mv agsy /usr/local/bin/
```

**Windows（PowerShell）**

```powershell
iwr https://github.com/IngSquared99/agent-sync/releases/latest/download/agsy_windows_x64.zip -OutFile "$env:TEMP\agsy.zip"; Expand-Archive "$env:TEMP\agsy.zip" "$env:LOCALAPPDATA\Programs\agsy" -Force; [Environment]::SetEnvironmentVariable("Path", [Environment]::GetEnvironmentVariable("Path","User") + ";$env:LOCALAPPDATA\Programs\agsy", "User")
```

Windows 裝完請**開一個新的終端機視窗**讓 PATH 生效；Arm 筆電把 x64 換成 arm64。

## 確認裝好了

```bash
agsy version
```

看到版本號就完成了。

?> Mac / Linux 過程中會要你輸入一次密碼——把檔案搬進 `/usr/local/bin` 需要它，屬正常現象。
用指令列下載不會觸發 macOS 的隔離檢查，所以不會出現「無法驗證開發者」的警告。

## 其他安裝方式

**用 Go 安裝**（已裝 Go 1.22+ 的開發者）：

```bash
go install github.com/IngSquared99/agent-sync/cmd/agsy@latest
```

**從原始碼建置**：

```bash
git clone https://github.com/IngSquared99/agent-sync.git
cd agent-sync
go build -o agsy ./cmd/agsy    # Windows: go build -o agsy.exe ./cmd/agsy
```

## 常見安裝問題

**Q：我自己從網頁下載了執行檔，Mac 說「無法驗證開發者」？**
瀏覽器下載會被 macOS 加上隔離標記（指令列下載不會）。點「完成」（不要點「丟到垃圾桶」），然後執行 `xattr -d com.apple.quarantine <檔案路徑>` 清掉即可。這是未付費簽章的正常結果，不是惡意軟體。

**Q：Windows 需要開發人員模式或系統管理員權限嗎？**
不用。agsy 在 Windows 用 junction 建立連結，一般帳號就夠。

裝好了？往下走 → [五分鐘上手](quickstart.md)
