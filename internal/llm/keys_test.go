package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveEnvKeyPrefersComradePrefixOverVendorEnvVar(t *testing.T) {
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "comrade-prefixed")
	t.Setenv("ANTHROPIC_API_KEY", "vendor-var")

	key, err := ResolveEnvKey("anthropic")

	require.NoError(t, err)
	assert.Equal(t, "comrade-prefixed", key)
}

func TestResolveEnvKeyFallsBackToVendorEnvVar(t *testing.T) {
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "vendor-var")

	key, err := ResolveEnvKey("anthropic")

	require.NoError(t, err)
	assert.Equal(t, "vendor-var", key)
}

func TestResolveEnvKeyMissingListsEveryCheckedVar(t *testing.T) {
	t.Setenv("COMRADE_GOOGLE_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")
	t.Setenv("GOOGLE_API_KEY", "")

	_, err := ResolveEnvKey("google")

	require.Error(t, err)
	assert.ErrorContains(t, err, "COMRADE_GOOGLE_API_KEY")
	assert.ErrorContains(t, err, "GEMINI_API_KEY")
	assert.ErrorContains(t, err, "GOOGLE_API_KEY")
}

func TestProviderEnvVarsReturnsExactPriorityOrderedList(t *testing.T) {
	assert.Equal(t, []string{"COMRADE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"}, ProviderEnvVars("anthropic"))
	assert.Equal(t, []string{"COMRADE_OPENAI_COMPAT_API_KEY", "OPENAI_API_KEY"}, ProviderEnvVars("openai_compat"))
	assert.Equal(t, []string{"COMRADE_GOOGLE_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"}, ProviderEnvVars("google"))
	assert.Empty(t, ProviderEnvVars("ollama"))
}

func TestProviderEnvVarsReturnsACopyNotTheInternalSlice(t *testing.T) {
	got := ProviderEnvVars("anthropic")
	got[0] = "mutated"

	assert.Equal(t, "COMRADE_ANTHROPIC_API_KEY", ProviderEnvVars("anthropic")[0],
		"mutating a slice returned by ProviderEnvVars must not affect the package's internal table")
}

func TestKnownAnthropicModelsIncludesTheConfiguredDefault(t *testing.T) {
	assert.Equal(t, []string{"claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"}, KnownAnthropicModels())
	assert.Contains(t, KnownAnthropicModels(), defaultAnthropicModel)
}

func TestKnownGoogleModelsIncludesTheConfiguredDefault(t *testing.T) {
	assert.Equal(t, []string{"gemini-3.5-flash", "gemini-3.1-flash-lite", "gemini-3.1-pro"}, KnownGoogleModels())
	assert.Contains(t, KnownGoogleModels(), defaultGoogleModel)
}

func TestDefaultOpenAICompatModelMatchesTheInternalConstant(t *testing.T) {
	assert.Equal(t, defaultOpenAICompatModel, DefaultOpenAICompatModel())
	assert.Equal(t, "gpt-5.4-mini", DefaultOpenAICompatModel())
}
