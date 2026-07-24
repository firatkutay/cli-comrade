package cli

import (
	"bufio"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/spf13/cobra"

	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// lastCommandFreshness is FAZ 4/7's 10-minute freshness window: a
// last_command.json record older than this is never silently used as
// `comrade fix`'s error context (see acquireErrorContext) — the record
// most likely belongs to a command the user has long since moved past,
// so re-diagnosing it would surprise them.
const lastCommandFreshness = 10 * time.Minute

// newFixCmd builds "comrade fix": docs/history/UYGULAMA_PLANI.md FAZ 7's main
// use-case command. It gathers the failing command's context via
// acquireErrorContext's fallback chain (fresh last_command.json →
// `--rerun` / `-- <command>` → interactive paste mode), diagnoses it with
// engine.Diagnoser, and — exactly like `comrade do` (see do.go's
// newDoCmd/runDo) — resolves the active mode and hands the resulting
// Plan to engine.Execute, reusing the identical execution/safety/audit
// machinery; `fix` never reimplements any of it.
func newFixCmd(newLoader loaderFactory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "fix [-- command...]",
		Short: "Diagnose the last failed command (or a given one) and fix it",
		Args:  cobra.ArbitraryArgs,
		// The optional "-- command..." text is arbitrary shell input, not
		// a value from any known candidate set (see explain.go's own
		// ValidArgsFunction doc comment for why this is set even without
		// DisableFlagParsing).
		ValidArgsFunction: cobra.NoFileCompletions,
	}
	flags := addExecutionFlags(cmd)
	rerun := cmd.Flags().Bool("rerun", false, enUsageDefault(i18n.MsgFlagRerun))
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		var explicitCommand string
		if dashAt := cmd.ArgsLenAtDash(); dashAt >= 0 {
			explicitCommand = strings.Join(args[dashAt:], " ")
		}
		return runFix(cmd, newLoader, flags, *rerun, explicitCommand)
	}
	return cmd
}

// runFix is FAZ 7's full pipeline: load config/build the LLM client
// (setupCLIRuntime, shared with runDo), collect system context, acquire
// the failing command's ErrorContext via acquireErrorContext, diagnose it
// (engine.Diagnoser), print the root cause + explanation, and then —
// unless --dry-run short-circuits straight to renderPlan — resolve the
// active mode and run the diagnosis's Plan through engine.Execute exactly
// as runDo does, followed by engine.OfferVerification's post-solution
// verification offer (FAZ 7 item 4).
func runFix(cmd *cobra.Command, newLoader loaderFactory, flags *executionFlags, rerun bool, explicitCommand string) error {
	modeFlag, err := flags.modeFlagValue()
	if err != nil {
		return err
	}

	tally := newUsageTally()
	cfg, client, err := setupCLIRuntime(cmd, newLoader, flags, tally.record)
	if err != nil {
		return fmt.Errorf("comrade fix: %w", err)
	}
	tr := newTranslator(cfg)

	// See runDo's identical defer for why this is deferred rather than
	// printed once before a single final return: it must still fire on
	// every other return path below that reached the LLM at least once.
	if cfg.General.ShowUsage || flags.usage {
		defer func() {
			// Best-effort: a stderr write failure here must never mask
			// runFix's own real result.
			_ = printUsageSummary(cmd.ErrOrStderr(), tr, tally, resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr()))
		}()
	}

	collector := contextpkg.NewCollector()
	sysCtx := collector.Collect(cmd.Context(), contextpkg.Options{
		SendHistory:  cfg.Context.SendHistory,
		HistoryDepth: cfg.Context.HistoryDepth,
		SendEnvNames: cfg.Context.SendEnvNames,
	})

	safetyEngine := safety.NewEngine(cfg)
	ex := executor.New(cmd.OutOrStdout(), cmd.ErrOrStderr())

	errCtx, err := acquireErrorContext(cmd, sysCtx, safetyEngine, ex, rerun, explicitCommand, tr)
	if err != nil {
		return fmt.Errorf("comrade fix: %w", err)
	}

	diagnoser := engine.NewDiagnoser(client, cfg)
	stopSpinner := startWaitSpinner(resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr()), cmd.ErrOrStderr(), tr)
	diagnosis, err := diagnoser.Diagnose(cmd.Context(), errCtx)
	stopSpinner()
	if err != nil {
		return translateLLMError(cmd.ErrOrStderr(), "comrade fix", tr, err)
	}

	// The explanation is printed first, in every mode, per docs/history/UYGULAMA_PLANI.md
	// FAZ 7 item 1c — so the user understands what actually went wrong
	// before either reading the plan (info) or being asked to approve it
	// (ask/auto). Headings match `comrade explain`'s own Summary:/Risk
	// note: style (docs/history/phases/FAZ-09.md).
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n\n", tr.T(i18n.MsgFixRootCauseHeading), diagnosis.RootCause); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(cmd.OutOrStdout(), "%s\n%s\n", tr.T(i18n.MsgFixExplanationHeading), diagnosis.Explanation); err != nil {
		return err
	}

	if flags.dryRun {
		if _, err := fmt.Fprintln(cmd.OutOrStdout()); err != nil {
			return err
		}
		return renderPlan(cmd.OutOrStdout(), diagnosis.Plan, tr)
	}

	mode, err := engine.ResolveMode(modeFlag, os.Getenv("COMRADE_MODE"), cfg.General.Mode)
	if err != nil {
		return fmt.Errorf("comrade fix: %w", err)
	}

	auditSink, err := buildAuditSink(cmd, cfg, tr)
	if err != nil {
		return fmt.Errorf("comrade fix: %w", err)
	}

	// Ctrl-C: canceling ctx propagates into engine.Execute, exactly like
	// runDo's identical wiring.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	colorEnabled := resolveColorEnabled(cfg, os.Environ(), cmd.OutOrStdout())
	deps := engine.RunDeps{
		Executor:           ex,
		Safety:             safetyEngine,
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
		Request:            "fix: " + errCtx.Command,
		Translator:         tr,
	}

	summary, err := engine.Execute(ctx, diagnosis.Plan, mode, deps)
	if err != nil {
		return fmt.Errorf("comrade fix: %w", err)
	}

	if mode != engine.ModeInfo {
		if err := printRunSummary(cmd.OutOrStdout(), summary, tr); err != nil {
			return err
		}
	}

	// Post-solution verification (FAZ 7 item 4): only offered once the
	// run actually reached a clean end — info mode never aborts (nothing
	// ran), ask/auto only qualify when the whole plan completed without
	// a Block/failure/cancellation.
	if mode == engine.ModeInfo || !summary.Aborted {
		if verr := engine.OfferVerification(ctx, deps, mode, errCtx.Command); verr != nil {
			return fmt.Errorf("comrade fix: %w", verr)
		}
	}

	if summary.Aborted {
		return fmt.Errorf("comrade fix: %s", summary.AbortReason)
	}
	return nil
}

// acquireErrorContext implements docs/history/UYGULAMA_PLANI.md FAZ 4/7's fallback
// chain, in order:
//
//  1. `comrade fix -- <command...>`: explicitCommand is non-empty — run
//     it via captureByRunning, ignoring last_command.json entirely (the
//     user named an exact command to diagnose).
//  2. `comrade fix --rerun`: re-run the recorded last_command.json entry
//     via captureByRunning. Errors if no last_command.json entry exists
//     at all — --rerun has nothing to rerun without one.
//  3. A FRESH (< lastCommandFreshness) last_command.json entry whose
//     exit_code != 0: used directly, with NO re-execution — its captured
//     stderr/stdout tails are already exactly what FAZ 4's shell hook
//     recorded.
//  4. Otherwise: a stale or successful (exit_code == 0) last_command.json
//     entry is never silently reused (a one-line notice is printed
//     explaining why), and the chain falls through to interactive paste
//     mode (pasteMode).
func acquireErrorContext(cmd *cobra.Command, sysCtx contextpkg.Context, safetyEngine *safety.Engine, ex engine.CommandExecutor, rerun bool, explicitCommand string, tr i18n.Translator) (engine.ErrorContext, error) {
	if explicitCommand != "" {
		return captureByRunning(cmd, safetyEngine, ex, explicitCommand, sysCtx, tr)
	}

	if rerun {
		if sysCtx.LastCommand == nil {
			return engine.ErrorContext{}, fmt.Errorf("%s", tr.T(i18n.MsgFixRerunNoLastCommandError))
		}
		return captureByRunning(cmd, safetyEngine, ex, sysCtx.LastCommand.Command, sysCtx, tr)
	}

	if sysCtx.LastCommand != nil {
		lc := *sysCtx.LastCommand
		switch {
		case lc.Age(time.Now()) >= lastCommandFreshness:
			fmt.Fprintln(cmd.ErrOrStderr(), tr.T(i18n.MsgFixStaleNotice)) //nolint:errcheck // best-effort notice; falling through to paste mode either way.
		case lc.ExitCode == 0:
			fmt.Fprintln(cmd.ErrOrStderr(), tr.T(i18n.MsgFixExitZeroNotice)) //nolint:errcheck
		default:
			return engine.ErrorContext{
				Command:  lc.Command,
				ExitCode: lc.ExitCode,
				Stderr:   lc.StderrTail,
				Stdout:   lc.StdoutTail,
				System:   sysCtx,
			}, nil
		}
	}

	return pasteMode(cmd, sysCtx, tr)
}

// captureByRunning is the shared helper behind both `--rerun` and
// `-- <command>`: before ever touching the executor, it classifies
// command through safetyEngine — declaring RiskRead as the floor, so the
// verdict comes entirely from the independent denylist/escalation
// machinery, not from any (nonexistent, here) LLM-declared risk — and
// refuses to execute it at all when that verdict is Block or
// RiskDestructive (docs/history/UYGULAMA_PLANI.md FAZ 7 item 2: re-running a failed
// `rm -rf` "to capture its error" would be catastrophic). A refused
// command falls through to pasteMode instead, exactly like a stale/exit-0
// last_command.json entry does.
func captureByRunning(cmd *cobra.Command, safetyEngine *safety.Engine, ex engine.CommandExecutor, command string, sysCtx contextpkg.Context, tr i18n.Translator) (engine.ErrorContext, error) {
	decision := safetyEngine.Evaluate(command, safety.RiskRead)
	if decision.Action == safety.Block || decision.EffectiveRisk == safety.RiskDestructive {
		classification := decision.EffectiveRisk.String()
		if decision.Action == safety.Block {
			classification = tr.T(i18n.MsgFixBlockedClassification)
		}
		fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgFixRefusalNotice, command, classification)) //nolint:errcheck // best-effort notice; falling through to paste mode either way.
		return pasteMode(cmd, sysCtx, tr)
	}

	res, err := ex.Run(cmd.Context(), command, executor.Options{})
	if err != nil {
		return engine.ErrorContext{}, fmt.Errorf("run %q: %w", command, err)
	}
	return engine.ErrorContext{
		Command:  command,
		ExitCode: res.ExitCode,
		Stderr:   res.Stderr,
		Stdout:   res.Stdout,
		System:   sysCtx,
	}, nil
}

// pasteMode is the fallback chain's last resort (docs/history/UYGULAMA_PLANI.md FAZ 4
// item 3c): prompts the user to paste the failing command on one line,
// then its error output terminated by a blank line or EOF. ExitCode is
// set to -1 (engine.ErrorContext's documented "unknown" sentinel) since a
// pasted transcript never carries a real exit code.
func pasteMode(cmd *cobra.Command, sysCtx contextpkg.Context, tr i18n.Translator) (engine.ErrorContext, error) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, tr.T(i18n.MsgFixPasteIntro))       //nolint:errcheck
	fmt.Fprint(out, tr.T(i18n.MsgFixPasteCommandPrompt)) //nolint:errcheck

	scanner := bufio.NewScanner(cmd.InOrStdin())
	command := ""
	if scanner.Scan() {
		command = strings.TrimSpace(scanner.Text())
	}

	fmt.Fprintln(out, tr.T(i18n.MsgFixPasteErrorPrompt)) //nolint:errcheck
	var errLines []string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			break
		}
		errLines = append(errLines, line)
	}
	if err := scanner.Err(); err != nil {
		return engine.ErrorContext{}, fmt.Errorf("read pasted error output: %w", err)
	}

	return engine.ErrorContext{
		Command:  command,
		ExitCode: -1,
		Stderr:   strings.Join(errLines, "\n"),
		System:   sysCtx,
	}, nil
}
