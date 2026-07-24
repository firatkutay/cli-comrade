package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// TestPricingTableEntriesAreValid is this table's derive-or-guard test
// (mirrors internal/secrets's TestKnownProvidersMatchesConfigProviderNamesMinusOllama):
// every pricingTable row's provider must be a real config.ProviderNames()
// member, and every price must be strictly positive — a 0 or negative
// price would silently mean "free", which is never true for a paid
// vendor row (ollama's genuine free/local case is handled by
// EstimateUSD's dedicated branch, never a table row — see pricingTable's
// own doc comment).
func TestPricingTableEntriesAreValid(t *testing.T) {
	valid := map[string]bool{}
	for _, p := range config.ProviderNames() {
		valid[p] = true
	}

	for _, e := range pricingTable {
		assert.Truef(t, valid[e.provider], "pricingTable entry %q has provider %q, which is not in config.ProviderNames()", e.modelPrefix, e.provider)
		assert.NotEqual(t, "ollama", e.provider, "ollama must never be a pricingTable row — EstimateUSD special-cases it instead")
		assert.Greaterf(t, e.inPerMTok, 0.0, "pricingTable entry %s/%s: inPerMTok must be > 0", e.provider, e.modelPrefix)
		assert.Greaterf(t, e.outPerMTok, 0.0, "pricingTable entry %s/%s: outPerMTok must be > 0", e.provider, e.modelPrefix)
		assert.Equal(t, "USD", e.currency)
	}
}

func TestEstimateUSDOllamaIsAlwaysZeroKnown(t *testing.T) {
	cost, ok := EstimateUSD("ollama", "llama3.1", Usage{InputTokens: 10_000_000, OutputTokens: 10_000_000})
	assert.True(t, ok)
	assert.Equal(t, 0.0, cost)
}

func TestEstimateUSDUnknownProviderIsUnknown(t *testing.T) {
	cost, ok := EstimateUSD("openai_compat", "some-unpriced-local-model", Usage{InputTokens: 1000, OutputTokens: 1000})
	assert.False(t, ok)
	assert.Equal(t, 0.0, cost)
}

func TestEstimateUSDUnknownModelUnderKnownProviderIsUnknown(t *testing.T) {
	_, ok := EstimateUSD("anthropic", "some-future-model-no-row-covers", Usage{InputTokens: 100, OutputTokens: 100})
	assert.False(t, ok)
}

func TestEstimateUSDExactPriceForKnownModel(t *testing.T) {
	// claude-haiku-4-5: $1/MTok in, $5/MTok out.
	cost, ok := EstimateUSD("anthropic", "claude-haiku-4-5", Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000})
	assert.True(t, ok)
	assert.InDelta(t, 6.0, cost, 1e-9)
}

func TestEstimateUSDLongestPrefixWinsOverShorterOne(t *testing.T) {
	// "claude-opus-4-8" ($5/$25) must be matched, not any hypothetical
	// shorter "claude-opus-4" prefix — bestPriceMatch always prefers the
	// longest matching modelPrefix.
	cost, ok := EstimateUSD("anthropic", "claude-opus-4-8", Usage{InputTokens: 1_000_000, OutputTokens: 0})
	assert.True(t, ok)
	assert.InDelta(t, 5.0, cost, 1e-9)
}

func TestEstimateUSDMatchesVersionedSuffixByPrefix(t *testing.T) {
	// A dated/versioned wire model name (e.g. a future
	// "claude-haiku-4-5-20260901"-shaped alias) must still match its base
	// row by prefix, not require an exact string match.
	cost, ok := EstimateUSD("anthropic", "claude-haiku-4-5-20260901", Usage{InputTokens: 1_000_000, OutputTokens: 0})
	assert.True(t, ok)
	assert.InDelta(t, 1.0, cost, 1e-9)
}

func TestEstimateUSDZeroUsageIsZeroCostButKnown(t *testing.T) {
	cost, ok := EstimateUSD("google", "gemini-3.5-flash", Usage{})
	assert.True(t, ok)
	assert.Equal(t, 0.0, cost)
}
