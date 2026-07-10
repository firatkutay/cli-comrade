package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// newTranslator builds the one Translator every cli command resolves its
// user-facing output through, from cfg's already-loaded general.language —
// i18n.ResolveLanguage/i18n.NewTranslator, injected here rather than kept
// as global state (CLAUDE.md's "Global state yok" rule; see
// internal/i18n's own package doc comment). i18n.SystemLocale is
// ResolveLanguage's last-resort step (only reached for "auto" with no
// COMRADE_LANG/LANG/LC_ALL set): a real OS-locale probe on Windows, a
// guaranteed no-op everywhere else.
func newTranslator(cfg config.Config) i18n.Translator {
	return i18n.NewTranslator(i18n.ResolveLanguage(cfg.General.Language, os.Getenv, i18n.SystemLocale))
}

// envOnlyTranslator resolves the active language from COMRADE_LANG/LANG/
// LC_ALL, then (on Windows only) the OS locale, skipping config
// general.language entirely — used for the handful of error/help paths
// that must report BEFORE any config is ever loaded (executionFlags.
// modeFlagValue's mutually-exclusive-flags error; auth.go's ollama/
// unknown-provider fast rejections; init.go's --print/--remove
// exclusivity and shell-detection errors; the root command's own
// bare-invocation version banner), so a CLI usage mistake is reported
// without ever touching the filesystem. Documented, minor, deliberate
// inconsistency: these specific messages honor COMRADE_LANG/LANG/LC_ALL/
// the OS locale but not a config general.language=tr with no matching
// env var or OS locale set — see docs/phases/FAZ-09.md.
func envOnlyTranslator() i18n.Translator {
	return i18n.NewTranslator(i18n.ResolveLanguage("", os.Getenv, i18n.SystemLocale))
}

// loadConfigWithNotice loads newLoader's effective config, printing the
// shared first-run notice to cmd's stderr when this call is what created
// the file, and returns the Translator built from that same config
// (general.language) alongside it — every caller needs both.
func loadConfigWithNotice(cmd *cobra.Command, newLoader loaderFactory) (config.Config, i18n.Translator, error) {
	loader, err := newLoader()
	if err != nil {
		return config.Config{}, i18n.Translator{}, err
	}
	cfg, created, err := loader.Load()
	if err != nil {
		return config.Config{}, i18n.Translator{}, err
	}
	tr := newTranslator(*cfg)
	if created {
		if _, err := fmt.Fprint(cmd.ErrOrStderr(), tr.T(i18n.MsgFirstRunNotice, loader.Path())); err != nil {
			return config.Config{}, i18n.Translator{}, err
		}
	}
	return *cfg, tr, nil
}

// buildLLMClient constructs the full llm.Client (fallback chain included)
// for cfg, resolving API keys through the same keychain/file/env chain
// every FAZ 8 command uses (see newSecretsStore/secretsKeyResolver).
func buildLLMClient(cmd *cobra.Command, cfg config.Config) (*llm.Client, error) {
	store, err := newSecretsStore(cmd.ErrOrStderr())
	if err != nil {
		return nil, err
	}
	return llm.New(cfg, llm.WithKeyResolver(secretsKeyResolver(store)))
}

// setupCLIRuntime is the config-load/first-run-notice/--yolo-warning/
// llm.Client-construction sequence shared verbatim by runDo (FAZ 6) and
// runFix (FAZ 7) — both are "load config, maybe build an LLM client, then
// run the FAZ 5/6 plan+execute machinery" pipelines, and this is exactly
// the part that never differs between them. It never wraps the returned
// error with either command's own prefix ("comrade do"/"comrade fix") —
// callers do that themselves, so each command's error text still reads
// naturally. `comrade explain` (FAZ 9) needs the config-load/LLM-client
// pieces but not the --yolo warning (it has no --yolo flag of its own),
// so it calls loadConfigWithNotice/buildLLMClient directly instead of
// this wrapper.
func setupCLIRuntime(cmd *cobra.Command, newLoader loaderFactory, flags *executionFlags) (config.Config, *llm.Client, error) {
	cfg, tr, err := loadConfigWithNotice(cmd, newLoader)
	if err != nil {
		return config.Config{}, nil, err
	}

	// CLAUDE.md security rule #6: --yolo prints a red warning on every
	// use, regardless of whether the config-side bypass conditions
	// (safety.confirm_destructive/confirm_elevated=false) actually let it
	// do anything this particular run.
	if flags.yolo {
		if err := tui.PrintWarning(cmd.ErrOrStderr(), tr.T(i18n.MsgYoloWarning), resolveColorEnabled(cfg, os.Environ(), cmd.ErrOrStderr())); err != nil {
			return config.Config{}, nil, err
		}
	}

	client, err := buildLLMClient(cmd, cfg)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, client, nil
}
