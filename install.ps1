# agsy installer for Windows (PowerShell).
# Usage: irm https://raw.githubusercontent.com/IngSquared99/agent-sync/main/install.ps1 | iex
# Steps: detect architecture -> download the release zip and checksums.txt
#        -> verify SHA-256 -> extract into the user's Programs directory -> add to PATH.
# No administrator rights required; everything is user-scoped.
$ErrorActionPreference = "Stop"

$repo = "IngSquared99/agent-sync"
$arch = if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { "arm64" } else { "x64" }
$file = "agsy_windows_$arch.zip"
$base = "https://github.com/$repo/releases/latest/download"
$dest = "$env:LOCALAPPDATA\Programs\agsy"

Write-Host "downloading $file ..."
$tmp = Join-Path $env:TEMP $file
Invoke-WebRequest -Uri "$base/$file" -OutFile $tmp

# Verify SHA-256 against the checksum file published with the release.
# Defends against corrupted or tampered downloads in transit.
$sums = (Invoke-WebRequest -Uri "$base/checksums.txt" -UseBasicParsing).Content
$entry = $sums -split "`n" | Where-Object { $_ -match [regex]::Escape($file) } | Select-Object -First 1
if (-not $entry) { throw "checksum entry for $file not found in checksums.txt" }
$expected = ($entry -split "\s+")[0].ToLower()
$actual = (Get-FileHash -Algorithm SHA256 -Path $tmp).Hash.ToLower()
if ($actual -ne $expected) { throw "checksum mismatch for $file (expected $expected, got $actual)" }
Write-Host "checksum OK"

Expand-Archive -Path $tmp -DestinationPath $dest -Force
Remove-Item $tmp

# Append to the user-scoped PATH unless already present.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$dest*") {
  [Environment]::SetEnvironmentVariable("Path", "$userPath;$dest", "User")
  Write-Host "added $dest to PATH."
}
Write-Host "done. Open a NEW terminal window, then run: agsy version"
