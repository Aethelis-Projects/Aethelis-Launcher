# ==============================================================================
# Nord Launcher - Windows Production Release Builder
# ==============================================================================
param(
    [string]$Version = "0.1.0"
)

$ErrorActionPreference = "Stop"

Write-Host "==> [1/5] Setting up build environment..." -ForegroundColor Cyan
$env:Path = "C:\Program Files\Go\bin;C:\Users\Home\AppData\Roaming\npm;$env:Path"

# Ensure output directory
$DistWin = Join-Path $PSScriptRoot "..\dist\windows"
if (Test-Path $DistWin) {
    Remove-Item -Recurse -Force $DistWin
}
New-Item -ItemType Directory -Force -Path $DistWin | Out-Null

Write-Host "==> [2/5] Building Frontend (SolidJS)..." -ForegroundColor Cyan
Push-Location (Join-Path $PSScriptRoot "..\frontend")
try {
    pnpm install
    pnpm build
} finally {
    Pop-Location
}

Write-Host "==> [3/5] Compiling Launcher Executable with Go..." -ForegroundColor Cyan
$BinaryPath = Join-Path $DistWin "NordLauncher.exe"
go build -ldflags="-s -w -H=windowsgui -X main.version=$Version" -o $BinaryPath ./cmd/launcher/main.go

$BinaryBytes = (Get-Item $BinaryPath).Length
$BinaryMB = [math]::Round($BinaryBytes / 1MB, 2)
Write-Host "Binary compiled: $BinaryPath ($BinaryMB MB)" -ForegroundColor Green

if ($BinaryBytes -gt 40MB) {
    Write-Error "ERROR: Launcher binary exceeds 40 MB budget ($BinaryMB MB)!"
}

Write-Host "==> [4/5] Checking NSIS Installer availability..." -ForegroundColor Cyan
$Makensis = Get-Command makensis -ErrorAction SilentlyContinue
if ($Makensis) {
    Write-Host "Building NSIS installer..." -ForegroundColor Cyan
    & makensis /DVERSION=$Version (Join-Path $PSScriptRoot "..\build\windows\installer.nsi")
} else {
    Write-Host "Notice: makensis not in PATH. Skipping NSIS packaging step." -ForegroundColor Yellow
}

Write-Host "==> [5/5] Generating SHA256 Checksum..." -ForegroundColor Cyan
$Hash = (Get-FileHash -Algorithm SHA256 $BinaryPath).Hash.ToLower()
$ChecksumContent = "$Hash  NordLauncher.exe`n"
Set-Content -Path (Join-Path $DistWin "SHA256SUMS.txt") -Value $ChecksumContent -NoNewline

Write-Host "==================================================================" -ForegroundColor Green
Write-Host " Production Release Built Successfully for Windows x64" -ForegroundColor Green
Write-Host " Executable: $BinaryPath ($BinaryMB MB)" -ForegroundColor Green
Write-Host " SHA256:     $Hash" -ForegroundColor Green
Write-Host "==================================================================" -ForegroundColor Green
