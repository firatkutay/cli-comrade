package safety

import (
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
