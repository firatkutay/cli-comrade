package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// Choice mirrors internal/tui.PromptChoice's five ask-mode outcomes as
// this package's own type, so Runner (and the PromptUI interface below)
// never has to import internal/tui's interactive bubbletea machinery —
// only internal/cli's real PromptUI implementation needs to know about
// tui.PromptChoice/tui.PromptStep, converting between the two shapes at
// the one place a concrete UI toolkit is wired in. (This package does
// still use internal/tui's plain, non-interactive rendering helpers —
// RiskBadge/PrintStatus/PrintWarning/PrintExplanation — since those take
// only an io.Writer/bool/string and carry no engine-shaped coupling in
// either direction.)
type Choice int

const (
	// ChoiceYes ("e"vet) runs the step as shown.
	ChoiceYes Choice = iota
	// ChoiceNo ("h"ayır) skips this step.
	ChoiceNo
	// ChoiceEdit ("d"üzenle) means the user edited the command;
	// PromptUI.Confirm's editedCommand return value carries the new text.
	ChoiceEdit
	// ChoiceExplain ("a"çıkla) asks for a detailed explanation, then
	// re-prompts.
	ChoiceExplain
	// ChoiceAll ("t"ümü) approves this step and every remaining
	// read/write/network step without asking again.
	ChoiceAll
)

// PromptUI is the minimal interactive-confirmation capability ask mode
// (and auto mode's forced destructive/elevated confirms) needs — a
// package-local, consumer-side interface so tests can inject a scripted
// fake with no bubbletea program involved at all. internal/cli's real
// implementation wraps internal/tui.Confirm, converting an engine.Step to
// a tui.PromptStep and a tui.PromptChoice back to this package's Choice.
type PromptUI interface {
	// Confirm shows the confirm prompt for step and returns the user's
	// choice; editedCommand is only meaningful when choice == ChoiceEdit,
	// and is NOT yet re-evaluated by safety — Runner does that itself
	// (see resolveAskChoice) before ever acting on it.
	Confirm(ctx context.Context, step Step) (choice Choice, editedCommand string, err error)
	// Explain fetches a detailed, user-facing explanation for step (the
	// [a]çıkla option), typically by calling the LLM.
	Explain(ctx context.Context, step Step) (string, error)
}

// CommandExecutor is the minimal internal/executor capability Runner
// needs: a package-local, consumer-side interface so tests can inject a
// fake executor (recording calls, or simulating a hang for the Ctrl-C
// scenario) without depending on internal/executor.Executor's concrete
// type. *executor.Executor satisfies this directly.
type CommandExecutor interface {
	Run(ctx context.Context, command string, opts executor.Options) (executor.Result, error)
}

// AuditSink is the minimal internal/audit capability Runner needs to
// record one JSONL entry per executed step. *audit.Logger satisfies this
// directly. A nil AuditSink (RunDeps.Audit) disables audit logging
// entirely — e.g. when audit.enabled=false in config.
type AuditSink interface {
	Append(entry audit.Entry) error
}

// selfCorrectionMaxAttempts caps the number of self-correction round-trips
// Execute performs across an entire plan run (not per step) —
// UYGULAMA_PLANI.md FAZ 6 item 2's "en fazla 3 self-correction denemesi".
const selfCorrectionMaxAttempts = 3

// stderrTailLimit is the maximum number of trailing bytes of a failed
// step's captured stderr sent to the LLM for self-correction — keeps the
// correction request small and avoids re-sending secrets-scale output
// (the underlying capture is already tail-truncated to 8KB by
// internal/executor; this trims it further for the prompt itself).
const stderrTailLimit = 2000

// selfCorrectionSystemPrompt is the system prompt sent when a step fails
// and Runner asks the LLM for a corrected replacement command. It
// requests the exact same single-step JSON shape internal/engine's plan
// system prompt uses per step, so the response can be parsed with the
// same rawStep-shaped decoding this package already has.
const selfCorrectionSystemPrompt = `You are cli-comrade's self-correction brain. A previously generated shell command failed when actually executed. Given the original request context, the exact command that failed, and its captured stderr, produce ONE corrected replacement command that is more likely to succeed.

Respond with a single JSON object and nothing else — no markdown code fences, no prose before or after it — shaped exactly like this:

{
  "command": "<the corrected command to run instead>",
  "rationale": "<why this correction should fix the failure, one sentence>",
  "risk": "<one of: read, write, network, elevated, destructive>",
  "reversible": true
}

Label "risk" using the same five classes and the same conservative rule as plan generation. If you cannot determine a plausible correction, return the same command unchanged with an explanation in "rationale".`

// correctionResponse is the self-correction request's JSON response
// shape, mirroring rawStep in planner.go (kept as its own local type
// since the two prompts are independent, even though the shape matches).
type correctionResponse struct {
	Command    string `json:"command"`
	Rationale  string `json:"rationale"`
	Risk       string `json:"risk"`
	Reversible bool   `json:"reversible"`
}

// RunDeps bundles every dependency Execute needs beyond the Plan/Mode/ctx
// themselves. Every field is injected — no package-level state, per
// CLAUDE.md's dependency-injection rule — so tests construct a RunDeps
// with fakes for Executor/Prompt/LLM/Audit and real, tiny values for
// everything else.
type RunDeps struct {
	Executor CommandExecutor
	Safety   *safety.Engine
	LLM      Completer
	Prompt   PromptUI
	// Audit may be nil to disable audit logging entirely.
	Audit AuditSink

	Stdout io.Writer
	Stderr io.Writer

	ColorEnabled bool

	// ConfirmDestructive/ConfirmElevated mirror config safety.
	// confirm_destructive/confirm_elevated: when false AND Yolo is true,
	// auto mode bypasses the forced confirm for that risk class (printing
	// a red warning each time instead) — CLAUDE.md's one, explicit,
	// non-default escape hatch from the destructive/elevated
	// confirmation requirement.
	ConfirmDestructive bool
	ConfirmElevated    bool
	Yolo               bool

	// StepTimeout is passed to every executor.Options.Timeout — zero
	// means no timeout, matching internal/executor's own contract.
	StepTimeout time.Duration

	// Request is the free-text request that produced Plan, recorded on
	// every audit.Entry.
	Request string

	// Now is an injectable clock for audit timestamps; nil defaults to
	// time.Now.
	Now func() time.Time

	// Translator resolves every user-facing string Execute itself prints
	// (BLOCKED/--yolo-bypass lines, RunSummary.AbortReason text) in the
	// user's resolved language (internal/cli builds it once per
	// invocation via i18n.ResolveLanguage/i18n.NewTranslator — see
	// docs/phases/FAZ-09.md). A zero-value Translator (every RunDeps this
	// package's own tests construct as a plain struct literal, and every
	// pre-FAZ-9 caller) defaults to English via tr() below, so this field
	// is purely additive: no existing caller/test needs to set it, and
	// every English string this package ever printed stays byte-for-byte
	// identical.
	Translator i18n.Translator
}

func (d RunDeps) now() time.Time {
	if d.Now != nil {
		return d.Now()
	}
	return time.Now()
}

// tr returns d.Translator if the caller set one (a non-zero-value
// Translator always has Lang() == LangTR or was explicitly built via
// i18n.NewTranslator(LangEN), either way behaving identically to the
// default below for English output), or a fresh English Translator
// otherwise — see Translator field's doc comment.
func (d RunDeps) tr() i18n.Translator {
	if d.Translator.Lang() == i18n.LangTR {
		return d.Translator
	}
	return i18n.NewTranslator(i18n.LangEN)
}

// StepOutcome is one StepResult's final disposition.
type StepOutcome int

const (
	// OutcomeExecuted means the step actually ran (whether it exited 0
	// or not is in StepResult.ExitCode).
	OutcomeExecuted StepOutcome = iota
	// OutcomeSkipped means the step was never run: the user declined it
	// ([h]ayır), or a run was in progress elsewhere and got canceled.
	OutcomeSkipped
	// OutcomeBlocked means safety.Engine.Evaluate returned Block for
	// this step: it was never run, in any mode, unconditionally.
	OutcomeBlocked
)

func (o StepOutcome) String() string {
	switch o {
	case OutcomeExecuted:
		return "executed"
	case OutcomeSkipped:
		return "skipped"
	case OutcomeBlocked:
		return "blocked"
	default:
		return "unknown"
	}
}

// StepResult is one plan step's outcome after Execute processed it.
// Command is the FINAL command text actually acted on — the original
// plan step's command, unless self-correction (SelfCorrected) or an
// ask-mode edit replaced it.
type StepResult struct {
	Index         int
	Command       string
	Outcome       StepOutcome
	ExitCode      int
	Duration      time.Duration
	SelfCorrected bool
}

// RunSummary is Execute's result: every step's disposition, plus whether
// the run stopped early (Aborted) and why (AbortReason) — a Block, a
// step failure that exhausted the self-correction budget, or a
// cancellation (Ctrl-C).
type RunSummary struct {
	Results     []StepResult
	Aborted     bool
	AbortReason string
}

// Execute runs plan under mode, using deps for everything I/O-shaped
// (execution, prompting, LLM self-correction, audit logging). It never
// runs a Blocked step in any mode, including auto with --yolo — that
// invariant is enforced structurally, and checked at TWO points per
// step, not one: (1) at the top of each mode's loop, against the plan's
// original Decision, and (2) again against the RESOLVED step's Decision
// after ask mode's resolveAskChoice (and auto mode's equivalent
// resolveAutoGate) returns — because [d]üzenle can re-evaluate an edited
// command to Block mid-loop, re-prompt, and then have the user pick
// [e]vet/[t]ümü on that same, still-Blocked step; check (1) alone would
// never see that, since it only ever looked at the original,
// pre-edit Decision. executeStepWithSelfCorrection also refuses to run a
// Blocked step as a final, defense-in-depth guard, independent of both
// checks above.
func Execute(ctx context.Context, plan Plan, mode Mode, deps RunDeps) (RunSummary, error) {
	switch mode {
	case ModeInfo:
		return executeInfo(deps, plan), nil
	case ModeAsk:
		return executeAsk(ctx, plan, deps)
	case ModeAuto:
		return executeAuto(ctx, plan, deps)
	default:
		return RunSummary{}, fmt.Errorf("engine: Execute: unknown mode %v", mode)
	}
}

// executeInfo implements info mode: print every step, numbered, with its
// command, risk badge (or BLOCKED(reason) for a Blocked step), and
// rationale. Nothing is ever executed.
func executeInfo(deps RunDeps, plan Plan) RunSummary {
	fmt.Fprintln(deps.Stdout, plan.Summary) //nolint:errcheck // best-effort stdout write; a write failure here has no recovery action
	fmt.Fprintln(deps.Stdout)               //nolint:errcheck

	for i, step := range plan.Steps {
		if step.Decision.Action == safety.Block {
			fmt.Fprint(deps.Stdout, deps.tr().T(i18n.MsgBlockedStep, i+1, step.Decision.Reason, step.Command)) //nolint:errcheck
		} else {
			badge := tui.RiskBadge(step.Decision.EffectiveRisk, deps.ColorEnabled)
			fmt.Fprintf(deps.Stdout, "%d. %s %s\n", i+1, badge, step.Command) //nolint:errcheck
		}
		if step.Rationale != "" {
			fmt.Fprintf(deps.Stdout, "   %s\n", step.Rationale) //nolint:errcheck
		}
	}
	return RunSummary{}
}

// executeAsk implements ask mode: every non-Blocked step is confirmed
// individually via resolveAskChoice, except once the user has picked
// [t]ümü, after which every remaining read/write/network step runs
// without asking (destructive/elevated steps still prompt individually,
// per resolveAskChoice's own risk-independent behavior — the
// autoApproveRemaining short-circuit below simply never calls it for
// those).
func executeAsk(ctx context.Context, plan Plan, deps RunDeps) (RunSummary, error) {
	var summary RunSummary
	correctionsUsed := 0
	autoApproveRemaining := false

	i := 0
	for ; i < len(plan.Steps); i++ {
		step := plan.Steps[i]

		if ctx.Err() != nil {
			summary.Aborted = true
			summary.AbortReason = deps.tr().T(i18n.MsgAbortCanceled)
			break
		}

		if step.Decision.Action == safety.Block {
			printBlocked(deps, i, step)
			summary.Results = append(summary.Results, StepResult{Index: i, Command: step.Command, Outcome: OutcomeBlocked})
			continue
		}

		if !autoApproveRemaining || step.Decision.EffectiveRisk >= safety.RiskElevated {
			choice, resolved, ok, err := resolveAskChoice(ctx, deps, step)
			if err != nil {
				return summary, err
			}
			if !ok {
				summary.Aborted = true
				summary.AbortReason = deps.tr().T(i18n.MsgAbortCanceled)
				break
			}
			step = resolved
			// The resolved step's Decision must be re-checked here,
			// regardless of which choice the user ultimately picked
			// ([e]vet/[t]ümü included): resolveAskChoice's [d]üzenle case
			// re-evaluates safety on the edited command and re-loops on a
			// newly-Blocked edit, but a subsequent [e]vet/[t]ümü on that
			// same (still-Blocked) step must still never run it — the
			// top-of-loop Block check above only guards the PLAN's
			// original Decision, never one produced by an in-loop edit.
			// See docs/phases/FAZ-06.md's post-review hardening note.
			if step.Decision.Action == safety.Block {
				printBlocked(deps, i, step)
				summary.Results = append(summary.Results, StepResult{Index: i, Command: step.Command, Outcome: OutcomeBlocked})
				continue
			}
			if choice == ChoiceAll {
				autoApproveRemaining = true
			}
			if choice == ChoiceNo {
				summary.Results = append(summary.Results, StepResult{Index: i, Command: step.Command, Outcome: OutcomeSkipped})
				continue
			}
		}

		aborted := runAndRecord(ctx, deps, ModeAsk, i, step, &correctionsUsed, &summary)
		if aborted {
			break
		}
	}

	fillSkippedTail(&summary, plan, i)
	return summary, nil
}

// executeAuto implements auto mode: every non-Blocked, non-destructive/
// non-elevated step runs immediately with a one-line status; destructive/
// elevated steps drop to the same confirm loop ask mode uses (unless the
// config+--yolo bypass fires for that exact risk class, which prints a
// red warning and proceeds); a Blocked step aborts the whole remaining
// plan (auto-abort-on-block — see docs/phases/FAZ-06.md).
func executeAuto(ctx context.Context, plan Plan, deps RunDeps) (RunSummary, error) {
	var summary RunSummary
	correctionsUsed := 0

	i := 0
	for ; i < len(plan.Steps); i++ {
		step := plan.Steps[i]

		if ctx.Err() != nil {
			summary.Aborted = true
			summary.AbortReason = deps.tr().T(i18n.MsgAbortCanceled)
			break
		}

		if step.Decision.Action == safety.Block {
			printBlocked(deps, i, step)
			summary.Results = append(summary.Results, StepResult{Index: i, Command: step.Command, Outcome: OutcomeBlocked})
			summary.Aborted = true
			summary.AbortReason = deps.tr().T(i18n.MsgAbortStepBlocked, i+1, step.Decision.Reason)
			break
		}

		resolved, proceed, aborted, skipped, err := resolveAutoGate(ctx, deps, step)
		if err != nil {
			return summary, err
		}
		if aborted {
			summary.Aborted = true
			summary.AbortReason = deps.tr().T(i18n.MsgAbortCanceled)
			break
		}
		// Same re-check as executeAsk's, and for the same reason: an
		// edit made during resolveAutoGate's dropped-to confirm loop may
		// have been re-evaluated to Block, and must never run just
		// because the user picked [e]vet/[t]ümü afterward. Auto mode's
		// own abort-on-block design decision applies here identically to
		// the plan's original Block case above.
		if resolved.Decision.Action == safety.Block {
			printBlocked(deps, i, resolved)
			summary.Results = append(summary.Results, StepResult{Index: i, Command: resolved.Command, Outcome: OutcomeBlocked})
			summary.Aborted = true
			summary.AbortReason = deps.tr().T(i18n.MsgAbortStepBlocked, i+1, resolved.Decision.Reason)
			break
		}
		if skipped {
			summary.Results = append(summary.Results, StepResult{Index: i, Command: resolved.Command, Outcome: OutcomeSkipped})
			continue
		}
		if !proceed {
			continue
		}

		if runAborted := runAndRecord(ctx, deps, ModeAuto, i, resolved, &correctionsUsed, &summary); runAborted {
			break
		}
	}

	fillSkippedTail(&summary, plan, i)
	return summary, nil
}

// resolveAutoGate decides, for one non-Blocked auto-mode step, whether it
// runs immediately (printing its status line), must drop to an
// interactive confirm, or bypasses that confirm via the config+--yolo
// escape hatch (printing the mandatory red warning). A [t]ümü choice from
// the dropped-to confirm is treated exactly like [e]vet for this single
// step — auto mode has no "approve all remaining" state to set, since it
// already runs every read/write/network step unprompted by default.
func resolveAutoGate(ctx context.Context, deps RunDeps, step Step) (resolved Step, proceed, aborted, skipped bool, err error) {
	risk := step.Decision.EffectiveRisk

	if risk == safety.RiskDestructive && !deps.ConfirmDestructive && deps.Yolo {
		printYoloBypassWarning(deps, step)
		return step, true, false, false, nil
	}
	if risk == safety.RiskElevated && !deps.ConfirmElevated && deps.Yolo {
		printYoloBypassWarning(deps, step)
		return step, true, false, false, nil
	}

	if risk != safety.RiskDestructive && risk != safety.RiskElevated {
		printAutoStatus(deps, step)
		return step, true, false, false, nil
	}

	choice, confirmed, ok, cerr := resolveAskChoice(ctx, deps, step)
	if cerr != nil {
		return step, false, false, false, cerr
	}
	if !ok {
		return step, false, true, false, nil
	}
	if choice == ChoiceNo {
		return confirmed, false, false, true, nil
	}
	return confirmed, true, false, false, nil
}

// resolveAskChoice drives the interactive confirm loop for one step: it
// shows the prompt, and handles [a]çıkla (fetch+print an explanation,
// re-loop) and [d]üzenle (re-evaluate safety on the edited command,
// refuse+re-loop on Block, otherwise re-loop showing the confirm prompt
// again for the edited version — "confirm edited version") itself,
// returning only once the user picks [e]vet/[h]ayır/[t]ümü, or ctx is
// canceled (ok=false) — including mid-prompt, since
// internal/tui.Confirm's real implementation surfaces ctx cancellation as
// an error, which this function treats as a graceful abort (not a hard
// error) whenever ctx.Err() is actually set.
func resolveAskChoice(ctx context.Context, deps RunDeps, step Step) (choice Choice, resolved Step, ok bool, err error) {
	for {
		if ctx.Err() != nil {
			return 0, step, false, nil
		}

		c, edited, cerr := deps.Prompt.Confirm(ctx, step)
		if cerr != nil {
			if ctx.Err() != nil {
				return 0, step, false, nil
			}
			return 0, step, false, cerr
		}

		switch c {
		case ChoiceExplain:
			explanation, eerr := deps.Prompt.Explain(ctx, step)
			if eerr != nil {
				fmt.Fprintf(deps.Stderr, "explain failed: %v\n", eerr) //nolint:errcheck
			} else {
				tui.PrintExplanation(deps.Stdout, explanation) //nolint:errcheck
			}
			continue

		case ChoiceEdit:
			newDecision := deps.Safety.Evaluate(edited, step.Risk)
			step = Step{
				Command:    edited,
				Rationale:  step.Rationale,
				Risk:       step.Risk,
				Reversible: step.Reversible,
				Decision:   newDecision,
			}
			if newDecision.Action == safety.Block {
				printBlockedEdit(deps, step)
			}
			continue

		default:
			return c, step, true, nil
		}
	}
}

// runAndRecord runs step (already past every confirm/gate decision) via
// executeStepWithSelfCorrection, appends its StepResult to summary.Results,
// and reports whether the mode loop must abort (a genuine failure that
// exhausted the self-correction budget, or a cancellation).
func runAndRecord(ctx context.Context, deps RunDeps, mode Mode, index int, step Step, correctionsUsed *int, summary *RunSummary) (aborted bool) {
	result, finalCommand, corrected, runErr := executeStepWithSelfCorrection(ctx, deps, mode, step, correctionsUsed)
	if runErr != nil {
		summary.Aborted = true
		summary.AbortReason = fmt.Sprintf("step %d: %v", index+1, runErr)
		return true
	}

	summary.Results = append(summary.Results, StepResult{
		Index:         index,
		Command:       finalCommand,
		Outcome:       OutcomeExecuted,
		ExitCode:      result.ExitCode,
		Duration:      result.Duration,
		SelfCorrected: corrected,
	})

	if result.Canceled {
		summary.Aborted = true
		summary.AbortReason = deps.tr().T(i18n.MsgAbortCanceled)
		return true
	}

	if result.ExitCode != 0 {
		suggestion := deps.tr().T(i18n.MsgRetrySuggestion)
		if corrected {
			summary.AbortReason = deps.tr().T(i18n.MsgAbortStepFailedAfterCorrection,
				index+1, selfCorrectionMaxAttempts, result.ExitCode, finalCommand, suggestion)
		} else {
			summary.AbortReason = deps.tr().T(i18n.MsgAbortStepFailed,
				index+1, result.ExitCode, finalCommand, suggestion)
		}
		summary.Aborted = true
		return true
	}

	return false
}

// executeStepWithSelfCorrection runs command via deps.Executor once, then
// — while the attempt failed with a nonzero exit code (TimedOut counts as
// a failure; Canceled never does — a cancellation always stops
// immediately, never triggers self-correction) and the global
// selfCorrectionMaxAttempts budget is not exhausted — asks deps.LLM for a
// corrected replacement, re-evaluates it through deps.Safety, and retries
// with the revision unless that revision itself is Blocked. Every attempt
// that actually reaches deps.Executor.Run is audited individually, since
// each one really executed on the host.
func executeStepWithSelfCorrection(ctx context.Context, deps RunDeps, mode Mode, step Step, correctionsUsed *int) (result executor.Result, finalCommand string, corrected bool, err error) {
	// Belt-and-suspenders final guard: every caller (executeAsk,
	// executeAuto/resolveAutoGate) is responsible for never passing a
	// Blocked step here in the first place — see their own re-checks of
	// the resolved step's Decision after an ask-mode edit. This is not
	// the primary fix (a caller bug here would produce a confusing
	// aborted-run error instead of the proper BLOCKED message the
	// callers print), but it ensures this package can never actually
	// execute a Blocked command even if a future caller forgets to check.
	if step.Decision.Action == safety.Block {
		return executor.Result{}, step.Command, false, fmt.Errorf("refusing to execute a Blocked step: %s", step.Decision.Reason)
	}

	command := step.Command
	risk := step.Decision.EffectiveRisk

	for {
		res, runErr := deps.Executor.Run(ctx, command, executor.Options{Timeout: deps.StepTimeout})
		if runErr != nil {
			return res, command, corrected, fmt.Errorf("engine: run step: %w", runErr)
		}
		appendAudit(deps, mode, command, risk, res)

		failed := res.ExitCode != 0 && !res.Canceled
		if !failed || res.Canceled || ctx.Err() != nil {
			return res, command, corrected, nil
		}
		if *correctionsUsed >= selfCorrectionMaxAttempts {
			return res, command, corrected, nil
		}
		*correctionsUsed++

		revised, correctErr := requestCorrection(ctx, deps, step, command, res.Stderr)
		if correctErr != nil {
			// Give up self-correcting; report the last real failure.
			return res, command, corrected, nil
		}
		decision := deps.Safety.Evaluate(revised.Command, revised.Risk)
		if decision.Action == safety.Block {
			// The revision itself is unsafe; stop here rather than ever
			// running it, and report the last real (pre-revision) failure.
			return res, command, corrected, nil
		}

		command = revised.Command
		risk = decision.EffectiveRisk
		corrected = true
	}
}

// requestCorrection asks deps.LLM for a corrected replacement for
// failedCommand, given its tail-truncated stderr.
func requestCorrection(ctx context.Context, deps RunDeps, step Step, failedCommand, stderrOutput string) (Step, error) {
	tail := stderrOutput
	if len(tail) > stderrTailLimit {
		tail = tail[len(tail)-stderrTailLimit:]
	}

	user := fmt.Sprintf(
		"Original request context: %s\nOriginal step rationale: %s\nFailed command: %s\nStderr (tail):\n%s\n",
		deps.Request, step.Rationale, failedCommand, tail)

	resp, err := deps.LLM.Complete(ctx, llm.CompletionRequest{
		System:         selfCorrectionSystemPrompt,
		Messages:       []llm.Message{{Role: "user", Content: user}},
		MaxTokens:      512,
		RequiredFields: []string{"command"},
	})
	if err != nil {
		return Step{}, fmt.Errorf("engine: request self-correction: %w", err)
	}

	var raw correctionResponse
	if err := json.Unmarshal(resp.JSON, &raw); err != nil {
		return Step{}, fmt.Errorf("engine: decode self-correction response: %w", err)
	}
	if strings.TrimSpace(raw.Command) == "" {
		return Step{}, fmt.Errorf("engine: self-correction response had an empty command")
	}

	risk, parseErr := safety.ParseRiskClass(raw.Risk)
	if parseErr != nil {
		risk = safety.RiskDestructive
	}

	return Step{
		Command:    raw.Command,
		Rationale:  raw.Rationale,
		Risk:       risk,
		Reversible: raw.Reversible,
	}, nil
}

// appendAudit writes one audit.Entry for a single executor.Run attempt.
// A nil deps.Audit disables logging entirely; a write failure is reported
// to deps.Stderr but never aborts the run — a local audit-log write
// failure must never block the user's actual task.
func appendAudit(deps RunDeps, mode Mode, command string, risk safety.RiskClass, result executor.Result) {
	if deps.Audit == nil {
		return
	}
	entry := audit.Entry{
		Timestamp:  deps.now(),
		Request:    deps.Request,
		Command:    command,
		Risk:       risk.String(),
		Mode:       mode.String(),
		ExitCode:   result.ExitCode,
		DurationMs: result.Duration.Milliseconds(),
	}
	if err := deps.Audit.Append(entry); err != nil {
		fmt.Fprintf(deps.Stderr, "audit: failed to record step: %v\n", err) //nolint:errcheck
	}
}

// fillSkippedTail appends an OutcomeSkipped StepResult for every plan step
// after lastIndex that the mode loop never reached, once it has aborted
// early (Ctrl-C, a Block, or an unrecoverable failure) — so RunSummary
// always enumerates every step's fate, not just the ones actually
// processed.
func fillSkippedTail(summary *RunSummary, plan Plan, lastIndex int) {
	if !summary.Aborted {
		return
	}
	for j := lastIndex + 1; j < len(plan.Steps); j++ {
		summary.Results = append(summary.Results, StepResult{Index: j, Command: plan.Steps[j].Command, Outcome: OutcomeSkipped})
	}
}

func printBlocked(deps RunDeps, index int, step Step) {
	fmt.Fprint(deps.Stdout, deps.tr().T(i18n.MsgBlockedStep, index+1, step.Decision.Reason, step.Command)) //nolint:errcheck
}

func printBlockedEdit(deps RunDeps, step Step) {
	fmt.Fprint(deps.Stdout, deps.tr().T(i18n.MsgBlockedStepEdit, step.Decision.Reason, step.Command)) //nolint:errcheck
}

func printAutoStatus(deps RunDeps, step Step) {
	badge := tui.RiskBadge(step.Decision.EffectiveRisk, deps.ColorEnabled)
	tui.PrintStatus(deps.Stdout, fmt.Sprintf("-> running: %s %s", badge, step.Command), deps.ColorEnabled) //nolint:errcheck
}

func printYoloBypassWarning(deps RunDeps, step Step) {
	line := deps.tr().T(i18n.MsgYoloBypass, step.Decision.EffectiveRisk.String(), step.Command)
	tui.PrintWarning(deps.Stdout, line, deps.ColorEnabled) //nolint:errcheck
}
