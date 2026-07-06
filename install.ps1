# k-vote-cli installer (Windows PowerShell)
#
#   irm https://raw.githubusercontent.com/JungHoonGhae/k-vote-cli/main/install.ps1 | iex
#
# Environment variables:
#   $env:KVOTE_VERSION  pin a version (e.g. v0.4.0, default: latest)
#   $env:INSTALL_DIR    install location (default: $env:LOCALAPPDATA\kvote)
$ErrorActionPreference = "Stop"

$Repo = "JungHoonGhae/k-vote-cli"
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { Join-Path $env:LOCALAPPDATA "kvote" }

$Arch = switch ((Get-CimInstance Win32_Processor).Architecture) {
    12 { "arm64" }   # ARM64
    default { "amd64" }
}

# Resolve the latest tag from the releases/latest redirect (no API rate limit).
$Version = $env:KVOTE_VERSION
if (-not $Version) {
    $resp = Invoke-WebRequest -Uri "https://github.com/$Repo/releases/latest" -MaximumRedirection 0 -SkipHttpErrorCheck -ErrorAction SilentlyContinue
    $Version = ($resp.Headers.Location | Select-Object -First 1) -replace ".*/tag/", ""
}
if (-not $Version) { throw "Could not resolve latest version." }
$VerNoV = $Version.TrimStart("v")

$Asset = "kvote_${VerNoV}_windows_${Arch}.zip"
$Url = "https://github.com/$Repo/releases/download/$Version/$Asset"

Write-Host "Installing kvote $Version ($Arch)..."
$Tmp = Join-Path ([System.IO.Path]::GetTempPath()) ([System.Guid]::NewGuid())
New-Item -ItemType Directory -Path $Tmp | Out-Null
try {
    $Zip = Join-Path $Tmp $Asset
    Invoke-WebRequest -Uri $Url -OutFile $Zip

    # Verify against checksums.txt from the same release.
    $ChecksumFile = Join-Path $Tmp "checksums.txt"
    Invoke-WebRequest -Uri "https://github.com/$Repo/releases/download/$Version/checksums.txt" -OutFile $ChecksumFile
    $Expected = (Select-String -Path $ChecksumFile -Pattern ([regex]::Escape($Asset))).Line.Split(" ")[0]
    $Actual = (Get-FileHash -Algorithm SHA256 -Path $Zip).Hash.ToLower()
    if ($Expected -ne $Actual) { throw "Checksum mismatch for $Asset" }

    Expand-Archive -Path $Zip -DestinationPath $Tmp -Force
    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Move-Item -Path (Join-Path $Tmp "kvote.exe") -Destination (Join-Path $InstallDir "kvote.exe") -Force
}
finally {
    Remove-Item -Recurse -Force $Tmp -ErrorAction SilentlyContinue
}

# Add to the user PATH once.
$UserPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($UserPath -notlike "*$InstallDir*") {
    [Environment]::SetEnvironmentVariable("Path", "$UserPath;$InstallDir", "User")
    Write-Host "Added $InstallDir to your user PATH. Restart the terminal to use 'kvote'."
}

Write-Host ""
Write-Host "Installed to $InstallDir\kvote.exe"
Write-Host "Next: kvote doctor"
