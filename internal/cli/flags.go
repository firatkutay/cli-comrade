package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// executionFlags bundles the flags that control how a free-text request
// is executed: --dry-run (print the plan, run nothing), the three
// mutually exclusive --auto/--ask/--info mode overrides (UYGULAMA_PLANI.md
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
}

// addExecutionFlags registers executionFlags on cmd and returns the
// struct cobra will populate when cmd runs.
func addExecutionFlags(cmd *cobra.Command) *executionFlags {
	f := &executionFlags{}
	cmd.Flags().BoolVar(&f.dryRun, "dry-run", false, "print the generated plan without executing it")
	cmd.Flags().BoolVar(&f.auto, "auto", false, "run in auto mode for this invocation (overrides COMRADE_MODE/config)")
	cmd.Flags().BoolVar(&f.ask, "ask", false, "run in ask mode for this invocation (overrides COMRADE_MODE/config)")
	cmd.Flags().BoolVar(&f.info, "info", false, "print the plan and explain it without executing anything")
	cmd.Flags().BoolVar(&f.yolo, "yolo", false, "DANGEROUS: bypass destructive/elevated confirmation in auto mode when safety.confirm_destructive/confirm_elevated is also disabled in config")
	return f
}

// modeFlagValue collapses the three mutually exclusive mode flags into
// the single string engine.ResolveMode's flagValue parameter expects
// ("" when none of the three was given).
func (f *executionFlags) modeFlagValue() (string, error) {
	switch {
	case f.auto && f.ask, f.auto && f.info, f.ask && f.info:
		return "", fmt.Errorf("only one of --auto, --ask, or --info may be given")
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
