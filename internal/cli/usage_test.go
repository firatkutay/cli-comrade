package cli

import (
	"bytes"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

func TestFormatThousandsGroupsEveryThreeDigits(t *testing.T) {
	cases := map[int]string{
		0:         "0",
		7:         "7",
		999:       "999",
		1000:      "1,000",
		1204:      "1,204",
		12345:     "12,345",
		123456:    "123,456",
		1234567:   "1,234,567",
		999999999: "999,999,999",
	}
	for n, want := range cases {
		assert.Equal(t, want, formatThousands(n), "formatThousands(%d)", n)
	}
}

func TestFormatUSDFourDecimalPlaces(t *testing.T) {
	assert.Equal(t, "$0.0021", formatUSD(0.0021))
	assert.Equal(t, "$0.0000", formatUSD(0))
	assert.Equal(t, "$12.3457", formatUSD(12.34567))
}

// TestNewUsageObserverFeedsBothSessionAndTurnTallies proves runChat's
// extracted dual-tally observer (chat.go's own inline closure before this
// extraction) records every event into BOTH tallies — this is the PTY-free
// unit test the previous inline closure could not get, since it was only
// ever reachable through a real bubbletea program.
func TestNewUsageObserverFeedsBothSessionAndTurnTallies(t *testing.T) {
	session := newUsageTally()
	turn := newUsageTally()
	observer := newUsageObserver(session, turn)

	observer(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 100, OutputTokens: 50}})

	sessionSnap := session.snapshot()
	turnSnap := turn.snapshot()
	assert.Equal(t, 1, sessionSnap.requests)
	assert.Equal(t, 100, sessionSnap.inTok)
	assert.Equal(t, 1, turnSnap.requests)
	assert.Equal(t, 100, turnSnap.inTok)
}

// TestNewUsageObserverSessionAccumulatesAcrossTurnResets proves the
// per-turn/session-total split runChat relies on: turn is reset between
// dispatches (chatController.turnTally's own doc comment) while session
// keeps accumulating — so a later turn's snapshot reflects ONLY that
// turn's events, but session's snapshot still reflects every event ever
// recorded through this same observer.
func TestNewUsageObserverSessionAccumulatesAcrossTurnResets(t *testing.T) {
	session := newUsageTally()
	turn := newUsageTally()
	observer := newUsageObserver(session, turn)

	observer(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 100, OutputTokens: 50}})
	turn.reset() // simulates chatdispatch.go resetting turnTally before the next line
	observer(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}})

	sessionSnap := session.snapshot()
	turnSnap := turn.snapshot()
	assert.Equal(t, 2, sessionSnap.requests, "session must accumulate across both turns")
	assert.Equal(t, 110, sessionSnap.inTok)
	assert.Equal(t, 1, turnSnap.requests, "turn must reflect only the events since its last reset")
	assert.Equal(t, 10, turnSnap.inTok)
}

// TestNewUsageObserverZeroUsageEventStillCountsAsOneRequestInBothTallies
// proves the no-usage-tokens case (e.g. a successful completion with no
// reported token counts): the observer must still record it as exactly
// one request in BOTH tallies, not silently drop it — printUsageSummary
// only treats an observer that was NEVER invoked (zero requests) as a
// no-op; an event with a zero-value Usage is a real recorded event, and
// this test can only pass if newUsageObserver actually forwards ev to
// both tallies' record (a broken/no-op observer, or one that only feeds
// one tally, both fail this exact assertion).
func TestNewUsageObserverZeroUsageEventStillCountsAsOneRequestInBothTallies(t *testing.T) {
	session := newUsageTally()
	turn := newUsageTally()
	observer := newUsageObserver(session, turn)

	observer(llm.UsageEvent{Provider: "ollama", Model: "llama3.1", Usage: llm.Usage{}})

	sessionSnap := session.snapshot()
	turnSnap := turn.snapshot()
	assert.Equal(t, 1, sessionSnap.requests)
	assert.Equal(t, 0, sessionSnap.inTok)
	assert.Equal(t, 0, sessionSnap.outTok)
	assert.Equal(t, 1, turnSnap.requests)
	assert.Equal(t, 0, turnSnap.inTok)
	assert.Equal(t, 0, turnSnap.outTok)
}

func TestUsageTallyRecordSumsAcrossMultipleEvents(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 100, OutputTokens: 50}})
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 200, OutputTokens: 25}})

	snap := tally.snapshot()
	assert.Equal(t, 300, snap.inTok)
	assert.Equal(t, 75, snap.outTok)
	assert.Equal(t, 2, snap.requests)
	assert.Equal(t, "anthropic", snap.provider)
	assert.Equal(t, "claude-haiku-4-5", snap.model)
}

func TestUsageTallyProviderModelReflectMostRecentEvent(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}})
	tally.record(llm.UsageEvent{Provider: "ollama", Model: "llama3.1", Usage: llm.Usage{InputTokens: 2, OutputTokens: 2}})

	snap := tally.snapshot()
	assert.Equal(t, "ollama", snap.provider)
	assert.Equal(t, "llama3.1", snap.model)
}

func TestUsageTallyCostKnownWhenEveryEventIsPriced(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000}})

	snap := tally.snapshot()
	assert.True(t, snap.costKnown)
	assert.InDelta(t, 6.0, snap.costUSD, 1e-9) // $1 in + $5 out per MTok
}

func TestUsageTallyCostUnknownWhenAnyEventIsUnpriced(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 1_000_000, OutputTokens: 0}})
	tally.record(llm.UsageEvent{Provider: "openai_compat", Model: "some-unpriced-model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}})

	snap := tally.snapshot()
	assert.False(t, snap.costKnown, "one unpriced event must make the whole tally's cost unknown")
}

// TestUsageTallyRecordUsesEventBaseURLForOpenAICompatPricing is this
// follow-up's load-bearing proof for usage.go's own EstimateUSD call
// site (record's ev.BaseURL argument): the SAME provider/model pair
// ("openai_compat"/"gpt-5.4-mini") must be priced or unpriced depending
// ENTIRELY on which endpoint served it, not on the model name alone.
// This is exactly the seam a reviewer found untested: hardcoding
// ev.BaseURL to the OpenAI default at record's EstimateUSD call site
// makes case (a) below fail (a Qwen-style gateway would get priced at
// OpenAI's rate — the "confidently wrong dollar figure" this whole
// feature exists to prevent); hardcoding it to "" makes case (b) and (c)
// fail (a real OpenAI user's cost silently vanishes).
func TestUsageTallyRecordUsesEventBaseURLForOpenAICompatPricing(t *testing.T) {
	t.Run("non-OpenAI gateway with an OpenAI-looking model name is cost-unknown", func(t *testing.T) {
		tally := newUsageTally()
		tally.record(llm.UsageEvent{
			Provider: "openai_compat",
			Model:    "gpt-5.4-mini",
			BaseURL:  "https://dashscope.aliyuncs.com/compatible-mode/v1",
			Usage:    llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
		})

		snap := tally.snapshot()
		assert.False(t, snap.costKnown, "a non-OpenAI gateway must never be priced from OpenAI's own rate")
	})

	t.Run("OpenAI's own default endpoint is priced at the exact expected cost", func(t *testing.T) {
		tally := newUsageTally()
		tally.record(llm.UsageEvent{
			Provider: "openai_compat",
			Model:    "gpt-5.4-mini",
			BaseURL:  config.Default().LLM.OpenAICompat.BaseURL,
			Usage:    llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
		})

		snap := tally.snapshot()
		assert.True(t, snap.costKnown)
		assert.InDelta(t, 5.25, snap.costUSD, 1e-9) // $0.75 in + $4.50 out per MTok
	})

	t.Run("trailing-slash spelling of the OpenAI default is still priced", func(t *testing.T) {
		tally := newUsageTally()
		tally.record(llm.UsageEvent{
			Provider: "openai_compat",
			Model:    "gpt-5.4-mini",
			BaseURL:  config.Default().LLM.OpenAICompat.BaseURL + "/",
			Usage:    llm.Usage{InputTokens: 1_000_000, OutputTokens: 1_000_000},
		})

		snap := tally.snapshot()
		assert.True(t, snap.costKnown, "a legal trailing-slash base_url must still count as the OpenAI default")
		assert.InDelta(t, 5.25, snap.costUSD, 1e-9)
	})
}

func TestUsageTallyCostStaysUnknownOnceFlippedEvenAfterAPricedEvent(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "openai_compat", Model: "some-unpriced-model", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}})
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}})

	assert.False(t, tally.snapshot().costKnown)
}

func TestUsageTallyResetClearsAllFieldsWithoutPanicking(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 5, OutputTokens: 5}})

	require.NotPanics(t, tally.reset)

	snap := tally.snapshot()
	assert.Equal(t, usageSnapshot{}, snap, "reset must clear every field back to its zero value")

	// tally must still be usable after reset (the mutex was not corrupted).
	require.NotPanics(t, func() {
		tally.record(llm.UsageEvent{Provider: "google", Model: "gemini-3.5-flash", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}})
	})
	assert.Equal(t, 1, tally.snapshot().requests)
}

func TestUsageTallyRecordIsSafeForConcurrentUse(t *testing.T) {
	tally := newUsageTally()
	var wg sync.WaitGroup
	const n = 100
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 1, OutputTokens: 1}})
		}()
	}
	wg.Wait()

	snap := tally.snapshot()
	assert.Equal(t, n, snap.requests)
	assert.Equal(t, n, snap.inTok)
	assert.Equal(t, n, snap.outTok)
}

func TestFormatUsageLineKnownCostEN(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	snap := usageSnapshot{
		inTok: 1204, outTok: 356, requests: 3,
		provider: "anthropic", model: "claude-haiku-4-5",
		costKnown: true, costUSD: 0.0021,
	}
	got := formatUsageLine(tr, snap)
	assert.Equal(t, "tokens: 1,204 in / 356 out across 3 requests (anthropic/claude-haiku-4-5) · est. $0.0021", got)
}

func TestFormatUsageLineUnknownCostOmitsCostSegmentEN(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	snap := usageSnapshot{
		inTok: 10, outTok: 20, requests: 1,
		provider: "openai_compat", model: "some-unpriced-model",
		costKnown: false,
	}
	got := formatUsageLine(tr, snap)
	assert.Equal(t, "tokens: 10 in / 20 out across 1 requests (openai_compat/some-unpriced-model)", got)
	assert.NotContains(t, got, "est.")
	assert.NotContains(t, got, "$")
}

func TestFormatUsageLineOllamaShowsLocalNotDollarEN(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	snap := usageSnapshot{
		inTok: 5, outTok: 5, requests: 1,
		provider: "ollama", model: "llama3.1",
		costKnown: true, costUSD: 0,
	}
	got := formatUsageLine(tr, snap)
	assert.Equal(t, "tokens: 5 in / 5 out across 1 requests (ollama/llama3.1) · local", got)
	assert.NotContains(t, got, "$")
}

func TestFormatUsageLineTR(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangTR)
	snap := usageSnapshot{
		inTok: 1204, outTok: 356, requests: 3,
		provider: "anthropic", model: "claude-haiku-4-5",
		costKnown: true, costUSD: 0.0021,
	}
	got := formatUsageLine(tr, snap)
	assert.Equal(t, "token: 1,204 giriş / 356 çıkış, 3 istekte (anthropic/claude-haiku-4-5) · tah. $0.0021", got)
}

func TestPrintUsageSummaryNoOpWhenZeroRequests(t *testing.T) {
	var buf bytes.Buffer
	err := printUsageSummary(&buf, i18n.NewTranslator(i18n.LangEN), newUsageTally(), false)
	require.NoError(t, err)
	assert.Empty(t, buf.String())
}

func TestPrintUsageSummaryWritesOneLineWhenRequestsRecorded(t *testing.T) {
	tally := newUsageTally()
	tally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 100, OutputTokens: 50}})

	var buf bytes.Buffer
	err := printUsageSummary(&buf, i18n.NewTranslator(i18n.LangEN), tally, false)
	require.NoError(t, err)
	assert.Equal(t, "tokens: 100 in / 50 out across 1 requests (anthropic/claude-haiku-4-5) · est. $0.0003\n", buf.String())
	// 100 input tokens @ $1/MTok + 50 output tokens @ $5/MTok = $0.0001 +
	// $0.00025 = $0.00035, which %.4f renders as $0.0003 (float64's
	// nearest binary representation of 0.00035 rounds down at 4 places).
}
