package cli

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// TestColorizeHelpTextStylesHeadersCommandsAndFlags pins colorizeHelpText's
// exact output against a fixed, representative --help block (a trimmed-down
// stand-in for real cobra/pflag output — same row shapes, same section
// headers) — exact ANSI codes, not just "contains an escape code", per
// this project's "assert exact values" testing convention.
func TestColorizeHelpTextStylesHeadersCommandsAndFlags(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	input := "comrade is a cross-platform AI CLI companion for the terminal\n" +
		"\n" +
		"Usage:\n" +
		"  comrade [command]\n" +
		"\n" +
		"Core:\n" +
		"  do          Generate a plan\n" +
		"  fix         Diagnose\n" +
		"\n" +
		"Flags:\n" +
		"      --auto      run in auto mode\n" +
		"  -h, --help      help for comrade\n"

	got := colorizeHelpText(input, tr)

	want := "comrade is a cross-platform AI CLI companion for the terminal\n" +
		"\n" +
		"\x1b[1;38;5;183mUsage:\x1b[m\n" +
		"  comrade [command]\n" +
		"\n" +
		"\x1b[1;38;5;183mCore:\x1b[m\n" +
		"  \x1b[38;5;115mdo\x1b[m          Generate a plan\n" +
		"  \x1b[38;5;115mfix\x1b[m         Diagnose\n" +
		"\n" +
		"\x1b[1;38;5;183mFlags:\x1b[m\n" +
		"      \x1b[38;5;216m--auto\x1b[m      run in auto mode\n" +
		"  \x1b[38;5;216m-h, --help\x1b[m      help for comrade\n"

	assert.Equal(t, want, got)
}

// TestColorizeHelpTextRecognizesResolvedLanguageGroupTitlesAsHeaders proves
// colorizeHelpText treats the CURRENT Translator's own group titles as
// section headers too, not just cobra's hardcoded English ones — e.g. TR's
// "Temel:" (Core's TR title), which never appears anywhere in cobra's own
// template strings.
func TestColorizeHelpTextRecognizesResolvedLanguageGroupTitlesAsHeaders(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangTR)
	input := "Temel:\n  do          Bir plan üretir\n"

	got := colorizeHelpText(input, tr)

	want := "\x1b[1;38;5;183mTemel:\x1b[m\n" +
		"  \x1b[38;5;115mdo\x1b[m          Bir plan üretir\n"

	assert.Equal(t, want, got)
}

// TestColorizeHelpTextLeavesUsageAndExampleBodyLinesUnstyled proves the
// section-state machine does NOT mistake "Usage:"/"Examples:" body lines
// (which have the exact same "  word ..." shape as a command-list row) for
// command rows — those two sections render argv-shaped text
// (`comrade [command]`, `comrade install docker`), not a name+description
// table, and must never be recolored as if they were.
func TestColorizeHelpTextLeavesUsageAndExampleBodyLinesUnstyled(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	input := "Usage:\n  comrade [command]\n\nExamples:\n  comrade install docker   # do mode\n"

	got := colorizeHelpText(input, tr)

	want := "\x1b[1;38;5;183mUsage:\x1b[m\n" +
		"  comrade [command]\n\n" +
		"\x1b[1;38;5;183mExamples:\x1b[m\n" +
		"  comrade install docker   # do mode\n"

	assert.Equal(t, want, got)
}

// TestHelpOutputColorizedWhenCliColorForced is the end-to-end proof (task's
// own explicit ANSI-verification requirement): a piped, non-TTY --help
// invocation with CLICOLOR_FORCE=1 set actually emits visible ANSI escape
// codes, through the real root command, not just colorizeHelpText in
// isolation.
func TestHelpOutputColorizedWhenCliColorForced(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("CLICOLOR_FORCE", "1")
	t.Setenv("NO_COLOR", "")

	out := execRoot(t, "dev", "--help")

	assert.Contains(t, out, "\x1b[1;38;5;183mCore:\x1b[m")
	assert.Contains(t, out, "\x1b[38;5;115mdo\x1b[m")
}

// TestHelpOutputStaysPlainWithoutCliColorForce is
// TestHelpOutputColorizedWhenCliColorForced's negative counterpart: the
// SAME piped invocation, with CLICOLOR_FORCE unset, must contain zero
// escape codes — this is what every pre-existing Contains-style help test
// implicitly depends on continuing to hold.
func TestHelpOutputStaysPlainWithoutCliColorForce(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("CLICOLOR_FORCE", "")
	t.Setenv("NO_COLOR", "")

	out := execRoot(t, "dev", "--help")

	assert.NotContains(t, out, "\x1b[")
}
