package cli

import (
	"context"
	"errors"
	"fmt"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
	undopkg "github.com/firatkutay/cli-comrade/internal/undo"
)

// errUndoNothingReversible is buildUndoPlan's sentinel for "every step in
// the target run either failed (never took effect) or otherwise has
// nothing left to reverse" — runUndo renders this as
// i18n.MsgUndoNothingReversibleError rather than ever calling the LLM
// tier for a run with nothing eligible left in it at all.
var errUndoNothingReversible = errors.New("cli: undo: nothing in this run is reversible")

// stepDerivation is one recorded step's outcome, in the target run's own
// original (oldest-first) order, after checking it against
// internal/undo's heuristic table and the cwd-mismatch safety rule.
type stepDerivation struct {
	entry audit.Entry
	// skipped is true when entry.ExitCode != 0 — it never took effect, so
	// there is nothing to reverse.
	skipped bool
	// commands is internal/undo.Derive's output for entry, non-empty only
	// when the heuristic table matched AND the result was not downgraded
	// (see downgraded below).
	commands []string
	// caveat is internal/undo.Derived.Caveat verbatim (e.g. Windows'
	// Remove-Item only actually deleting an EMPTY directory) — carried
	// alongside commands so buildUndoPlan can surface it to the user in
	// the derived step's own Rationale, rather than silently dropping a
	// qualification the heuristic table itself computed. Empty whenever
	// commands is empty, or the matched rule has no caveat of its own.
	caveat string
	// downgraded is true when the heuristic DID match but must not be
	// trusted blindly: its derived command uses a relative path, and
	// entry's own recorded working directory differs from the directory
	// this undo run is actually executing in (internal/undo.Derived.
	// UsesRelativePath's own doc comment explains why this is never
	// silently rewritten).
	downgraded      bool
	downgradeReason string
}

// deriveStepsForRun classifies every step in run.Steps (original,
// oldest-first order preserved) against internal/undo's heuristic table,
// given the operating system the recorded commands were written for
// (goos) and the directory this undo invocation is actually running in
// (currentCwd).
func deriveStepsForRun(run undoRun, goos, currentCwd string) []stepDerivation {
	out := make([]stepDerivation, len(run.Steps))
	for i, e := range run.Steps {
		d := stepDerivation{entry: e}
		switch {
		case e.ExitCode != 0:
			d.skipped = true
		default:
			derived, ok := undopkg.Derive(undopkg.Recorded{
				Command:  e.Command,
				Cwd:      e.Cwd,
				GOOS:     goos,
				ExitCode: e.ExitCode,
			})
			switch {
			case !ok:
				// Unrecognized shape — falls through to the LLM tier.
			case derived.UsesRelativePath && e.Cwd != "" && currentCwd != "" && e.Cwd != currentCwd:
				d.downgraded = true
				d.downgradeReason = fmt.Sprintf("%s (recorded cwd) vs %s (current cwd)", e.Cwd, currentCwd)
			default:
				d.commands = derived.Commands
				d.caveat = derived.Caveat
			}
		}
		out[i] = d
	}
	return out
}

// buildUndoPlan derives the full undo Plan for run, per internal/cli's
// own documented, deliberately all-or-nothing-per-run design: if EVERY
// eligible (non-skipped) step in the run resolves via internal/undo's
// deterministic heuristic table, the returned Plan is built directly from
// those derived commands — no LLM call is ever made, and nothing about
// the run leaves this machine. If even ONE eligible step could not be
// resolved that way (no heuristic match, or downgraded per the
// cwd-mismatch rule), the WHOLE run is instead handed to undoer.
// DeriveUndo, which asks the model to reason about every eligible step
// together.
//
// This is a deliberate simplification over a per-step heuristic/LLM
// splice: interleaving a partial heuristic-derived plan with a partial
// LLM-derived one (matching each back to its correct position in
// REVERSE order, when the LLM's own response may consolidate, reorder,
// or decline individual steps) adds real complexity for comparatively
// little benefit — the LLM tier can, and typically does, correctly
// reproduce the same obvious reversals the heuristics would have chosen
// for the steps they DID recognize, while additionally reasoning about
// the ones they didn't. See the task's own doc comment on this file for
// the resolved ambiguity.
//
// Returns errUndoNothingReversible when every step in the run is skipped
// (nonzero exit code) — there is genuinely nothing to ask the LLM about.
// notes are user-facing, already-translated lines the caller should
// print alongside the returned Plan (nolint:onestep-per-line, one per
// skipped/downgraded step, plus the LLM-fallback notice when it fires).
func buildUndoPlan(ctx context.Context, undoer *engine.Undoer, safetyEngine *safety.Engine, run undoRun, goos, currentCwd string, tr i18n.Translator) (engine.Plan, []string, error) {
	steps := deriveStepsForRun(run, goos, currentCwd)

	var eligible []audit.Entry
	needsLLM := false
	var notes []string
	for i, d := range steps {
		switch {
		case d.skipped:
			notes = append(notes, tr.T(i18n.MsgUndoStepSkippedNote, i+1, d.entry.Command))
		case d.downgraded:
			needsLLM = true
			eligible = append(eligible, d.entry)
			notes = append(notes, tr.T(i18n.MsgUndoStepDowngradedNote, d.entry.Command, d.entry.Cwd, currentCwd))
		case len(d.commands) == 0:
			needsLLM = true
			eligible = append(eligible, d.entry)
		default:
			eligible = append(eligible, d.entry)
		}
	}

	if len(eligible) == 0 {
		return engine.Plan{}, notes, errUndoNothingReversible
	}

	if needsLLM {
		notes = append(notes, tr.T(i18n.MsgUndoLLMFallbackNote))
		plan, err := undoer.DeriveUndo(ctx, engine.UndoTarget{
			RunID:   run.RunID,
			Request: run.Request,
			Cwd:     currentCwd,
			Steps:   eligible,
		})
		return plan, notes, err
	}

	plan := engine.Plan{Summary: tr.T(i18n.MsgUndoPlanSummary, countCommands(steps), run.RunID)}
	for i := len(steps) - 1; i >= 0; i-- {
		d := steps[i]
		if d.skipped || len(d.commands) == 0 {
			continue
		}
		rationale := tr.T(i18n.MsgUndoHeuristicRationale, d.entry.Command)
		if d.caveat != "" {
			// d.caveat is internal/undo.Derived.Caveat: a plain, English,
			// deliberately untranslated qualification (that field's own
			// doc comment — the same convention doctor.Result.Fix already
			// established for a shell-adjacent technical note that is not
			// prose to route through i18n), appended to the translated
			// rationale so the user sees it both in `--dry-run`'s table
			// and in the ask-mode confirm prompt (both render Step.
			// Rationale) before ever confirming a heuristic-derived step.
			rationale += " (" + d.caveat + ")"
		}
		for _, cmd := range d.commands {
			plan.Steps = append(plan.Steps, engine.Step{
				Command:   cmd,
				Rationale: rationale,
				Risk:      safety.RiskWrite,
			})
		}
	}
	for i := range plan.Steps {
		plan.Steps[i].Decision = safetyEngine.Evaluate(plan.Steps[i].Command, plan.Steps[i].Risk)
	}
	return plan, notes, nil
}

// countCommands totals every derived command across every non-skipped
// step — used only for the purely-heuristic Plan's own Summary line
// (buildUndoPlan), so it reads "reverses N steps" using the actual
// number of commands that will run, not just the number of original
// steps they came from (a heuristic rule may one day produce more than
// one command per step — see internal/undo.Derived.Commands' own doc
// comment).
func countCommands(steps []stepDerivation) int {
	n := 0
	for _, d := range steps {
		n += len(d.commands)
	}
	return n
}
