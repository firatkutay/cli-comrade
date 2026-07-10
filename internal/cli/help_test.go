package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// buildRepresentativeCommandTree builds a *cobra.Command tree that
// exercises ALL EIGHT sections usageTemplateFor's template can render —
// not just 6 of them — so the byte-identical shape comparison
// (TestUsageTemplateForMatchesCobraDefaultShapeInEnglish) actually
// covers every fragment usageTemplateFor hand-copied from cobra's own
// defaultUsageTemplate, rather than leaving Aliases:/Additional
// Commands:/Additional help topics: unexercised (any upstream drift
// specifically in one of THOSE three fragments would otherwise pass the
// guard silently, since a never-rendered branch can never disagree):
//
//   - Usage:                — root (Runnable + HasAvailableSubCommands)
//   - Examples:             — root.Example
//   - Flags:                — root's own persistent "global-flag" (its
//     OWN local flags, from root's own --help) AND sub's own local
//     "flag" (from sub's own --help)
//   - Aliases:               — sub has Aliases: []string{"s"} (renders on
//     SUB's own --help, not root's)
//   - Available Commands:    — sub itself registers NO Group of its own,
//     and has one visible child (subsub), so sub's OWN --help takes the
//     "no Groups" template branch
//   - Additional Commands:   — root DOES register a Group ("grp"), sub is
//     assigned to it, but "ungrouped" is a second, group-less child —
//     AllChildCommandsHaveGroup() is false because of it, so root's own
//     --help renders "Additional Commands:" for "ungrouped" (cobra's
//     Available-Commands/Additional-Commands branches are mutually
//     exclusive PER RENDER — that's why this needs sub's OWN --help for
//     "Available Commands:" and root's for "Additional Commands:", two
//     different commands' renders, not one)
//   - Global Flags:          — sub's own --help shows root's persistent
//     "global-flag" as INHERITED (root has no parent, so this section
//     never renders on root's own --help — only on a child's)
//   - Additional help topics: — "topic" has no Run/RunE and no
//     subcommands of its own, so IsAdditionalHelpTopicCommand() is true;
//     it also fails IsAvailableCommand() (not Runnable, no subcommands),
//     so it renders ONLY there, never double-counted under Additional
//     Commands too
func buildRepresentativeCommandTree() *cobra.Command {
	root := &cobra.Command{
		Use:     "widget",
		Short:   "widget short",
		Example: "  widget sub --flag value",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	root.PersistentFlags().Bool("global-flag", false, "a persistent flag")
	root.AddGroup(&cobra.Group{ID: "grp", Title: "Grouped:"})

	sub := &cobra.Command{
		Use:     "sub",
		Short:   "sub short",
		Aliases: []string{"s"},
		GroupID: "grp",
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	sub.Flags().String("flag", "", "a local flag")
	root.AddCommand(sub)

	subsub := &cobra.Command{
		Use:   "subsub",
		Short: "subsub short",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	sub.AddCommand(subsub)

	ungrouped := &cobra.Command{
		Use:   "ungrouped",
		Short: "ungrouped short",
		RunE:  func(*cobra.Command, []string) error { return nil },
	}
	root.AddCommand(ungrouped)

	topic := &cobra.Command{
		Use:   "topic",
		Short: "topic short",
	}
	root.AddCommand(topic)

	return root
}

// renderRootAndSubUsage runs "widget --help" and "widget sub --help"
// against root (whatever usage template/help func is currently set on
// it) and returns both rendered bodies concatenated — a single
// comparable string covering every section usageTemplateFor can
// produce: root's own --help carries Usage/Examples/Flags/Additional
// Commands/Additional help topics; sub's own --help carries
// Aliases/Available Commands/Flags/Global Flags (see
// buildRepresentativeCommandTree's own doc comment for exactly why each
// section renders where it does).
func renderRootAndSubUsage(t *testing.T, root *cobra.Command) string {
	t.Helper()
	var out bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&out)

	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())

	out.Reset()
	root.SetArgs([]string{"sub", "--help"})
	require.NoError(t, root.Execute())
	subOut := out.String()

	var rootOut bytes.Buffer
	root.SetOut(&rootOut)
	root.SetErr(&rootOut)
	root.SetArgs([]string{"--help"})
	require.NoError(t, root.Execute())

	return rootOut.String() + "\n---\n" + subOut
}

// TestUsageTemplateForMatchesCobraDefaultShapeInEnglish is
// usageTemplateFor's own doc-comment-promised drift guard (QA D4b):
// usageTemplateFor is a hand-maintained structural COPY of spf13/cobra's
// own unexported defaultUsageTemplate, with only the eight literal
// English section labels swapped for tr.T(...) calls. Since this
// project's EN catalog values for those eight IDs are byte-identical to
// cobra's own hardcoded English text (see catalog.go's MsgHelpLabel*
// entries), rendering the SAME representative command tree through (a)
// usageTemplateFor(EN translator) and (b) cobra's own completely
// untouched default template must produce BYTE-IDENTICAL output. If a
// future cobra version changes defaultUsageTemplate's shape (a new
// section, reordered fields, different padding), this comparison stops
// matching and this test fails loudly — the signal to manually re-sync
// usageTemplateFor against that version's actual template, rather than
// silently drifting.
func TestUsageTemplateForMatchesCobraDefaultShapeInEnglish(t *testing.T) {
	translated := buildRepresentativeCommandTree()
	translated.SetUsageTemplate(usageTemplateFor(i18n.NewTranslator(i18n.LangEN)))
	translatedOut := renderRootAndSubUsage(t, translated)

	stockDefault := buildRepresentativeCommandTree()
	// No SetUsageTemplate call at all: stockDefault uses cobra's own,
	// completely untouched defaultUsageTemplate.
	defaultOut := renderRootAndSubUsage(t, stockDefault)

	assert.Equal(t, defaultOut, translatedOut)
}

// TestColorizeHelpTextRecognizesTranslatedStructuralHeaders is QA D4b's
// direct regression guard for the bug the coordinator's follow-up round
// specifically flagged: usageTemplateFor renders TRANSLATED structural
// headers now, so colorizeHelpText's own header-recognizer maps must
// look for the SAME resolved language's labels, not a hardcoded English
// literal — otherwise a TR render's "Kullanım:"/"Bayraklar:" would never
// match and silently stay uncolored even with color enabled.
func TestColorizeHelpTextRecognizesTranslatedStructuralHeaders(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangTR)
	input := "Kullanım:\n  comrade [command]\n\nBayraklar:\n  -h, --help      yardım\n"

	got := colorizeHelpText(input, tr)

	want := "\x1b[1;38;5;183mKullanım:\x1b[m\n" +
		"  comrade [command]\n\n" +
		"\x1b[1;38;5;183mBayraklar:\x1b[m\n" +
		"  \x1b[38;5;216m-h, --help\x1b[m      yardım\n"

	assert.Equal(t, want, got)
}

// TestRootHelpStructuralHeadersRenderInTurkish is the end-to-end proof,
// through the real root command: every one of usageTemplateFor's eight
// structural labels that a root --help render actually reaches (Usage/
// Examples/Flags; Aliases/Global Flags/Additional help topics never
// render for this tree — no command has Aliases, root has no parent so
// no inherited flags, and no command is an "additional help topic")
// appears in Turkish, and none of the English originals do.
func TestRootHelpStructuralHeadersRenderInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	out := execRoot(t, "dev", "--help")

	assert.Contains(t, out, "Kullanım:")
	assert.Contains(t, out, "Örnekler:")
	assert.Contains(t, out, "Bayraklar:")
	assert.Contains(t, out, `Bir komut hakkında daha fazla bilgi için "comrade [command] --help" kullanın.`)
	assert.NotContains(t, out, "\nUsage:")
	assert.NotContains(t, out, "\nExamples:")
	assert.NotContains(t, out, "\nFlags:")
	assert.NotContains(t, out, "Use \"comrade")
}

// TestCompletionCommandIsHiddenFromHelpButStillFunctional is QA D4b's
// "hide, don't disable" judgment call: cobra's auto-added "completion"
// command (several KB of its own internal, un-i18n-able generated help
// text) never appears in --help output, but "comrade completion bash"
// still actually works for a user who already knows to ask for it.
func TestCompletionCommandIsHiddenFromHelpButStillFunctional(t *testing.T) {
	withIsolatedConfigDir(t)

	out := execRoot(t, "dev", "--help")
	assert.NotContains(t, out, "completion")

	completionOut, _, err := execRootSplit(t, "dev", "completion", "bash")
	require.NoError(t, err)
	assert.Contains(t, completionOut, "bash completion")
}

// TestHookHelpRendersNonEmptyUsageLine is QA D5's regression guard:
// "comrade hook --help" used to render a completely blank "Usage:" line
// (Hidden "hook" whose only child, "hook record", is ALSO Hidden, so
// neither of cobra's two Usage-line branches — Runnable or
// HasAvailableSubCommands — ever fired). newHookCmd's RunE (hook.go)
// fixes this by making hook Runnable; "hook" itself must still be
// completely absent from root's own --help listing.
func TestHookHelpRendersNonEmptyUsageLine(t *testing.T) {
	withIsolatedConfigDir(t)

	out := execRoot(t, "dev", "hook", "--help")
	assert.Contains(t, out, "Usage:\n  comrade hook [flags]")
	assert.NotContains(t, out, "Usage:\n\n", "the Usage: line must never render empty")

	rootOut := execRoot(t, "dev", "--help")
	assert.NotContains(t, rootOut, "\n  hook ", "hook must stay hidden from root's own --help listing")
}
