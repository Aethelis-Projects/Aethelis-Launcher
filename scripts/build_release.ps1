# ==============================================================================
# Nord Launcher - Windows Production Release Builder
# ==============================================================================
param(
    [string]$Version = "0.1.0",
    [string]$CurseForgeKey = ""
)

$ErrorActionPreference = "Stop"

# Keep existing environment PATH intact

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

if ($CurseForgeKey) {
    Set-Content -Path (Join-Path $DistWin "cf.key") -Value $CurseForgeKey -NoNewline
    Write-Host "Wrote sidecar key to: $(Join-Path $DistWin 'cf.key')" -ForegroundColor Cyan
}

Write-Host "==> [3/5] Compiling Launcher Executable with Go..." -ForegroundColor Cyan
$BinaryPath = Join-Path $DistWin "NordLauncher.exe"
go build "-ldflags=-s -w -H=windowsgui -X main.version=$Version" -o $BinaryPath ./cmd/launcher/main.go

# Verify binary does NOT leak key in buildinfo or strings
$buildInfo = go version -m $BinaryPath | Out-String
if ($buildInfo -match "CurseForgeKey" -or ($CurseForgeKey -and $buildInfo.Contains($CurseForgeKey))) {
    Write-Error "CRITICAL SECURITY ERROR: CurseForgeKey leaked in Windows buildinfo!"
}

if ($CurseForgeKey) {
    $found = Select-String -Path $BinaryPath -Pattern $CurseForgeKey -SimpleMatch -Quiet
    if ($found) {
        Write-Error "CRITICAL SECURITY ERROR: Secret key found in Windows binary strings!"
    } else {
        Write-Host "PASS: Windows binary verified clean of secrets." -ForegroundColor Green
    }

    if (-not (Test-Path (Join-Path $DistWin "cf.key"))) {
        Write-Error "ERROR: cf.key was not generated in $DistWin!"
    } else {
        Write-Host "PASS: Verified cf.key present for installer packaging." -ForegroundColor Green
    }
}

$BinaryBytes = (Get-Item $BinaryPath).Length
$BinaryMB = [math]::Round($BinaryBytes / 1MB, 2)
Write-Host "Binary compiled: $BinaryPath ($BinaryMB MB)" -ForegroundColor Green

if ($BinaryBytes -gt 40MB) {
    Write-Error "ERROR: Launcher binary exceeds 40 MB budget ($BinaryMB MB)!"
}

Write-Host "==> [4/6] Applying Windows Authenticode Code Signing..." -ForegroundColor Cyan
& (Join-Path $PSScriptRoot "sign_windows.ps1") -TargetFile $BinaryPath

Write-Host "==> [5/6] Checking NSIS Installer availability..." -ForegroundColor Cyan
$MakensisCmd = Get-Command makensis -ErrorAction SilentlyContinue
$MakensisPath = if ($MakensisCmd) { $MakensisCmd.Source } elseif (Test-Path "$env:TEMP\nsis\nsis-3.10\makensis.exe") { "$env:TEMP\nsis\nsis-3.10\makensis.exe" } else { $null }

$InstallerPath = Join-Path $DistWin "NordLauncher-Setup.exe"
if ($MakensisPath) {
    Write-Host "Building NSIS installer via $MakensisPath..." -ForegroundColor Cyan
    $NsiScript = Join-Path $PSScriptRoot "..\build\windows\installer.nsi"
    & $MakensisPath "/DVERSION=$Version" $NsiScript
    $DefaultOut = Join-Path $PSScriptRoot "..\build\windows\NordLauncher-Setup.exe"
    if (Test-Path $DefaultOut) {
        Move-Item -Force $DefaultOut $InstallerPath
    }
    if (Test-Path $InstallerPath) {
        Write-Host "Applying Windows Authenticode Code Signing to Installer..." -ForegroundColor Cyan
        & (Join-Path $PSScriptRoot "sign_windows.ps1") -TargetFile $InstallerPath
        $InstallerBytes = (Get-Item $InstallerPath).Length
        $InstallerMB = [math]::Round($InstallerBytes / 1MB, 2)
        Write-Host "Installer compiled: $InstallerPath ($InstallerMB MB)" -ForegroundColor Green
    }
} else {
    Write-Host "Notice: makensis not in PATH. Skipping NSIS packaging step." -ForegroundColor Yellow
}

Write-Host "==> [6/7] Building Windows Portable Zip (6th Release Asset)..." -ForegroundColor Cyan
$ZipName = "nord-launcher-v$Version-windows-x64-portable.zip"
$PortableZipPath = Join-Path $DistWin $ZipName
$StagingDir = Join-Path $DistWin "portable-staging"
if (Test-Path $StagingDir) { Remove-Item -Recurse -Force $StagingDir }
New-Item -ItemType Directory -Force -Path $StagingDir | Out-Null
Copy-Item $BinaryPath (Join-Path $StagingDir "NordLauncher.exe")
if (Test-Path (Join-Path $DistWin "cf.key")) {
    Copy-Item (Join-Path $DistWin "cf.key") (Join-Path $StagingDir "cf.key")
}
Compress-Archive -Path "$StagingDir\*" -DestinationPath $PortableZipPath -Force
Remove-Item -Recurse -Force $StagingDir
$ZipBytes = (Get-Item $PortableZipPath).Length
$ZipMB = [math]::Round($ZipBytes / 1MB, 2)
Write-Host "Portable Zip compiled: $PortableZipPath ($ZipMB MB)" -ForegroundColor Green

Write-Host "==> [7/7] Generating SHA256 Checksums..." -ForegroundColor Cyan
$HashBin = (Get-FileHash -Algorithm SHA256 $BinaryPath).Hash.ToLower()
$ChecksumContent = "$HashBin  NordLauncher.exe`n"
if (Test-Path $InstallerPath) {
    $HashInst = (Get-FileHash -Algorithm SHA256 $InstallerPath).Hash.ToLower()
    $ChecksumContent += "$HashInst  NordLauncher-Setup.exe`n"
}
if (Test-Path $PortableZipPath) {
    $HashZip = (Get-FileHash -Algorithm SHA256 $PortableZipPath).Hash.ToLower()
    $ChecksumContent += "$HashZip  $ZipName`n"
}
Set-Content -Path (Join-Path $DistWin "SHA256SUMS.txt") -Value $ChecksumContent -NoNewline

Write-Host "==================================================================" -ForegroundColor Green
Write-Host " Production Release Built Successfully for Windows x64" -ForegroundColor Green
Write-Host " Executable: $BinaryPath ($BinaryMB MB)" -ForegroundColor Green
if (Test-Path $InstallerPath) {
    Write-Host " Installer:  $InstallerPath ($InstallerMB MB)" -ForegroundColor Green
}
Write-Host " SHA256SUMS:`n$ChecksumContent" -ForegroundColor Green
Write-Host "==================================================================" -ForegroundColor Green
