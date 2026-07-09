package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// setupCLIRuntime is the config-load/first-run-notice/--yolo-warning/
// llm.Client-construction sequence shared verbatim by runDo (FAZ 6) and
// runFix (FAZ 7) — both are "load config, maybe build an LLM client, then
// run the FAZ 5/6 plan+execute machinery" pipelines, and this is exactly
// the part that never differs between them. It never wraps the returned
// error with either command's own prefix ("comrade do"/"comrade fix") —
// callers do that themselves, so each command's error text still reads
// naturally.
func setupCLIRuntime(cmd *cobra.Command, newLoader loaderFactory, flags *executionFlags) (config.Config, *llm.Client, error) {
	loader, err := newLoader()
	if err != nil {
		return config.Config{}, nil, err
	}
	cfg, created, err := loader.Load()
	if err != nil {
		return config.Config{}, nil, err
	}
	if created {
		if _, err := fmt.Fprintf(cmd.ErrOrStderr(), firstRunNoticeFormat, loader.Path()); err != nil {
			return config.Config{}, nil, err
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
			return config.Config{}, nil, err
		}
	}

	client, err := llm.New(*cfg)
	if err != nil {
		return config.Config{}, nil, err
	}
	return *cfg, client, nil
}
