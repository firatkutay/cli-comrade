package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// executionFlags bundles the flags that control how a free-text request
// is executed: --dry-run (print the plan, run nothing), the three
// mutually exclusive --auto/--ask/--info mode overrides (docs/history/UYGULAMA_PLANI.md
// FAZ 6 item 2's mode-precedence flag source), and --yolo (CLAUDE.md
// security rule #6 / FAZ 6's auto-mode bypass escape hatch). Both the
// hidden `do` subcommand and the root command's free-text fallback
// register their own *cobra.Command-local copy of these flags via
// addExecutionFlags, so `comrade do "docker kur" --auto` and
// `comrade docker kur --auto` both work identically.
type executionFlags struct {
	dryRun bool
	auto   bool
	ask    bool
	info   bool
	yolo   bool
	// usage is --usage: force the per-run token/cost summary line on for
	// this invocation, regardless of general.show_usage — see
	// internal/cli/usage.go.
	usage bool
	// review/noReview are --review/--no-review: force the interactive
	// plan-preview/edit screen (internal/tui.ReviewPlan, via
	// internal/cli/planreview.go) on or off for this one invocation,
	// regardless of general.plan_review. --no-review always wins when
	// both are somehow set (see reviewFlagValue) — an explicit
	// per-invocation "don't show it" must never be silently overridden by
	// general.plan_review=="ask".
	review   bool
	noReview bool
}

// reviewFlagValue collapses --review/--no-review into the single
// tri-state shouldShowPlanReview (planreview.go) actually needs:
// forceOn/forceOff/unset. --no-review always wins over --review when a
// caller somehow sets both (cobra itself never does this — they are two
// independent bool flags, not a mutually-exclusive pair — but a direct
// executionFlags literal, as tests may build, could).
func (f *executionFlags) reviewFlagValue() (forceOn, forceOff bool) {
	if f.noReview {
		return false, true
	}
	return f.review, false
}

// addExecutionFlags registers executionFlags on cmd and returns the
// struct cobra will populate when cmd runs. Each flag's description is
// registered with its ENGLISH catalog value (enUsageDefault, help.go) —
// not a raw string literal — since no per-invocation Translator exists
// yet at command-construction time; internal/cli/help.go's
// applyTranslatedHelp overwrites every one of these with the resolved
// language's own text immediately before cobra actually renders the
// "Flags:" section, exactly like it does for Short text.
func addExecutionFlags(cmd *cobra.Command) *executionFlags {
	f := &executionFlags{}
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, enUsageDefault(i18n.MsgFlagDryRun))
	cmd.Flags().BoolVar(&f.auto, "auto", false, enUsageDefault(i18n.MsgFlagAuto))
	cmd.Flags().BoolVar(&f.ask, "ask", false, enUsageDefault(i18n.MsgFlagAsk))
	cmd.Flags().BoolVar(&f.info, "info", false, enUsageDefault(i18n.MsgFlagInfo))
	cmd.Flags().BoolVar(&f.yolo, "yolo", false, enUsageDefault(i18n.MsgFlagYolo))
	cmd.Flags().BoolVar(&f.usage, "usage", false, enUsageDefault(i18n.MsgFlagUsage))
	cmd.Flags().BoolVar(&f.review, "review", false, enUsageDefault(i18n.MsgFlagReview))
	cmd.Flags().BoolVar(&f.noReview, "no-review", false, enUsageDefault(i18n.MsgFlagNoReview))
	return f
}

// modeFlagValue collapses the three mutually exclusive mode flags into
// the single string engine.ResolveMode's flagValue parameter expects
// ("" when none of the three was given). This runs BEFORE any config is
// ever loaded (it is the first thing runDo/runFix/root's RunE do), so
// its one error message is translated via envOnlyTranslator (runtime.go)
// — COMRADE_LANG/LANG/LC_ALL only, deliberately skipping config
// general.language — rather than requiring the config load that would
// otherwise be needed just to report a CLI usage mistake, exactly like
// root.go's bare-invocation version banner.
func (f *executionFlags) modeFlagValue() (string, error) {
	switch {
	case f.auto && f.ask, f.auto && f.info, f.ask && f.info:
		return "", fmt.Errorf("%s", envOnlyTranslator().T(i18n.MsgFlagsModeExclusiveError))
	case f.auto:
		return "auto", nil
	case f.ask:
		return "ask", nil
	case f.info:
		return "info", nil
	default:
		return "", nil
	}
}
