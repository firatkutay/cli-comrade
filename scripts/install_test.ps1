#Requires -Version 5.1
<#
.SYNOPSIS
    PowerShell unit tests for scripts/install.ps1's checksums.txt
    cosign-signature verification (GitHub issue #43) -- the Windows-side
    counterpart of scripts/install_test.sh's signature tests for
    scripts/install.sh (GitHub issue #28).

.DESCRIPTION
    Runs entirely offline: no network download, no real "comrade" install.
    Every signature test signs and verifies with an EPHEMERAL,
    in-test-generated P-256 key pair (never the real embedded $CosignPub /
    production key) -- the same approach scripts/install_test.sh and
    internal/update/signature_test.go both already use.

    Dot-sources install.ps1 with $env:COMRADE_INSTALL_PS1_TEST set, which
    skips its top-level Invoke-Main call (see install.ps1's own trailing
    guard) -- the PowerShell equivalent of install_test.sh's
    COMRADE_INSTALL_SH_TEST=1 sourcing trick. This intentionally does not
    depend on Pester (bundled Pester versions differ meaningfully between
    Windows PowerShell 5.1 and PowerShell 7), so this runs identically,
    with no extra module dependency, on both runtimes -- exactly the
    property this test suite exists to prove.

.EXAMPLE
    pwsh -NoProfile -File scripts/install_test.ps1
    powershell -NoProfile -File scripts/install_test.ps1
#>
[CmdletBinding()]
param()

$ErrorActionPreference = "Stop"

$scriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$env:COMRADE_INSTALL_PS1_TEST = "1"
. (Join-Path $scriptDir "install.ps1")

$script:TestsRun = 0
$script:Failures = 0

function Invoke-ComradeTest {
    param(
        [Parameter(Mandatory = $true)][string]$Name,
        [Parameter(Mandatory = $true)][scriptblock]$Body
    )
    $script:TestsRun++
    try {
        & $Body
        Write-Host "ok: $Name"
    }
    catch {
        $script:Failures++
        Write-Host "FAIL: $Name" -ForegroundColor Red
        Write-Host "  $($_.Exception.Message)" -ForegroundColor Red
    }
}

function Assert-True {
    param(
        [Parameter(Mandatory = $true)][bool]$Condition,
        [Parameter(Mandatory = $true)][string]$Message
    )
    if (-not $Condition) {
        throw $Message
    }
}

# ConvertTo-TestSubjectPublicKeyInfoPem wraps a fresh ECDsaCng key's own
# EccPublicBlob (magic+cbKey header, then X, then Y) into a full P-256
# SubjectPublicKeyInfo PEM, using install.ps1's own $P256SpkiPrefix -- so
# the fixture this builds is parsed by the EXACT SAME
# Get-P256PointFromSubjectPublicKeyInfo code path production callers use,
# not a separate test-only implementation.
function ConvertTo-TestSubjectPublicKeyInfoPem {
    param([Parameter(Mandatory = $true)][byte[]]$EccPublicBlob)

    $headerLen = $EccPublicBlob.Length - 64
    $x = $EccPublicBlob[$headerLen..($headerLen + 31)]
    $y = $EccPublicBlob[($headerLen + 32)..($headerLen + 63)]
    $der = $P256SpkiPrefix + $x + $y
    $b64 = [Convert]::ToBase64String($der)

    $lines = New-Object System.Collections.Generic.List[string]
    for ($i = 0; $i -lt $b64.Length; $i += 64) {
        $len = [Math]::Min(64, $b64.Length - $i)
        $lines.Add($b64.Substring($i, $len))
    }
    return "-----BEGIN PUBLIC KEY-----`n" + ($lines -join "`n") + "`n-----END PUBLIC KEY-----`n"
}

# ConvertTo-DerInteger DER-encodes one unsigned big-endian coordinate as an
# ASN.1 INTEGER: strips redundant leading zero bytes (DER's minimal-encoding
# rule), then prepends a single 0x00 if the remaining high bit would
# otherwise read as a sign bit -- the test-side mirror of install.ps1's
# Read-DerUnsignedInteger, used here to build realistic cosign-shaped DER
# signature fixtures from a raw P1363 signature.
function ConvertTo-DerInteger {
    param([Parameter(Mandatory = $true)][byte[]]$Coordinate)

    $i = 0
    while ($i -lt ($Coordinate.Length - 1) -and $Coordinate[$i] -eq 0x00) { $i++ }
    $trimmed = $Coordinate[$i..($Coordinate.Length - 1)]
    if ($trimmed[0] -band 0x80) {
        $trimmed = [byte[]](0x00) + $trimmed
    }
    return [byte[]](0x02, [byte]$trimmed.Length) + $trimmed
}

# ConvertTo-DerEcdsaSignature converts a raw 64-byte P1363 r||s signature
# (what ECDsaCng.SignData produces) into cosign/openssl's ASN.1 DER
# SEQUENCE-of-two-INTEGERs format -- the reverse of install.ps1's
# ConvertFrom-DerEcdsaSignature, used here to build realistic test fixtures.
function ConvertTo-DerEcdsaSignature {
    param([Parameter(Mandatory = $true)][byte[]]$RawSignature)

    if ($RawSignature.Length -ne 64) {
        throw "install_test.ps1: internal error -- expected a 64-byte raw P1363 signature, got $($RawSignature.Length)"
    }
    $rInt = ConvertTo-DerInteger -Coordinate $RawSignature[0..31]
    $sInt = ConvertTo-DerInteger -Coordinate $RawSignature[32..63]
    $body = $rInt + $sInt
    return [byte[]](0x30, [byte]$body.Length) + $body
}

# New-TestFixture generates one ephemeral P-256 signing key plus a signed
# checksums.txt fixture, returning everything a signature test needs.
# Callers own disposing the returned SigningKey.
function New-TestFixture {
    $signingKey = [System.Security.Cryptography.ECDsaCng]::new(256)
    $signingKey.HashAlgorithm = [System.Security.Cryptography.CngAlgorithm]::Sha256

    $pubBlob = $signingKey.Key.Export([System.Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
    $pubPem = ConvertTo-TestSubjectPublicKeyInfoPem -EccPublicBlob $pubBlob

    $checksumsText = "deadbeef00112233445566778899aabbccddeeff00112233445566778899aa  comrade_9.9.9_linux_amd64.tar.gz`n"
    $checksumsBytes = [System.Text.Encoding]::ASCII.GetBytes($checksumsText)

    $rawSig = $signingKey.SignData($checksumsBytes)
    $derSig = ConvertTo-DerEcdsaSignature -RawSignature $rawSig
    $sigBase64 = [Convert]::ToBase64String($derSig)

    return [PSCustomObject]@{
        SigningKey     = $signingKey
        PubPem         = $pubPem
        ChecksumsBytes = $checksumsBytes
        ChecksumsText  = $checksumsText
        SignatureBase64 = $sigBase64
    }
}

# --- a validly-signed checksums.txt is accepted ---
function Test-ValidSignatureAccepted {
    $fixture = New-TestFixture
    try {
        $verifier = New-CosignEcdsaVerifier -PubPem $fixture.PubPem
        try {
            Test-ChecksumsSignature -Verifier $verifier -ChecksumsBytes $fixture.ChecksumsBytes -SignatureBase64 $fixture.SignatureBase64
        }
        finally {
            $verifier.Dispose()
        }
    }
    finally {
        $fixture.SigningKey.Dispose()
    }
}

# --- a checksums.txt tampered with AFTER signing is rejected ---
#
# Non-vacuity proof for this test suite (see this PR's own report for the
# executed proof): temporarily changing Test-ChecksumsSignature's
# `-not $Verifier.VerifyData(...)` check to always treat the signature as
# valid makes this test FAIL, confirming it actually exercises rejection
# rather than vacuously passing.
function Test-TamperedChecksumsRejected {
    $fixture = New-TestFixture
    try {
        $tamperedBytes = [System.Text.Encoding]::ASCII.GetBytes($fixture.ChecksumsText + "tampered-extra-line`n")

        $verifier = New-CosignEcdsaVerifier -PubPem $fixture.PubPem
        $rejected = $false
        try {
            try {
                Test-ChecksumsSignature -Verifier $verifier -ChecksumsBytes $tamperedBytes -SignatureBase64 $fixture.SignatureBase64
            }
            catch {
                $rejected = $true
            }
        }
        finally {
            $verifier.Dispose()
        }
        Assert-True $rejected "expected a tampered checksums.txt to be REJECTED, but Test-ChecksumsSignature reported success"
    }
    finally {
        $fixture.SigningKey.Dispose()
    }
}

# --- a checksums.txt.sig that doesn't match the key at all is rejected ---
function Test-WrongKeyRejected {
    $fixture = New-TestFixture
    try {
        $otherKey = [System.Security.Cryptography.ECDsaCng]::new(256)
        try {
            $otherPubBlob = $otherKey.Key.Export([System.Security.Cryptography.CngKeyBlobFormat]::EccPublicBlob)
            $otherPubPem = ConvertTo-TestSubjectPublicKeyInfoPem -EccPublicBlob $otherPubBlob

            $verifier = New-CosignEcdsaVerifier -PubPem $otherPubPem
            $rejected = $false
            try {
                try {
                    Test-ChecksumsSignature -Verifier $verifier -ChecksumsBytes $fixture.ChecksumsBytes -SignatureBase64 $fixture.SignatureBase64
                }
                catch {
                    $rejected = $true
                }
            }
            finally {
                $verifier.Dispose()
            }
            Assert-True $rejected "expected a signature verified against the WRONG public key to be REJECTED"
        }
        finally {
            $otherKey.Dispose()
        }
    }
    finally {
        $fixture.SigningKey.Dispose()
    }
}

# --- Confirm-AllowUnsignedOrFail: default policy is fail-closed ---
function Test-AllowUnsignedOrFailAbortsByDefault {
    Remove-Item Env:\COMRADE_INSTALL_ALLOW_UNSIGNED -ErrorAction SilentlyContinue
    $threw = $false
    try {
        Confirm-AllowUnsignedOrFail -Reason "test reason."
    }
    catch {
        $threw = $true
    }
    Assert-True $threw "expected Confirm-AllowUnsignedOrFail to throw when COMRADE_INSTALL_ALLOW_UNSIGNED is unset"
}

# --- Confirm-AllowUnsignedOrFail: explicit override warns and continues ---
function Test-AllowUnsignedOrFailWarnsAndContinuesWhenOverridden {
    $env:COMRADE_INSTALL_ALLOW_UNSIGNED = "1"
    try {
        $warnings = $null
        Confirm-AllowUnsignedOrFail -Reason "test reason." -WarningVariable warnings -WarningAction SilentlyContinue
        $warningText = ($warnings | ForEach-Object { $_.Message }) -join ' '
        Assert-True ($warningText -match [regex]::Escape("test reason.")) "expected the WARNING to mention the reason, got: $warningText"
        Assert-True ($warningText -match [regex]::Escape("COMRADE_INSTALL_ALLOW_UNSIGNED is set")) "expected the WARNING to mention the override, got: $warningText"
    }
    finally {
        Remove-Item Env:\COMRADE_INSTALL_ALLOW_UNSIGNED -ErrorAction SilentlyContinue
    }
}

# --- ConvertFrom-DerEcdsaSignature: round-trips a freshly-signed signature ---
function Test-DerSignatureRoundTrip {
    $key = [System.Security.Cryptography.ECDsaCng]::new(256)
    try {
        $key.HashAlgorithm = [System.Security.Cryptography.CngAlgorithm]::Sha256
        $data = [System.Text.Encoding]::ASCII.GetBytes("round-trip-test")
        $rawSig = $key.SignData($data)
        $der = ConvertTo-DerEcdsaSignature -RawSignature $rawSig
        $roundTripped = ConvertFrom-DerEcdsaSignature -Der $der

        Assert-True ($roundTripped.Length -eq 64) "expected a 64-byte round-tripped signature, got $($roundTripped.Length)"
        for ($i = 0; $i -lt 64; $i++) {
            Assert-True ($roundTripped[$i] -eq $rawSig[$i]) "byte $i mismatched after DER round-trip (expected $($rawSig[$i]), got $($roundTripped[$i]))"
        }
    }
    finally {
        $key.Dispose()
    }
}

# --- Get-P256PointFromSubjectPublicKeyInfo: rejects a malformed key ---
function Test-RejectsMalformedSubjectPublicKeyInfo {
    $bogus = [byte[]](1..40)
    $threw = $false
    try {
        Get-P256PointFromSubjectPublicKeyInfo -Der $bogus | Out-Null
    }
    catch {
        $threw = $true
    }
    Assert-True $threw "expected a malformed SubjectPublicKeyInfo to be rejected"
}

# --- ConvertFrom-DerEcdsaSignature: rejects a truncated/malformed signature ---
function Test-RejectsMalformedDerSignature {
    $bogus = [byte[]](0x30, 0x04, 0x02, 0x01, 0x01, 0x99) # SEQUENCE len mismatch
    $threw = $false
    try {
        ConvertFrom-DerEcdsaSignature -Der $bogus | Out-Null
    }
    catch {
        $threw = $true
    }
    Assert-True $threw "expected a malformed DER signature to be rejected"
}

Invoke-ComradeTest "Test-ValidSignatureAccepted" { Test-ValidSignatureAccepted }
Invoke-ComradeTest "Test-TamperedChecksumsRejected" { Test-TamperedChecksumsRejected }
Invoke-ComradeTest "Test-WrongKeyRejected" { Test-WrongKeyRejected }
Invoke-ComradeTest "Test-AllowUnsignedOrFailAbortsByDefault" { Test-AllowUnsignedOrFailAbortsByDefault }
Invoke-ComradeTest "Test-AllowUnsignedOrFailWarnsAndContinuesWhenOverridden" { Test-AllowUnsignedOrFailWarnsAndContinuesWhenOverridden }
Invoke-ComradeTest "Test-DerSignatureRoundTrip" { Test-DerSignatureRoundTrip }
Invoke-ComradeTest "Test-RejectsMalformedSubjectPublicKeyInfo" { Test-RejectsMalformedSubjectPublicKeyInfo }
Invoke-ComradeTest "Test-RejectsMalformedDerSignature" { Test-RejectsMalformedDerSignature }

Write-Host "----"
Write-Host "install_test.ps1: $($script:TestsRun) test(s) run, $($script:Failures) failure(s)"

if ($script:Failures -ne 0) {
    exit 1
}
