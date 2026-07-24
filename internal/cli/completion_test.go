package cli

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// completionCandidates runs "comrade __complete <args...>" — cobra's own
// shell-completion request protocol (spf13/cobra/completions.go's
// initCompleteCmd): the LAST word is the partial word being completed
// ("" for "just hit Tab after a trailing space"), every word before it
// is what's already been typed on the command line. It returns the
// candidate NAMES cobra printed, in emission order (cobra.Commands()'s
// own alphabetical sort for subcommand-name completion; a
// ValidArgsFunction's own order for value completion — e.g.
// secrets.KnownProviders is NOT alphabetical, so callers that care about
// order assert on it explicitly), stripped of any tab-separated
// description, plus the trailing ":<directive>" line's integer value.
func completionCandidates(t *testing.T, args ...string) (candidates []string, directive int) {
	t.Helper()
	fullArgs := append([]string{"__complete"}, args...)
	stdout, _, err := execRootSplit(t, "dev", fullArgs...)
	require.NoError(t, err, "the __complete pseudo-command itself must never fail")

	lines := strings.Split(strings.TrimRight(stdout, "\n"), "\n")
	require.NotEmpty(t, lines, "expected at least the trailing directive line")

	last := lines[len(lines)-1]
	require.True(t, strings.HasPrefix(last, ":"), "expected the last line to be cobra's own \":<directive>\" line, got: %q", last)
	d, err := strconv.Atoi(strings.TrimPrefix(last, ":"))
	require.NoError(t, err)
	directive = d

	for _, line := range lines[:len(lines)-1] {
		name := strings.SplitN(line, "\t", 2)[0]
		candidates = append(candidates, name)
	}
	return candidates, directive
}

// TestCompleteRootSuggestsTopLevelCommandsExcludingHidden proves
// subcommand-name completion works out of the box at the root level
// (cobra's own built-in behavior, no ValidArgsFunction needed for this
// part) — every visible top-level command appears exactly once, in
// cobra's own alphabetical Commands() order, and the Hidden ones (hook,
// completion — see root.go's CompletionOptions.HiddenDefaultCmd and
// hook.go's Hidden: true) never appear at all.
func TestCompleteRootSuggestsTopLevelCommandsExcludingHidden(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "")

	assert.Equal(t, []string{
		"auth", "chat", "config", "do", "doctor", "explain", "fix",
		"help", "history", "init", "undo", "upgrade",
	}, candidates)
	assert.NotContains(t, candidates, "hook")
	assert.NotContains(t, candidates, "completion")
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteAuthSuggestsSubcommands proves subcommand-name completion
// one level deep, under a parent this round gave its own RunE/Args
// (translatedUnknownSubcommand) — proving that change didn't disturb
// cobra's built-in subcommand completion.
func TestCompleteAuthSuggestsSubcommands(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "auth", "")

	assert.Equal(t, []string{"login", "logout", "status"}, candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteConfigSuggestsSubcommandsExcludingHidden proves "config"'s
// subcommand completion excludes its Hidden "test-llm" diagnostic
// command.
func TestCompleteConfigSuggestsSubcommandsExcludingHidden(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "config", "")

	assert.Equal(t, []string{"edit", "get", "list", "models", "path", "profile", "set"}, candidates)
	assert.NotContains(t, candidates, "test-llm")
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteAuthLoginSuggestsKnownProviders proves auth login's
// ValidArgsFunction (completeFirstArgFromList, completion.go) offers
// secrets.KnownProviders for its first argument, in that slice's own
// order (not re-sorted).
func TestCompleteAuthLoginSuggestsKnownProviders(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "auth", "login", "")

	assert.Equal(t, secrets.KnownProviders, candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteAuthLogoutSuggestsKnownProviders is auth login's
// counterpart for logout.
func TestCompleteAuthLogoutSuggestsKnownProviders(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "auth", "logout", "")

	assert.Equal(t, secrets.KnownProviders, candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteAuthLoginSecondArgSuggestsNothing proves
// completeFirstArgFromList only offers candidates for the FIRST
// positional argument: once a provider is already typed, completing the
// (nonexistent) second argument returns no candidates, but still
// NoFileComp — never cobra's file-completion fallback.
func TestCompleteAuthLoginSecondArgSuggestsNothing(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "auth", "login", "anthropic", "")

	assert.Empty(t, candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteInitSuggestsShellNames proves init's ValidArgsFunction
// offers shellinit.All's four shell names, in shellinit's own fixed
// display order.
func TestCompleteInitSuggestsShellNames(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "init", "")

	assert.Equal(t, []string{"bash", "zsh", "fish", "powershell"}, candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteConfigGetSuggestsKnownKeys proves config get's
// ValidArgsFunction reuses config.Keys() (the schema's own single
// source of truth — internal/config/validate.go) rather than a
// hand-maintained second list.
func TestCompleteConfigGetSuggestsKnownKeys(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "config", "get", "")

	assert.Equal(t, config.Keys(), candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteConfigSetFirstArgSuggestsKnownKeys proves config set's
// ValidArgsFunction fires DESPITE DisableFlagParsing being set on that
// command (config.go's own comment on this documents exactly why cobra
// still calls it: DisableFlagParsing only short-circuits the '-'-prefixed
// flag-name-completion branch, never the general ValidArgsFunction call
// at the end of getCompletions) — offering config.Keys() for the first
// (key) argument.
func TestCompleteConfigSetFirstArgSuggestsKnownKeys(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "config", "set", "")

	assert.Equal(t, config.Keys(), candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteConfigSetSecondArgSuggestsNothing proves config set's
// value argument (the second word) offers no candidates — this round's
// spec asked for first-arg-only key completion, not a value-aware
// second-arg completer.
func TestCompleteConfigSetSecondArgSuggestsNothing(t *testing.T) {
	withIsolatedConfigDir(t)

	candidates, directive := completionCandidates(t, "config", "set", "general.mode", "")

	assert.Empty(t, candidates)
	assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
}

// TestCompleteFreeTextCommandsSuggestNoFileCompletions proves every
// free-text command (do, explain, fix, chat) returns NO candidates AND
// ShellCompDirectiveNoFileComp — NOT cobra's ShellCompDirectiveDefault
// fallback, which falls back to filename completion. This is the
// regression this test guards: cobra's OWN default (no ValidArgsFunction
// at all) is Default/file-completion, so this only passes because each
// command explicitly sets ValidArgsFunction: cobra.NoFileCompletions.
func TestCompleteFreeTextCommandsSuggestNoFileCompletions(t *testing.T) {
	for _, cmdName := range []string{"do", "explain", "fix", "chat"} {
		t.Run(cmdName, func(t *testing.T) {
			withIsolatedConfigDir(t)

			candidates, directive := completionCandidates(t, cmdName, "")

			assert.Empty(t, candidates)
			assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive,
				"expected NoFileComp, not cobra's file-completion-fallback Default directive")
		})
	}
}

// TestCompleteNoArgsCommandsSuggestNoFileCompletions is the same
// no-file-completion-fallback proof for every command that takes no
// positional arguments at all (history, upgrade, config list/edit/path/
// models, auth status).
func TestCompleteNoArgsCommandsSuggestNoFileCompletions(t *testing.T) {
	cases := [][]string{
		{"history", ""},
		{"upgrade", ""},
		{"config", "list", ""},
		{"config", "edit", ""},
		{"config", "path", ""},
		{"config", "models", ""},
		{"auth", "status", ""},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			withIsolatedConfigDir(t)

			candidates, directive := completionCandidates(t, args...)

			assert.Empty(t, candidates)
			assert.Equal(t, int(cobra.ShellCompDirectiveNoFileComp), directive)
		})
	}
}

// TestCompletionRequestNeverLoadsConfigOrTouchesStderr proves
// "comrade __complete ..." — the same machinery every real shell's Tab
// keypress invokes on every partial word — never loads/creates a config
// file (checked directly on disk, not just "no first-run notice
// observed") and never writes anything to stderr BEYOND cobra's own
// unconditional "Completion ended with directive: ..." protocol line
// (spf13/cobra/completions.go's initCompleteCmd always prints exactly
// that one line, regardless of what this project's own code does) — so
// completion stays fast and silent enough to run on every keystroke,
// with no first-run notice or other comrade-side stderr output mixed
// in.
func TestCompletionRequestNeverLoadsConfigOrTouchesStderr(t *testing.T) {
	dir := withIsolatedConfigDir(t)

	_, stderr, err := execRootSplit(t, "dev", "__complete", "auth", "login", "")

	require.NoError(t, err)
	assert.Equal(t, "Completion ended with directive: ShellCompDirectiveNoFileComp\n", stderr,
		"expected ONLY cobra's own protocol line, no comrade-side output (e.g. a first-run notice) mixed in")
	_, statErr := os.Stat(filepath.Join(dir, "cli-comrade", "config.toml"))
	assert.True(t, os.IsNotExist(statErr), "the completion request must never create a config file as a side effect")
}
