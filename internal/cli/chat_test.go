package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunChatNonInteractiveStdinReportsFriendlyErrorInsteadOfHanging is
// QA MINOR-5's "also record"/(b) fix: `comrade chat` against a non-TTY
// stdin used to reach bubbletea, which needs a real terminal and hangs
// rather than failing — QA's own observed symptom. The guard fires
// before any config load (isTerminal is checked first in runChat), so
// this needs no isolated config dir, no LLM client, nothing — a real
// hang here would time out the whole test run rather than merely fail
// it, which is exactly what proves the guard runs BEFORE bubbletea ever
// starts, not after.
func TestRunChatNonInteractiveStdinReportsFriendlyErrorInsteadOfHanging(t *testing.T) {
	withIsolatedConfigDir(t)
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(&bytes.Buffer{})

	err := runChat(cmd, newTestLoaderFactory(), fakeTTY(false), false)

	require.Error(t, err)
	assert.Equal(t, "comrade chat needs an interactive terminal (stdin is not a TTY) — run it directly in a terminal, not piped or redirected.", err.Error())
}

// TestRunChatNonInteractiveStdinReportsFriendlyErrorInsteadOfHangingInTurkish
// is the same proof in Turkish — bestEffortTranslator resolves
// general.language from the isolated config dir's own file, with no
// COMRADE_LANG/LANG/LC_ALL env var set, exactly like Round C's residual
// TR-translation fix this reuses.
func TestRunChatNonInteractiveStdinReportsFriendlyErrorInsteadOfHangingInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "general.language", "tr")
	require.NoError(t, err)

	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetIn(&bytes.Buffer{})

	err = runChat(cmd, newTestLoaderFactory(), fakeTTY(false), false)

	require.Error(t, err)
	assert.Equal(t, "comrade chat, etkileşimli bir terminal gerektirir (stdin bir TTY değil) — doğrudan bir terminalde çalıştırın, boru hattına yönlendirmeyin.", err.Error())
}
