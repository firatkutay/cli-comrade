package cli

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// newDoCmd builds "comrade do <request...>": the product's real entry
// point (FAZ 5 built the safety-annotated plan; FAZ 6 wires it to the
// three-mode execution loop). It is no longer Hidden — see
// docs/history/phases/FAZ-06.md's note on why do_test.go's earlier
// "TestDoIsHiddenFromHelp" no longer applies. The root command's own
// free-text fallback (see root.go) calls the exact same runDo function
// this command's RunE calls, so `comrade do "docker kur" --auto` and
// `comrade docker kur --auto` behave identically.
func newDoCmd(newLoader loaderFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:               "do <request...>",
		Short:             "Generate a plan for a free-text request and run it per the active mode",
		Args:              translatedMinArgs(newLoader, 1, i18n.MsgDoUsageError),
		ValidArgsFunction: cobra.NoFileCompletions,
	}
	flags := addExecutionFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		return runDo(cmd, newLoader, strings.Join(args, " "), flags)
	}
	return cmd
}

// runDo is the full FAZ 6 pipeline: load config, generate a risk-labeled
// plan (FAZ 5), and — unless --dry-run short-circuits straight to
// renderPlan — resolve the active mode and run it through
// internal/engine.Execute, wiring the real executor, the real
// tui-backed PromptUI, and (when audit.enabled) the real audit.Logger.
func runDo(cmd *cobra.Command, newLoader loaderFactory, request string, flags *executionFlags) error {
	modeFlag, err := flags.modeFlagValue()
	if err != nil {
		return err
	}

	tally := newUsageTally()
	cfg, client, err := setupCLIRuntime(cmd, newLoader, flags, tally.record)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}
	tr := newTranslator(cfg)

	// The tally is always attached above (zero-cost when display is
	// off); this defer is what actually PRINTS it, and only when the
	// run's own show_usage/--usage resolution says to. Deferred (rather
	// than a single print before runDo's final `return nil`) so the
	// summary still appears on every other return path below that still
	// reached the LLM at least once — --dry-run's early return, a mode-
	// resolution error, an engine.Execute error — printUsageSummary
	// itself is a no-op when tally recorded zero requests, so this is
	// safe even for the paths above this point that fail before ever
	// calling GeneratePlan.
	if cfg.General.ShowUsage || flags.usage {
		defer func() {
			// Best-effort: a stderr write failure here must never mask
			// runDo's own real result.
			_ = printUsageSummary(cmd.ErrOrStderr(), tr, tally, resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr()))
		}()
	}

	collector := contextpkg.NewCollector()
	sysCtx := collector.Collect(cmd.Context(), contextpkg.Options{
		SendHistory:  cfg.Context.SendHistory,
		HistoryDepth: cfg.Context.HistoryDepth,
		SendEnvNames: cfg.Context.SendEnvNames,
	})

	planner := engine.NewPlanner(client, cfg)
	stopSpinner := startWaitSpinner(resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr()), cmd.ErrOrStderr(), tr)
	plan, err := planner.GeneratePlan(cmd.Context(), request, sysCtx)
	stopSpinner()
	if err != nil {
		return translateLLMError(cmd.ErrOrStderr(), "comrade do", tr, err)
	}

	if flags.dryRun {
		return renderPlan(cmd.OutOrStdout(), plan, tr)
	}

	mode, err := engine.ResolveMode(modeFlag, os.Getenv("COMRADE_MODE"), cfg.General.Mode)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	auditSink, err := buildAuditSink(cmd, cfg, tr)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	// Ctrl-C: canceling ctx propagates into engine.Execute, which the
	// currently-running executor.Run call observes and kills its process
	// group for, per internal/executor's own contract.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	colorEnabled := resolveColorEnabled(cfg, os.Environ(), cmd.OutOrStdout())
	deps := engine.RunDeps{
		Executor:           executor.New(cmd.OutOrStdout(), cmd.ErrOrStderr()),
		Safety:             safety.NewEngine(cfg),
		LLM:                client,
		Prompt:             &tuiPromptUI{in: cmd.InOrStdin(), out: cmd.OutOrStdout(), colorEnabled: colorEnabled, llm: client, tr: tr},
		Audit:              auditSink,
		Stdout:             cmd.OutOrStdout(),
		Stderr:             cmd.ErrOrStderr(),
		ColorEnabled:       colorEnabled,
		ConfirmDestructive: cfg.Safety.ConfirmDestructive,
		ConfirmElevated:    cfg.Safety.ConfirmElevated,
		Yolo:               flags.yolo,
		StepTimeout:        time.Duration(cfg.Executor.StepTimeoutSeconds) * time.Second,
		Request:            request,
		RunID:              newRunID(),
		WorkingDir:         sysCtx.WorkingDir,
		Translator:         tr,
	}

	summary, err := engine.Execute(ctx, plan, mode, deps)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	if mode != engine.ModeInfo {
		if err := printRunSummary(cmd.OutOrStdout(), summary, tr); err != nil {
			return err
		}
	}

	if summary.Aborted {
		return fmt.Errorf("comrade do: %s", summary.AbortReason)
	}
	return nil
}

// buildAuditSink builds the real *audit.Logger for this invocation,
// applying its lazy retention cleanup once, or returns a nil
// engine.AuditSink when audit.enabled=false. A retention-cleanup failure
// is reported to stderr but never aborts the run — see
// internal/audit.Logger.ApplyRetention's own doc comment for why this is
// safe to treat as non-fatal.
func buildAuditSink(cmd *cobra.Command, cfg config.Config, tr i18n.Translator) (engine.AuditSink, error) {
	if !cfg.Audit.Enabled {
		return nil, nil
	}

	path, err := audit.DefaultPath()
	if err != nil {
		return nil, err
	}
	logger, err := audit.NewLogger(path)
	if err != nil {
		return nil, err
	}
	if err := logger.ApplyRetention(cfg.Audit.RetentionDays, time.Now()); err != nil {
		if _, ferr := fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgAuditRetentionFailed, err)); ferr != nil {
			return nil, ferr
		}
	}
	return logger, nil
}

// renderPlan prints plan.Summary followed by a tabwriter-aligned
// STEP/COMMAND/RISK/REVERSIBLE/RATIONALE table, per docs/history/UYGULAMA_PLANI.md FAZ
// 5 item 4. The RISK column always renders internal/safety's independent
// EffectiveRisk, never the LLM's raw step.Risk label — that is the whole
// point of this table: to surface the second check, not to redisplay
// what the model claimed. A Blocked step renders "BLOCKED(<reason>)"; a
// step the safety engine escalated to Confirm renders
// "CONFIRM(<effective risk>)" so a risk bump is visible even when it
// isn't severe enough to Block; a plain Allow renders just the risk name.
func renderPlan(w io.Writer, plan engine.Plan, tr i18n.Translator) error {
	if _, err := fmt.Fprintln(w, plan.Summary); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgPlanTableHeader)); err != nil {
		return err
	}
	for i, step := range plan.Steps {
		risk := step.Decision.EffectiveRisk.String()
		switch step.Decision.Action {
		case safety.Block:
			risk = tr.T(i18n.MsgPlanBlockedCell, step.Decision.Reason)
		case safety.Confirm:
			risk = tr.T(i18n.MsgPlanConfirmCell, step.Decision.EffectiveRisk.String())
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%t\t%s\n", i+1, step.Command, risk, step.Reversible, step.Rationale); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// printRunSummary prints the final "N executed, M skipped, K blocked"
// line (plus, on an aborted run, the reason) after ask/auto mode
// finishes — docs/history/UYGULAMA_PLANI.md FAZ 6's "abort remaining ... özet bas"
// requirement. info mode never calls this (it never produces a
// RunSummary worth summarizing — see runDo).
func printRunSummary(w io.Writer, summary engine.RunSummary, tr i18n.Translator) error {
	var executed, skipped, blocked int
	for _, r := range summary.Results {
		switch r.Outcome {
		case engine.OutcomeExecuted:
			executed++
		case engine.OutcomeSkipped:
			skipped++
		case engine.OutcomeBlocked:
			blocked++
		}
	}
	if _, err := fmt.Fprintf(w, "\n%s\n", tr.T(i18n.MsgRunSummaryCounts, executed, skipped, blocked)); err != nil {
		return err
	}
	if summary.Aborted {
		_, err := fmt.Fprintf(w, "%s\n", tr.T(i18n.MsgRunSummaryAbortedLine, summary.AbortReason))
		return err
	}
	return nil
}
