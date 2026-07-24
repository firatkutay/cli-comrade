package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// undoPipelineDeps bundles every OS/I/O seam runUndoCore needs beyond
// config/the target run itself — exactly like RunDeps bundles Execute's
// own dependencies. Production code (runUndo, below) wires the real
// executor/tuiPromptUI/audit.Logger in; tests inject a scripted fake
// PromptUI (mirroring internal/engine's own fakePrompt) alongside a REAL
// executor.New(...) so an end-to-end test can prove an actual derived
// command (e.g. `rmdir` in a t.TempDir()) really runs.
type undoPipelineDeps struct {
	Prompt       engine.PromptUI
	Executor     engine.CommandExecutor
	Audit        engine.AuditSink
	Stdout       io.Writer
	Stderr       io.Writer
	ColorEnabled bool
	StepTimeout  time.Duration
	GOOS         string
	CurrentCwd   string
}

// undoOutcome is runUndoCore's result: the derived Plan, the (already
// translated) notes buildUndoPlan attached, and — only when the plan was
// actually handed to engine.Execute (neither dryRun nor an empty-Steps
// honest refusal) — its RunSummary.
type undoOutcome struct {
	Plan    engine.Plan
	Notes   []string
	Summary engine.RunSummary
	// Executed is true only when Summary is meaningful — i.e.
	// engine.Execute actually ran, under ModeAsk, non-negotiably (see
	// newUndoCmd's own doc comment on why there is no --yolo path here at
	// all).
	Executed bool
}

// runUndoCore is `comrade undo`'s entire cobra-decoupled pipeline:
// derive the undo Plan for target (buildUndoPlan — heuristics-first, LLM
// fallback), and — unless dryRun short-circuits straight to the caller,
// or the derived Plan has no steps at all (an honest refusal, never
// executed) — run it through engine.Execute, ALWAYS under ModeAsk, with
// NO --yolo bypass wired in anywhere in this function: CLAUDE.md's
// destructive/elevated confirmation requirement is non-negotiable
// everywhere else in this codebase, and an undo plan gets no special
// exception from it — if anything, a reversal deserves MORE scrutiny than
// the original action, not less.
func runUndoCore(ctx context.Context, cfg config.Config, client engine.Completer, target undoRun, dryRun bool, tr i18n.Translator, deps undoPipelineDeps) (undoOutcome, error) {
	safetyEngine := safety.NewEngine(cfg)
	undoer := engine.NewUndoer(client, cfg)

	plan, notes, err := buildUndoPlan(ctx, undoer, safetyEngine, target, deps.GOOS, deps.CurrentCwd, tr)
	if err != nil {
		return undoOutcome{Notes: notes}, err
	}

	if dryRun || len(plan.Steps) == 0 {
		return undoOutcome{Plan: plan, Notes: notes}, nil
	}

	runDeps := engine.RunDeps{
		Executor:           deps.Executor,
		Safety:             safetyEngine,
		LLM:                client,
		Prompt:             deps.Prompt,
		Audit:              deps.Audit,
		Stdout:             deps.Stdout,
		Stderr:             deps.Stderr,
		ColorEnabled:       deps.ColorEnabled,
		ConfirmDestructive: cfg.Safety.ConfirmDestructive,
		ConfirmElevated:    cfg.Safety.ConfirmElevated,
		StepTimeout:        deps.StepTimeout,
		Request:            "undo: " + target.RunID,
		RunID:              newRunID(),
		WorkingDir:         deps.CurrentCwd,
		UndoOf:             target.RunID,
		Translator:         tr,
	}

	summary, err := engine.Execute(ctx, plan, engine.ModeAsk, runDeps)
	if err != nil {
		return undoOutcome{Plan: plan, Notes: notes}, err
	}
	return undoOutcome{Plan: plan, Notes: notes, Summary: summary, Executed: true}, nil
}

// newUndoCmd builds "comrade undo [--run <id>] [--dry-run] [--list]":
// reverses the last reversible action recorded in the audit log, or
// shows manual/LLM-proposed undo steps when nothing can be reversed
// automatically. It ALWAYS runs its derived plan in ask mode — there is
// no --auto/--info override and no --yolo flag anywhere on this command,
// unlike do/fix — undoing an already-executed action is exactly the kind
// of step CLAUDE.md's confirmation requirement exists for, and this
// command never lets it be bypassed.
//
// The undo horizon is bounded by audit.retention_days: a run older than
// the configured retention is no longer in the log at all, and so can
// never be selected as a target (see `comrade history`'s own identical
// retention-bounded view).
func newUndoCmd(newLoader loaderFactory) *cobra.Command {
	var (
		runFlag string
		dryRun  bool
		list    bool
		limit   int
	)

	cmd := &cobra.Command{
		Use:   "undo",
		Short: "Reverse the last reversible action, or show manual undo steps",
		Long: "Reverse the last reversible action, or show manual undo steps.\n" +
			"Always confirms every step interactively (ask mode) — there is no --auto/--yolo bypass for undo.\n" +
			"Only runs recorded within audit.retention_days are eligible targets.",
		Args:              translatedNoArgs(newLoader),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUndo(cmd, newLoader, runFlag, dryRun, list, limit)
		},
	}

	cmd.Flags().StringVar(&runFlag, "run", "", enUsageDefault(i18n.MsgFlagUndoRun))
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, enUsageDefault(i18n.MsgFlagDryRun))
	cmd.Flags().BoolVar(&list, "list", false, enUsageDefault(i18n.MsgFlagUndoList))
	cmd.Flags().IntVar(&limit, "limit", 20, enUsageDefault(i18n.MsgFlagLimit))
	return cmd
}

// runUndo is newUndoCmd's cobra-wired RunE body: load config, read the
// audit log, resolve the target run (or list candidates and return),
// then delegate the entire derive/dry-run/execute pipeline to
// runUndoCore with the real executor/tuiPromptUI/audit.Logger wired in.
func runUndo(cmd *cobra.Command, newLoader loaderFactory, runFlag string, dryRun, list bool, limit int) error {
	cfg, tr, err := loadConfigWithNotice(cmd, newLoader)
	if err != nil {
		return err
	}

	auditPath, err := audit.DefaultPath()
	if err != nil {
		return err
	}
	logger, err := audit.NewLogger(auditPath)
	if err != nil {
		return err
	}
	entries, err := logger.ReadAll()
	if err != nil {
		return err
	}

	if list {
		return printUndoCandidates(cmd.OutOrStdout(), listUndoCandidates(entries, limit), tr)
	}

	var target undoRun
	if runFlag != "" {
		var ok bool
		target, ok = findUndoRunByID(entries, runFlag)
		if !ok {
			return fmt.Errorf("%s", tr.T(i18n.MsgUndoRunNotFoundError, runFlag))
		}
	} else {
		var ok bool
		target, ok = selectUndoTarget(entries)
		if !ok {
			return fmt.Errorf("%s", tr.T(i18n.MsgUndoNoTargetError))
		}
	}

	currentCwd, _ := os.Getwd() //nolint:errcheck // an unresolvable cwd degrades to "" — deriveStepsForRun's cwd-mismatch check simply never fires (its own `currentCwd != ""` guard), not a fatal condition for the whole command.

	tally := newUsageTally()
	client, err := buildLLMClient(cmd, cfg, tr, tally.record)
	if err != nil {
		return fmt.Errorf("comrade undo: %w", err)
	}

	auditSink, err := buildAuditSink(cmd, cfg, tr)
	if err != nil {
		return fmt.Errorf("comrade undo: %w", err)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	colorEnabled := resolveColorEnabled(cfg, os.Environ(), cmd.OutOrStdout())
	deps := undoPipelineDeps{
		Prompt:       &tuiPromptUI{in: cmd.InOrStdin(), out: cmd.OutOrStdout(), colorEnabled: colorEnabled, llm: client, tr: tr},
		Executor:     executor.New(cmd.OutOrStdout(), cmd.ErrOrStderr()),
		Audit:        auditSink,
		Stdout:       cmd.OutOrStdout(),
		Stderr:       cmd.ErrOrStderr(),
		ColorEnabled: colorEnabled,
		StepTimeout:  time.Duration(cfg.Executor.StepTimeoutSeconds) * time.Second,
		GOOS:         runtime.GOOS,
		CurrentCwd:   currentCwd,
	}

	outcome, err := runUndoCore(ctx, cfg, client, target, dryRun, tr, deps)
	for _, note := range outcome.Notes {
		fmt.Fprintln(cmd.ErrOrStderr(), note) //nolint:errcheck // a note is best-effort diagnostic context; its own write failure must never mask the command's real result.
	}
	if err != nil {
		if errors.Is(err, errUndoNothingReversible) {
			return fmt.Errorf("%s", tr.T(i18n.MsgUndoNothingReversibleError))
		}
		return fmt.Errorf("comrade undo: %w", err)
	}

	if len(outcome.Plan.Steps) == 0 {
		// Honest refusal (task spec: "never a guessed command") — the
		// model's own Summary already explains why, in the resolved
		// language (DeriveUndo's system prompt requires this).
		_, ferr := fmt.Fprintln(cmd.OutOrStdout(), outcome.Plan.Summary)
		return ferr
	}

	if dryRun {
		return renderPlan(cmd.OutOrStdout(), outcome.Plan, tr)
	}

	if perr := printRunSummary(cmd.OutOrStdout(), outcome.Summary, tr); perr != nil {
		return perr
	}
	if outcome.Summary.Aborted {
		return fmt.Errorf("comrade undo: %s", outcome.Summary.AbortReason)
	}
	return nil
}

// printUndoCandidates renders runs as a tabwriter-aligned
// RUN ID/TIME/STEPS/REQUEST table (`comrade undo --list`), or
// MsgUndoListEmpty when there are none — mirroring
// internal/cli/history.go's printHistoryTable exactly.
func printUndoCandidates(w io.Writer, runs []undoRun, tr i18n.Translator) error {
	if len(runs) == 0 {
		_, err := fmt.Fprintln(w, tr.T(i18n.MsgUndoListEmpty))
		return err
	}

	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(tw, tr.T(i18n.MsgUndoListHeader)); err != nil {
		return err
	}
	for _, r := range runs {
		if _, err := fmt.Fprintf(tw, "%s\t%s\t%d\t%s\n",
			r.RunID, r.Latest.Timestamp.Local().Format(time.RFC3339), len(r.Steps), r.Request); err != nil {
			return err
		}
	}
	return tw.Flush()
}
