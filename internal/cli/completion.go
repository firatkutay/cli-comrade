package cli

import (
	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/shellinit"
)

// completeFirstArgFromList returns a cobra.CompletionFunc offering
// candidates ONLY for a command's first positional argument: several
// leaf commands here (auth login/logout's provider, init's shell name,
// config get/set's key) take exactly one meaningful value-completable
// argument from a small, known, static set — secrets.KnownProviders,
// shellinitShellNames(), config.Keys() — so there is nothing left to
// suggest once len(args) > 0 (a second word has already been typed).
// Returning cobra.ShellCompDirectiveNoFileComp in BOTH cases (candidates
// offered or not) is what stops the shell from falling back to filename
// completion — cobra's own default (no ValidArgsFunction at all) is
// ShellCompDirectiveDefault, which DOES fall back to filenames, exactly
// the noisy behavior this project's completion wiring exists to avoid
// for arguments that are never a file path.
func completeFirstArgFromList(candidates []string) cobra.CompletionFunc {
	return func(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		if len(args) != 0 {
			return nil, cobra.ShellCompDirectiveNoFileComp
		}
		return candidates, cobra.ShellCompDirectiveNoFileComp
	}
}

// shellinitShellNames returns shellinit.All (the four shells "comrade
// init" supports) as plain strings, in shellinit's own fixed display
// order — the derive-don't-hand-maintain source for init's completion
// candidates, so a future fifth supported shell is picked up here
// automatically instead of needing a second, hand-copied list.
func shellinitShellNames() []string {
	names := make([]string, len(shellinit.All))
	for i, s := range shellinit.All {
		names[i] = string(s)
	}
	return names
}
