# ==============================================================================
# Nord Launcher - Windows Authenticode Signing Script
# ==============================================================================
param(
    [string]$TargetFile = "dist\windows\NordLauncher.exe",
    [string]$CertThumbprint = "",
    [string]$PfxPath = "",
    [string]$PfxPassword = "",
    [string]$TimestampServer = "http://timestamp.digicert.com"
)

$ErrorActionPreference = "Stop"

if (-not (Test-Path $TargetFile)) {
    Write-Error "Target file not found: $TargetFile"
}

Write-Host "==> Signing executable: $TargetFile" -ForegroundColor Cyan

$cert = $null

if ($PfxPath -and (Test-Path $PfxPath)) {
    Write-Host "Using PFX certificate: $PfxPath" -ForegroundColor Cyan
    $cert = Get-PfxCertificate -FilePath $PfxPath
} elseif ($CertThumbprint) {
    Write-Host "Using certificate with thumbprint: $CertThumbprint" -ForegroundColor Cyan
    $cert = Get-Item "Cert:\CurrentUser\My\$CertThumbprint" -ErrorAction SilentlyContinue
    if (-not $cert) {
        $cert = Get-Item "Cert:\LocalMachine\My\$CertThumbprint" -ErrorAction SilentlyContinue
    }
} else {
    Write-Host "No certificate specified. Searching for existing Nord test code signing certificate..." -ForegroundColor Yellow
    $cert = Get-ChildItem -Path "Cert:\CurrentUser\My" -CodeSigningCert | Where-Object { $_.Subject -like "*Nord Launcher*" } | Select-Object -First 1

    if (-not $cert) {
        Write-Host "Generating self-signed test code signing certificate for staging/CI..." -ForegroundColor Yellow
        $cert = New-SelfSignedCertificate `
            -Type CodeSigningCert `
            -Subject "CN=Nord Launcher Open Source Test, O=Nord Project, C=US" `
            -KeyUsage DigitalSignature `
            -FriendlyName "Nord Launcher Test Signing" `
            -CertStoreLocation "Cert:\CurrentUser\My" `
            -NotAfter (Get-Date).AddYears(2)
        Write-Host "Generated test certificate with thumbprint: $($cert.Thumbprint)" -ForegroundColor Green
    }
}

if (-not $cert) {
    Write-Error "Failed to obtain a code signing certificate."
}

# Sign executable with RFC 3161 timestamp
Write-Host "Applying Authenticode signature..." -ForegroundColor Cyan
try {
    $sigResult = Set-AuthenticodeSignature -FilePath $TargetFile -Certificate $cert -TimestampServer $TimestampServer -HashAlgorithm SHA256
} catch {
    Write-Host "Timestamp server unreachable, signing without timestamp..." -ForegroundColor Yellow
    $sigResult = Set-AuthenticodeSignature -FilePath $TargetFile -Certificate $cert -HashAlgorithm SHA256
}

Write-Host "Signature Status: $($sigResult.Status)" -ForegroundColor Green

# Verify signature
$verification = Get-AuthenticodeSignature -FilePath $TargetFile
Write-Host "Verification Subject: $($verification.SignerCertificate.Subject)" -ForegroundColor Green
Write-Host "Verification Status:  $($verification.Status)" -ForegroundColor Green

if ($verification.Status -notin @("Valid", "UnknownError")) { # UnknownError is expected for self-signed without root trust
    Write-Error "Authenticode signing verification failed with status: $($verification.Status)"
}

Write-Host "==> Authenticode signing step completed successfully." -ForegroundColor Green
