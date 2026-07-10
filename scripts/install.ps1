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

function Get-ComradeArch {
    switch ($env:PROCESSOR_ARCHITECTURE) {
        "AMD64" { return "amd64" }
        "ARM64" { return "arm64" }
        default { throw "install.ps1: unsupported architecture: $env:PROCESSOR_ARCHITECTURE" }
    }
}

# Resolve-BaseUrl deliberately avoids api.github.com/repos/.../releases/latest:
# that endpoint is unauthenticated and rate-limited to 60 req/hr per source
# IP, which is hostile to an irm|iex one-liner shared publicly. GitHub's
# no-API "latest/download/<asset>" redirect has no such limit, so the
# default (unpinned) path resolves to that; a pinned -Version/$env:COMRADE_VERSION
# uses the equivalent tag-scoped download URL instead. Either way the actual
# version number is read back out of checksums.txt's matched filename below,
# never out of a separate API/version-lookup call.
function Resolve-BaseUrl {
    param([string]$Requested)
    if ($Requested) {
        return "https://github.com/$Repo/releases/download/$Requested"
    }
    return "https://github.com/$Repo/releases/latest/download"
}

$arch = Get-ComradeArch
$baseUrl = Resolve-BaseUrl -Requested $Version
$archiveSuffix = "_windows_${arch}.zip"

$work = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
New-Item -ItemType Directory -Path $work | Out-Null
try {
    Write-Host "install.ps1: fetching checksums..."
    Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile (Join-Path $work "checksums.txt")

    $checksums = Get-Content (Join-Path $work "checksums.txt")
    $expectedLine = $checksums | Where-Object { $_ -match ([regex]::Escape($archiveSuffix) + '$') }
    if (-not $expectedLine) {
        throw "install.ps1: no release asset found for windows/$arch (checked $baseUrl/checksums.txt)"
    }
    if ($expectedLine -is [array]) {
        $expectedLine = $expectedLine[0]
    }
    $parts = -split $expectedLine
    $expectedHash = $parts[0]
    $archive = $parts[1]
    $versionNumber = ($archive -replace '^comrade_', '') -replace ([regex]::Escape($archiveSuffix) + '$'), ''

    Write-Host "install.ps1: downloading $archive (v$versionNumber)..."
    Invoke-WebRequest -Uri "$baseUrl/$archive" -OutFile (Join-Path $work $archive)

    Write-Host "install.ps1: verifying checksum..."
    $actualHash = (Get-FileHash -Algorithm SHA256 (Join-Path $work $archive)).Hash
    if ($actualHash.ToLower() -ne $expectedHash.ToLower()) {
        throw "install.ps1: checksum mismatch for $archive (expected $expectedHash, got $actualHash)"
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

    Write-Host "install.ps1: run 'comrade init powershell' to set up shell integration (error capture + completions)."
}
finally {
    Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}
