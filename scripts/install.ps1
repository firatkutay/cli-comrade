<#
.SYNOPSIS
    Installs the comrade CLI (https://github.com/firatkutay/cli-comrade) by
    downloading the latest (or -Version-pinned) GitHub release artifact,
    verifying its checksum, and adding it to the user's PATH.

.EXAMPLE
    irm https://raw.githubusercontent.com/firatkutay/cli-comrade/main/scripts/install.ps1 | iex
#>
#Requires -Version 5.1
[CmdletBinding()]
param(
    [string]$Version = $env:COMRADE_VERSION,
    [string]$InstallDir = (Join-Path $env:LOCALAPPDATA "Programs\cli-comrade")
)

$ErrorActionPreference = "Stop"

$Repo = "firatkutay/cli-comrade"
$BinName = "comrade.exe"

function Resolve-ComradeVersion {
    param([string]$Requested)
    if ($Requested) { return $Requested }
    $release = Invoke-RestMethod -Uri "https://api.github.com/repos/$Repo/releases/latest"
    return $release.tag_name
}

function Get-ComradeArch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "install.ps1: unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

$version = Resolve-ComradeVersion -Requested $Version
if (-not $version) {
    throw "install.ps1: could not resolve a version to install (pass -Version or set `$env:COMRADE_VERSION)"
}
$versionNumber = $version.TrimStart("v")
$arch = Get-ComradeArch
$archive = "comrade_${versionNumber}_windows_${arch}.zip"
$baseUrl = "https://github.com/$Repo/releases/download/$version"

$work = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    Write-Host "install.ps1: downloading $archive ($version)..."
    Invoke-WebRequest -Uri "$baseUrl/$archive" -OutFile (Join-Path $work $archive)
    Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile (Join-Path $work "checksums.txt")

    Write-Host "install.ps1: verifying checksum..."
    $checksums = Get-Content (Join-Path $work "checksums.txt")
    $expectedLine = $checksums | Where-Object { $_ -match [regex]::Escape($archive) + '$' }
    if (-not $expectedLine) {
        throw "install.ps1: no checksum entry found for $archive"
    }
    $expectedHash = ($expectedLine -split '\s+')[0]
    $actualHash = (Get-FileHash -Algorithm SHA256 (Join-Path $work $archive)).Hash
    if ($actualHash.ToLower() -ne $expectedHash.ToLower()) {
        throw "install.ps1: checksum mismatch for $archive"
    }

    Expand-Archive -Path (Join-Path $work $archive) -DestinationPath $work -Force

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    Copy-Item -Path (Join-Path $work $BinName) -Destination (Join-Path $InstallDir $BinName) -Force
    Write-Host "install.ps1: installed $BinName to $InstallDir"

    $userPath = [Environment]::GetEnvironmentVariable("Path", "User")
    if ($userPath -notlike "*$InstallDir*") {
        [Environment]::SetEnvironmentVariable("Path", "$userPath;$InstallDir", "User")
        Write-Host "install.ps1: added $InstallDir to your user PATH. Restart your terminal for it to take effect."
    }

    Write-Host "install.ps1: run 'comrade init' to set up shell integration."
}
finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
