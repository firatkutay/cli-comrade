package llm

import "strings"

// priceEntry is one hand-maintained row of pricingTable: provider plus a
// model-name prefix (never an exact match — see bestPriceMatch) mapped to
// that model's current per-million-token USD rate.
type priceEntry struct {
	provider    string
	modelPrefix string
	inPerMTok   float64
	outPerMTok  float64
	currency    string
}

// pricingTable is a hand-maintained snapshot of each paid connector's
// current per-model USD pricing — mirrors models.go's own dated-snapshot
// convention (defaultAnthropicModel etc.), and is deliberately NOT a
// live-queried source: none of anthropic/openai_compat/google expose a
// public, unauthenticated pricing API this CLI could poll instead.
// Revisit whenever a vendor changes pricing or this project adopts a new
// default/known model (models.go).
//
// prices verified 2026-07-24, against each vendor's own current pricing
// page:
//   - Anthropic: https://platform.claude.com/docs/en/about-claude/pricing
//     — Claude Sonnet 5's row uses its introductory $2/$10 per-MTok rate,
//     in effect through 2026-08-31; the standard $3/$15 rate takes over
//     after that date and this row should be updated then.
//   - OpenAI-compatible default model (gpt-5.4-mini):
//     https://developers.openai.com/api/docs/pricing — this table only
//     prices defaultOpenAICompatModel; a self-hosted/third-party
//     openai_compat endpoint (Mistral/Groq/GLM/Qwen/Kimi/OpenRouter/LM
//     Studio, per CLAUDE.md) serving a different model has no row here
//     and correctly falls through to EstimateUSD's (_, false) unknown
//     case.
//   - Google: https://ai.google.dev/gemini-api/docs/pricing (paid tier).
//     Gemini 3.1 Pro's row is its <=200k-input-token tier; this table
//     does not model Google's long-context price step for larger
//     prompts.
//
// ollama has no row here: it is a local runtime, not a priced vendor —
// EstimateUSD special-cases it to an unconditional (0, true)/"local"
// result instead. A 0-priced row in this table would fail
// TestPricingTableEntriesHavePositivePrices's every-price-must-be-
// positive guard, which exists specifically to catch a real paid model
// silently priced at 0 by a copy-paste mistake — ollama needs a
// different code path, not an exception carved into that guard.
var pricingTable = []priceEntry{
	{provider: "anthropic", modelPrefix: "claude-opus-4-8", inPerMTok: 5, outPerMTok: 25, currency: "USD"},
	{provider: "anthropic", modelPrefix: "claude-sonnet-5", inPerMTok: 2, outPerMTok: 10, currency: "USD"},
	{provider: "anthropic", modelPrefix: "claude-haiku-4-5", inPerMTok: 1, outPerMTok: 5, currency: "USD"},
	{provider: "openai_compat", modelPrefix: "gpt-5.4-mini", inPerMTok: 0.75, outPerMTok: 4.50, currency: "USD"},
	{provider: "google", modelPrefix: "gemini-3.5-flash", inPerMTok: 1.50, outPerMTok: 9, currency: "USD"},
	{provider: "google", modelPrefix: "gemini-3.1-flash-lite", inPerMTok: 0.25, outPerMTok: 1.50, currency: "USD"},
	{provider: "google", modelPrefix: "gemini-3.1-pro", inPerMTok: 2, outPerMTok: 12, currency: "USD"},
}

// EstimateUSD estimates u's USD cost for one completion against
// provider/model. ollama is special-cased to an unconditional (0, true)
// — see pricingTable's own doc comment for why it is not a table row —
// so a caller can treat "known, zero cost" (local) and "known, priced"
// identically and only needs a separate branch when it wants to render
// ollama's cost as the word "local" instead of a dollar amount.
//
// Every other provider is matched against pricingTable by the LONGEST
// modelPrefix that is a prefix of model (bestPriceMatch) — never an
// exact string match, since a connector's actual wire model name can
// carry a dated/versioned suffix models.go's own default constants don't
// (e.g. a future "claude-haiku-4-5-20260901"-shaped alias must still
// match the "claude-haiku-4-5" row). No match at all — an unrecognized
// provider, or a model this table has no row for — returns (0, false):
// the cost is unknown, never assumed to be free.
func EstimateUSD(provider, model string, u Usage) (float64, bool) {
	if provider == "ollama" {
		return 0, true
	}

	entry, ok := bestPriceMatch(provider, model)
	if !ok {
		return 0, false
	}
	cost := float64(u.InputTokens)/1_000_000*entry.inPerMTok + float64(u.OutputTokens)/1_000_000*entry.outPerMTok
	return cost, true
}

// bestPriceMatch returns pricingTable's entry for provider whose
// modelPrefix is both a prefix of model and the LONGEST such prefix
// among that provider's own rows, so a more specific row (e.g.
// "claude-opus-4-8") always outranks a hypothetical broader one (e.g.
// a future "claude-opus-4") for a model string both would otherwise
// match.
func bestPriceMatch(provider, model string) (priceEntry, bool) {
	var best priceEntry
	found := false
	for _, e := range pricingTable {
		if e.provider != provider || !strings.HasPrefix(model, e.modelPrefix) {
			continue
		}
		if !found || len(e.modelPrefix) > len(best.modelPrefix) {
			best = e
			found = true
		}
	}
	return best, found
}
