package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// linuxSafetyEngine builds a real safety.Engine (the built-in denylist and
// escalation rule set is fixed, package-level data — see
// internal/safety/engine.go's own doc comment — so a plain config.Config{}
// with no denylist_extra is enough to exercise it exactly as production
// does) fixed to the Unix/AST dialect regardless of the host actually
// running this test, mirroring internal/safety's own test seam
// (newEngineForGOOS) and this repo's cross-OS-safety-testing convention
// documented in CLAUDE.md's "Test Stratejisi" section.
func linuxSafetyEngine(t *testing.T) *safety.Engine {
	t.Helper()
	return safety.NewEngine(config.Config{})
}

// evaluatedStep builds an engine.Step whose Decision is a REAL,
// already-Evaluated safety.Decision (safetyEngine.Evaluate(command,
// safety.RiskRead)) — exactly how internal/engine.GeneratePlan populates
// every Step before internal/cli ever sees it — rather than a hand-built
// safety.Decision literal, which would let a test's expectations drift
// silently away from what the real classifier actually does. Every test
// below declares RiskRead (the lowest floor) deliberately: it makes every
// escalation this file exercises (to Confirm/elevated, to Block) visibly
// the safety engine's OWN doing, never inherited from an already-elevated
// declared risk.
func evaluatedStep(t *testing.T, safetyEngine *safety.Engine, command string) engine.Step {
	t.Helper()
	return engine.Step{
		Command:    command,
		Rationale:  "why: " + command,
		Risk:       safety.RiskRead,
		Reversible: true,
		Decision:   safetyEngine.Evaluate(command, safety.RiskRead),
	}
}

// --- identityIndices -------------------------------------------------------

func TestIdentityIndicesBuildsZeroBasedSequence(t *testing.T) {
	assert.Equal(t, []int{0, 1, 2}, identityIndices(3))
}

func TestIdentityIndicesEmptyForZero(t *testing.T) {
	assert.Empty(t, identityIndices(0))
}

// --- planToReviewSteps ------------------------------------------------------

func TestPlanToReviewStepsConvertsEveryFieldOneToOne(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	plan := engine.Plan{
		Summary: "irrelevant here",
		Steps: []engine.Step{
			evaluatedStep(t, safetyEngine, "echo hi"),
			evaluatedStep(t, safetyEngine, "rm -rf /"),
		},
	}

	rows := planToReviewSteps(plan)

	require.Len(t, rows, 2)
	assert.Equal(t, tui.PlanReviewStep{
		Command:   "echo hi",
		Rationale: "why: echo hi",
		Risk:      safety.RiskRead,
		Blocked:   false,
	}, rows[0])

	assert.Equal(t, "rm -rf /", rows[1].Command)
	assert.True(t, rows[1].Blocked)
	assert.Equal(t, "matches denylist rule: rm -rf / (or ~ / $HOME root delete)", rows[1].BlockReason)
	assert.Equal(t, safety.RiskDestructive, rows[1].Risk)
}

// --- resolveReviewPass (the security-critical core) ------------------------

// TestResolveReviewPassUneditedApprovalReturnsSamePlanUnchanged is case (a):
// nothing was edited/reordered/skipped/deleted — the pass finishes
// immediately with the exact same commands, in the exact same order, and
// (since no command text differs from the original) with each step's
// ORIGINAL Decision object carried through untouched, never re-evaluated.
func TestResolveReviewPassUneditedApprovalReturnsSamePlanUnchanged(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	stepA := evaluatedStep(t, safetyEngine, "echo a")
	stepB := evaluatedStep(t, safetyEngine, "echo b")
	plan := engine.Plan{Summary: "s", Steps: []engine.Step{stepA, stepB}}
	origIdx := identityIndices(2)
	outcome := tui.ReviewOutcome{
		Approved: true,
		Steps: []tui.ReviewedStep{
			{OriginalIndex: 0, Command: "echo a"},
			{OriginalIndex: 1, Command: "echo b"},
		},
	}

	finalPlan, nextSteps, nextOrigIdx, done := resolveReviewPass(plan, outcome, origIdx, safetyEngine)

	require.True(t, done)
	assert.Nil(t, nextSteps)
	assert.Nil(t, nextOrigIdx)
	require.Len(t, finalPlan.Steps, 2)
	assert.Equal(t, "echo a", finalPlan.Steps[0].Command)
	assert.Equal(t, "echo b", finalPlan.Steps[1].Command)
	// The original, already-Evaluated Decision is carried through
	// unchanged — never re-run through safetyEngine.Evaluate again — for
	// every row whose text is untouched.
	assert.Equal(t, stepA.Decision, finalPlan.Steps[0].Decision)
	assert.Equal(t, stepB.Decision, finalPlan.Steps[1].Decision)
}

// TestResolveReviewPassEditToBlockedCommandNeverFinalizes is case (b): the
// user edits a benign row into a denylisted command. The pass must NOT
// finalize (done=false) and the returned finalPlan must be the zero value
// (never acted on) — the caller (reviewPlan) is required to loop back into
// the editor instead of silently running or silently dropping the
// newly-dangerous command.
func TestResolveReviewPassEditToBlockedCommandNeverFinalizes(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	original := evaluatedStep(t, safetyEngine, "echo hi")
	require.Equal(t, safety.Allow, original.Decision.Action, "sanity: original step must start as a plain Allow")

	plan := engine.Plan{Steps: []engine.Step{original}}
	origIdx := identityIndices(1)
	outcome := tui.ReviewOutcome{
		Approved: true,
		Steps:    []tui.ReviewedStep{{OriginalIndex: 0, Command: "rm -rf /"}},
	}

	finalPlan, nextSteps, nextOrigIdx, done := resolveReviewPass(plan, outcome, origIdx, safetyEngine)

	assert.False(t, done)
	assert.Equal(t, engine.Plan{}, finalPlan, "no plan must be handed back while a row is freshly Blocked")
	require.Len(t, nextSteps, 1)
	assert.True(t, nextSteps[0].Blocked)
	assert.Equal(t, "rm -rf /", nextSteps[0].Command)
	assert.Equal(t, []int{0}, nextOrigIdx)
}

// TestResolveReviewPassEditToElevatedCommandStillFinalizesConfirmable is
// case (c): the edited command is benign-but-elevated (sudo), NOT
// denylisted. This must finalize (done=true, ready for engine.Execute) but
// the returned step must carry the FRESHLY re-evaluated Confirm/elevated
// Decision — proof that a plan-review edit can never silently downgrade a
// step to something engine.Execute's own `>= RiskElevated` re-prompt (see
// internal/engine/runner.go) fails to catch.
func TestResolveReviewPassEditToElevatedCommandStillFinalizesConfirmable(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	original := evaluatedStep(t, safetyEngine, "echo hi")
	require.Equal(t, safety.Allow, original.Decision.Action, "sanity: original step must start as a plain Allow")

	plan := engine.Plan{Steps: []engine.Step{original}}
	origIdx := identityIndices(1)
	outcome := tui.ReviewOutcome{
		Approved: true,
		Steps:    []tui.ReviewedStep{{OriginalIndex: 0, Command: "sudo systemctl restart nginx"}},
	}

	finalPlan, nextSteps, nextOrigIdx, done := resolveReviewPass(plan, outcome, origIdx, safetyEngine)

	require.True(t, done)
	assert.Nil(t, nextSteps)
	assert.Nil(t, nextOrigIdx)
	require.Len(t, finalPlan.Steps, 1)
	got := finalPlan.Steps[0]
	assert.Equal(t, "sudo systemctl restart nginx", got.Command)
	assert.Equal(t, safety.Confirm, got.Decision.Action)
	assert.Equal(t, safety.RiskElevated, got.Decision.EffectiveRisk)
	assert.NotEqual(t, original.Decision, got.Decision, "the edited step must carry a freshly re-evaluated Decision, not the stale Allow one")
}

// TestResolveReviewPassOriginalBlockedRowLeftUnresolvedLoopsBack is case
// (e): a row that was ALREADY Blocked in the original plan (rendered
// Blocked from the very first pass, per planToReviewSteps) is left
// untouched by the user — not edited, not skipped, not deleted. Since its
// command text is unchanged, resolveReviewPass never re-runs
// safetyEngine.Evaluate on it, but it must still surface as Blocked (its
// carried-through original Decision already says Block) and force the loop
// back — a Blocked row can never silently finalize into an executable
// plan.
func TestResolveReviewPassOriginalBlockedRowLeftUnresolvedLoopsBack(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	blocked := evaluatedStep(t, safetyEngine, "rm -rf /")
	require.Equal(t, safety.Block, blocked.Decision.Action, "sanity: this step must start Blocked")

	plan := engine.Plan{Steps: []engine.Step{blocked}}
	origIdx := identityIndices(1)
	outcome := tui.ReviewOutcome{
		Approved: true,
		Steps:    []tui.ReviewedStep{{OriginalIndex: 0, Command: "rm -rf /"}},
	}

	finalPlan, nextSteps, nextOrigIdx, done := resolveReviewPass(plan, outcome, origIdx, safetyEngine)

	assert.False(t, done)
	assert.Equal(t, engine.Plan{}, finalPlan)
	require.Len(t, nextSteps, 1)
	assert.True(t, nextSteps[0].Blocked)
	assert.Equal(t, []int{0}, nextOrigIdx)
}

// TestResolveReviewPassSkippedRowsAreDroppedEntirely exercises the
// skip/delete equivalence resolveReviewPass's own doc comment describes:
// a Skipped row and a wholly-omitted (deleted) row both vanish from the
// final plan with no trace and no safety re-evaluation.
func TestResolveReviewPassSkippedRowsAreDroppedEntirely(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	stepA := evaluatedStep(t, safetyEngine, "echo a")
	stepB := evaluatedStep(t, safetyEngine, "echo b")
	stepC := evaluatedStep(t, safetyEngine, "echo c")
	plan := engine.Plan{Steps: []engine.Step{stepA, stepB, stepC}}
	origIdx := identityIndices(3)
	outcome := tui.ReviewOutcome{
		Approved: true,
		Steps: []tui.ReviewedStep{
			{OriginalIndex: 0, Command: "echo a"},
			{OriginalIndex: 1, Command: "echo b", Skipped: true}, // toggled skip
			// index 2 (C) is entirely absent: deleted.
		},
	}

	finalPlan, _, _, done := resolveReviewPass(plan, outcome, origIdx, safetyEngine)

	require.True(t, done)
	require.Len(t, finalPlan.Steps, 1)
	assert.Equal(t, "echo a", finalPlan.Steps[0].Command)
}

// TestResolveReviewPassReorderDeleteSkipAcrossTwoPasses is case (d): a
// full two-pass loop covering reorder + a transient block + a fix compared
// against the TRUE original (not the prior pass's edited text) + delete +
// skip, asserting the origIdx remapping is correct at every step.
//
// Original plan (plan.Steps order): [A, B, C, D], all initially Allow.
//
// Pass 1 (origIdx = identity [0,1,2,3]): the reviewer reorders to
// [B, D, A, C] and edits D's command to "rm -rf /" — a fresh Block.
// resolveReviewPass must refuse to finalize, and must hand back
// nextOrigIdx that correctly maps this NEW row order back to the TRUE
// plan.Steps indices ([1,3,0,2]).
//
// Pass 2 (origIdx = pass 1's nextOrigIdx [1,3,0,2]): the reviewer fixes D
// back to its exact ORIGINAL text ("echo d") — proving the re-evaluation
// compares against plan.Steps' true original, not pass 1's "rm -rf /" —
// and additionally skips C and deletes A entirely. The final plan must be
// exactly [B, D] in that order, with D's Decision the untouched original
// Allow (never re-evaluated, since its final text matches the original).
func TestResolveReviewPassReorderDeleteSkipAcrossTwoPasses(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	stepA := evaluatedStep(t, safetyEngine, "echo a")
	stepB := evaluatedStep(t, safetyEngine, "echo b")
	stepC := evaluatedStep(t, safetyEngine, "echo c")
	stepD := evaluatedStep(t, safetyEngine, "echo d")
	plan := engine.Plan{Steps: []engine.Step{stepA, stepB, stepC, stepD}}

	// --- pass 1 ---
	pass1Outcome := tui.ReviewOutcome{
		Approved: true,
		Steps: []tui.ReviewedStep{
			{OriginalIndex: 1, Command: "echo b"},   // B, unedited
			{OriginalIndex: 3, Command: "rm -rf /"}, // D, EDITED -> blocked
			{OriginalIndex: 0, Command: "echo a"},   // A, unedited
			{OriginalIndex: 2, Command: "echo c"},   // C, unedited
		},
	}
	finalPlan1, nextSteps1, nextOrigIdx1, done1 := resolveReviewPass(plan, pass1Outcome, identityIndices(4), safetyEngine)

	require.False(t, done1, "a freshly-Blocked D must force another pass")
	assert.Equal(t, engine.Plan{}, finalPlan1)
	require.Len(t, nextSteps1, 4)
	assert.Equal(t, []int{1, 3, 0, 2}, nextOrigIdx1, "nextOrigIdx must map pass 2's row order back to the TRUE plan.Steps indices")
	assert.False(t, nextSteps1[0].Blocked, "B")
	assert.True(t, nextSteps1[1].Blocked, "D must render Blocked after the edit")
	assert.False(t, nextSteps1[2].Blocked, "A")
	assert.False(t, nextSteps1[3].Blocked, "C")

	// --- pass 2 --- (row order now [B, D, A, C], origIdx = [1,3,0,2])
	pass2Outcome := tui.ReviewOutcome{
		Approved: true,
		Steps: []tui.ReviewedStep{
			{OriginalIndex: 0, Command: "echo b"},                // B, unedited
			{OriginalIndex: 1, Command: "echo d"},                // D, fixed back to its TRUE original text
			{OriginalIndex: 3, Command: "echo c", Skipped: true}, // C, now skipped
			// A (row index 2 in this pass) is entirely omitted: deleted.
		},
	}
	finalPlan2, nextSteps2, nextOrigIdx2, done2 := resolveReviewPass(plan, pass2Outcome, nextOrigIdx1, safetyEngine)

	require.True(t, done2)
	assert.Nil(t, nextSteps2)
	assert.Nil(t, nextOrigIdx2)
	require.Len(t, finalPlan2.Steps, 2)
	assert.Equal(t, "echo b", finalPlan2.Steps[0].Command)
	assert.Equal(t, "echo d", finalPlan2.Steps[1].Command)
	// D's final text matches the TRUE original exactly -> never
	// re-evaluated -> the untouched original Decision comes through.
	assert.Equal(t, stepD.Decision, finalPlan2.Steps[1].Decision)
}

// --- shouldShowPlanReview (full truth table) --------------------------------

func TestShouldShowPlanReview(t *testing.T) {
	tests := []struct {
		name             string
		mode             engine.Mode
		planReviewConfig string
		forceOn          bool
		forceOff         bool
		isTerminalNow    bool
		stepCount        int
		want             bool
	}{
		{
			name: "no-review flag always wins, even ask+config=ask+TTY+multi-step",
			mode: engine.ModeAsk, planReviewConfig: "ask", forceOn: true, forceOff: true,
			isTerminalNow: true, stepCount: 5, want: false,
		},
		{
			name: "info mode never shows it even when everything else says yes",
			mode: engine.ModeInfo, planReviewConfig: "ask", forceOn: true,
			isTerminalNow: true, stepCount: 5, want: false,
		},
		{
			name: "non-TTY skips even with --review forced on",
			mode: engine.ModeAsk, planReviewConfig: "ask", forceOn: true,
			isTerminalNow: false, stepCount: 5, want: false,
		},
		{
			name: "fewer than 2 steps skips even with --review forced on",
			mode: engine.ModeAsk, planReviewConfig: "ask", forceOn: true,
			isTerminalNow: true, stepCount: 1, want: false,
		},
		{
			name: "ask mode, config off, no force: never shown",
			mode: engine.ModeAsk, planReviewConfig: "off",
			isTerminalNow: true, stepCount: 2, want: false,
		},
		{
			name: "ask mode, config ask: shown",
			mode: engine.ModeAsk, planReviewConfig: "ask",
			isTerminalNow: true, stepCount: 2, want: true,
		},
		{
			name: "ask mode, config off, forceOn: shown anyway",
			mode: engine.ModeAsk, planReviewConfig: "off", forceOn: true,
			isTerminalNow: true, stepCount: 2, want: true,
		},
		{
			name: "auto mode, config ask, no force: never shown (config alone must not affect auto)",
			mode: engine.ModeAuto, planReviewConfig: "ask",
			isTerminalNow: true, stepCount: 2, want: false,
		},
		{
			name: "auto mode, forceOn: shown",
			mode: engine.ModeAuto, planReviewConfig: "off", forceOn: true,
			isTerminalNow: true, stepCount: 2, want: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldShowPlanReview(tc.mode, tc.planReviewConfig, tc.forceOn, tc.forceOff, tc.isTerminalNow, tc.stepCount)
			assert.Equal(t, tc.want, got)
		})
	}
}

// --- reviewPlan (the driving loop, via a scripted fake reviewer) -----------

// fakePlanReviewer scripts a fixed sequence of tui.ReviewOutcome values,
// one per call to Review, and records every steps argument it was called
// with — so a test can assert reviewPlan actually re-entered the reviewer
// with the freshly re-rendered Blocked state, not just that it eventually
// returned the right plan.
type fakePlanReviewer struct {
	outcomes []tui.ReviewOutcome
	calls    [][]tui.PlanReviewStep
}

func (f *fakePlanReviewer) Review(_ context.Context, steps []tui.PlanReviewStep) (tui.ReviewOutcome, error) {
	f.calls = append(f.calls, steps)
	idx := len(f.calls) - 1
	if idx >= len(f.outcomes) {
		return tui.ReviewOutcome{}, errors.New("fakePlanReviewer: no scripted outcome left")
	}
	return f.outcomes[idx], nil
}

// TestReviewPlanReEntersOnNewlyBlockedThenReturnsSafePlan is the
// integration-shaped counterpart to the resolveReviewPass unit tests
// above: it drives reviewPlan itself (the actual loop, not just one pass)
// through a scripted reviewer that first edits a step into a Blocked
// command, then fixes it, and asserts reviewPlan (a) calls the reviewer
// exactly twice, (b) the second call is shown the row rendered Blocked,
// and (c) the final returned plan is the safe, fixed one.
func TestReviewPlanReEntersOnNewlyBlockedThenReturnsSafePlan(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	original := evaluatedStep(t, safetyEngine, "echo hi")
	plan := engine.Plan{Steps: []engine.Step{original}}

	reviewer := &fakePlanReviewer{
		outcomes: []tui.ReviewOutcome{
			{Approved: true, Steps: []tui.ReviewedStep{{OriginalIndex: 0, Command: "rm -rf /"}}},
			{Approved: true, Steps: []tui.ReviewedStep{{OriginalIndex: 0, Command: "echo hi-fixed"}}},
		},
	}

	finalPlan, ok, err := reviewPlan(context.Background(), plan, safetyEngine, reviewer)

	require.NoError(t, err)
	require.True(t, ok)
	require.Len(t, reviewer.calls, 2, "reviewPlan must re-enter the reviewer exactly once after the fresh Block")
	require.Len(t, reviewer.calls[1], 1)
	assert.True(t, reviewer.calls[1][0].Blocked, "the second call must show the row rendered Blocked from pass 1's edit")

	require.Len(t, finalPlan.Steps, 1)
	assert.Equal(t, "echo hi-fixed", finalPlan.Steps[0].Command)
	assert.Equal(t, safety.Allow, finalPlan.Steps[0].Decision.Action)
}

// TestReviewPlanCanceledOutcomeReturnsNotOK asserts a canceled
// (Approved=false) outcome — ctrl+c or esc in the real screen — surfaces
// as ok=false with no usable plan, exactly like a canceled ask-mode
// confirm prompt aborts the whole run today.
func TestReviewPlanCanceledOutcomeReturnsNotOK(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	plan := engine.Plan{Steps: []engine.Step{evaluatedStep(t, safetyEngine, "echo hi")}}
	reviewer := &fakePlanReviewer{outcomes: []tui.ReviewOutcome{{Approved: false}}}

	finalPlan, ok, err := reviewPlan(context.Background(), plan, safetyEngine, reviewer)

	require.NoError(t, err)
	assert.False(t, ok)
	assert.Equal(t, engine.Plan{}, finalPlan)
	assert.Len(t, reviewer.calls, 1)
}

// TestReviewPlanReviewerErrorPropagates confirms a hard error from the
// reviewer (e.g. the underlying bubbletea program failing) is returned
// as-is rather than swallowed or retried.
func TestReviewPlanReviewerErrorPropagates(t *testing.T) {
	safetyEngine := linuxSafetyEngine(t)
	plan := engine.Plan{Steps: []engine.Step{evaluatedStep(t, safetyEngine, "echo hi")}}
	reviewer := &fakePlanReviewer{} // no scripted outcomes at all -> errors on first call

	_, ok, err := reviewPlan(context.Background(), plan, safetyEngine, reviewer)

	require.Error(t, err)
	assert.False(t, ok)
}

// --- tuiPlanReviewer.Review (the one concrete-bubbletea wiring point) -----

// TestTuiPlanReviewerReviewDelegatesToRealReviewPlan drives the real
// internal/tui.ReviewPlan through tuiPlanReviewer.Review exactly once,
// using the same PTY-free, reader-based, time-bounded pattern
// internal/tui/planreview_test.go's own headless ReviewPlan tests use
// (strings.NewReader feeding one scripted keypress, never a live
// stdin/PTY) — proving the thin CLI-layer wrapper actually delegates
// colorEnabled/tr/in/out through correctly, not just that ReviewPlan
// itself works (already covered in internal/tui).
func TestTuiPlanReviewerReviewDelegatesToRealReviewPlan(t *testing.T) {
	reviewer := &tuiPlanReviewer{
		in:           strings.NewReader("a"), // EN: approve all
		out:          &bytes.Buffer{},
		colorEnabled: false,
		tr:           i18n.NewTranslator(i18n.LangEN),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	outcome, err := reviewer.Review(ctx, []tui.PlanReviewStep{{Command: "echo hi", Risk: safety.RiskRead}})

	require.NoError(t, err)
	require.True(t, outcome.Approved)
	require.Len(t, outcome.Steps, 1)
	assert.Equal(t, "echo hi", outcome.Steps[0].Command)
}
