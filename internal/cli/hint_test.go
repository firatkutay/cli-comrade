package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/secrets"
	"github.com/firatkutay/cli-comrade/internal/update"
)

// execHint runs "comrade __hint -- <buffer...>" — the exact invocation
// form the zsh/PowerShell space-triggered widgets use (see
// internal/shellinit/snippets/) — and returns stdout with its single
// trailing newline trimmed, plus stderr verbatim (expected empty on
// every path: hint.go's Run only ever writes to OutOrStdout).
func execHint(t *testing.T, buffer ...string) (hint, stderr string) {
	t.Helper()
	args := append([]string{"__hint", "--"}, buffer...)
	stdout, errOut, err := execRootSplit(t, "dev", args...)
	require.NoError(t, err, "comrade __hint must never itself fail")
	return strings.TrimRight(stdout, "\n"), errOut
}

// TestHintTableDriven is this feature's core contract: for a given
// partially-typed buffer, __hint prints exactly the bracketed next-word
// list (or nothing) that cobra's own command tree/ValidArgsFunction
// would offer — see hint.go's renderHint doc comment for the full
// resolution rules each case below exercises.
func TestHintTableDriven(t *testing.T) {
	rootList := "[auth|chat|config|do|doctor|explain|fix|help|history|init|undo|upgrade]"
	authProviders := "[" + strings.Join(secrets.KnownProviders, "|") + "]"

	cases := []struct {
		name   string
		buffer []string
		want   string
	}{
		{"root lists top-level commands", []string{"comrade"}, rootList},
		{"root with no tokens at all", nil, rootList},
		{"auth lists its subcommands", []string{"comrade", "auth"}, "[login|logout|status]"},
		{"auth login lists known providers via ValidArgsFunction", []string{"comrade", "auth", "login"}, authProviders},
		{"auth logout lists known providers via ValidArgsFunction", []string{"comrade", "auth", "logout"}, authProviders},
		{"config lists its subcommands", []string{"comrade", "config"}, "[edit|get|list|models|path|profile|set]"},
		{"init lists supported shell names", []string{"comrade", "init"}, "[bash|zsh|fish|powershell]"},
		{"unknown top-level token prints nothing", []string{"comrade", "bogus"}, ""},
		{"unknown nested token prints nothing", []string{"comrade", "auth", "bogus"}, ""},
		{"comrade.exe first token is stripped", []string{"comrade.exe", "auth"}, "[login|logout|status]"},
		{"COMRADE.EXE first token is stripped case-insensitively", []string{"COMRADE.EXE", "auth"}, "[login|logout|status]"},
		{"a full comrade path is stripped", []string{"/usr/local/bin/comrade", "auth"}, "[login|logout|status]"},
		{"a Windows comrade.exe path is stripped", []string{`C:\Program Files\cli-comrade\comrade.exe`, "auth"}, "[login|logout|status]"},
		{"second positional arg to a first-arg-only completer suggests nothing", []string{"comrade", "auth", "login", "anthropic"}, ""},
		{"a free-text leaf command (NoFileCompletions) suggests nothing", []string{"comrade", "do"}, ""},
		{"config get's second positional arg suggests nothing", []string{"comrade", "config", "get", "general.mode"}, ""},
		// "--bogus=val" is an "=" form flag, so Command.Traverse collects
		// it as a pending flag and only calls ParseFlags on configCmd once
		// it next matches a real subcommand name ("get") — configCmd has
		// no "--bogus" flag, so that ParseFlags call itself fails and
		// Traverse returns a non-nil error (verified directly against
		// root.Traverse in isolation, not merely assumed). renderHint's
		// own "if err != nil { return \"\" }" branch is what this covers.
		{"an invalid flag encountered mid-traversal prints nothing", []string{"comrade", "config", "--bogus=val", "get"}, ""},
		// __hint itself is a real (if Hidden) leaf command with no
		// subcommands, no ValidArgsFunction, and no ValidArgs — the one
		// real path in this tree that reaches renderHint's final
		// "return \"\"" fallthrough rather than one of its two named
		// branches.
		{"__hint itself has nothing to suggest", []string{"comrade", "__hint"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withIsolatedConfigDir(t)
			hint, stderr := execHint(t, tc.buffer...)
			assert.Equal(t, tc.want, hint)
			assert.Empty(t, stderr, "comrade __hint must never write to stderr")
		})
	}
}

// TestHintWorksWithoutALeadingDoubleDash proves hintTokens' "--" strip
// is defensive, not required: a caller that (unlike the documented
// "comrade __hint -- <buffer...>" contract) omits the separator still
// gets a correct hint, because isComradeInvocation's own strip runs
// regardless.
func TestHintWorksWithoutALeadingDoubleDash(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, _, err := execRootSplit(t, "dev", "__hint", "comrade", "auth")
	require.NoError(t, err)
	assert.Equal(t, "[login|logout|status]", strings.TrimRight(stdout, "\n"))
}

// TestHintConfigGetTruncatesLongKeyList proves the real "comrade config
// get " case — config.Keys() is long enough that the joined "[a|b|...]"
// form exceeds hintMaxLen — is truncated with a trailing "|…]" rather
// than printed in full or suppressed entirely, and that the names
// actually included are a genuine prefix of config.Keys()'s own sorted
// order (never a hand-copied, driftable second list).
func TestHintConfigGetTruncatesLongKeyList(t *testing.T) {
	withIsolatedConfigDir(t)

	hint, stderr := execHint(t, "comrade", "config", "get")

	require.True(t, strings.HasPrefix(hint, "["), "expected a bracketed hint, got: %q", hint)
	require.True(t, strings.HasSuffix(hint, "|…]"), "expected config get's key list to be truncated with a trailing ellipsis, got: %q", hint)
	assert.LessOrEqual(t, len(hint), hintMaxLen+len("|…]"),
		"the ellipsis-truncated hint must never grow far past hintMaxLen")
	assert.Empty(t, stderr)

	inner := strings.TrimSuffix(strings.TrimPrefix(hint, "["), "|…]")
	gotKeys := strings.Split(inner, "|")
	require.NotEmpty(t, gotKeys)
	allKeys := config.Keys()
	require.Greater(t, len(allKeys), len(gotKeys), "config.Keys() must be long enough to actually exercise truncation")
	assert.Equal(t, allKeys[:len(gotKeys)], gotKeys, "the truncated hint must be an exact, in-order prefix of config.Keys()")
}

// TestRenderHintUsesBareValidArgsWhenNoFunctionIsSet proves renderHint's
// "else if ... ValidArgs" branch: no command in this repo's own tree
// currently sets the bare ValidArgs field directly (every leaf uses
// ValidArgsFunction instead — completion.go's completeFirstArgFromList),
// so this exercises that branch of cobra's own documented contract
// ("only one of ValidArgs and ValidArgsFunction can be used") against a
// small synthetic tree instead, rather than leaving it silently
// uncovered.
func TestRenderHintUsesBareValidArgsWhenNoFunctionIsSet(t *testing.T) {
	root := &cobra.Command{Use: "comrade"}
	leaf := &cobra.Command{Use: "leaf", ValidArgs: []string{"one", "two", "three"}}
	root.AddCommand(leaf)

	assert.Equal(t, "[one|two|three]", renderHint(root, []string{"leaf"}))
}

// TestFormatHintListJoinsNamesInBracketForm pins formatHintList's exact
// output for a short list that fits comfortably under hintMaxLen — no
// truncation involved, just the plain "[a|b|c]" join.
func TestFormatHintListJoinsNamesInBracketForm(t *testing.T) {
	assert.Equal(t, "[login|logout|status]", formatHintList([]string{"login", "logout", "status"}))
}

// TestFormatHintListEmptyIsEmptyString proves an empty candidate list
// renders as "" (comrade __hint's own universal "print nothing" signal),
// never a bare, useless "[]".
func TestFormatHintListEmptyIsEmptyString(t *testing.T) {
	assert.Equal(t, "", formatHintList(nil))
	assert.Equal(t, "", formatHintList([]string{}))
}

// TestFormatHintListTruncatesPastMaxLen pins the exact truncation
// boundary against a synthetic, hand-verified 26-name list (the NATO
// alphabet) — independent of config.Keys()'s own real, driftable
// content — so a regression in the cap/ellipsis arithmetic itself fails
// here even if config.Keys() ever changes size.
func TestFormatHintListTruncatesPastMaxLen(t *testing.T) {
	names := []string{
		"alpha", "bravo", "charlie", "delta", "echo", "foxtrot", "golf",
		"hotel", "india", "juliett", "kilo", "lima", "mike", "november",
		"oscar", "papa", "quebec", "romeo", "sierra", "tango", "uniform",
		"victor", "whiskey", "xray", "yankee", "zulu",
	}
	got := formatHintList(names)
	want := "[alpha|bravo|charlie|delta|echo|foxtrot|golf|hotel|india|juliett|kilo|lima|mike|november|…]"
	assert.Equal(t, want, got)
	assert.True(t, strings.HasSuffix(got, "|…]"))
}

// TestFormatHintListAlwaysIncludesFirstNameEvenIfOverlong proves a
// single pathologically long first name is still emitted in full rather
// than being replaced outright by "[|…]" — formatHintList's own doc
// comment's stated guarantee.
func TestFormatHintListAlwaysIncludesFirstNameEvenIfOverlong(t *testing.T) {
	longName := strings.Repeat("x", hintMaxLen+20)
	got := formatHintList([]string{longName, "short"})
	assert.Equal(t, "["+longName+"|…]", got)
}

// TestHintCommandIsHiddenFromHelpAndCompletion proves "comrade __hint"
// never leaks into --help output or "comrade __complete"'s own
// subcommand-name completion — it is an internal entry point only,
// exactly like "hook" (hook.go) and cobra's own "completion"/"__complete".
func TestHintCommandIsHiddenFromHelpAndCompletion(t *testing.T) {
	withIsolatedConfigDir(t)

	helpOut := execRoot(t, "dev", "--help")
	assert.NotContains(t, helpOut, "__hint")

	candidates, _ := completionCandidates(t, "")
	assert.NotContains(t, candidates, "__hint")
}

// TestHintNeverTouchesConfigOrNetworkOnRealVersion proves comrade
// __hint's zero-config/zero-network contract holds even on a REAL
// (non-"dev") build — using "dev" everywhere else in this file would
// only prove the update-notice's OWN update.IsDevBuild short-circuit
// skipped it, not that __hint's explicit PersistentPostRunE bypass
// (root.go) actually fired. A countingFetcher that flips called=true on
// any invocation, combined with never creating a config file on disk,
// is proof neither maybeNotifyUpdate's config load nor its network call
// ever ran.
func TestHintNeverTouchesConfigOrNetworkOnRealVersion(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	fetcherCalled := false
	fetcher := countingFetcher{called: &fetcherCalled, release: update.Release{TagName: "v9.9.9"}}

	root := newRootCmd("v1.2.3", fetcher)
	var outBuf, errBuf strings.Builder
	root.SetOut(&outBuf)
	root.SetErr(&errBuf)
	root.SetArgs([]string{"__hint", "--", "comrade", "auth"})
	require.NoError(t, root.Execute())

	assert.Equal(t, "[login|logout|status]", strings.TrimRight(outBuf.String(), "\n"))
	assert.Empty(t, errBuf.String())
	assert.False(t, fetcherCalled, "comrade __hint must never trigger the background update-release fetch")

	_, statErr := os.Stat(filepath.Join(dir, "cli-comrade", "config.toml"))
	assert.True(t, os.IsNotExist(statErr), "comrade __hint must never create a config file as a side effect")
}
