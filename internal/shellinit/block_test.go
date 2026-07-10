package shellinit_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

func TestApplyBlockInstallsIntoEmptyFile(t *testing.T) {
	updated, status, err := shellinit.ApplyBlock("", shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, shellinit.StatusInstalled, status)

	block, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, block+"\n", updated)
}

func TestApplyBlockAppendsAfterExistingContent(t *testing.T) {
	original := "export PATH=$PATH:/opt/tools\nalias ll='ls -la'\n"

	updated, status, err := shellinit.ApplyBlock(original, shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, shellinit.StatusInstalled, status)

	block, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, original+"\n"+block+"\n", updated)
	assert.Equal(t, 1, strings.Count(updated, shellinit.MarkerBegin))
}

func TestApplyBlockIsIdempotentOnSecondRun(t *testing.T) {
	original := "# my bashrc\nexport EDITOR=vim\n"

	first, status1, err := shellinit.ApplyBlock(original, shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, shellinit.StatusInstalled, status1)
	assert.Equal(t, 1, strings.Count(first, shellinit.MarkerBegin))

	second, status2, err := shellinit.ApplyBlock(first, shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, shellinit.StatusAlreadyInstalled, status2)
	assert.Equal(t, first, second, "a second install must not change a byte")
	assert.Equal(t, 1, strings.Count(second, shellinit.MarkerBegin))
}

func TestApplyBlockUpgradesChangedContentInPlace(t *testing.T) {
	oldBlock := shellinit.MarkerBegin + "\n# an older cli-comrade hook body\n" + shellinit.MarkerEnd
	original := "# rc file header\n\n" + oldBlock + "\n\n# trailing user config\nexport FOO=bar\n"

	updated, status, err := shellinit.ApplyBlock(original, shellinit.Bash)
	require.NoError(t, err)
	assert.Equal(t, shellinit.StatusUpgraded, status)
	assert.Equal(t, 1, strings.Count(updated, shellinit.MarkerBegin), "upgrade must not leave a duplicate block")

	newBlock, err := shellinit.Block(shellinit.Bash)
	require.NoError(t, err)
	want := "# rc file header\n\n" + newBlock + "\n\n# trailing user config\nexport FOO=bar\n"
	assert.Equal(t, want, updated)
}

// TestApplyBlockUpgradesRealPreExitCodeFixPowerShellSnippet is the
// concrete regression proof (not a generic placeholder-body upgrade like
// TestApplyBlockUpgradesChangedContentInPlace above) that a user who
// already ran "comrade init powershell" before the $?/$LASTEXITCODE
// exit-code fix gets the fix automatically, correctly, and in place the
// next time they re-run "comrade init powershell" — no manual --remove/
// reinstall needed. oldRealSnippet is the EXACT pre-fix
// internal/shellinit/snippets/powershell.ps1 content (the one that
// recorded a CommandNotFoundException, e.g. a typo'd command, as exit 0
// — see docs/history/PROGRESS.md for the live bug report this fixes).
func TestApplyBlockUpgradesRealPreExitCodeFixPowerShellSnippet(t *testing.T) {
	oldRealSnippet := `if (Get-Command comrade -ErrorAction SilentlyContinue) {
    $global:__ComradeOriginalPrompt = $function:prompt
    $global:__ComradeLastCommand = $null
    function global:prompt {
        $ec = $global:LASTEXITCODE
        if ($null -eq $ec) { $ec = 0 }
        try {
            $last = Get-History -Count 1
            if ($last) {
                $cmd = $last.CommandLine
                if ($cmd -and $cmd -ne $global:__ComradeLastCommand) {
                    $global:__ComradeLastCommand = $cmd
                    comrade hook record --shell powershell --exit $ec --command $cmd 2>$null | Out-Null
                }
            }
        } catch {
        }
        if ($global:__ComradeOriginalPrompt) {
            & $global:__ComradeOriginalPrompt
        } else {
            "PS $($executionContext.SessionState.Path.CurrentLocation)$('>' * ($nestedPromptLevel + 1)) "
        }
    }
}`
	oldBlock := shellinit.MarkerBegin + "\n" + oldRealSnippet + "\n" + shellinit.MarkerEnd
	original := oldBlock + "\n"

	updated, status, err := shellinit.ApplyBlock(original, shellinit.PowerShell)
	require.NoError(t, err)
	assert.Equal(t, shellinit.StatusUpgraded, status, "a user's existing pre-fix profile must be reported as upgraded, not silently left alone")
	assert.Equal(t, 1, strings.Count(updated, shellinit.MarkerBegin), "upgrade must not leave a duplicate block")

	newBlock, err := shellinit.Block(shellinit.PowerShell)
	require.NoError(t, err)
	assert.Equal(t, newBlock+"\n", updated)
	assert.Contains(t, updated, "$success = $?", "the upgraded profile must contain the fixed $?-first-statement logic")
	assert.NotContains(t, updated, "$ec = $global:LASTEXITCODE\n        if ($null -eq $ec) { $ec = 0 }", "the upgraded profile must no longer contain the buggy LASTEXITCODE-only logic")
}

func TestApplyBlockUnsupportedShellErrors(t *testing.T) {
	_, _, err := shellinit.ApplyBlock("", shellinit.Shell("tcsh"))
	assert.Error(t, err)
}

func TestApplyBlockMismatchedMarkersErrors(t *testing.T) {
	_, _, err := shellinit.ApplyBlock(shellinit.MarkerBegin+"\nbroken, no end marker\n", shellinit.Bash)
	assert.ErrorContains(t, err, "matching")
}

func TestRemoveBlockDeletesInstalledBlock(t *testing.T) {
	installed, status, err := shellinit.ApplyBlock("", shellinit.Bash)
	require.NoError(t, err)
	require.Equal(t, shellinit.StatusInstalled, status)

	updated, removed := shellinit.RemoveBlock(installed)
	assert.True(t, removed)
	assert.Equal(t, "", updated)
	assert.False(t, strings.Contains(updated, shellinit.MarkerBegin))
}

func TestRemoveBlockPreservesSurroundingUserContent(t *testing.T) {
	original := "before line\nsecond before line\n"
	installed, _, err := shellinit.ApplyBlock(original, shellinit.Bash)
	require.NoError(t, err)

	// Simulate the user adding more config below the installed block.
	withTrailer := installed + "after line\n"

	updated, removed := shellinit.RemoveBlock(withTrailer)
	assert.True(t, removed)
	assert.Equal(t, original+"after line\n", updated)
	assert.False(t, strings.Contains(updated, shellinit.MarkerBegin))
}

func TestRemoveBlockNoMarkersIsFriendlyNoop(t *testing.T) {
	original := "just some ordinary rc content\nalias x=y\n"

	updated, removed := shellinit.RemoveBlock(original)
	assert.False(t, removed)
	assert.Equal(t, original, updated)
}

func TestRemoveBlockMismatchedMarkersIsNoop(t *testing.T) {
	original := shellinit.MarkerBegin + "\nbroken, no end marker\n"

	updated, removed := shellinit.RemoveBlock(original)
	assert.False(t, removed)
	assert.Equal(t, original, updated)
}

func TestApplyThenRemoveIsIdempotentRoundTrip(t *testing.T) {
	original := "# header\nexport A=1\n"

	installed, _, err := shellinit.ApplyBlock(original, shellinit.Zsh)
	require.NoError(t, err)

	removed, ok := shellinit.RemoveBlock(installed)
	require.True(t, ok)
	assert.Equal(t, original, removed)

	// Removing again is a no-op.
	removedAgain, ok := shellinit.RemoveBlock(removed)
	assert.False(t, ok)
	assert.Equal(t, removed, removedAgain)
}
