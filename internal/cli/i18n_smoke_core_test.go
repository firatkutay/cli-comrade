package cli

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// blockedDecoyPlanJSON is a minimal one-step plan whose lone step is a
// denylisted `rm -rf /` decoy — used purely to exercise info mode's
// Blocked-step rendering path (engine.RunDeps.Translator's MsgBlockedStep)
// under a mock provider, without needing any real API key.
const blockedDecoyPlanJSON = `{
  "summary": "a plan the model never should have produced",
  "steps": [
    {"command": "rm -rf /", "rationale": "a decoy the model must never actually produce", "risk": "read", "reversible": false}
  ]
}`

// TestI18nSmokeCoreCommandsRenderTurkish is FAZ 9's required TR smoke
// test across the product's three most load-bearing surfaces:
// `comrade do --info` (engine.RunDeps.Translator's Blocked-step
// rendering), `comrade auth status` (the status table's header/labels),
// and `comrade history` (the table header / empty-log message) — each
// run with COMRADE_LANG=tr and asserted to render the Turkish catalog
// text, never the English one.
func TestI18nSmokeCoreCommandsRenderTurkish(t *testing.T) {
	t.Run("do --info", func(t *testing.T) {
		withIsolatedConfigDir(t)
		server := newMockPlanServer(t, blockedDecoyPlanJSON)
		defer server.Close()
		t.Setenv("COMRADE_PROVIDER", "openai_compat")
		t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
		t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")
		t.Setenv("COMRADE_LANG", "tr")

		stdout, stderr, err := execRootSplit(t, "dev", "do", "print", "a", "marker", "--info")
		require.NoError(t, err, "stderr: %s", stderr)

		assert.Contains(t, stdout, "ENGELLENDİ(", "expected the Turkish BLOCKED rendering, got: %s", stdout)
		assert.NotContains(t, stdout, "BLOCKED(")
	})

	t.Run("auth status", func(t *testing.T) {
		withIsolatedConfigDir(t)
		t.Setenv("COMRADE_LANG", "tr")
		for _, envVar := range []string{
			"COMRADE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY",
			"COMRADE_OPENAI_COMPAT_API_KEY", "OPENAI_API_KEY",
			"COMRADE_GOOGLE_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY",
		} {
			t.Setenv(envVar, "")
		}

		stdout, stderr, err := execRootSplit(t, "dev", "auth", "status")
		require.NoError(t, err, "stderr: %s", stderr)

		assert.Contains(t, stdout, "SAĞLAYICI", "expected the Turkish table header, got: %s", stdout)
		assert.Contains(t, stdout, "DURUM", "expected the Turkish table header, got: %s", stdout)
		assert.Contains(t, stdout, "kayıtlı değil", "expected the Turkish not-set label, got: %s", stdout)
		assert.Contains(t, stdout, "anahtar gerekmez", "expected the Turkish ollama row, got: %s", stdout)
		assert.NotContains(t, stdout, "PROVIDER")
		assert.NotContains(t, stdout, "not set")
	})

	t.Run("history", func(t *testing.T) {
		withIsolatedConfigDir(t)
		t.Setenv("COMRADE_LANG", "tr")

		stdout, stderr, err := execRootSplit(t, "dev", "history")
		require.NoError(t, err, "stderr: %s", stderr)

		assert.Contains(t, stdout, "Henüz kayıtlı komut yok.", "expected the Turkish empty-log message, got: %s", stdout)
		assert.NotContains(t, stdout, "No commands recorded yet.")
	})
}

// TestI18nSmokeHelpTextRendersTurkish proves cobra's own --help output
// (a command's Short line, both standalone and inside a parent's
// "Available Commands" listing) is localized too — internal/cli/help.go's
// applyTranslatedHelp mechanism, distinct from every other command's
// direct tr.T(...) calls.
func TestI18nSmokeHelpTextRendersTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	out := execRoot(t, "dev", "--help")

	assert.Contains(t, out, "Serbest metinli bir istek için plan üretir ve aktif moda göre çalıştırır",
		"expected `do`'s Turkish Short text in the parent's Available Commands listing, got: %s", out)
	assert.NotContains(t, out, "Generate a plan for a free-text request")

	sub := execRoot(t, "dev", "auth", "login", "--help")
	assert.Contains(t, sub, "Bir sağlayıcı için API anahtarı kaydeder",
		"expected a nested command's own Turkish Short text, got: %s", sub)
}

// TestI18nSmokeFlagDescriptionRendersTurkish is this round's required TR
// smoke assertion: a per-flag --help description (help.go's
// flagUsageByName override, distinct from the Short-text mechanism above)
// must render in Turkish under COMRADE_LANG=tr, in the SAME --help block
// as the (already Turkish) command Short line — proving a TR user no
// longer sees a mixed-language --help screen.
func TestI18nSmokeFlagDescriptionRendersTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	out := execRoot(t, "dev", "do", "--help")

	assert.Contains(t, out, "Serbest metinli bir istek için plan üretir ve aktif moda göre çalıştırır",
		"expected `do`'s Turkish Short text, got: %s", out)
	assert.Contains(t, out, "üretilen planı çalıştırmadan yazdırır",
		"expected --dry-run's Turkish flag description in the same --help block, got: %s", out)
	assert.NotContains(t, out, "print the generated plan without executing it")

	history := execRoot(t, "dev", "history", "--help")
	assert.Contains(t, history, "gösterilecek en yeni kayıtların azami sayısı",
		"expected --limit's Turkish flag description, got: %s", history)
	assert.NotContains(t, history, "maximum number of most-recent entries to show")
}

// TestI18nSmokeFullSentenceErrorsRenderTurkish proves the standalone,
// full-sentence fmt.Errorf/errors.New user-facing error messages this
// round migrated (docs/phases/FAZ-09.md) actually render in Turkish under
// COMRADE_LANG=tr — the error text the user reads at the exact moment
// they need help, per the coordinator's own emphasis.
func TestI18nSmokeFullSentenceErrorsRenderTurkish(t *testing.T) {
	t.Run("auth login unknown provider (envOnlyTranslator, no config load)", func(t *testing.T) {
		t.Setenv("COMRADE_LANG", "tr")
		_, _, err := execRootSplit(t, "dev", "auth", "login", "bogus-provider")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bilinmeyen sağlayıcı")
		assert.NotContains(t, err.Error(), "unknown provider")
	})

	t.Run("flags --auto/--ask mutually exclusive (envOnlyTranslator, no config load)", func(t *testing.T) {
		t.Setenv("COMRADE_LANG", "tr")
		_, _, err := execRootSplit(t, "dev", "do", "something", "--auto", "--ask")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "yalnızca biri verilebilir")
		assert.NotContains(t, err.Error(), "only one of")
	})

	t.Run("init --print/--remove mutually exclusive (envOnlyTranslator, no config load)", func(t *testing.T) {
		t.Setenv("COMRADE_LANG", "tr")
		_, _, err := execRootSplit(t, "dev", "init", "bash", "--print", "--remove")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "birlikte kullanılamaz")
		assert.NotContains(t, err.Error(), "mutually exclusive")
	})

	t.Run("config models unknown provider (config-aware translator, function-level)", func(t *testing.T) {
		// fetchModelsForProvider's "unknown provider" branch only fires
		// for a provider string `config set`'s own validation wouldn't
		// normally allow through — exercised directly at the function
		// level, exactly like models_test.go's own unit tests of
		// fetchModelsForProvider, to reach that branch deterministically.
		cfg := config.Default()
		cfg.LLM.Provider = "bogus"
		tr := i18n.NewTranslator(i18n.LangTR)

		_, _, err := fetchModelsForProvider(context.Background(), io.Discard, cfg, tr)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "bilinmeyen sağlayıcı")
		assert.NotContains(t, err.Error(), "unknown provider")
	})
}
