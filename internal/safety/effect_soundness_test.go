package safety

import (
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// --- MAY-NOT-EXECUTE body soundness: the CRITICAL regression a follow-up
// security audit found and proved with its own differential harness (51+
// verified-unsound cases over a focused corpus, 21 action-lowerings vs
// origin/main over a 5,396-command generated corpus). See
// analyzeBashEffect's doc comment (effect_bash.go) for the full
// false-Allow / mirror-false-negative analysis resolveMayNotExecute
// fixes. This file pins BOTH directions, for all four wrapper families
// the audit named, so neither direction can silently regress again:
//
//   - "dangerous-before, benign-inside" (the ORIGINAL bug): a variable is
//     already dangerous BEFORE a may-not-execute body that reassigns it
//     to something benign — since this analyzer cannot know the body
//     ran, the dangerous value must NOT be silently overwritten with the
//     benign one. Sharing env directly with the body (the pre-fix
//     behavior) got this wrong: `R=rm; while false; do R=echo; done;
//     $R -rf /` resolved to the safe-looking "echo -rf /" and stayed
//     Allow, even though real bash never runs the while body at all and
//     genuinely executes `rm -rf /`.
//   - "benign-before, dangerous-inside" (the MIRROR case): a variable is
//     benign BEFORE a may-not-execute body that reassigns it to
//     something dangerous. Discarding the body's assignment
//     unconditionally (the way Subshell handling always has, correctly,
//     for a REAL subshell) would get THIS direction wrong instead:
//     `R=echo; for i in 1; do R=rm; done; $R -rf /` really does run its
//     body (Items is the non-empty literal list "1"), so silently
//     keeping the stale R=echo would resolve to a safe-looking value for
//     a command that genuinely runs `rm -rf /`.
//
// Both directions must land on Confirm (via "effect: indeterminate" —
// the variable is invalidated, not resolved to either value), never
// Allow. The four wrapper families: `while false`, `until true`,
// `for i in $UNSET` / `for i in $(false)`, and a skipped "elif" Cond.

func TestEvaluateEffectMayNotExecuteBodyNeverOverwritesKnownValue(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"while false: body never runs, must not overwrite the already-dangerous R=rm with R=echo",
			"R=rm; while false; do R=echo; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"until true: body never runs (until loops while the condition is FALSE), same overwrite risk",
			"R=rm; until true; do R=echo; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"for i in $UNSET: an unset variable's iteration list is empty, zero iterations",
			"R=rm; for i in $UNSET; do R=echo; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"for i in $(false): a command-substitution iteration list can resolve empty at runtime",
			"R=rm; for i in $(false); do R=echo; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"skipped elif Cond: the elif branch (and its Cond) is reached only if the first Cond was false",
			"R=rm; if false; then :; elif false; then R=echo; fi; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"while [ -f /nonexistent ]: a realistic (non-literal-false) condition that is also never true",
			"R=rm; while [ -f /nonexistent ]; do R=echo; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
	}
	runEvalCases(t, engine, cases)
}

func TestEvaluateEffectMayNotExecuteBodyNeverLeaksGuaranteedAssignment(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"for i in 1 (mirror of while-false): the body DOES run (non-empty Items), R=rm must not be discarded as if benign",
			"R=echo; for i in 1; do R=rm; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"while true (mirror of until-true): the body runs at least once, same leak risk",
			"R=echo; while true; do R=rm; break; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"for i in a b c (mirror of for-empty): a non-empty, non-dynamic Items list",
			"R=echo; for i in a b c; do R=rm; done; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"if true; then R=rm; fi (mirror of skipped elif): the FIRST if's Then, reached unconditionally if Cond is truthy",
			"R=echo; if true; then R=rm; fi; $R -rf /", RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateEffectMayNotExecuteBodyStillEscalatesWithinItself re-affirms
// issue #15's ORIGINAL ask still holds after this fix: an
// assignment-and-use INSIDE the SAME may-not-execute body still escalates
// via denylist-signature reuse (RiskDestructive), because the body is
// still resolved in its own child clone before any invalidation happens —
// resolveMayNotExecute's clone step, not its invalidate step, is what
// this pins.
func TestEvaluateEffectMayNotExecuteBodyStillEscalatesWithinItself(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"for i in 1; do R=rm; $R -rf /; done -- intra-body escalation must still fire",
			"for i in 1; do R=rm; $R -rf /; done", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
		{
			"while true; do R=rm; $R -rf /; done -- intra-body escalation must still fire",
			"while true; do R=rm; $R -rf /; done", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
		{
			"until false; do R=rm; $R -rf /; done -- intra-body escalation must still fire",
			"until false; do R=rm; $R -rf /; done", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
		{
			"if x; then R=echo; elif true; then R=rm; $R -rf /; fi -- intra-elif-body escalation must still fire",
			"if x; then R=echo; elif true; then R=rm; $R -rf /; fi", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
	}
	runEvalCases(t, engine, cases)
}

// --- Invalidation COMPOSITION across nested may-not-execute bodies: a
// HIGH-severity follow-up the security audit found at depth >= 2, after
// the depth-1 CRITICAL above was fixed. resolveMayNotExecute's own
// invalidation loop originally iterated the CHILD's clone ("for name,
// newVal := range clone"), which only ever sees a key the child ADDED or
// MODIFIED — never one an INNER resolveMayNotExecute (deeper in the same
// body) already DELETED from that intermediate clone as ambiguous. So a
// TWO-level-deep ambiguity (an if/case/loop nested inside another
// if/case/loop) went completely unnoticed by the OUTER level: the
// deleted key is simply absent from the child's clone, not "changed",
// and the old buggy loop only ever compared "changed" values, so the
// outer level's own invalidation never re-ran and the outermost, stale
// pre-construct value silently survived.
//
// The fix (now in resolveMayNotExecute) inverts the loop to iterate
// r.env — the PARENT's keys — instead: "for name, oldVal := range r.env
// { if newVal, stillPresent := clone[name]; !stillPresent || newVal !=
// oldVal { delete(r.env, name) } }". A key MISSING from clone (deleted
// at ANY inner depth) now triggers invalidation exactly like a CHANGED
// one. This is what makes invalidation compose at ANY nesting depth, by
// induction on the number of enclosing resolveMayNotExecute calls: each
// level's invalidation pass only ever needs to compare ITS OWN immediate
// child's clone against ITS OWN r.env, and "value differs OR is now
// absent" treats an inner deletion identically to a same-level
// modification — so the ambiguity signal (delete) propagates outward
// through every enclosing level unchanged, one level at a time, all the
// way to the top-level env, however many levels deep it originated. This
// is exactly the property this test file pins: NOT "depth 2 is fixed",
// but "composition holds at any depth" — see the depth-3+ cases below.
func TestEvaluateEffectMayNotExecuteInvalidationComposesAcrossNesting(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"for wrapping if: the audit's exact depth-2 repro",
			"R=echo; for i in 1; do if true; then R=rm; fi; done; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"while wrapping if",
			"R=echo; while true; do if true; then R=rm; fi; done; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"case wrapping if",
			"R=echo; case a in a) if true; then R=rm; fi ;; esac; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"if wrapping if (the very first if's Then containing a nested if)",
			"R=echo; if true; then if true; then R=rm; fi; fi; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"for wrapping case",
			"R=echo; for i in 1; do case a in a) R=rm ;; esac; done; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"while wrapping case",
			"R=echo; while true; do case a in a) R=rm ;; esac; done; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"if wrapping for",
			"R=echo; if true; then for i in 1; do R=rm; done; fi; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		{
			"case wrapping while",
			"R=echo; case a in a) while true; do R=rm; break; done ;; esac; $R -rf /", RiskRead, Confirm, RiskElevated,
			"effect: indeterminate",
		},
		// --- DEPTH 3+: three (and four) levels of may-not-execute
		// nesting, not merely a depth-2 patch -- pins composition as a
		// property, not a special case for exactly two levels.
		{
			"depth 3: for -> case -> if",
			"R=echo; for i in 1; do case a in a) if true; then R=rm; fi ;; esac; done; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"depth 3: while -> if -> case",
			"R=echo; while true; do if true; then case a in a) R=rm ;; esac; fi; done; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"depth 3: if -> for -> while",
			"R=echo; if true; then for i in 1; do while true; do R=rm; break; done; done; fi; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"depth 4: for -> while -> case -> if",
			"R=echo; for i in 1; do while true; do case a in a) if true; then R=rm; fi ;; esac; break; done; done; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateEffectMayNotExecuteBodyStillEscalatesAcrossNestedLevels
// re-affirms the intra-body escalation property (already pinned at depth
// 1 by TestEvaluateEffectMayNotExecuteBodyStillEscalatesWithinItself)
// still holds when the assignment-and-use pair sits INSIDE a nested
// may-not-execute body: composing the invalidation fix outward must
// never interfere with a body's own internal resolution of its own
// clone, at any depth.
func TestEvaluateEffectMayNotExecuteBodyStillEscalatesAcrossNestedLevels(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"for wrapping if, assignment and use both inside the innermost if",
			"for i in 1; do if true; then R=rm; $R -rf /; fi; done", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
		{
			"depth 3 (for -> case -> if), assignment and use both inside the innermost if",
			"for i in 1; do case a in a) if true; then R=rm; $R -rf /; fi ;; esac; done", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateEffectMayNotExecuteDoesNotOverInvalidateSameValue pins the
// PRECISION direction the audit's mutation-testing pass flagged: making
// the delete in resolveMayNotExecute's invalidation loop UNCONDITIONAL
// (deleting every key the child clone still holds, regardless of whether
// its value actually changed) would fail no other test in this suite,
// yet would be a real precision regression — a variable a may-not-execute
// body reassigns to the EXACT SAME value it already held is not
// ambiguous at all (whichever branch runs, or doesn't, the value is
// identical either way), so it must NOT be invalidated, and a
// command-word reference to it afterward must still resolve with full
// confidence.
func TestEvaluateEffectMayNotExecuteDoesNotOverInvalidateSameValue(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"for i in 1; do R=rm; done reassigns R to the SAME value R already had -- not ambiguous, must stay resolved",
			"R=rm; for i in 1; do R=rm; done; $R -rf /", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
		{
			"while true; do R=rm; break; done reassigns R to the SAME value -- same precision pin",
			"R=rm; while true; do R=rm; break; done; $R -rf /", RiskRead, Confirm, RiskDestructive,
			"effect: resolved argv matches denylist signature",
		},
	}
	runEvalCases(t, engine, cases)
}

// --- Resource guard (memory-amplification DoS): a case statement forks
// one clone PER ITEM, so nesting many-branch case/if/loop constructs
// multiplies (not merely adds to) total clone volume with nesting depth
// -- a follow-up security audit measured a ~19x/494MB blow-up (vs ~26MB
// on origin/main) from a single ~48KB crafted input exploiting exactly
// this shape. safeCloneEnv's shared resolverBudget must fail closed
// (indeterminate -> Confirm) once exhausted, well before that kind of
// amplification, and must never crash/hang/OOM even on a deliberately
// hostile shape -- this package parses LLM-generated, untrusted text.

// deeplyNestedIfCommand returns a command with n levels of nested "if
// true; then ...; fi", each level assigning its own uniquely-named
// variable, closing with a reference to the innermost one in
// command-word position -- if the guard did not fire, this would force n
// scope-forking clones (one per nested Then, each a clone of the
// accumulated env from all outer levels), which is exactly the
// depth-driven amplification shape the guard bounds.
func deeplyNestedIfCommand(n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		b.WriteString("if true; then V")
		b.WriteString(intToStr(i))
		b.WriteString("=v; ")
	}
	b.WriteString("$V0 -rf /")
	for i := 0; i < n; i++ {
		b.WriteString("; fi")
	}
	return b.String()
}

func intToStr(i int) string {
	if i == 0 {
		return "0"
	}
	var digits []byte
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}

func TestEvaluateEffectScopeGuardFailsClosedOnDeepNesting(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	// maxScopeForks is 512; nesting well beyond that (2000 levels) must
	// exhaust the budget and fail closed rather than allocating an
	// unbounded number of clones.
	cmd := deeplyNestedIfCommand(2000)
	got := engine.Evaluate(cmd, RiskRead)
	assert.Equal(t, Confirm, got.Action, "deeply nested input must fail closed to Confirm, never Allow")
	assert.Equal(t, RiskElevated, got.EffectiveRisk)
	assert.Contains(t, got.MatchedRule, scopeGuardReason)
}

// TestEvaluateEffectScopeGuardFailsClosedOnWideBranching pins the
// fan-out half of the guard: a SINGLE case statement with many items,
// each forking its own clone, must also be bounded by the same shared
// budget -- proving the guard is a cumulative FORK COUNT cap, not merely
// a nesting-depth cap (a depth-only cap would let a single flat case
// statement with thousands of items allocate thousands of clones with no
// nesting at all).
func TestEvaluateEffectScopeGuardFailsClosedOnWideBranching(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	var b strings.Builder
	b.WriteString("case x in ")
	for i := 0; i < maxScopeForks+50; i++ {
		b.WriteString("p")
		b.WriteString(intToStr(i))
		b.WriteString(") R=rm ;; ")
	}
	b.WriteString("esac; $UNKNOWNVAR -rf /")
	got := engine.Evaluate(b.String(), RiskRead)
	assert.Equal(t, Confirm, got.Action, "wide branching beyond the fork budget must fail closed to Confirm, never Allow")
	assert.Equal(t, RiskElevated, got.EffectiveRisk)
	assert.Contains(t, got.MatchedRule, scopeGuardReason)
}

// nestedCaseCommand builds a SYNTACTICALLY VALID command with `depth`
// levels of case statements, each with `itemsPerLevel` items: every item
// except the last is a flat no-op ("pN) : ;;"), and the LAST item's body
// is the next nested level's case statement -- fan-out-times-depth, one
// path deep, itemsPerLevel siblings wide at every level, matching the
// shape the security audit's crafted ~48KB input exploited for a
// ~19x/494MB memory blow-up (vs ~26MB on origin/main) before this fix.
func nestedCaseCommand(depth, itemsPerLevel int) string {
	body := "$UNKNOWNVAR -rf /"
	for d := 0; d < depth; d++ {
		var b strings.Builder
		b.WriteString("case x in ")
		for i := 0; i < itemsPerLevel-1; i++ {
			b.WriteString("p")
			b.WriteString(intToStr(i))
			b.WriteString(") : ;; ")
		}
		b.WriteString("p")
		b.WriteString(intToStr(itemsPerLevel - 1))
		b.WriteString(") ")
		b.WriteString(body)
		b.WriteString(" ;; esac")
		body = b.String()
	}
	return body
}

// TestEvaluateEffectScopeGuardBoundsHostileFanoutTimesDepthMemory pushes
// far past maxScopeForks on BOTH the depth AND fan-out axes at once (600
// levels x 5 items/level = 3,000 total case items, ~6x the fork budget,
// from a ~35KB command comparable in scale to the audit's ~48KB
// crafted input) and asserts BOTH that the guard fails closed
// (RiskElevated via scopeGuardReason, never a silent Allow) AND that
// total allocation stays a small, bounded amount — not the ~19x/494MB
// blow-up this exact shape caused before the guard existed. TotalAlloc
// (not HeapAlloc, which a GC pass can shrink back down regardless of how
// much was actually allocated along the way) is the meaningful signal
// for "how much work did this analysis actually do".
func TestEvaluateEffectScopeGuardBoundsHostileFanoutTimesDepthMemory(t *testing.T) {
	cmd := nestedCaseCommand(600, 5)
	t.Logf("command length: %d bytes; total case items %d (far exceeds maxScopeForks=%d)",
		len(cmd), 600*5, maxScopeForks)

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	verdict := analyzeBashEffect(cmd)

	runtime.GC()
	runtime.ReadMemStats(&after)

	totalAllocMB := float64(after.TotalAlloc-before.TotalAlloc) / (1024 * 1024)
	t.Logf("verdict: risk=%v reason=%q; TotalAlloc delta=%.3f MB", verdict.risk, verdict.reason, totalAllocMB)

	assert.Equal(t, RiskElevated, verdict.risk, "a hostile fan-out-times-depth input must fail closed, never silently resolve")
	assert.Contains(t, verdict.reason, scopeGuardReason)
	assert.Less(t, totalAllocMB, 20.0,
		"total allocation for this adversarial input must stay a small, bounded amount — regressing the guard would reproduce the audit's ~494MB blow-up")
}

// TestSafeCloneEnvGuard pins safeCloneEnv's exact contract directly,
// independent of how any particular construct calls it: budget
// exhaustion and env-size overflow each independently refuse the clone,
// and a successful clone decrements the shared budget by exactly one.
func TestSafeCloneEnvGuard(t *testing.T) {
	t.Run("budget exhausted refuses the clone", func(t *testing.T) {
		budget := &resolverBudget{forksRemaining: 0}
		clone, ok := safeCloneEnv(map[string]string{"A": "1"}, budget)
		assert.False(t, ok)
		assert.Nil(t, clone)
	})

	t.Run("env over maxEnvSize refuses the clone", func(t *testing.T) {
		budget := &resolverBudget{forksRemaining: maxScopeForks}
		big := make(map[string]string, maxEnvSize+1)
		for i := 0; i < maxEnvSize+1; i++ {
			big[intToStr(i)] = "v"
		}
		clone, ok := safeCloneEnv(big, budget)
		assert.False(t, ok)
		assert.Nil(t, clone)
		assert.Equal(t, maxScopeForks, budget.forksRemaining, "a refused clone must not consume budget")
	})

	t.Run("successful clone decrements the shared budget by exactly one", func(t *testing.T) {
		budget := &resolverBudget{forksRemaining: 5}
		clone, ok := safeCloneEnv(map[string]string{"A": "1"}, budget)
		assert.True(t, ok)
		assert.Equal(t, map[string]string{"A": "1"}, clone)
		assert.Equal(t, 4, budget.forksRemaining)
	})
}
