package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// newExplainCmd builds "comrade explain <command...>" (UYGULAMA_PLANI.md
// FAZ 9 item 3): a two-layer command explanation that NEVER executes
// anything, ever — there is no executor.Executor anywhere in this
// command's dependency graph, unlike do/fix.
//
// Layer 1 (local, authoritative for the risk warning): the exact same
// safety.Engine every other command uses classifies the command text.
// A destructive or denylisted (Blocked) verdict prints a prominent
// warning FIRST, before anything the LLM said — see runExplain.
//
// Layer 2 (LLM, secondary): engine.Explainer asks the model for a plain-
// language summary, a flag-by-flag breakdown, and its own risk note,
// rendered after the safety layer's verdict.
func newExplainCmd(newLoader loaderFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "explain <command...>",
		Short: "Explain what a command does, flag by flag, without running it",
		Args:  cobra.MinimumNArgs(1),
		// The command being explained is itself arbitrary shell text and
		// routinely starts with a dash (`-rf`, `-la`, ...): without this,
		// cobra/pflag would try to parse those tokens as comrade's OWN
		// flags and reject them as "unknown shorthand flag". Exactly the
		// same fix config.go's "set" command applies for the same reason.
		DisableFlagParsing: true,
	}
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runExplain(cmd, newLoader, strings.Join(args, " "))
	}
	return cmd
}

// runExplain loads config (no --yolo handling: explain has no --yolo
// flag of its own — see loadConfigWithNotice/buildLLMClient's doc
// comments), evaluates command through safety.Engine, prints that verdict
// first when it warrants a warning, then asks engine.Explainer for its
// breakdown and renders it. mode/dry-run/audit/executor — everything
// runDo/runFix need to actually run a plan — do not exist here at all.
func runExplain(cmd *cobra.Command, newLoader loaderFactory, command string) error {
	cfg, tr, err := loadConfigWithNotice(cmd, newLoader)
	if err != nil {
		return fmt.Errorf("comrade explain: %w", err)
	}
	client, err := buildLLMClient(cmd, cfg)
	if err != nil {
		return fmt.Errorf("comrade explain: %w", err)
	}

	safetyEngine := safety.NewEngine(cfg)
	// RiskRead is the declared-risk floor here, exactly like `comrade
	// fix`'s captureByRunning (fix.go): there is no LLM-declared risk to
	// start from at all in this command — Evaluate's own
	// denylist/escalation rules are what actually determine the verdict.
	decision := safetyEngine.Evaluate(command, safety.RiskRead)

	if err := printExplainSafetyVerdict(cmd, cfg, tr, decision); err != nil {
		return fmt.Errorf("comrade explain: %w", err)
	}

	explainer := engine.NewExplainer(client, cfg)
	stopSpinner := startWaitSpinner(resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr()), cmd.ErrOrStderr(), tr)
	explanation, err := explainer.Explain(cmd.Context(), command)
	stopSpinner()
	if err != nil {
		return fmt.Errorf("comrade explain: %w", err)
	}

	return renderExplanation(cmd.OutOrStdout(), tr, explanation)
}

// printExplainSafetyVerdict prints the prominent, authoritative safety
// warning when decision warrants one — EffectiveRisk destructive, or
// Action Block (a denylisted command, which is always at least as
// severe) — and prints nothing at all for a benign command: `comrade
// explain "ls -la"` must not manufacture a warning that isn't there.
func printExplainSafetyVerdict(cmd *cobra.Command, cfg config.Config, tr i18n.Translator, decision safety.Decision) error {
	if decision.EffectiveRisk != safety.RiskDestructive && decision.Action != safety.Block {
		return nil
	}
	reason := ""
	if decision.Reason != "" {
		reason = ": " + decision.Reason
	}
	line := tr.T(i18n.MsgExplainSafetyWarning, decision.EffectiveRisk.String(), reason)
	return tui.PrintWarning(cmd.OutOrStdout(), line, resolveColorEnabled(cfg, os.Environ(), cmd.OutOrStdout()))
}

// renderExplanation prints explanation as: the LLM's summary (headed by
// MsgExplainSummaryHeading), the flag-by-flag breakdown (headed by
// MsgExplainPartsHeading, one "- token: meaning" line per part), and —
// only when non-empty — the LLM's own risk note (headed by
// MsgExplainRiskHeading), secondary to and printed after
// printExplainSafetyVerdict's authoritative warning.
func renderExplanation(w io.Writer, tr i18n.Translator, explanation engine.Explanation) error {
	if _, err := fmt.Fprintf(w, "%s\n%s\n\n", tr.T(i18n.MsgExplainSummaryHeading), explanation.Summary); err != nil {
		return err
	}

	if len(explanation.Parts) > 0 {
		if _, err := fmt.Fprintln(w, tr.T(i18n.MsgExplainPartsHeading)); err != nil {
			return err
		}
		for _, p := range explanation.Parts {
			if _, err := fmt.Fprintf(w, "  - %s: %s\n", p.Token, p.Meaning); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintln(w); err != nil {
			return err
		}
	}

	if strings.TrimSpace(explanation.RiskNote) != "" {
		if _, err := fmt.Fprintf(w, "%s\n%s\n", tr.T(i18n.MsgExplainRiskHeading), explanation.RiskNote); err != nil {
			return err
		}
	}
	return nil
}
