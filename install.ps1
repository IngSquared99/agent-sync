# agsy one-line installer (Windows PowerShell)
# 用法:irm https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.ps1 | iex
# 行為:偵測架構 → 從 GitHub Releases 下載 zip → 解壓到使用者程式目錄 → 加入 PATH
$ErrorActionPreference = "Stop"

$repo = "IngSquared99/agent-sync"
# ARM64 筆電抓 arm64 版,其餘一律 x64
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "x64" }
$file = "agsy_windows_$arch.zip"
$url  = "https://github.com/$repo/releases/latest/download/$file"
$dest = "$env:LOCALAPPDATA\Programs\agsy"

Write-Host "downloading $file ..."
$tmp = Join-Path $env:TEMP "agsy.zip"
Invoke-WebRequest -Uri $url -OutFile $tmp
Expand-Archive -Path $tmp -DestinationPath $dest -Force
Remove-Item $tmp

# 加入使用者層級 PATH(已存在則略過;不需要系統管理員)
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dest*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dest", "User")
  Write-Host "added to PATH."
}
Write-Host "done. Open a NEW terminal window, then run: agsy version"
