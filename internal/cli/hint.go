package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// hintMaxLen caps the length of the "[a|b|c]" line comrade __hint prints
// — the ~90-character budget both the zsh (POSTDISPLAY ghost text) and
// PowerShell (PossibleCompletions) space-triggered widgets in
// internal/shellinit/snippets/ render below the user's cursor. "comrade
// config get " alone already exceeds it (20+ config keys), which is
// exactly the case this cap exists for; see formatHintList.
const hintMaxLen = 90

// newHintCmd builds the hidden "comrade __hint" command: a fast,
// zero-config, zero-network, single-source-of-truth resolver for "which
// word(s) can come next" on a partially typed command line — invoked
// once per keystroke by the space-triggered shell widgets, so it must
// stay as close to instantaneous as cobra's own "__complete" pseudo-
// command. Every branch below reuses the exact same command tree, the
// exact same ValidArgsFunction/ValidArgs, and the exact same visibility
// rules "comrade __complete" already exercises (completion.go), so a
// hint can never drift from what Tab-completion itself would offer.
//
// DisableFlagParsing mirrors cobra's own "__complete" definition
// (spf13/cobra@v1.10.2 completions.go's initCompleteCmd): the buffer
// tokens this command receives may themselves contain flags meant for
// the TRAVERSED command, never for __hint, so cobra must never try to
// parse them against __hint's own (nonexistent) flag set. Args is
// cobra.ArbitraryArgs — not MinimumNArgs/ExactArgs — so this command's
// own Args validation can never fail: this command's whole contract
// (Run's own doc comment) is "never error, never print anything but the
// hint itself, exit 0 unconditionally".
func newHintCmd() *cobra.Command {
	return &cobra.Command{
		Use:                "__hint",
		Short:              "Print the next-word hint for a partially typed command line (internal, not for direct use)",
		Hidden:             true,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		// Run, not RunE: a returned error would print cobra's own error
		// text to stderr and, absent SilenceErrors (only guaranteed on
		// root, per root.go's own comment on why it sets that flag),
		// risk a nonzero exit — an interactive shell widget that shells
		// out to this command on every keystroke must never see either.
		// recover() additionally survives a genuine panic anywhere in
		// cobra's own Traverse/ValidArgsFunction machinery (a
		// third-party or future ValidArgsFunction could panic on
		// unexpected partial input) — "fail utterly silent" is this
		// command's entire design contract, not a best-effort nicety.
		Run: func(cmd *cobra.Command, args []string) {
			defer func() { _ = recover() }()
			hint := renderHint(cmd.Root(), hintTokens(args))
			if hint == "" {
				return
			}
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), hint)
		},
	}
}

// hintTokens reduces the raw args comrade __hint received down to just
// the shell buffer's own command-line tokens. Two things are stripped,
// in order:
//
//  1. The literal leading "--" separator: DisableFlagParsing (above)
//     means cobra never interprets "--" as a terminator for this
//     command, so it survives into args exactly as the shell widgets'
//     documented "comrade __hint -- <buffer tokens...>" invocation form
//     puts it (verified against a real DisableFlagParsing command's
//     args, not assumed).
//  2. The first remaining token, if it's how the user's shell buffer
//     invoked comrade itself — see isComradeInvocation.
func hintTokens(args []string) []string {
	if len(args) > 0 && args[0] == "--" {
		args = args[1:]
	}
	if len(args) > 0 && isComradeInvocation(args[0]) {
		args = args[1:]
	}
	return args
}

// isComradeInvocation reports whether token is how the user's shell
// buffer invoked comrade itself — bare "comrade", a full
// "/usr/local/bin/comrade" path, or Windows' "comrade.exe"/a full
// "C:\...\comrade.exe" path, compared case-insensitively — never a real
// subcommand or argument that happens to be spelled the same way.
//
// The basename split is done by hand (LastIndexAny on both "/" and
// "\\") rather than via path/filepath.Base: token is a raw shell-buffer
// string typed by the user, not a filesystem path being resolved on
// THIS process's own OS — a Linux-built comrade binary must still
// recognize a "C:\...\comrade.exe"-shaped token exactly like a
// Windows-built one would (and vice versa for a "/usr/.../comrade"
// token on Windows), which path/filepath.Base cannot do since it only
// ever treats runtime.GOOS's own separator as a separator.
func isComradeInvocation(token string) bool {
	base := token
	if idx := strings.LastIndexAny(base, `/\`); idx != -1 {
		base = base[idx+1:]
	}
	base = strings.TrimSuffix(strings.ToLower(base), ".exe")
	return base == "comrade"
}

// renderHint resolves tokens against root's own live command tree
// (root.Traverse — the exact same tree "comrade --help" and "comrade
// __complete" walk, so this can never drift from what the CLI actually
// supports) and renders the bracketed next-word hint cobra's own
// subcommand-name completion or ValidArgsFunction/ValidArgs would offer.
// It returns "" for absolutely any case that isn't a clean, confident
// match — an error from Traverse, an unmatched trailing token past a
// command that only has subcommands (never a real subcommand name), or
// a leaf command with nothing left to suggest. A hint must never
// mislead; printing nothing is always the safe fallback (Run's own doc
// comment).
func renderHint(root *cobra.Command, tokens []string) string {
	finalCmd, remaining, err := root.Traverse(tokens)
	if err != nil {
		return ""
	}

	if names := visibleSubcommandNames(finalCmd); len(names) > 0 {
		// Command.Traverse only ever returns a non-empty remaining slice
		// here when a token failed to match any of finalCmd's own
		// subcommand names (its findNext-returns-nil fallback — see
		// spf13/cobra@v1.10.2 command.go:821 Traverse). That is exactly
		// the "comrade bogus " / "comrade auth bogus " unknown-token
		// case this command must stay silent for, never fall back to
		// listing finalCmd's subcommands as though the bad token had
		// never been typed.
		if len(remaining) > 0 {
			return ""
		}
		return formatHintList(names)
	}

	if finalCmd.ValidArgsFunction != nil {
		completions, _ := finalCmd.ValidArgsFunction(finalCmd, remaining, "")
		return formatHintList(stripCompletionDescriptions(completions))
	}

	if len(finalCmd.ValidArgs) > 0 {
		return formatHintList(finalCmd.ValidArgs)
	}

	return ""
}

// visibleSubcommandNames (argvalidation.go) is reused here unchanged —
// see its own doc comment for the exact cobra filter it applies.

// stripCompletionDescriptions strips any cobra "name\tdescription"
// completion-with-description formatting down to bare names — a hint
// line has no room for prose, only the next word itself.
func stripCompletionDescriptions(completions []cobra.Completion) []string {
	names := make([]string, len(completions))
	for i, c := range completions {
		names[i] = strings.SplitN(c, "\t", 2)[0]
	}
	return names
}

// formatHintList renders names as "[a|b|c]" — the compact bracketed
// next-word hint form both the zsh and PowerShell widgets render
// verbatim. Once the joined names would grow past hintMaxLen characters
// ("comrade config get "'s 20+ config keys is the real case that
// triggers this, not a hypothetical), it truncates after the last name
// that still fits and appends "|…]" instead of running the hint off the
// edge of the terminal — but the very first name is always included in
// full regardless of its own length, so a single pathologically long
// name can never collapse the whole hint down to an empty "[|…]". An
// empty names returns "" (never a bare "[]"), so callers — and comrade
// __hint's own Run — can treat "" as this command's universal "nothing
// to suggest" signal.
func formatHintList(names []string) string {
	if len(names) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteByte('[')
	b.WriteString(names[0])
	for _, name := range names[1:] {
		// +1 for the "|" separator about to be written, +1 for the
		// closing ']' this line hasn't written yet.
		if b.Len()+1+len(name)+1 > hintMaxLen {
			b.WriteString("|…]")
			return b.String()
		}
		b.WriteByte('|')
		b.WriteString(name)
	}
	b.WriteByte(']')
	return b.String()
}
