package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// OfferVerification implements docs/history/UYGULAMA_PLANI.md FAZ 7 item 4's
// post-solution verification: once `comrade fix`'s plan has finished
// (successfully, in ask/auto mode — see internal/cli's runFix, the only
// caller) it offers to re-run originalCommand — the command that
// originally failed and prompted the diagnosis — to confirm the fix
// actually worked.
//
// originalCommand is evaluated through deps.Safety with RiskRead as the
// floor declared risk (mirroring toPlan/GeneratePlan's own "declared risk
// is only ever a floor an escalation rule may raise" contract): a floor
// of RiskRead never hides real destructiveness, since Engine.Evaluate's
// denylist and escalation rules independently re-derive the effective
// risk straight from the command text, regardless of what was declared.
// Verification is skipped entirely — never offered, in any mode — when
// that independent verdict is Block or RiskDestructive, exactly as FAZ 7
// item 4 requires ("orijinal komutu (destructive değilse) tekrar deneme
// öner"). An empty originalCommand (paste mode never captured one, or the
// fix flow never had one to begin with) also skips silently.
func OfferVerification(ctx context.Context, deps RunDeps, mode Mode, originalCommand string) error {
	command := strings.TrimSpace(originalCommand)
	if command == "" {
		return nil
	}

	decision := deps.Safety.Evaluate(command, safety.RiskRead)
	if decision.Action == safety.Block || decision.EffectiveRisk == safety.RiskDestructive {
		return nil
	}

	switch mode {
	case ModeInfo:
		printVerificationSuggestion(deps, command)
		return nil
	case ModeAsk:
		return offerVerificationInteractive(ctx, deps, mode, command, decision)
	case ModeAuto:
		// Auto mode still must never bypass CLAUDE.md's non-negotiable
		// destructive/elevated confirmation requirement — destructive is
		// already excluded above, so only "elevated" needs the same
		// confirm-drop resolveAutoGate uses for a real plan step; every
		// other risk class runs the verification directly.
		if decision.EffectiveRisk == safety.RiskElevated {
			return offerVerificationInteractive(ctx, deps, mode, command, decision)
		}
		return runVerification(ctx, deps, mode, command, decision.EffectiveRisk)
	default:
		return nil
	}
}

// printVerificationSuggestion is info mode's (and every other mode's
// never-executing) rendering of the offered verification command — a
// single copyable line, matching executeInfo's own numbered-step
// rendering style.
func printVerificationSuggestion(deps RunDeps, command string) {
	fmt.Fprint(deps.Stdout, deps.tr().T(i18n.MsgVerificationSuggestion, command)) //nolint:errcheck // best-effort stdout write; see executeInfo's identical rationale.
}

// offerVerificationInteractive drives the interactive confirm loop for
// the verification step — reusing resolveAskChoice verbatim (the same
// [e]vet/[h]ayır/[d]üzenle/[a]çıkla/[t]ümü loop a real plan step gets),
// so [d]üzenle re-evaluates safety on an edited command and refuses a
// newly-Blocked edit exactly like a real plan step would.
func offerVerificationInteractive(ctx context.Context, deps RunDeps, mode Mode, command string, decision safety.Decision) error {
	step := Step{
		Command:   command,
		Rationale: "confirm the originally failing command now succeeds",
		Decision:  decision,
	}

	choice, resolved, ok, err := resolveAskChoice(ctx, deps, step)
	if err != nil {
		return fmt.Errorf("engine: offer verification: %w", err)
	}
	if !ok || choice == ChoiceNo {
		return nil
	}
	if resolved.Decision.Action == safety.Block {
		printBlockedEdit(deps, resolved)
		return nil
	}

	return runVerification(ctx, deps, mode, resolved.Command, resolved.Decision.EffectiveRisk)
}

// runVerification actually re-runs command via deps.Executor, reporting
// success/failure with a one-line status and recording the attempt via
// appendAudit — CLAUDE.md security rule #4 ("Audit log her yürütülen
// komutu kaydeder") applies to this re-run exactly as it does to every
// other command engine.Execute itself runs, so it is never silently
// exempted just because it happens after the main run finished.
func runVerification(ctx context.Context, deps RunDeps, mode Mode, command string, risk safety.RiskClass) error {
	res, err := deps.Executor.Run(ctx, command, executor.Options{Timeout: deps.StepTimeout})
	if err != nil {
		return fmt.Errorf("engine: run verification: %w", err)
	}
	appendAudit(deps, mode, command, risk, res)

	if res.ExitCode == 0 && !res.Canceled && !res.TimedOut {
		tui.PrintStatus(deps.Stdout, deps.tr().T(i18n.MsgVerificationSucceeded, command), deps.ColorEnabled) //nolint:errcheck,gosec // stdout print failure is never actionable here (G104: unhandled error)
	} else {
		tui.PrintWarning(deps.Stdout, deps.tr().T(i18n.MsgVerificationStillFails, command, res.ExitCode), deps.ColorEnabled) //nolint:errcheck,gosec // stdout print failure is never actionable here (G104: unhandled error)
	}
	return nil
}
