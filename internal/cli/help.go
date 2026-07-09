package cli

import (
	"os"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// helpShortByPath maps a command's full CommandPath() (e.g.
// "comrade auth login") to the MessageID for its Short one-line
// description. cobra reads a command's Short field DIRECTLY — both when
// rendering that command's own "--help"/usage block, and when a parent
// renders its "Available Commands:" listing (which reads every child's
// Short field straight off the struct, not through a function call) — a
// plain string baked in at command-construction time, long before any
// per-invocation config is ever loaded. applyTranslatedHelp is what
// bridges that gap: it overwrites every command's Short from this
// catalog, by CommandPath(), immediately before cobra actually renders
// help/usage (wired via root.SetHelpFunc/SetUsageFunc in NewRootCmd), so
// --help output is localized exactly like every other command's output —
// UYGULAMA_PLANI.md FAZ 9's "cobra Short/Long help text ... these render
// in --help and are user-facing" requirement.
//
// Long is not covered: no command in this tree sets it (every command
// uses Short only), so there is nothing to translate there.
var helpShortByPath = map[string]i18n.MessageID{
	"comrade":                 i18n.MsgHelpShortRoot,
	"comrade do":              i18n.MsgHelpShortDo,
	"comrade fix":             i18n.MsgHelpShortFix,
	"comrade explain":         i18n.MsgHelpShortExplain,
	"comrade chat":            i18n.MsgHelpShortChat,
	"comrade config":          i18n.MsgHelpShortConfig,
	"comrade config get":      i18n.MsgHelpShortConfigGet,
	"comrade config set":      i18n.MsgHelpShortConfigSet,
	"comrade config list":     i18n.MsgHelpShortConfigList,
	"comrade config edit":     i18n.MsgHelpShortConfigEdit,
	"comrade config path":     i18n.MsgHelpShortConfigPath,
	"comrade config test-llm": i18n.MsgHelpShortConfigTestLLM,
	"comrade config models":   i18n.MsgHelpShortConfigModels,
	"comrade auth":            i18n.MsgHelpShortAuth,
	"comrade auth login":      i18n.MsgHelpShortAuthLogin,
	"comrade auth logout":     i18n.MsgHelpShortAuthLogout,
	"comrade auth status":     i18n.MsgHelpShortAuthStatus,
	"comrade init":            i18n.MsgHelpShortInit,
	"comrade history":         i18n.MsgHelpShortHistory,
	"comrade hook":            i18n.MsgHelpShortHook,
	"comrade hook record":     i18n.MsgHelpShortHookRecord,
	"comrade upgrade":         i18n.MsgHelpShortUpgrade,
}

// flagUsageByName maps a flag's NAME (not its command path — the same
// flag name always carries the same description everywhere it appears in
// this tree, since addExecutionFlags registers --auto/--ask/--info/
// --yolo/--dry-run identically on root/`do`/`fix`) to its description's
// MessageID. `comrade hook record`'s three flags (--shell/--exit/
// --command) are deliberately absent — see catalogCoverageAllowlist's
// hook.go entry (internal-only, invoked by generated shell snippets,
// never read by an end user via --help).
var flagUsageByName = map[string]i18n.MessageID{
	"dry-run": i18n.MsgFlagDryRun,
	"auto":    i18n.MsgFlagAuto,
	"ask":     i18n.MsgFlagAsk,
	"info":    i18n.MsgFlagInfo,
	"yolo":    i18n.MsgFlagYolo,
	"rerun":   i18n.MsgFlagRerun,
	"json":    i18n.MsgFlagJSON,
	"limit":   i18n.MsgFlagLimit,
	"print":   i18n.MsgFlagPrint,
	"remove":  i18n.MsgFlagRemove,
	"yes":     i18n.MsgFlagYes,
	"check":   i18n.MsgFlagCheck,
}

// enUsageDefault renders id in English — the flag-registration-time
// default every addExecutionFlags/newFixCmd/newHistoryCmd/newInitCmd call
// site uses instead of a raw string literal, since no per-invocation
// Translator exists yet at command-construction time. applyTranslatedHelp
// (below) always overwrites it with the resolved language's own text
// immediately before cobra renders "--help"; this default is only ever
// visible if that override somehow never ran. Registering this (a
// function call) rather than a raw string literal is also what keeps
// every flag-registration site out of catalog_coverage_test.go's
// flag-description scan — there is no literal left for it to flag.
func enUsageDefault(id i18n.MessageID) string {
	return i18n.NewTranslator(i18n.LangEN).T(id)
}

// registerTranslatedHelp overrides root's HelpFunc/UsageFunc so every
// "--help"/usage render in the tree first re-translates every command's
// Short text (applyTranslatedHelp) in the resolved language, then falls
// through to cobra's own default rendering (captured here, BEFORE being
// overridden, so the actual template/formatting logic is untouched — only
// the Short strings feeding it change). Setting this once on root is
// sufficient for the whole tree: cobra's Command.HelpFunc()/UsageFunc()
// walk up to the nearest ancestor with one set when a child has none of
// its own, and no command in this tree ever sets its own.
func registerTranslatedHelp(root *cobra.Command, newLoader loaderFactory) {
	defaultHelp := root.HelpFunc()
	defaultUsage := root.UsageFunc()

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		applyTranslatedHelp(root, newLoader)
		defaultHelp(cmd, args)
	})
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		applyTranslatedHelp(root, newLoader)
		return defaultUsage(cmd)
	})
}

// applyTranslatedHelp resolves the active Translator (helpTranslator) and,
// walking every command in root's tree: overwrites Short when
// CommandPath() is a helpShortByPath key, AND overwrites every one of
// that command's own (non-inherited) flags' Usage when the flag's name is
// a flagUsageByName key — cobra/pflag read both fields directly off the
// struct when rendering, so both need the same lazy, render-time
// override.
func applyTranslatedHelp(root *cobra.Command, newLoader loaderFactory) {
	tr := helpTranslator(newLoader)
	var walk func(cmd *cobra.Command)
	walk = func(cmd *cobra.Command) {
		if id, ok := helpShortByPath[cmd.CommandPath()]; ok {
			cmd.Short = tr.T(id)
		}
		cmd.Flags().VisitAll(func(f *pflag.Flag) {
			if id, ok := flagUsageByName[f.Name]; ok {
				f.Usage = tr.T(id)
			}
		})
		for _, child := range cmd.Commands() {
			walk(child)
		}
	}
	walk(root)
}

// helpTranslator resolves the Translator help text uses: config
// general.language when a config load succeeds (loader.Load() may create
// a default config file as a side effect on first run, exactly like
// every other command already does — "--help" is not special-cased away
// from that), otherwise (a construction-time loader failure) falls back
// to envOnlyTranslator (runtime.go) — the same env-only resolution
// root.go's bare-invocation version banner and executionFlags.
// modeFlagValue's pre-config error both use.
func helpTranslator(newLoader loaderFactory) i18n.Translator {
	loader, err := newLoader()
	if err == nil {
		if cfg, _, loadErr := loader.Load(); loadErr == nil {
			return i18n.NewTranslator(i18n.ResolveLanguage(cfg.General.Language, os.Getenv))
		}
	}
	return envOnlyTranslator()
}
