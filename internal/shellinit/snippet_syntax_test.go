package shellinit_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// writeSnippetToTempFile writes shell's embedded Snippet content to a
// fresh temp file with ext, so an external interpreter's own syntax
// checker can parse it directly — mirrors internal/cli/scripts_test.go's
// exact pattern for scripts/install.sh and scripts/install.ps1, applied
// here to the go:embed'd hook snippets instead of the standalone
// installer scripts.
func writeSnippetToTempFile(t *testing.T, shell shellinit.Shell, ext string) string {
	t.Helper()
	body, err := shellinit.Snippet(shell)
	require.NoError(t, err)
	path := filepath.Join(t.TempDir(), "snippet"+ext)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// TestZshSnippetIsSyntacticallyValid syntax-checks the embedded zsh hook
// snippet (snippets/zsh.sh, including this round's space-triggered hint
// widget) with `zsh -n` (parse-only, no execution) — skipped when zsh
// isn't on PATH, exactly like TestInstallShIsValidPOSIXShell skips
// without sh (internal/cli/scripts_test.go).
func TestZshSnippetIsSyntacticallyValid(t *testing.T) {
	zshPath, err := exec.LookPath("zsh")
	if err != nil {
		t.Skip("zsh not found on PATH; skipping zsh snippet syntax check")
	}

	path := writeSnippetToTempFile(t, shellinit.Zsh, ".sh")
	cmd := exec.Command(zshPath, "-n", path)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "zsh -n snippets/zsh.sh failed: %s", out)
}

// TestPowerShellSnippetIsSyntacticallyValid parses the embedded
// PowerShell hook snippet (snippets/powershell.ps1, including this
// round's Spacebar key-handler block) with PowerShell's own AST parser
// (again, no execution) — skipped when neither pwsh nor Windows
// PowerShell is on PATH, mirroring
// TestInstallPs1IsSyntacticallyValidPowerShell (internal/cli/scripts_test.go).
func TestPowerShellSnippetIsSyntacticallyValid(t *testing.T) {
	pwshPath, err := exec.LookPath("pwsh")
	if err != nil {
		pwshPath, err = exec.LookPath("powershell")
		if err != nil {
			t.Skip("neither pwsh nor powershell found on PATH; skipping PowerShell snippet syntax check")
		}
	}

	path := writeSnippetToTempFile(t, shellinit.PowerShell, ".ps1")
	check := fmt.Sprintf(
		`$errors = $null; [void][System.Management.Automation.Language.Parser]::ParseFile(%q, [ref]$null, [ref]$errors); if ($errors.Count -gt 0) { $errors | ForEach-Object { Write-Error $_ }; exit 1 }`,
		path,
	)
	cmd := exec.Command(pwshPath, "-NoProfile", "-Command", check)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "PowerShell syntax check of snippets/powershell.ps1 failed: %s", out)
}
