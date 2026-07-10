package cli

import (
	"errors"
	"fmt"
	"io"
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
// env var or OS locale set — see docs/history/phases/FAZ-09.md.
func envOnlyTranslator() i18n.Translator {
	return i18n.NewTranslator(i18n.ResolveLanguage("", os.Getenv, i18n.SystemLocale))
}

// bestEffortTranslator resolves the SAME Translator the rest of the
// command would (config general.language, via loadConfigWithNotice —
// full precedence: config > COMRADE_LANG > LANG > LC_ALL > Windows
// locale > English), for a usage-error path that fires BEFORE the
// command's own real work would otherwise load config — e.g. `comrade
// explain` with no arguments, or `comrade config set` with the wrong
// number of arguments. Unlike envOnlyTranslator (which skips config
// entirely, by design, for paths that must never touch the filesystem —
// see its own doc comment), this DOES load config, on the reasoning that
// a plain usage-error render is not the kind of filesystem side effect
// that rationale was protecting; it matches loadConfigWithNotice's own
// first-run-file-creation/notice behavior, exactly like every other
// command already has (e.g. `comrade config get <bad-key>` already
// creates the file/prints the notice before its own unknown-key error).
//
// It never fails the caller's error path itself: if newLoader() or
// Load() fails for any reason (a missing config directory it can't
// create, a malformed on-disk TOML, ...), this falls back to
// envOnlyTranslator's env-only resolution instead of propagating that
// second error — a usage-error message must always render in SOME
// language, never itself error out.
func bestEffortTranslator(cmd *cobra.Command, newLoader loaderFactory) i18n.Translator {
	_, tr, err := loadConfigWithNotice(cmd, newLoader)
	if err != nil {
		return envOnlyTranslator()
	}
	return tr
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

// classifyLLMError re-renders err — the error a blocking LLM call
// (planner.GeneratePlan/diagnoser.Diagnose/explainer.Explain/chatTurn/
// runChatDo) returned — through tr when it is classifiable, returning
// (true, translated); returns (false, nil) for anything else, leaving
// the caller's own existing handling untouched (QA MAJOR-1's regression
// fix — the previous behavior was to surface the raw internal wrap-chain
// verbatim: "llm: all providers failed: anthropic: no API key found for
// provider \"anthropic\"; set one of: ..." — English regardless of
// general.language, and internal detail (the wrap-chain, the literal env
// var names) a terminal beginner cannot act on).
//
// Currently classifies exactly one case: *llm.KeyMissingError (every
// configured provider/fallback attempt failed because no credential was
// found at all) — internal/llm's fallback loop already wraps this with
// %w at every level (Client.Complete/Stream's per-attempt wrap, then
// finalChainError's "all providers failed" wrap), so errors.As sees
// through the whole chain to recover the original *KeyMissingError's own
// Provider field, without this function ever parsing error TEXT. This is
// the SAME errors.Is/As-at-the-CLI-boundary pattern
// translateConfigError (config.go) and translateUpgradeFetchError
// (upgrade.go) already established for their own respective error
// families — a third application of one existing pattern, not a new one.
//
// A pure function — no I/O — so chatdispatch.go's dispatchChatLine (which
// deliberately has no bubbletea/terminal/cmd access at all — see its own
// doc comment) can call it directly for chat's in-model error rendering;
// translateLLMError (below) is the version WITH the COMRADE_DEBUG detail
// dump, for callers that do have a cmd/stderr to dump to.
func classifyLLMError(tr i18n.Translator, err error) (bool, error) {
	var keyMissing *llm.KeyMissingError
	if errors.As(err, &keyMissing) {
		return true, fmt.Errorf("%s", tr.T(i18n.MsgLLMNoKeyError, keyMissing.Provider, keyMissing.Provider))
	}
	return false, nil
}

// translateLLMError is classifyLLMError plus the two things a plain CLI
// command (do/fix/explain — every LLM-reaching command except chat, which
// uses classifyLLMError directly) needs around it: the original,
// un-translated err's full detail is written to w, but ONLY when
// COMRADE_DEBUG is set (matching hook.go's own established debug-gated-
// detail convention elsewhere in this tree) — the wrap-chain is
// SUPPRESSED from the primary message, never silently discarded; and
// every OTHER (unclassified) error is returned wrapped exactly as it
// always was (fmt.Errorf("prefix: %w", err)) — nothing about an
// unclassified failure's own detail changes.
func translateLLMError(w io.Writer, prefix string, tr i18n.Translator, err error) error {
	if ok, translated := classifyLLMError(tr, err); ok {
		if os.Getenv("COMRADE_DEBUG") != "" {
			_, _ = fmt.Fprintf(w, "%s: %v\n", prefix, err)
		}
		return translated
	}
	return fmt.Errorf("%s: %w", prefix, err)
}

// isTerminalFunc reports whether fd is an interactive terminal. Its
// production value is golang.org/x/term.IsTerminal (matches that
// function's own signature exactly, so no wrapping is needed at the call
// site). Commands take it as a parameter — exactly like passwordReader
// (auth.go) — so tests can simulate both a real TTY (the overwhelmingly
// common case) and a non-TTY (QA MINOR-5's actual bug) deterministically,
// without depending on go test's own stdin, which is unpredictable across
// environments (a real terminal when run locally by hand, virtually never
// one under CI or a piped/scripted invocation).
type isTerminalFunc func(fd int) bool

// requireInteractiveTTY returns a friendly, i18n'd error (rendered
// through tr as msgID, a no-arg MessageID) when stdin — as reported by
// isTerminal — is not an interactive terminal, and nil otherwise. Shared
// by `comrade auth login` (MsgAuthLoginRequiresTTY) and `comrade chat`
// (MsgChatRequiresTTY), QA MINOR-5's fix: both previously let the
// underlying library (x/term.ReadPassword / bubbletea) fail on its own —
// a raw platform errno ("inappropriate ioctl for device" on Unix) for
// auth login, an indefinite hang for chat (bubbletea needs a real TTY) —
// neither of which names a cause a non-expert user could act on. Checked
// up front, before any config load or provider I/O, so a non-interactive
// invocation fails fast and cleanly instead of however the library
// underneath would otherwise fail.
func requireInteractiveTTY(tr i18n.Translator, isTerminal isTerminalFunc, msgID i18n.MessageID) error {
	if isTerminal(int(os.Stdin.Fd())) {
		return nil
	}
	return fmt.Errorf("%s", tr.T(msgID))
}

// buildLLMClient constructs the full llm.Client (fallback chain included)
// for cfg, resolving API keys through the same keychain/file/env chain
// every FAZ 8 command uses (see newSecretsStore/secretsKeyResolver). tr
// is only used for the file-fallback keychain-unavailable notice's
// language (QA MINOR-4) — every caller already has it in scope from its
// own loadConfigWithNotice call.
func buildLLMClient(cmd *cobra.Command, cfg config.Config, tr i18n.Translator) (*llm.Client, error) {
	store, err := newSecretsStore(cmd.ErrOrStderr(), tr)
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

	client, err := buildLLMClient(cmd, cfg, tr)
	if err != nil {
		return config.Config{}, nil, err
	}
	return cfg, client, nil
}
