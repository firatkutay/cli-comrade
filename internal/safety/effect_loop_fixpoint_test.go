package safety

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// --- Loop-carried variable dependency (GitHub issue #33): a for/while
// body used to be resolved in a SINGLE pass (resolveMayNotExecute),
// seeded once from the pre-loop env. A value that only becomes dangerous
// on the SECOND (or later) simulated iteration was therefore invisible:
// pass 1 alone can coincidentally reproduce the pre-loop value for a
// variable that real bash's later iterations do change, so the old
// single-pass invalidation check ("does pass 1 differ from the pre-loop
// env?") wrongly concluded "unchanged" and left the stale value fully
// resolved. resolveLoopBody (effect_bash.go) now resolves a for/while/
// until body to a FIXPOINT — repeatedly re-applying the body to its own
// prior result, bounded by maxLoopFixpointIterations — and invalidates
// any name that ever changes anywhere along that chain. See
// resolveLoopBody's own doc comment for the full mechanism.
//
// Every case below is the auditor's original repro plus adversarial
// variants this fix must also catch: a longer relay chain that only
// resolves on iteration 3, a while-loop equivalent, a two-variable swap,
// a nested loop, and an invalidate-then-unconditionally-rebind shape. All
// must land on at least Confirm (RiskElevated, "effect: indeterminate")
// — the sink variable must be invalidated, never silently resolved to
// either its pre-loop or its post-pass-1 value.
func TestEvaluateEffectLoopCarriedDependencyEscalates(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"issue #33's exact repro: X only becomes 'rm' on the loop's SECOND iteration",
			"X=echo; R=echo; for i in 1 2; do X=$R; R=rm; done; $X -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"3-hop relay chain (X<-Y<-Z<-rm): X only becomes dangerous on the THIRD iteration",
			"X=echo; Y=echo; Z=echo; for i in 1 2 3; do X=$Y; Y=$Z; Z=rm; done; $X -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"while-loop equivalent of the exact repro -- same iteration-carried dependency, different construct",
			"X=echo; R=echo; while true; do X=$R; R=rm; break; done; $X -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"swap two variables inside the loop: real parity determines the final value, which this analyzer cannot know",
			"A=echo; B=rm; for i in 1 2; do T=$A; A=$B; B=$T; done; $A -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"nested for-in-for: the inner loop's own reassignment must still surface through the outer loop's fixpoint",
			"R=echo; for i in 1 2; do for j in 1 2; do R=rm; done; done; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"invalidate-then-rebind: an inner if invalidates R, then the SAME body unconditionally rebinds it afterward",
			"R=echo; for i in 1; do if true; then R=dummy; fi; R=rm; done; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"a longer (10-item) Items list changing the sink on iteration 1 must still invalidate, not just short loops",
			"R=echo; for i in 1 2 3 4 5 6 7 8 9 10; do R=rm; done; $R -rf /",
			RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateEffectLoopFixpointDoesNotOverInvalidate is the PRECISION
// negative control the fix must not regress: a benign multi-iteration
// loop that reassigns a variable to the SAME value on every single pass
// (so the fixpoint search converges immediately, and every simulated
// iteration agrees with the pre-loop value) must resolve with full
// confidence and stay exactly Allow -- never Confirm just because the
// command happens to contain a multi-iteration loop. Over-invalidating
// every loop regardless of whether anything is actually ambiguous would
// be "safe" in the narrow sense of never lowering risk, but would
// destroy the tool's usability (every ordinary loop one-liner would
// start prompting in auto mode) -- exactly the failure mode this test
// guards against.
func TestEvaluateEffectLoopFixpointDoesNotOverInvalidate(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"CMD reassigned to the SAME value on every one of 3 passes -- must resolve fully, stay Allow",
			"CMD=echo; for i in 1 2 3; do CMD=echo; done; $CMD hello",
			RiskRead, Allow, RiskRead, "",
		},
		{
			"CMD reassigned to the SAME value on every one of 5 passes -- must resolve fully, stay Allow",
			"CMD=echo; for i in 1 2 3 4 5; do CMD=echo; done; $CMD hi",
			RiskRead, Allow, RiskRead, "",
		},
		{
			"while-loop equivalent: CMD reassigned to the SAME value, must resolve fully, stay Allow",
			"CMD=echo; while true; do CMD=echo; break; done; $CMD ok",
			RiskRead, Allow, RiskRead, "",
		},
		{
			"R already dangerous before the loop, reassigned to the SAME dangerous value across 3 passes -- must resolve fully to Destructive, not fall back to the less-precise indeterminate path",
			"R=rm; for i in 1 2 3; do R=rm; done; $R -rf /",
			RiskRead, Confirm, RiskDestructive, "effect: resolved argv matches denylist signature",
		},
	}
	runEvalCases(t, engine, cases)
}

// relayChainCommand builds an n-variable relay chain command whose SINK
// variable (V1) only becomes "rm" once the fixpoint search has propagated
// the assignment all the way from V(n) down to V1 -- exactly the shape a
// follow-up security audit used to find a CRITICAL false-Allow in an
// earlier version of resolveLoopBody. With n links, the loop body is
// `V1=$V2; V2=$V3; ...; V(n-1)=$Vn; Vn=rm`, so the sink first changes on
// simulated pass n-1 and the fixpoint search needs pass n to CONFIRM
// stability -- n=9 needs 9 passes total to converge, past
// maxLoopFixpointIterations=8, which is exactly the boundary the audit's
// exploit crossed.
func relayChainCommand(n int) string {
	var seeds, body, items strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&seeds, "V%d=echo; ", i)
		if i > 1 {
			items.WriteString(" ")
		}
		fmt.Fprintf(&items, "%d", i)
	}
	for i := 1; i < n; i++ {
		fmt.Fprintf(&body, "V%d=$V%d; ", i, i+1)
	}
	fmt.Fprintf(&body, "V%d=rm; ", n)
	return fmt.Sprintf("%sfor i in %s; do %sdone; $V1 -rf /", seeds.String(), items.String(), body.String())
}

// TestEvaluateEffectLoopCapExhaustionFailsClosed is the regression test
// for a CRITICAL false-Allow a follow-up security audit found in the
// original resolveLoopBody: when maxLoopFixpointIterations was hit
// WITHOUT the search reaching a genuine fixpoint, the function only
// invalidated names it had directly OBSERVED changing within the passes
// actually run (the `changed` set) -- but a name that stays stable
// through every observed pass and only changes on the NEXT (unobserved)
// one is exactly as ambiguous as one that visibly changed, and was
// wrongly left fully resolved. A 9-link relayChainCommand only changes
// its sink (V1) on simulated pass 8, so with the ORIGINAL 8-pass cap the
// search exhausted its budget one pass short of ever observing V1
// change at all -- `changed` never contained "V1", so V1 kept its stale
// pre-loop "echo" and the command classified read/Allow despite real
// bash ending with V1=rm and genuinely running `rm -rf /` unprompted in
// auto mode. The fix (the `!converged` branch in resolveLoopBody)
// invalidates the ENTIRE parent env whenever the cap is hit without
// converging, not merely the observed `changed` subset. n=9 is the
// audit's own minimal exploit; n=12 additionally proves the fix holds
// well past the exact boundary, not merely at it.
func TestEvaluateEffectLoopCapExhaustionFailsClosed(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"n=9 relay chain: the audit's exact minimal exploit -- sink only changes on the pass the original 8-iteration cap could not observe",
			relayChainCommand(9), RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
		{
			"n=12 relay chain: well past the exact cap boundary, proving the fix isn't a boundary-only patch",
			relayChainCommand(12), RiskRead, Confirm, RiskElevated, "effect: indeterminate",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateEffectLoopBodyMultiPassTextJoin pins the load-bearing
// property that EVERY simulated pass's own reconstructed text is joined
// into the returned text, not just the final pass's: a value that is
// only dangerous WITHIN a later simulated iteration (here, $R resolves to
// the still-safe "echo" on pass 1 but to the already-dangerous "rm" on
// pass 2, since R was reassigned to "rm" at the END of pass 1) must still
// surface via the denylist-signature-reuse match analyzeBashEffect runs
// over the whole reconstructed command. This is silently untested by
// every other case in this file (they all pin the Confirm/indeterminate
// INVALIDATION path, not the text-join path) -- mutating resolveLoopBody
// to keep only pass 1's text passes every other test in this package, yet
// is a real regression: `$R -rf /` on pass 2 is destructive text this
// analyzer would otherwise silently drop.
func TestEvaluateEffectLoopBodyMultiPassTextJoin(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	cases := []evalCase{
		{
			"$R resolves to 'echo' on pass 1 but 'rm' on pass 2 -- the pass-2 dangerous text must not be dropped",
			"R=echo; for i in 1 2; do $R -rf /; R=rm; done",
			RiskRead, Confirm, RiskDestructive, "effect: resolved argv matches denylist signature",
		},
	}
	runEvalCases(t, engine, cases)
}

// TestEvaluateEffectLoopBodyCloneGuardFailsClosed covers the
// safeCloneEnv failure branch INSIDE resolveLoopBody's own fixpoint loop
// (as opposed to TestEvaluateEffectScopeGuardFailsClosedOnDeepNesting/
// OnWideBranching in effect_soundness_test.go, which exercise the SAME
// guard via resolveMayNotExecute's if/case call sites -- a different
// line, needing its own direct test). maxEnvSize (256) variable
// assignments before the loop means the very first safeCloneEnv call
// resolveLoopBody makes already refuses to clone, so this fails closed to
// Confirm/RiskElevated via scopeGuardReason before ever attempting a
// single fixpoint pass.
func TestEvaluateEffectLoopBodyCloneGuardFailsClosed(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	var b strings.Builder
	for i := 0; i <= maxEnvSize; i++ {
		b.WriteString("V")
		b.WriteString(intToStr(i))
		b.WriteString("=v; ")
	}
	b.WriteString("for i in 1; do R=rm; done; $UNKNOWNVAR -rf /")
	got := engine.Evaluate(b.String(), RiskRead)
	assert.Equal(t, Confirm, got.Action, "env already over maxEnvSize before the loop must fail closed to Confirm, never Allow")
	assert.Equal(t, RiskElevated, got.EffectiveRisk)
	assert.Contains(t, got.MatchedRule, scopeGuardReason)
}

// TestEvaluateEffectLoopBodyIndeterminateChildFailsClosed covers the
// indeterminate-propagation branch INSIDE resolveLoopBody's fixpoint
// loop: a loop body that is itself strictly-unresolvable (here, an
// unresolved command word inside the Do body) must propagate that
// indeterminate result out of resolveLoopBody directly on the very first
// pass, rather than being silently swallowed or treated as "no change".
func TestEvaluateEffectLoopBodyIndeterminateChildFailsClosed(t *testing.T) {
	engine := newEngineForGOOS(config.Default(), "linux")
	got := engine.Evaluate("for i in 1; do $UNKNOWNVAR -rf /; done", RiskRead)
	assert.Equal(t, Confirm, got.Action)
	assert.Equal(t, RiskElevated, got.EffectiveRisk)
	assert.Contains(t, got.MatchedRule, "effect: indeterminate")
}

// TestEnvsEqual pins envsEqual's exact contract directly: same key set AND
// same values for every key, in either direction (extra/missing/differing
// keys all count as unequal).
func TestEnvsEqual(t *testing.T) {
	cases := []struct {
		name string
		a, b map[string]string
		want bool
	}{
		{"both empty", map[string]string{}, map[string]string{}, true},
		{"identical single entry", map[string]string{"A": "1"}, map[string]string{"A": "1"}, true},
		{"identical multiple entries", map[string]string{"A": "1", "B": "2"}, map[string]string{"A": "1", "B": "2"}, true},
		{"differing value for same key", map[string]string{"A": "1"}, map[string]string{"A": "2"}, false},
		{"b has an extra key", map[string]string{"A": "1"}, map[string]string{"A": "1", "B": "2"}, false},
		{"a has an extra key", map[string]string{"A": "1", "B": "2"}, map[string]string{"A": "1"}, false},
		{"disjoint keys, same size", map[string]string{"A": "1"}, map[string]string{"B": "1"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, envsEqual(tc.a, tc.b))
		})
	}
}
