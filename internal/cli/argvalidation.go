package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// translatedPositionalArgs returns a cobra.PositionalArgs validator that
// calls valid(len(args)) to decide whether args satisfies the command's
// arity contract; on failure it returns a friendly, translated usage
// error built from msgID and msgArgs, rendered through
// bestEffortTranslator(cmd, newLoader) — the SAME config
// general.language-first resolution every other usage-error path in
// this tree already uses (see runtime.go's own doc comment on why NOT
// envOnlyTranslator) — instead of cobra's own raw, untranslated English
// "accepts N arg(s), received M" (a wrong-arity leaf command) or
// "unknown command %q for %q" (a wrong-arity command WITH subcommands,
// cobra's legacyArgs default).
//
// This is the ONE shared implementation behind every translatedXArgs
// helper below, and the whole fix for the class of bug this file exists
// to close: every arg-count usage error in internal/cli goes through
// exactly this one code path, rather than each command hand-rolling its
// own bestEffortTranslator call (explain.go's and config.go's "set"
// subcommand are the two pre-existing, deliberately-untouched
// exceptions — see their own doc comments for why their arg handling
// needs to be more than a simple arity check).
func translatedPositionalArgs(newLoader loaderFactory, valid func(n int) bool, msgID i18n.MessageID, msgArgs ...any) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if valid(len(args)) {
			return nil
		}
		return fmt.Errorf("%s", bestEffortTranslator(cmd, newLoader).T(msgID, msgArgs...))
	}
}

// translatedExactArgs requires exactly n positional arguments — the
// translated replacement for cobra.ExactArgs(n).
func translatedExactArgs(newLoader loaderFactory, n int, msgID i18n.MessageID, msgArgs ...any) cobra.PositionalArgs {
	return translatedPositionalArgs(newLoader, func(got int) bool { return got == n }, msgID, msgArgs...)
}

// translatedMinArgs requires at least n positional arguments — the
// translated replacement for cobra.MinimumNArgs(n).
func translatedMinArgs(newLoader loaderFactory, n int, msgID i18n.MessageID, msgArgs ...any) cobra.PositionalArgs {
	return translatedPositionalArgs(newLoader, func(got int) bool { return got >= n }, msgID, msgArgs...)
}

// translatedMaxArgs requires at most n positional arguments — the
// translated replacement for cobra.MaximumNArgs(n).
func translatedMaxArgs(newLoader loaderFactory, n int, msgID i18n.MessageID, msgArgs ...any) cobra.PositionalArgs {
	return translatedPositionalArgs(newLoader, func(got int) bool { return got <= n }, msgID, msgArgs...)
}

// translatedNoArgs is the translated replacement for cobra.NoArgs,
// shared by every leaf command in this tree that takes no positional
// arguments at all: it renders the ONE shared i18n.MsgUsageNoArgsError
// with the resolved command's own CommandPath (e.g. "comrade chat"),
// read from cmd inside the returned closure — cmd, and therefore its
// full path, is only known at validation time, not at construction
// time — so every such command shares one MessageID instead of needing
// a dedicated one apiece.
//
// Also used on "hook" (a Runnable parent command with a subcommand),
// correcting a DIFFERENT, milder pre-existing gap than the leaf
// commands above: hook's own Args field was nil, and cobra's
// ValidateArgs default for nil is ArbitraryArgs (accepts anything,
// silently) — NOT cobra's "legacyArgs" unknown-subcommand check, which
// (see (*cobra.Command).Find, command.go) only ever fires for a nil-Args
// command with NO PARENT, i.e. only the root command itself (root.go
// deliberately sets Args: cobra.ArbitraryArgs specifically to neutralize
// that one case too). So "comrade hook bogus" never raised cobra's raw
// English "unknown command" text — it silently printed help and exited
// 0, swallowing the typo. translatedNoArgs turns that silent no-op into
// an actionable, translated error instead — a UX improvement, not a
// class-of-bug fix, unlike every ExactArgs/MinimumNArgs/MaximumNArgs/
// NoArgs replacement elsewhere in this file, which DID replace a real,
// verified raw-English cobra error.
func translatedNoArgs(newLoader loaderFactory) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return fmt.Errorf("%s", bestEffortTranslator(cmd, newLoader).T(i18n.MsgUsageNoArgsError, cmd.CommandPath()))
	}
}

// translatedUnknownSubcommand is the Args validator for a PARENT command
// with real, visible subcommands and no positional arguments of its own
// (auth, config): once cobra's Find() fails to resolve args[0] against
// any of this command's children, Find returns THIS command with the
// unresolved args intact (see (*cobra.Command).Find, command.go's
// innerfind) — so by construction, this validator only ever sees
// len(args) > 0 when the first arg did NOT match a real subcommand name;
// a genuine subcommand invocation (e.g. "auth login ...") recurses all
// the way down to that leaf command's OWN Args instead, and this
// validator never runs for it at all. len(args) == 0 (a bare "comrade
// auth"/"comrade config") is always valid here — cmd's own RunE (help)
// handles that case, exactly like newHookCmd's does.
//
// NOTE: this validator only has any effect once the command it's
// attached to is Runnable (has its own Run/RunE) — cobra's
// (*cobra.Command).execute returns flag.ErrHelp for ANY invocation of a
// non-Runnable command BEFORE its Args validator ever runs at all (the
// "if !c.Runnable() { return flag.ErrHelp }" check precedes
// ValidateArgs unconditionally), so a parent command using this must
// also set its own RunE (see newAuthCmd/newConfigCmd, which both mirror
// newHookCmd's "RunE: return cmd.Help()" pattern for exactly this
// reason) — otherwise this validator would be silently dead code.
func translatedUnknownSubcommand(newLoader loaderFactory) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == 0 {
			return nil
		}
		return fmt.Errorf("%s", bestEffortTranslator(cmd, newLoader).T(
			i18n.MsgUnknownSubcommandError, args[0], cmd.CommandPath(), strings.Join(visibleSubcommandNames(cmd), ", ")))
	}
}

// visibleSubcommandNames returns cmd's own non-Hidden child command
// names, in cmd.Commands()'s own alphabetical order (cobra sorts by
// default) — the dynamic, derive-don't-hand-maintain source
// translatedUnknownSubcommand's error message lists, so a future
// subcommand added under auth/config is picked up automatically instead
// of needing a second, hand-copied list kept in sync by hand.
func visibleSubcommandNames(cmd *cobra.Command) []string {
	var names []string
	for _, sub := range cmd.Commands() {
		if !sub.Hidden {
			names = append(names, sub.Name())
		}
	}
	return names
}
