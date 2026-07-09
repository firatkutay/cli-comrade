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
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// newDoCmd builds "comrade do <request...>": the product's real entry
// point (FAZ 5 built the safety-annotated plan; FAZ 6 wires it to the
// three-mode execution loop). It is no longer Hidden — see
// docs/phases/FAZ-06.md's note on why do_test.go's earlier
// "TestDoIsHiddenFromHelp" no longer applies. The root command's own
// free-text fallback (see root.go) calls the exact same runDo function
// this command's RunE calls, so `comrade do "docker kur" --auto` and
// `comrade docker kur --auto` behave identically.
func newDoCmd(newLoader loaderFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "do <request...>",
		Short: "Generate a plan for a free-text request and run it per the active mode",
		Args:  cobra.MinimumNArgs(1),
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

	loader, err := newLoader()
	if err != nil {
		return err
	}
	cfg, created, err := loader.Load()
	if err != nil {
		return err
	}
	if created {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), firstRunNoticeFormat, loader.Path()); err != nil {
			return err
		}
	}

	// CLAUDE.md security rule #6: --yolo prints a red warning on every
	// use, regardless of whether the config-side bypass conditions
	// (safety.confirm_destructive/confirm_elevated=false) actually let it
	// do anything this particular run.
	if flags.yolo {
		if err := tui.PrintWarning(cmd.ErrOrStderr(),
			"--yolo is set: destructive/elevated steps may run WITHOUT confirmation in auto mode, if safety.confirm_destructive/confirm_elevated is also disabled in config.",
			cfg.General.Color); err != nil {
			return err
		}
	}

	client, err := llm.New(*cfg)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	collector := contextpkg.NewCollector()
	sysCtx := collector.Collect(cmd.Context(), contextpkg.Options{
		SendHistory:  cfg.Context.SendHistory,
		HistoryDepth: cfg.Context.HistoryDepth,
		SendEnvNames: cfg.Context.SendEnvNames,
	})

	planner := engine.NewPlanner(client, *cfg)
	plan, err := planner.GeneratePlan(cmd.Context(), request, sysCtx)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	if flags.dryRun {
		return renderPlan(cmd.OutOrStdout(), plan)
	}

	mode, err := engine.ResolveMode(modeFlag, os.Getenv("COMRADE_MODE"), cfg.General.Mode)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	auditSink, err := buildAuditSink(cmd, *cfg)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	// Ctrl-C: canceling ctx propagates into engine.Execute, which the
	// currently-running executor.Run call observes and kills its process
	// group for, per internal/executor's own contract.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	deps := engine.RunDeps{
		Executor:           executor.New(cmd.OutOrStdout(), cmd.ErrOrStderr()),
		Safety:             safety.NewEngine(*cfg),
		LLM:                client,
		Prompt:             &tuiPromptUI{in: cmd.InOrStdin(), out: cmd.OutOrStdout(), colorEnabled: cfg.General.Color, llm: client},
		Audit:              auditSink,
		Stdout:             cmd.OutOrStdout(),
		Stderr:             cmd.ErrOrStderr(),
		ColorEnabled:       cfg.General.Color,
		ConfirmDestructive: cfg.Safety.ConfirmDestructive,
		ConfirmElevated:    cfg.Safety.ConfirmElevated,
		Yolo:               flags.yolo,
		StepTimeout:        time.Duration(cfg.Executor.StepTimeoutSeconds) * time.Second,
		Request:            request,
	}

	summary, err := engine.Execute(ctx, plan, mode, deps)
	if err != nil {
		return fmt.Errorf("comrade do: %w", err)
	}

	if mode != engine.ModeInfo {
		if err := printRunSummary(cmd.OutOrStdout(), summary); err != nil {
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
func buildAuditSink(cmd *cobra.Command, cfg config.Config) (engine.AuditSink, error) {
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
		if _, ferr := fmt.Fprintf(cmd.ErrOrStderr(), "audit: retention cleanup failed: %v\n", err); ferr != nil {
			return nil, ferr
		}
	}
	return logger, nil
}

// renderPlan prints plan.Summary followed by a tabwriter-aligned
// STEP/COMMAND/RISK/REVERSIBLE/RATIONALE table, per UYGULAMA_PLANI.md FAZ
// 5 item 4. The RISK column always renders internal/safety's independent
// EffectiveRisk, never the LLM's raw step.Risk label — that is the whole
// point of this table: to surface the second check, not to redisplay
// what the model claimed. A Blocked step renders "BLOCKED(<reason>)"; a
// step the safety engine escalated to Confirm renders
// "CONFIRM(<effective risk>)" so a risk bump is visible even when it
// isn't severe enough to Block; a plain Allow renders just the risk name.
func renderPlan(w io.Writer, plan engine.Plan) error {
	if _, err := fmt.Fprintln(w, plan.Summary); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w); err != nil {
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, "STEP\tCOMMAND\tRISK\tREVERSIBLE\tRATIONALE"); err != nil {
		return err
	}
	for i, step := range plan.Steps {
		risk := step.Decision.EffectiveRisk.String()
		switch step.Decision.Action {
		case safety.Block:
			risk = fmt.Sprintf("BLOCKED(%s)", step.Decision.Reason)
		case safety.Confirm:
			risk = fmt.Sprintf("CONFIRM(%s)", step.Decision.EffectiveRisk.String())
		}
		if _, err := fmt.Fprintf(tw, "%d\t%s\t%s\t%t\t%s\n", i+1, step.Command, risk, step.Reversible, step.Rationale); err != nil {
			return err
		}
	}
	return tw.Flush()
}

// printRunSummary prints the final "N executed, M skipped, K blocked"
// line (plus, on an aborted run, the reason) after ask/auto mode
// finishes — UYGULAMA_PLANI.md FAZ 6's "abort remaining ... özet bas"
// requirement. info mode never calls this (it never produces a
// RunSummary worth summarizing — see runDo).
func printRunSummary(w io.Writer, summary engine.RunSummary) error {
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
	if _, err := fmt.Fprintf(w, "\n%d executed, %d skipped, %d blocked\n", executed, skipped, blocked); err != nil {
		return err
	}
	if summary.Aborted {
		_, err := fmt.Fprintf(w, "aborted: %s\n", summary.AbortReason)
		return err
	}
	return nil
}
