package cli

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// usageTally accumulates llm.UsageEvent data across every LLM call made
// through one *llm.Client — do/fix's plan generation AND every
// self-correction retry engine.Execute issues through that same client
// (deps.LLM), explain's one Explain call, and chat's per-turn/"/do"
// calls all attach the SAME tally via llm.WithUsageObserver, so a single
// snapshot genuinely reflects the whole run/session, not one call.
// Mutex-guarded: engine's self-correction retries and chat's bubbletea
// goroutine both call record via the Client's own usageObserver, which
// runs on whatever goroutine issued the Complete call.
type usageTally struct {
	mu       sync.Mutex
	inTok    int
	outTok   int
	requests int

	// provider/model reflect the MOST RECENT successful event's own
	// attribution — sufficient for a single display line's "(provider/
	// model)" segment, since a fallback mid-run (a different provider
	// serving part of one run) is the rare case this line does not try
	// to represent exhaustively.
	provider string
	model    string

	// costKnown is true only when EVERY recorded event's own
	// (provider, model) had a pricing.go table entry (llm.EstimateUSD).
	// The moment any one event is unpriced, costKnown flips to false for
	// the rest of this tally's life: a partial cost total would
	// understate the real spend, so the whole line omits cost rather
	// than mislead — see formatUsageLine.
	costKnown bool
	costUSD   float64
}

// newUsageTally returns a zero-value, ready-to-use usageTally. A nil
// *usageTally is never passed to llm.WithUsageObserver — every caller
// that wants usage tracking constructs one via this constructor first.
func newUsageTally() *usageTally {
	return &usageTally{}
}

// record is usageTally's llm.UsageEvent observer — pass t.record
// directly to llm.WithUsageObserver. See usage.go's package doc comment
// (WithUsageObserver) for why this must stay cheap and non-blocking: it
// only ever does in-memory arithmetic under mu, no I/O.
func (t *usageTally) record(ev llm.UsageEvent) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inTok += ev.Usage.InputTokens
	t.outTok += ev.Usage.OutputTokens
	t.requests++
	t.provider = ev.Provider
	t.model = ev.Model

	cost, ok := llm.EstimateUSD(ev.Provider, ev.Model, ev.Usage)
	if t.requests == 1 {
		t.costKnown = ok
	} else if !ok {
		t.costKnown = false
	}
	if ok {
		t.costUSD += cost
	}
}

// reset clears t back to its zero-value state in place — used by chat
// (chatdispatch.go) to turn a single shared usageTally into a
// per-turn-scoped one: reset immediately before dispatching a line, read
// via snapshot immediately after that dispatch completes. Never resets
// via a whole-struct assignment (`*t = usageTally{}`): mu is embedded by
// value, and reset always runs while mu is already held by this very
// call's own Lock/Unlock pair — replacing the Mutex value out from under
// an active lock would desynchronize Unlock from the lock it thinks it
// holds. Fields are cleared individually instead, leaving mu itself
// untouched.
func (t *usageTally) reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.inTok, t.outTok, t.requests = 0, 0, 0
	t.provider, t.model = "", ""
	t.costKnown, t.costUSD = false, 0
}

// usageSnapshot is an immutable point-in-time read of a usageTally,
// returned by snapshot() so formatting code never has to hold t.mu.
type usageSnapshot struct {
	inTok, outTok, requests int
	provider, model         string
	costKnown               bool
	costUSD                 float64
}

// snapshot returns t's current state.
func (t *usageTally) snapshot() usageSnapshot {
	t.mu.Lock()
	defer t.mu.Unlock()
	return usageSnapshot{
		inTok:     t.inTok,
		outTok:    t.outTok,
		requests:  t.requests,
		provider:  t.provider,
		model:     t.model,
		costKnown: t.costKnown,
		costUSD:   t.costUSD,
	}
}

// formatUsageLine renders snap as the single-line summary every usage
// display surface shows (do/fix/explain's post-run stderr line, chat's
// per-turn suffix and session total) — MsgUsageSummary's base
// "tokens: N in / M out across K requests (provider/model)" text, plus
// exactly one of: nothing (cost unknown), MsgUsageCostLocal ("· local",
// when snap.provider is ollama — see usageTally.provider's own doc
// comment for why the MOST RECENT event's provider is what gates this),
// or MsgUsageCostEstimate ("· est. $X.XXXX", when snap.costKnown).
func formatUsageLine(tr i18n.Translator, snap usageSnapshot) string {
	base := tr.T(i18n.MsgUsageSummary,
		formatThousands(snap.inTok),
		formatThousands(snap.outTok),
		snap.requests,
		snap.provider,
		snap.model,
	)

	switch {
	case snap.provider == "ollama":
		return base + tr.T(i18n.MsgUsageCostLocal)
	case snap.costKnown:
		return base + tr.T(i18n.MsgUsageCostEstimate, formatUSD(snap.costUSD))
	default:
		return base
	}
}

// formatThousands renders a non-negative int with a "," thousands
// separator every 3 digits (e.g. 1204 -> "1,204") — token counts are the
// only caller, and are never negative.
func formatThousands(n int) string {
	digits := fmt.Sprintf("%d", n)
	if len(digits) <= 3 {
		return digits
	}

	var b strings.Builder
	lead := len(digits) % 3
	if lead == 0 {
		lead = 3
	}
	b.WriteString(digits[:lead])
	for i := lead; i < len(digits); i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

// formatUSD renders amount as a 4-decimal-place dollar string (e.g.
// "$0.0021") — 4 places so a genuinely small per-request cost never
// rounds down to a misleading "$0.00".
func formatUSD(amount float64) string {
	return fmt.Sprintf("$%.4f", amount)
}

// printUsageSummary writes tally's single dim summary line to w, styled
// via internal/tui's existing statusStyle (tui.PrintStatus) — the same
// style every other one-line, non-warning status message in this tree
// already uses. It is a no-op (writes nothing) when tally recorded zero
// requests: a run that never reached the LLM (e.g. a config-load failure
// before any Complete call) has nothing to report.
func printUsageSummary(w io.Writer, tr i18n.Translator, tally *usageTally, colorEnabled bool) error {
	snap := tally.snapshot()
	if snap.requests == 0 {
		return nil
	}
	return tui.PrintStatus(w, formatUsageLine(tr, snap), colorEnabled)
}
