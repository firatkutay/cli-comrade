package cli

import (
	"context"
	"io"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// planReviewer is the minimal interactive plan-preview/edit capability
// runDo/runFix need — a package-local, consumer-side interface (exactly
// like engine.PromptUI's own design) so tests can inject a scripted fake
// with no bubbletea program involved at all. tuiPlanReviewer wraps the
// real internal/tui.ReviewPlan.
type planReviewer interface {
	Review(ctx context.Context, steps []tui.PlanReviewStep) (tui.ReviewOutcome, error)
}

// tuiPlanReviewer is the one place a concrete UI toolkit (bubbletea, via
// internal/tui) is wired into planReviewer — mirroring tuiPromptUI's own
// role for engine.PromptUI (promptui.go).
type tuiPlanReviewer struct {
	in           io.Reader
	out          io.Writer
	colorEnabled bool
	tr           i18n.Translator
}

func (r *tuiPlanReviewer) Review(ctx context.Context, steps []tui.PlanReviewStep) (tui.ReviewOutcome, error) {
	return tui.ReviewPlan(ctx, steps, r.colorEnabled, r.in, r.out, r.tr)
}

// shouldShowPlanReview is the pure gate deciding whether runDo/runFix
// show the plan-preview/edit screen at all, before ever constructing a
// planReviewer or touching stdin:
//
//   - forceOff (--no-review) always wins: never shown, regardless of
//     everything else.
//   - info mode: never — nothing executes, so there is nothing to
//     preview beyond what info mode already prints step-by-step.
//   - non-interactive stdin, or a plan with fewer than 2 steps: never —
//     there is nothing meaningful to reorder/edit in a single-step plan,
//     and a non-TTY invocation (piped/scripted) has no way to drive the
//     screen at all.
//   - ask mode: shown when general.plan_review=="ask" OR forceOn
//     (--review).
//   - auto mode: shown ONLY when forceOn (--review) — general.
//     plan_review=="ask" alone must never start showing a review screen
//     in a hands-off auto run (e.g. a cron/automation invocation); only
//     an explicit per-invocation flag forces it there.
func shouldShowPlanReview(mode engine.Mode, planReviewConfig string, forceOn, forceOff, isTerminalNow bool, stepCount int) bool {
	if forceOff {
		return false
	}
	if mode == engine.ModeInfo || !isTerminalNow || stepCount < 2 {
		return false
	}
	switch mode {
	case engine.ModeAsk:
		return planReviewConfig == "ask" || forceOn
	case engine.ModeAuto:
		return forceOn
	default:
		return false
	}
}

// planToReviewSteps converts plan's steps 1:1 into the tui-facing slice
// ReviewPlan's first call renders — Blocked/BlockReason come straight
// from each step's own (already-Evaluated) safety.Decision, exactly like
// renderPlan's dry-run table derives its RISK column from the same
// field, never from the LLM's raw declared risk.
func planToReviewSteps(plan engine.Plan) []tui.PlanReviewStep {
	steps := make([]tui.PlanReviewStep, len(plan.Steps))
	for i, s := range plan.Steps {
		steps[i] = tui.PlanReviewStep{
			Command:     s.Command,
			Rationale:   s.Rationale,
			Risk:        s.Decision.EffectiveRisk,
			Blocked:     s.Decision.Action == safety.Block,
			BlockReason: s.Decision.Reason,
		}
	}
	return steps
}

// reviewPlan drives the whole plan-preview/edit flow for plan: it shows
// the screen (via reviewer), and — as long as the user approves — checks
// every edited command against safetyEngine one more time. As long as
// that finds a newly-Blocked command, it re-enters the screen (a fresh
// reviewer.Review call) with that row now rendered as Blocked, instead of
// silently either running the newly-dangerous command or silently
// dropping it — the user must explicitly resolve it (edit again, skip,
// or delete) before the run can proceed. This is the ONLY place
// internal/cli ever calls safety.Engine.Evaluate for plan review — the
// tui package itself never evaluates safety at all (see
// internal/tui/planreview.go's own doc comment on ReviewPlan).
//
// Returns ok=false (with a zero Plan) when the user canceled the whole
// review (ctrl+c or esc) — the caller must abort the run entirely in
// that case, exactly like a Ctrl-C-canceled ask-mode confirm prompt
// aborts today.
func reviewPlan(ctx context.Context, plan engine.Plan, safetyEngine *safety.Engine, reviewer planReviewer) (engine.Plan, bool, error) {
	steps := planToReviewSteps(plan)
	origIdx := identityIndices(len(plan.Steps))

	for {
		outcome, err := reviewer.Review(ctx, steps)
		if err != nil {
			return engine.Plan{}, false, err
		}
		if !outcome.Approved {
			return engine.Plan{}, false, nil
		}

		finalPlan, nextSteps, nextOrigIdx, done := resolveReviewPass(plan, outcome, origIdx, safetyEngine)
		if done {
			return finalPlan, true, nil
		}
		steps, origIdx = nextSteps, nextOrigIdx
	}
}

// identityIndices returns [0, 1, ..., n-1] — the origIdx mapping
// reviewPlan's very first pass uses, before any reorder/delete has ever
// happened.
func identityIndices(n int) []int {
	idx := make([]int, n)
	for i := range idx {
		idx[i] = i
	}
	return idx
}

// resolveReviewPass is reviewPlan's pure, per-pass core (deliberately
// factored out so it is testable with no reviewer/bubbletea involved at
// all — see planreview_test.go's TestResolveReviewPass* table, including
// the two-pass reorder/delete/skip case and the edit-to-Blocked/
// edit-to-elevated security-critical cases): given one pass's
// ReviewOutcome and the origIdx mapping from THAT pass's row positions
// back to plan's own step indices, it:
//
//  1. drops every Skipped row entirely (skip and delete are equivalent
//     from this point on — neither survives into the final Plan);
//  2. for every surviving row, re-evaluates safety ONLY when its command
//     text actually differs from the ORIGINAL plan step's command (never
//     re-evaluating an unedited step, which already carries a real,
//     Evaluated Decision) — comparing against the true original, not the
//     previous pass's edited text, so a step edited across multiple
//     passes is always checked against what the LLM actually proposed;
//  3. if any surviving row's (possibly freshly re-evaluated) Decision is
//     Block, returns done=false plus the next pass's steps/origIdx
//     (current order, current text, Blocked rendered for whichever
//     row(s) just triggered this) — the caller loops back to the
//     reviewer instead of finalizing;
//  4. otherwise returns done=true plus the final engine.Plan, built from
//     the surviving rows in their current order with each one's
//     (possibly freshly re-evaluated) Decision attached.
func resolveReviewPass(plan engine.Plan, outcome tui.ReviewOutcome, origIdx []int, safetyEngine *safety.Engine) (finalPlan engine.Plan, nextSteps []tui.PlanReviewStep, nextOrigIdx []int, done bool) {
	finalSteps := make([]engine.Step, 0, len(outcome.Steps))
	reviewSteps := make([]tui.PlanReviewStep, 0, len(outcome.Steps))
	idxMap := make([]int, 0, len(outcome.Steps))
	blockedAgain := false

	for _, rs := range outcome.Steps {
		if rs.Skipped {
			continue
		}
		trueIdx := origIdx[rs.OriginalIndex]
		original := plan.Steps[trueIdx]

		decision := original.Decision
		if rs.Command != original.Command {
			decision = safetyEngine.Evaluate(rs.Command, original.Risk)
		}
		if decision.Action == safety.Block {
			blockedAgain = true
		}

		finalSteps = append(finalSteps, engine.Step{
			Command:    rs.Command,
			Rationale:  original.Rationale,
			Risk:       original.Risk,
			Reversible: original.Reversible,
			Decision:   decision,
		})
		reviewSteps = append(reviewSteps, tui.PlanReviewStep{
			Command:     rs.Command,
			Rationale:   original.Rationale,
			Risk:        decision.EffectiveRisk,
			Blocked:     decision.Action == safety.Block,
			BlockReason: decision.Reason,
		})
		idxMap = append(idxMap, trueIdx)
	}

	if blockedAgain {
		return engine.Plan{}, reviewSteps, idxMap, false
	}
	return engine.Plan{Summary: plan.Summary, Steps: finalSteps}, nil, nil, true
}
