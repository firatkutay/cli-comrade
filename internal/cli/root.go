package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// NewRootCmd builds the "comrade" root command. version is injected at
// build time via -ldflags; it defaults to "dev" for local, non-release
// builds. Running "comrade" with no arguments prints the version followed
// by the standard cobra help output.
//
// This is a thin wrapper around newRootCmd wiring the real
// update.GitHubClient in as the passive version-notification's
// ReleaseFetcher (see maybeNotifyUpdate) — production code and the vast
// majority of tests (which never reach that code path: bare invocation,
// dev builds, `comrade upgrade` itself, and general.update_check=false
// are all skipped before any fetcher is consulted) both go through this.
// The one test that exercises a SUCCESSFUL background check end-to-end
// calls newRootCmd directly with a fake ReleaseFetcher instead — this
// package's test files are `package cli`, so that unexported constructor
// is directly reachable without needing to export it.
func NewRootCmd(version string) *cobra.Command {
	return newRootCmd(version, &update.GitHubClient{})
}

// newRootCmd is NewRootCmd's real implementation, parameterized on the
// update.ReleaseFetcher the passive version-notification hook uses —
// dependency injection identical in spirit to upgradeDeps/initDeps, kept
// as a second, unexported constructor instead of widening NewRootCmd's
// own public signature (main.go and every other test call that one
// unchanged).
func newRootCmd(version string, updateFetcher update.ReleaseFetcher) *cobra.Command {
	root := &cobra.Command{
		Use:     "comrade",
		Short:   "comrade is a cross-platform AI CLI companion for the terminal",
		Example: enUsageDefault(i18n.MsgHelpExamplesRoot),
		Version: version,
		// cmd/comrade/main.go already prints Execute()'s returned error
		// exactly once to stderr; without these two, cobra would ALSO
		// print its own "Error: ..." line and a full Usage: block for
		// every subcommand error, tripling up on a single failure.
		// Cobra checks these flags on whichever command actually ran
		// OR on root — set here, they cover every subcommand in this
		// tree, including ones added below, with no per-subcommand
		// opt-in required.
		SilenceErrors: true,
		SilenceUsage:  true,
		// Args is set to cobra.ArbitraryArgs (rather than left nil) so
		// that cobra's own legacyArgs check — which otherwise rejects any
		// arg cobra.Command.Find couldn't match to a known subcommand
		// with "unknown command %q" — never fires here. That is exactly
		// what docs/history/UYGULAMA_PLANI.md FAZ 6 item 3's root-command fallback
		// needs: `comrade docker kur` doesn't match any subcommand name,
		// so Find returns the root command itself with args =
		// ["docker","kur"] intact; RunE below is what turns that into a
		// `do` request. A genuine subcommand typo (e.g. "comrade fx")
		// is therefore no longer rejected with a helpful "unknown
		// command" suggestion — it free-text-dispatches to `do` instead,
		// which is this UX pattern's deliberate, documented tradeoff
		// (see docs/history/phases/FAZ-06.md).
		Args: cobra.ArbitraryArgs,
		// The free-text tail RunE dispatches to `do` (below) is arbitrary
		// natural-language request text, never a file path — same
		// rationale as explain's own ValidArgsFunction. This does NOT
		// interfere with cobra's automatic subcommand-name completion
		// (chat/do/explain/.../help): that happens unconditionally,
		// earlier in cobra's own getCompletions, before ValidArgsFunction
		// is ever consulted — see completion_test.go's
		// TestCompleteRootSuggestsTopLevelCommandsExcludingHidden.
		ValidArgsFunction: cobra.NoFileCompletions,
		// QA D4b: cobra's own auto-added "completion" command generates
		// several KB of its own internal help text (per-shell usage,
		// eval/source instructions, flag descriptions) with no i18n hook
		// of any kind — genuinely impractical to translate, unlike the
		// eight structural section labels (help.go's usageTemplateFor)
		// this same QA round DID translate. HiddenDefaultCmd (not
		// DisableDefaultCmd) is the judgment call this task explicitly
		// allows: it stays fully FUNCTIONAL for a power user who already
		// knows to type "comrade completion bash" — it simply never
		// appears in --help output, decluttering the tree for this
		// project's actual (non-technical, terminal-averse) target
		// audience. Documented in docs/history/PROGRESS.md's i18n-exceptions
		// note alongside every other residual, judgment-based exception.
		CompletionOptions: cobra.CompletionOptions{HiddenDefaultCmd: true},
	}
	root.SetVersionTemplate("comrade version {{.Version}}\n")

	// newLoader is resolved lazily, once per subcommand invocation, rather
	// than once here: the config file path depends on environment
	// variables (XDG_CONFIG_HOME, APPDATA) that tests set per-case, and
	// resolving eagerly would bake in whatever the environment looked
	// like at process startup instead of at command-execution time. This
	// is the "Loader constructed ... passed to subcommands" dependency
	// injection CLAUDE.md calls for: no package-level viper/config state
	// anywhere in this tree.
	newLoader := func() (*config.Loader, error) {
		return config.NewLoader("")
	}

	rootFlags := addExecutionFlags(root)
	root.RunE = func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			// A bare `comrade` invocation never loads config (no
			// first-run config-file side effect just from asking for
			// help) — the version banner's language is resolved from
			// COMRADE_LANG/LANG/LC_ALL only (envOnlyTranslator,
			// runtime.go), skipping the config general.language layer
			// every other command's loadConfigWithNotice/setupCLIRuntime
			// applies once it has actually loaded one.
			tr := envOnlyTranslator()
			if _, err := fmt.Fprint(cmd.OutOrStdout(), tr.T(i18n.MsgVersionBanner, cmd.Version)); err != nil {
				return err
			}
			return cmd.Help()
		}
		return runDo(cmd, newLoader, strings.Join(args, " "), rootFlags)
	}

	// The passive version-update notification (docs/history/UYGULAMA_PLANI.md FAZ 10
	// item 4) is wired as root's own PersistentPostRunE: cobra only runs
	// it after the invoked command's RunE returns nil (see
	// spf13/cobra's Command.execute), and — since no command in this
	// tree sets its own PersistentPostRunE — it fires for whichever
	// subcommand actually ran (cmd is that leaf command, not root; see
	// maybeNotifyUpdate's own doc comment). Two cases are deliberately
	// skipped: a true bare `comrade` invocation (cmd == root AND no
	// args — the version banner/help path; a free-text `comrade
	// <request>` do-dispatch is cmd == root too, but WITH args, and is
	// NOT skipped), and `comrade upgrade` itself (no point nagging about
	// a new version immediately after the user just checked/installed
	// one).
	root.PersistentPostRunE = func(cmd *cobra.Command, args []string) error {
		// __hint (hint.go) is invoked once per keystroke by the
		// space-triggered shell widgets and must stay zero-config/
		// zero-network — see newHintCmd's own doc comment. Without this
		// skip, every hint request would fall through to
		// maybeNotifyUpdate below exactly like any other successful
		// subcommand (cobra's own execute() walks PersistentPostRunE up
		// from whichever command actually ran, with no built-in
		// exception for a Hidden one), silently loading config on a hot
		// path that must never touch it.
		if (cmd == root && len(args) == 0) || cmd.Name() == "upgrade" || cmd.Name() == "__hint" {
			return nil
		}
		maybeNotifyUpdate(cmd, newLoader, version, updateFetcher)
		return nil
	}

	// Command groups (help.go's applyTranslatedHelp overrides each Title
	// per resolved language at render time, same as every Short/flag/
	// Example string above/below it): Core is the everyday do/fix/
	// explain/chat loop; Setup is one-time-ish account/shell/config
	// setup; Info is read-only status/maintenance. `hook` (and its
	// hidden `hook record` child) and `config test-llm` are internal/
	// diagnostic-only and stay Hidden — cobra's own template never lists
	// a Hidden command regardless of GroupID, so they are deliberately
	// left with no GroupID at all rather than assigned to one of these
	// three for real.
	root.AddGroup(
		&cobra.Group{ID: groupCore, Title: enUsageDefault(i18n.MsgHelpGroupCore)},
		&cobra.Group{ID: groupSetup, Title: enUsageDefault(i18n.MsgHelpGroupSetup)},
		&cobra.Group{ID: groupInfo, Title: enUsageDefault(i18n.MsgHelpGroupInfo)},
	)

	doCmd := newDoCmd(newLoader)
	fixCmd := newFixCmd(newLoader)
	explainCmd := newExplainCmd(newLoader)
	chatCmd := newChatCmd(newLoader)
	doCmd.GroupID = groupCore
	fixCmd.GroupID = groupCore
	explainCmd.GroupID = groupCore
	chatCmd.GroupID = groupCore

	authCmd := newAuthCmd(newLoader)
	initCmd := newInitCmd(defaultInitDeps(), newLoader)
	configCmd := newConfigCmd(newLoader)
	authCmd.GroupID = groupSetup
	initCmd.GroupID = groupSetup
	configCmd.GroupID = groupSetup

	historyCmd := newHistoryCmd(newLoader)
	upgradeCmd := newUpgradeCmd(newLoader, defaultUpgradeDeps(version))
	historyCmd.GroupID = groupInfo
	upgradeCmd.GroupID = groupInfo

	root.AddCommand(
		doCmd, fixCmd, explainCmd, chatCmd,
		authCmd, initCmd, configCmd,
		historyCmd, upgradeCmd,
		newHookCmd(newLoader),
		newHintCmd(),
	)

	// Localizes every command's --help/usage output (help.go) — must run
	// after every AddCommand above, since applyTranslatedHelp walks the
	// whole tree by CommandPath(), which only resolves correctly once
	// every child is actually attached.
	registerTranslatedHelp(root, newLoader)

	return root
}
