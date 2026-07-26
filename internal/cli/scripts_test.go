package cli

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestInstallShIsValidPOSIXShell syntax-checks scripts/install.sh with
// `sh -n` (parse-only, no execution) — no network access or actual
// installation is exercised, just that the script is syntactically
// well-formed POSIX sh.
func TestInstallShIsValidPOSIXShell(t *testing.T) {
	shPath, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sh not found on PATH; skipping install.sh syntax check")
	}

	scriptPath := filepath.Join(repoRoot(t), "scripts", "install.sh")
	cmd := exec.Command(shPath, "-n", scriptPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "sh -n install.sh failed: %s", out)
}

// TestInstallShConfigurePathInRc runs scripts/install_test.sh — the
// POSIX-sh unit tests for configure_path_in_rc and its helpers (the PATH
// auto-setup logic install.sh runs when the resolved install dir isn't
// already on PATH). Runs entirely offline against throwaway HOME dirs;
// no network access, no real install, and no changes to this machine's
// actual shell rc files. Prefers dash when available, since that's the
// real /bin/sh on Debian/Ubuntu — the actual `curl | sh` runtime — and
// falls back to sh otherwise.
func TestInstallShConfigurePathInRc(t *testing.T) {
	shPath, err := exec.LookPath("dash")
	if err != nil {
		shPath, err = exec.LookPath("sh")
		if err != nil {
			t.Skip("neither dash nor sh found on PATH; skipping install_test.sh")
		}
	}

	scriptPath := filepath.Join(repoRoot(t), "scripts", "install_test.sh")
	cmd := exec.Command(shPath, scriptPath)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "install_test.sh failed:\n%s", out)
}

// TestInstallPs1IsSyntacticallyValidPowerShell parses scripts/install.ps1
// with PowerShell's own AST parser (again, no execution) when pwsh or
// Windows PowerShell is available; skipped otherwise (neither is
// installed in this sandbox — see docs/history/phases/FAZ-04.md's deferred
// Windows-side verification note).
func TestInstallPs1IsSyntacticallyValidPowerShell(t *testing.T) {
	pwshPath, err := exec.LookPath("pwsh")
	if err != nil {
		pwshPath, err = exec.LookPath("powershell")
		if err != nil {
			t.Skip("neither pwsh nor powershell found on PATH; skipping install.ps1 syntax check")
		}
	}

	scriptPath := filepath.Join(repoRoot(t), "scripts", "install.ps1")
	check := fmt.Sprintf(
		`$errors = $null; [void][System.Management.Automation.Language.Parser]::ParseFile(%q, [ref]$null, [ref]$errors); if ($errors.Count -gt 0) { $errors | ForEach-Object { Write-Error $_ }; exit 1 }`,
		scriptPath,
	)
	cmd := exec.Command(pwshPath, "-NoProfile", "-Command", check)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "PowerShell syntax check of install.ps1 failed: %s", out)
}

// TestInstallPs1ChecksumsSignatureVerification runs scripts/install_test.ps1
// — the PowerShell unit tests for install.ps1's cosign checksums.txt
// signature verification added for GitHub issue #43 (New-CosignEcdsaVerifier
// / Test-ChecksumsSignature / Confirm-AllowUnsignedOrFail / the DER<->P1363
// conversion helpers). Runs entirely offline against ephemeral,
// in-test-generated ECDSA keys — no network access, no real install.
//
// Gated on runtime.GOOS == "windows" in addition to pwsh/powershell being on
// PATH: install_test.ps1 exercises System.Security.Cryptography.ECDsaCng and
// CngKey, which wrap the Windows CNG API and are unavailable (throw
// PlatformNotSupportedException) even under a `pwsh` installed on
// ubuntu-latest/macos-26's GitHub Actions runners — unlike
// TestInstallPs1IsSyntacticallyValidPowerShell above, which only parses the
// AST and so works identically cross-platform. Runs against BOTH `pwsh`
// (PowerShell 7) and `powershell` (Windows PowerShell 5.1) as independent
// subtests when each is present, since the ECDSA-verification API
// differences between the two runtimes are exactly what GitHub issue #43
// needed proven, not just one of them — windows-latest's runner image ships
// both.
func TestInstallPs1ChecksumsSignatureVerification(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("scripts/install_test.ps1 exercises Windows-only CNG APIs (ECDsaCng/CngKey); skipping on non-Windows")
	}

	scriptPath := filepath.Join(repoRoot(t), "scripts", "install_test.ps1")
	runtimes := []struct {
		label string
		bin   string
	}{
		{"pwsh (PowerShell 7)", "pwsh"},
		{"powershell (Windows PowerShell 5.1)", "powershell"},
	}

	for _, rt := range runtimes {
		rt := rt
		t.Run(rt.label, func(t *testing.T) {
			binPath, err := exec.LookPath(rt.bin)
			if err != nil {
				t.Skipf("%s not found on PATH; skipping", rt.bin)
			}
			cmd := exec.Command(binPath, "-NoProfile", "-File", scriptPath)
			out, err := cmd.CombinedOutput()
			require.NoError(t, err, "install_test.ps1 failed under %s:\n%s", rt.bin, out)
		})
	}
}
