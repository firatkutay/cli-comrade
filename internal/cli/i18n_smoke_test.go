package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestI18nSmokeFirstRunNoticeInTurkish is FAZ 9's migration-regression TR
// smoke test: with the language resolved to Turkish (via COMRADE_LANG,
// exactly like the phase's own acceptance criterion), the first-run
// notice `comrade config get` prints on first use must be the Turkish
// catalog string, not the English one.
func TestI18nSmokeFirstRunNoticeInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, stderr, err := execRootSplit(t, "dev", "config", "get", "general.mode")
	require.NoError(t, err)

	assert.Contains(t, stderr, "Varsayılan ayar dosyası oluşturuldu", "expected the Turkish first-run notice, got: %s", stderr)
	assert.NotContains(t, stderr, "Created default config at")
}

// TestI18nSmokeYoloWarningInTurkish is the second of FAZ 9's "a couple of
// commands" TR smoke tests: `comrade do --yolo` must print CLAUDE.md's
// mandatory --yolo warning in Turkish when the language resolves to tr.
// No mock LLM server is set up (matching root_test.go's own
// TestRootDispatchUnmatchedArgsFallsBackToDo pattern) — the warning is
// printed by setupCLIRuntime BEFORE plan generation is ever attempted, so
// it appears on stderr regardless of what happens afterward.
func TestI18nSmokeYoloWarningInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	_, stderr, _ := execRootSplit(t, "dev", "do", "something", "--yolo")

	assert.Contains(t, stderr, "auto modda destructive/elevated adımlar ONAY ALINMADAN çalışabilir",
		"expected the Turkish --yolo warning, got: %s", stderr)
	assert.NotContains(t, stderr, "destructive/elevated steps may run WITHOUT confirmation")
}
