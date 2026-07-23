package cli

import (
	"bytes"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// realNoKeyChainError builds the EXACT error shape internal/llm.Client.
// Complete/Stream actually returns for "no provider has a key configured
// at all" (see client.go's Complete/finalChainError) — not a
// hand-simplified stand-in — so classifyLLMError/translateLLMError are
// proven against the real wrap-chain shape, not an assumption about it.
// Always "anthropic" — every test using this only needs ONE concrete
// provider name to pin an exact message against, never a varying one.
func realNoKeyChainError() error {
	const provider = "anthropic"
	keyMissing := &llm.KeyMissingError{Provider: provider, EnvVars: []string{"COMRADE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"}}
	lastErr := fmt.Errorf("%s: %w", provider, keyMissing)
	return fmt.Errorf("llm: all providers failed: %w", lastErr)
}

// TestClassifyLLMErrorRecognizesRealNoKeyChain is QA MAJOR-1's core unit
// proof: errors.As sees straight through internal/llm's real, multi-level
// wrap chain to the original *llm.KeyMissingError, and the rendered
// message is the exact new friendly EN text — not a substring check, a
// full pin, so the wrap-chain (English, "all providers failed", the raw
// env var list) is provably ABSENT, not just "a friendly bit is present
// somewhere."
func TestClassifyLLMErrorRecognizesRealNoKeyChain(t *testing.T) {
	err := realNoKeyChainError()

	ok, translated := classifyLLMError(i18n.NewTranslator(i18n.LangEN), err)

	require.True(t, ok)
	assert.Equal(t, `no API key configured for anthropic yet — run "comrade auth login anthropic" to set one up (or export its env var directly; see "comrade auth login --help")`, translated.Error())
	assert.NotContains(t, translated.Error(), "all providers failed")
	assert.NotContains(t, translated.Error(), "COMRADE_ANTHROPIC_API_KEY")
}

// TestClassifyLLMErrorRecognizesRealNoKeyChainInTurkish is the same proof
// in Turkish — this project's established TR-smoke-test convention.
func TestClassifyLLMErrorRecognizesRealNoKeyChainInTurkish(t *testing.T) {
	err := realNoKeyChainError()

	ok, translated := classifyLLMError(i18n.NewTranslator(i18n.LangTR), err)

	require.True(t, ok)
	assert.Equal(t, `anthropic için henüz bir API anahtarı yapılandırılmamış — kurmak için "comrade auth login anthropic" çalıştırın (ya da doğrudan ortam değişkenini ayarlayın; ayrıntılar için "comrade auth login --help")`, translated.Error())
}

// TestClassifyLLMErrorLeavesOtherErrorsUnclassified proves the
// classification is NARROW: any error that is not (or does not wrap) an
// *llm.KeyMissingError — including internal/llm's own OTHER sentinel-
// classified errors (ErrOffline, ErrAuthRejected, ErrOverloaded) — falls
// through unclassified, so a genuinely different failure keeps its own,
// already-informative detail instead of being silently swallowed into
// the no-key message.
func TestClassifyLLMErrorLeavesOtherErrorsUnclassified(t *testing.T) {
	cases := []error{
		fmt.Errorf("llm: all providers failed: %w", llm.ErrOffline),
		fmt.Errorf("llm: all providers failed: %w", llm.ErrAuthRejected),
		fmt.Errorf("llm: all providers failed: %w", llm.ErrOverloaded),
		fmt.Errorf("some unrelated error"),
	}
	for _, err := range cases {
		ok, _ := classifyLLMError(i18n.NewTranslator(i18n.LangEN), err)
		assert.False(t, ok, "must not classify: %v", err)
	}
}

// TestTranslateLLMErrorSuppressesWrapChainForNoKey proves
// translateLLMError (the do/fix/explain-facing wrapper around
// classifyLLMError) never adds its own "prefix: " wrapping around the
// classified, friendly message — QA MAJOR-1's explicit "suppress the
// wrap-chain for this case" requirement.
func TestTranslateLLMErrorSuppressesWrapChainForNoKey(t *testing.T) {
	var stderr bytes.Buffer
	err := realNoKeyChainError()

	got := translateLLMError(&stderr, "comrade do", i18n.NewTranslator(i18n.LangEN), err)

	assert.NotContains(t, got.Error(), "comrade do:")
	assert.Equal(t, `no API key configured for anthropic yet — run "comrade auth login anthropic" to set one up (or export its env var directly; see "comrade auth login --help")`, got.Error())
}

// TestTranslateLLMErrorKeepsPrefixForUnclassifiedErrors is the
// counterpart: an unclassified error keeps EXACTLY the same
// "prefix: %w" wrapping every LLM-reaching command already used before
// this fix — nothing about that path changes.
func TestTranslateLLMErrorKeepsPrefixForUnclassifiedErrors(t *testing.T) {
	var stderr bytes.Buffer
	err := fmt.Errorf("llm: all providers failed: %w", llm.ErrOffline)

	got := translateLLMError(&stderr, "comrade do", i18n.NewTranslator(i18n.LangEN), err)

	assert.Equal(t, "comrade do: llm: all providers failed: llm: could not reach provider (network unreachable)", got.Error())
}

// TestTranslateLLMErrorDumpsDetailOnlyWhenComradeDebugSet proves the
// original wrap-chain is reachable ONLY behind COMRADE_DEBUG (matching
// hook.go's/upgrade.go's own established debug-gated-detail convention),
// never by default, for the classified (no-key) case specifically —
// this is exactly the detail translateLLMError's own return value never
// contains.
func TestTranslateLLMErrorDumpsDetailOnlyWhenComradeDebugSet(t *testing.T) {
	err := realNoKeyChainError()

	var withoutDebug bytes.Buffer
	_ = translateLLMError(&withoutDebug, "comrade do", i18n.NewTranslator(i18n.LangEN), err)
	assert.Empty(t, withoutDebug.String(), "no COMRADE_DEBUG: zero detail written, anywhere")

	t.Setenv("COMRADE_DEBUG", "1")
	var withDebug bytes.Buffer
	_ = translateLLMError(&withDebug, "comrade do", i18n.NewTranslator(i18n.LangEN), err)
	assert.Contains(t, withDebug.String(), "all providers failed")
	assert.Contains(t, withDebug.String(), "comrade do:")
}

// TestTranslateLLMErrorNeverDumpsDetailForUnclassifiedErrors proves the
// COMRADE_DEBUG dump is specific to the classified case — an
// unclassified error already carries its own full detail in its
// returned Error() text (the unchanged "prefix: %w" wrap), so dumping it
// AGAIN to stderr would be pure noise/duplication.
func TestTranslateLLMErrorNeverDumpsDetailForUnclassifiedErrors(t *testing.T) {
	t.Setenv("COMRADE_DEBUG", "1")
	var stderr bytes.Buffer
	err := fmt.Errorf("llm: all providers failed: %w", llm.ErrOffline)

	_ = translateLLMError(&stderr, "comrade do", i18n.NewTranslator(i18n.LangEN), err)

	assert.Empty(t, stderr.String())
}

// realBaseURLRejectedError builds the EXACT error shape
// internal/llm/client.go's buildProvider actually returns for SAST finding
// #3's point-of-use reject (see buildProvider's own doc comment) — not a
// hand-simplified stand-in — so translateBaseURLRejectedError is proven
// against the real wrap-chain shape.
func realBaseURLRejectedError() error {
	invalid := &config.InvalidValueError{
		Key:    "llm.openai_compat.base_url",
		Raw:    "http://169.254.169.254/latest/meta-data/",
		Reason: config.ReasonMetadataOrLinkLocal,
	}
	return fmt.Errorf("llm: %s: %w", "openai_compat", invalid)
}

// TestTranslateBaseURLRejectedErrorRendersActionableMessage is SAST
// finding #3's client-build-time i18n proof: errors.As sees straight
// through internal/llm's wrap chain to the original
// *config.InvalidValueError, and the rendered message is the exact
// friendly EN text (MsgLLMBaseURLRejected) — pointing at `comrade config
// set` to fix it — not the raw "llm: openai_compat: invalid value ..."
// wrap chain.
func TestTranslateBaseURLRejectedErrorRendersActionableMessage(t *testing.T) {
	err := realBaseURLRejectedError()

	got := translateBaseURLRejectedError(i18n.NewTranslator(i18n.LangEN), err)

	assert.Equal(t,
		`refusing to send your API key to llm.openai_compat.base_url (currently "http://169.254.169.254/latest/meta-data/") — it is not a safe endpoint; fix it with: comrade config set llm.openai_compat.base_url <valid-url>`,
		got.Error())
	assert.NotContains(t, got.Error(), "InvalidValueError")
}

// TestTranslateBaseURLRejectedErrorRendersActionableMessageInTurkish is the
// same proof in Turkish — this project's established TR-smoke-test
// convention.
func TestTranslateBaseURLRejectedErrorRendersActionableMessageInTurkish(t *testing.T) {
	err := realBaseURLRejectedError()

	got := translateBaseURLRejectedError(i18n.NewTranslator(i18n.LangTR), err)

	assert.Equal(t,
		`API anahtarınız llm.openai_compat.base_url (şu an "http://169.254.169.254/latest/meta-data/") adresine gönderilmeyecek — güvenli bir uç nokta değil; düzeltmek için: comrade config set llm.openai_compat.base_url <geçerli-url>`,
		got.Error())
}

// TestTranslateBaseURLRejectedErrorLeavesOtherErrorsUnchanged proves the
// translation is NARROW: any error that is not (or does not wrap) a
// *config.InvalidValueError with Reason ReasonNotURL/
// ReasonMetadataOrLinkLocal — including an *config.InvalidValueError with
// a DIFFERENT Reason, which base_url's own connector construction never
// actually produces but this proves is not accidentally caught either —
// is returned completely unchanged.
func TestTranslateBaseURLRejectedErrorLeavesOtherErrorsUnchanged(t *testing.T) {
	cases := []error{
		fmt.Errorf("llm: unknown provider %q", "bogus"),
		fmt.Errorf("llm: openai_compat: %w", &config.InvalidValueError{Key: "llm.timeout_seconds", Raw: "x", Reason: config.ReasonNotInteger}),
	}
	for _, err := range cases {
		got := translateBaseURLRejectedError(i18n.NewTranslator(i18n.LangEN), err)
		assert.Equal(t, err, got, "must not translate: %v", err)
	}
}

func init() {
	// Guard against an ambient COMRADE_DEBUG bleeding into
	// TestTranslateLLMErrorDumpsDetailOnlyWhenComradeDebugSet's "without
	// debug" half from the host running this test binary.
	_ = os.Unsetenv("COMRADE_DEBUG")
}
