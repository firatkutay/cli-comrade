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

# CosignPub is the project's real cosign ECDSA P-256 public key, embedded as
# a literal PEM here-string -- MUST stay byte-identical to
# internal/update/cosign.pub (the same key `comrade upgrade` and
# scripts/install.sh both already embed). Guarded by
# internal/update/install_sh_mirror_test.go's
# TestInstallPs1EmbedsExactCosignPub (a bidirectional drift check, the same
# shape as that file's pre-existing install.sh guard): it fails if this
# block and cosign.pub ever diverge. Do not hand-edit one without the other
# -- update cosign.pub first, then copy its exact bytes here. (That test
# normalizes CRLF/LF before comparing, since this file is committed with
# CRLF line endings per .gitattributes' `*.ps1 text eol=crlf`, while
# cosign.pub stays LF -- the PEM content must match byte-for-byte, but the
# file's own mandated line-ending convention is not part of "the key".)
#
# See "Trust model" in the module comment below for why the key travels
# WITH this script rather than being fetched at install time.
$CosignPub = @'
-----BEGIN PUBLIC KEY-----
MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAEH3Y238cPtsFJ3QnAzJvWnAXlhFHJ
Dp2q9+ZzFq1dNAeDgSbvLFXjvxsRTyqQCZbNq4MVWBxmeXch3wjW/ntoQQ==
-----END PUBLIC KEY-----
'@

# P256SpkiPrefix is the fixed 27-byte ASN.1 DER prefix every P-256
# (prime256v1) X.509 SubjectPublicKeyInfo encodes ahead of its 64-byte
# uncompressed EC point: SEQUENCE { SEQUENCE { OBJECT id-ecPublicKey, OBJECT
# prime256v1 }, BIT STRING (0x00 unused-bits byte, then 0x04
# uncompressed-point marker) }. Verified against internal/update/cosign.pub
# itself via `openssl asn1parse -dump` -- the same encoding cosign and
# openssl both produce for a plain (non-certificate) P-256 public key, and
# the same one $CosignPub above already is. Get-P256PointFromSubjectPublicKeyInfo
# below checks the embedded key matches this exact shape before trusting any
# bytes out of it, rather than assuming.
$P256SpkiPrefix = [byte[]](
    0x30, 0x59, 0x30, 0x13, 0x06, 0x07, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x02, 0x01,
    0x06, 0x08, 0x2a, 0x86, 0x48, 0xce, 0x3d, 0x03, 0x01, 0x07, 0x03, 0x42, 0x00, 0x04
)

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

# $arch/$baseUrl/$archiveSuffix are computed at top level (not inside
# Invoke-Main) and deliberately left UNINDENTED: internal/cli/release_names_test.go's
# drift guard greps for a column-0 `$archiveSuffix = "..."` line to cross-check
# this script's checksums.txt line-selection suffix against .goreleaser.yaml's
# own archive name template. Computing these here has no side effects beyond
# reading $env:PROCESSOR_ARCHITECTURE, so it is harmless even when this script
# is dot-sourced for testing (scripts/install_test.ps1) rather than actually run.
$arch = Get-ComradeArch
$baseUrl = Resolve-BaseUrl -Requested $Version
$archiveSuffix = "_windows_${arch}.zip"

# === checksums.txt cosign-signature verification (GitHub issue #43) ===
#
# Trust model (see GitHub issues #28/#43 and docs/SECURITY.md's "Trust model
# of the `curl | sh` bootstrap path" section for the full writeup):
# checksums.txt is downloaded over the same channel as the release archive,
# so a bare SHA-256 checksum only proves the archive matches the manifest --
# it proves nothing about who WROTE the manifest. This authenticates
# checksums.txt itself first, via a cosign ECDSA-P256/SHA-256 signature
# (checksums.txt.sig) checked against $CosignPub -- the exact mechanism
# `comrade upgrade` (internal/update/signature.go) and scripts/install.sh
# already use. Only once that signature verifies is the archive's own
# checksum trusted.
#
# PowerShell-specific note (why this isn't a straight port of install.sh's
# `openssl dgst -sha256 -verify`): the ECDSA-verification API differs
# between PowerShell 7 (`pwsh`, .NET Core/.NET 5+) and Windows PowerShell
# 5.1 (.NET Framework) -- .NET Framework's ECDsa never gained
# ImportSubjectPublicKeyInfo/ExportSubjectPublicKeyInfo or the
# DSASignatureFormat-based VerifyData overloads (both .NET 5+ additions), so
# a version built against those would only work under `pwsh`. Instead, this
# uses System.Security.Cryptography.ECDsaCng + CngKey.Import with a hand-built
# CNG ECCPUBLICBLOB -- ECDsaCng has existed with this exact shape since .NET
# Framework 4.6.1 (see "What's new in .NET Framework 4.6.1"'s ECDSA section)
# and is equally available under PowerShell 7 on Windows (CNG wraps the same
# OS API either way) -- ONE code path instead of a version branch. The
# ECCPUBLICBLOB header (BCRYPT_ECCKEY_BLOB's dwMagic + cbKey) is derived at
# runtime from a throwaway keypair (see New-EccPublicKeyBlob) rather than
# hardcoded from the bcrypt.h constant, since no Windows runtime was
# available in the sandbox that wrote this to verify that constant against.
# Cosign's signature format is ASN.1 DER (SEQUENCE of two INTEGERs); this
# converts it to the raw, fixed-width r||s (IEEE P1363) format ECDsaCng's
# classic VerifyData(data, signature) overload expects -- documented as
# ECDsa's default/only signature format prior to .NET Core 3.0's
# DSASignatureFormat (see ECDsa.SignData's own "Remarks": "This method will
# use IeeeP1363FixedFieldConcatenation").
#
# Fail-closed policy, identical to install.sh's: a machine that can't check
# the signature at all (couldn't load the embedded key, or no
# checksums.txt.sig was published for this release) gets the SAME abort a
# machine with a bad signature would, UNLESS $env:COMRADE_INSTALL_ALLOW_UNSIGNED
# is set -- which prints a loud warning every time and NEVER applies to an
# actual signature MISMATCH (that always aborts unconditionally, no
# override; see Test-ChecksumsSignature below).

# Confirm-AllowUnsignedOrFail is the single policy decision point for every
# way checksums.txt's cosign signature can fail to be CHECKED at all (as
# opposed to being checked and found invalid, which Test-ChecksumsSignature
# handles separately and NEVER routes through here).
function Confirm-AllowUnsignedOrFail {
    param([Parameter(Mandatory = $true)][string]$Reason)

    if ($env:COMRADE_INSTALL_ALLOW_UNSIGNED) {
        Write-Warning "install.ps1: WARNING -- $Reason `$env:COMRADE_INSTALL_ALLOW_UNSIGNED is set, so continuing with checksum-only verification (the same weaker guarantee this feature exists to close -- see docs/SECURITY.md)."
        return
    }
    throw "install.ps1: refusing to install -- $Reason Set `$env:COMRADE_INSTALL_ALLOW_UNSIGNED=1 to explicitly accept the weaker checksum-only guarantee (see docs/SECURITY.md), or resolve the problem and re-run."
}

# ConvertFrom-PemToDerBytes strips PEM armor (any "-----...-----" line) from
# $PemText and base64-decodes the remainder into raw DER bytes.
function ConvertFrom-PemToDerBytes {
    param([Parameter(Mandatory = $true)][string]$PemText)
    # Single-quoted so the backslashes reach -split's regex engine literally
    # -- a double-quoted "`r?`n" would have PowerShell's own backtick escapes
    # turn `` `r ``/`` `n `` into literal CR/LF characters BEFORE -split ever
    # sees them, corrupting the intended "optional \r, then \n" pattern.
    $lines = $PemText -split '\r?\n' | Where-Object { $_ -and $_ -notmatch '-----' }
    $b64 = ($lines -join '')
    return [Convert]::FromBase64String($b64)
}

# Get-P256PointFromSubjectPublicKeyInfo validates that $Der is a P-256
# SubjectPublicKeyInfo matching $P256SpkiPrefix exactly, and returns its
# uncompressed EC point's X/Y coordinates (32 bytes each, big-endian) -- the
# exact bytes CngKey.Import's EccPublicBlob format needs. This intentionally
# is not a general ASN.1 parser: it recognizes only the one fixed encoding
# cosign/openssl produce for a P-256 public key (the same shape $CosignPub
# already is), and rejects anything else (wrong curve, compressed point,
# malformed DER) rather than guessing.
function Get-P256PointFromSubjectPublicKeyInfo {
    param([Parameter(Mandatory = $true)][byte[]]$Der)

    $prefixLen = $P256SpkiPrefix.Length
    $expectedLen = $prefixLen + 64
    if ($Der.Length -ne $expectedLen) {
        throw "install.ps1: embedded cosign public key is not a $expectedLen-byte P-256 SubjectPublicKeyInfo (got $($Der.Length) bytes) -- refusing to install"
    }
    for ($i = 0; $i -lt $prefixLen; $i++) {
        if ($Der[$i] -ne $P256SpkiPrefix[$i]) {
            throw "install.ps1: embedded cosign public key does not match the expected P-256 SubjectPublicKeyInfo encoding at byte $i -- refusing to install"
        }
    }
    $x = $Der[$prefixLen..($prefixLen + 31)]
    $y = $Der[($prefixLen + 32)..($prefixLen + 63)]
    return [PSCustomObject]@{ X = $x; Y = $y }
}

# New-EccPublicKeyBlob builds a CNG ECCPUBLICBLOB (the byte format
# CngKey.Import expects for CngKeyBlobFormat.EccPublicBlob: BCRYPT_ECCKEY_BLOB
# header immediately followed by X then Y) from raw P-256 X/Y coordinates.
#
# Rather than hardcoding BCRYPT_ECCKEY_BLOB's 8-byte header (dwMagic + cbKey)
# from bcrypt.h's BCRYPT_ECDSA_PUBLIC_P256_MAGIC constant -- unverifiable
# against a real Windows runtime in the sandbox that wrote this -- this
# generates a throwaway P-256 ECDsaCng keypair on WHICHEVER runtime is
# actually executing the script, exports ITS OWN EccPublicBlob to read back
# the header bytes that runtime's own CNG provider produces, then splices
# our X/Y coordinates onto that real header. This is correct on both
# Windows PowerShell 5.1 and PowerShell 7 by construction, since both derive
# the header from the same underlying Windows CNG API -- it is derived at
# runtime, not assumed from documentation.
function New-EccPublicKeyBlob {
    param(
        [Parameter(Mandatory = $true)][byte[]]$X,
        [Parameter(Mandatory = $true)][byte[]]$Y
    )

    if ($X.Length -ne 32 -or $Y.Length -ne 32) {
        throw "install.ps1: internal error -- expected 32-byte P-256 X/Y coordinates, got $($X.Length)/$($Y.Length)"
    }

    $sampleKey = [System.Security.Cryptography.ECDsaCng]::new(256)
    try {
        $sampleBlob = $sampleKey.Key.Export([System.Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
    }
    finally {
        $sampleKey.Dispose()
    }

    $headerLen = $sampleBlob.Length - 64
    if ($headerLen -le 0) {
        throw "install.ps1: internal error -- unexpected EccPublicBlob length $($sampleBlob.Length) from this runtime's CNG provider"
    }
    $header = $sampleBlob[0..($headerLen - 1)]
    return $header + $X + $Y
}

# New-CosignEcdsaVerifier builds a ready-to-use ECDsaCng verifier from a PEM
# SubjectPublicKeyInfo (production callers always pass $CosignPub; tests
# pass an ephemeral test key's PEM instead -- see scripts/install_test.ps1).
# Callers own disposing the returned object.
function New-CosignEcdsaVerifier {
    param([Parameter(Mandatory = $true)][string]$PubPem)

    $der = ConvertFrom-PemToDerBytes -PemText $PubPem
    $point = Get-P256PointFromSubjectPublicKeyInfo -Der $der
    $blob = New-EccPublicKeyBlob -X $point.X -Y $point.Y

    $cngKey = [System.Security.Cryptography.CngKey]::Import($blob, [System.Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
    $ecdsa = [System.Security.Cryptography.ECDsaCng]::new($cngKey)
    $ecdsa.HashAlgorithm = [System.Security.Cryptography.CngAlgorithm]::Sha256
    return $ecdsa
}

# Read-DerUnsignedInteger reads one DER INTEGER (tag 0x02) at $Offset in
# $Bytes, returning its value as a fixed 32-byte big-endian array (stripping
# DER's single leading 0x00 sign-disambiguation padding byte if present, or
# left-padding with zeros if the encoded integer was shorter) plus the
# offset immediately after it -- the fixed coordinate width IEEE P1363
# expects for a P-256 signature component.
function Read-DerUnsignedInteger {
    param(
        [Parameter(Mandatory = $true)][byte[]]$Bytes,
        [Parameter(Mandatory = $true)][int]$Offset
    )

    if ($Offset + 2 -gt $Bytes.Length -or $Bytes[$Offset] -ne 0x02) {
        throw "install.ps1: checksums.txt.sig is not a valid DER ECDSA signature (expected a DER INTEGER at offset $Offset)"
    }
    $len = $Bytes[$Offset + 1]
    if ($len -band 0x80) {
        throw "install.ps1: checksums.txt.sig uses a long-form DER integer length, which is unexpected for a P-256 ECDSA signature"
    }
    if ($len -eq 0) {
        throw "install.ps1: checksums.txt.sig contains an empty DER INTEGER"
    }
    $start = $Offset + 2
    if ($start + $len -gt $Bytes.Length) {
        throw "install.ps1: checksums.txt.sig's DER INTEGER length ($len) runs past the end of the signature"
    }

    $content = if ($len -eq 1) { , $Bytes[$start] } else { $Bytes[$start..($start + $len - 1)] }

    if ($content.Length -eq 33 -and $content[0] -eq 0x00) {
        $content = $content[1..32]
    }
    if ($content.Length -gt 32) {
        throw "install.ps1: checksums.txt.sig contains a coordinate longer than a P-256 integer"
    }
    if ($content.Length -lt 32) {
        $content = ([byte[]]::new(32 - $content.Length)) + $content
    }

    return [PSCustomObject]@{ Value = $content; NextOffset = $start + $len }
}

# ConvertFrom-DerEcdsaSignature converts a cosign/openssl-style ASN.1 DER
# ECDSA signature (SEQUENCE of two INTEGERs, r then s) into the raw 64-byte
# r||s (IEEE P1363 fixed-field concatenation) format ECDsaCng's classic
# VerifyData expects.
function ConvertFrom-DerEcdsaSignature {
    param([Parameter(Mandatory = $true)][byte[]]$Der)

    if ($Der.Length -lt 8 -or $Der[0] -ne 0x30) {
        throw "install.ps1: checksums.txt.sig is not a valid DER ECDSA signature (missing SEQUENCE tag)"
    }
    $seqLen = $Der[1]
    if ($seqLen -band 0x80) {
        throw "install.ps1: checksums.txt.sig uses a long-form DER SEQUENCE length, which is unexpected for a P-256 ECDSA signature"
    }
    if (2 + $seqLen -ne $Der.Length) {
        throw "install.ps1: checksums.txt.sig's DER SEQUENCE length does not match the file size; refusing to install"
    }

    $r = Read-DerUnsignedInteger -Bytes $Der -Offset 2
    $s = Read-DerUnsignedInteger -Bytes $Der -Offset $r.NextOffset
    if ($s.NextOffset -ne $Der.Length) {
        throw "install.ps1: checksums.txt.sig has trailing bytes after its two DER INTEGERs; refusing to install"
    }

    return $r.Value + $s.Value
}

# Test-ChecksumsSignature verifies $ChecksumsBytes (checksums.txt's own raw
# bytes) against $SignatureBase64 (checksums.txt.sig's contents, cosign's
# base64-encoded ASN.1 DER ECDSA-P256/SHA-256 signature format) using
# $Verifier (from New-CosignEcdsaVerifier). This is the PowerShell-side
# mirror of internal/update/signature.go's verifyChecksumsSignatureWith and
# scripts/install.sh's verify_checksums_signature.
#
# A verification failure here is ALWAYS a hard, unconditional throw --
# $env:COMRADE_INSTALL_ALLOW_UNSIGNED does NOT apply (see
# Confirm-AllowUnsignedOrFail's own comment for why "couldn't check" and "a
# real mismatch" are different, non-overridable cases).
function Test-ChecksumsSignature {
    param(
        [Parameter(Mandatory = $true)][System.Security.Cryptography.ECDsaCng]$Verifier,
        [Parameter(Mandatory = $true)][byte[]]$ChecksumsBytes,
        [Parameter(Mandatory = $true)][string]$SignatureBase64
    )

    try {
        $sigDer = [Convert]::FromBase64String($SignatureBase64.Trim())
    }
    catch {
        throw "install.ps1: checksums.txt.sig is not valid base64; refusing to install."
    }

    $rawSignature = ConvertFrom-DerEcdsaSignature -Der $sigDer

    if (-not $Verifier.VerifyData($ChecksumsBytes, $rawSignature)) {
        throw "install.ps1: checksums.txt signature verification FAILED -- the downloaded checksums.txt does not match its signature. This can mean a compromised release, a corrupted download, or a tampered mirror. Refusing to install."
    }

    Write-Host "install.ps1: checksums.txt signature verified."
}

function Invoke-Main {
    # $arch, $baseUrl, $archiveSuffix, $Version, and $InstallDir are all
    # script-scope (see the top-level `param()` block and the unindented
    # assignments above Get-ComradeArch/Resolve-BaseUrl's definitions) and
    # referenced here directly, the same closure pattern $Repo/$BinName/
    # $CosignPub already use -- kept out of Invoke-Main's own parameters so
    # $archiveSuffix's assignment stays at column 0 for
    # internal/cli/release_names_test.go's drift guard (see its comment above).
    $work = Join-Path ([System.IO.Path]::GetTempPath()) ([System.IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $work | Out-Null
    try {
        Write-Host "install.ps1: fetching checksums..."
        $checksumsPath = Join-Path $work "checksums.txt"
        Invoke-WebRequest -Uri "$baseUrl/checksums.txt" -OutFile $checksumsPath -ErrorAction Stop

        # Authenticate checksums.txt itself -- via its cosign signature --
        # BEFORE any of its content (including the archive filename/version
        # parsed out of it below) is trusted. See the module-level "checksums.txt
        # cosign-signature verification" comment above for the full mechanism
        # and the fail-open/fail-closed policy.
        $cosignVerifier = $null
        try {
            $cosignVerifier = New-CosignEcdsaVerifier -PubPem $CosignPub
        }
        catch {
            Confirm-AllowUnsignedOrFail "the embedded cosign public key could not be loaded on this system ($($_.Exception.Message)), so checksums.txt's signature cannot be checked."
        }

        if ($cosignVerifier) {
            try {
                Write-Host "install.ps1: fetching checksums.txt.sig..."
                $sigPath = Join-Path $work "checksums.txt.sig"
                $sigDownloaded = $true
                try {
                    Invoke-WebRequest -Uri "$baseUrl/checksums.txt.sig" -OutFile $sigPath -ErrorAction Stop
                }
                catch {
                    $sigDownloaded = $false
                }

                if ($sigDownloaded) {
                    Write-Host "install.ps1: verifying checksums.txt signature..."
                    $checksumsBytes = [System.IO.File]::ReadAllBytes($checksumsPath)
                    $sigText = Get-Content -Raw -Path $sigPath
                    Test-ChecksumsSignature -Verifier $cosignVerifier -ChecksumsBytes $checksumsBytes -SignatureBase64 $sigText
                }
                else {
                    Confirm-AllowUnsignedOrFail "no checksums.txt.sig could be downloaded for this release (missing signature asset, or a network error), so checksums.txt's signature cannot be checked."
                }
            }
            finally {
                $cosignVerifier.Dispose()
            }
        }
        # $cosignVerifier being $null means Confirm-AllowUnsignedOrFail already
        # printed its warning above (or this line is unreached because it threw);
        # nothing further to do for signature verification in that case.

        $checksums = Get-Content $checksumsPath
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
        Invoke-WebRequest -Uri "$baseUrl/$archive" -OutFile (Join-Path $work $archive) -ErrorAction Stop

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
}

# Invoke-Main is skipped when this script is dot-sourced with
# $env:COMRADE_INSTALL_PS1_TEST set -- scripts/install_test.ps1 uses that to
# source install.ps1 and unit-test the signature-verification functions
# above directly, in isolation, with no network access and no real install.
# The PowerShell equivalent of install.sh's COMRADE_INSTALL_SH_TEST=1 guard.
if (-not $env:COMRADE_INSTALL_PS1_TEST) {
    Invoke-Main
}
