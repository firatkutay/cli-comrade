package cli

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/firatkutay/cli-comrade/internal/config"
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
// groupCore/groupSetup/groupInfo are root's three cobra command-group IDs
// (root.go's root.AddGroup / each subcommand's GroupID) — plain string
// constants, never user-visible themselves; only each *cobra.Group's Title
// (groupTitleByID below) is.
const (
	groupCore  = "core"
	groupSetup = "setup"
	groupInfo  = "info"
)

// groupTitleByID maps a *cobra.Group's ID (root.go's root.AddGroup) to the
// MessageID for its rendered Title — applyTranslatedHelp overwrites every
// registered group's Title from this, exactly like helpShortByPath does
// for a command's Short text, immediately before cobra renders
// --help/usage.
var groupTitleByID = map[string]i18n.MessageID{
	groupCore:  i18n.MsgHelpGroupCore,
	groupSetup: i18n.MsgHelpGroupSetup,
	groupInfo:  i18n.MsgHelpGroupInfo,
}

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
// Short text (applyTranslatedHelp) in the resolved language, then renders
// through cobra's own default rendering (captured here, BEFORE being
// overridden, so the actual template/formatting logic is untouched — only
// the Short strings feeding it change) — colorized per resolveColorEnabled
// (internal/cli's single color-decision point; see color.go) when that
// resolves true, completely unchanged (still cobra's own byte-for-byte
// plain output) when it resolves false. Setting this once on root is
// sufficient for the whole tree: cobra's Command.HelpFunc()/UsageFunc()
// walk up to the nearest ancestor with one set when a child has none of
// its own, and no command in this tree ever sets its own.
func registerTranslatedHelp(root *cobra.Command, newLoader loaderFactory) {
	defaultHelp := root.HelpFunc()
	defaultUsage := root.UsageFunc()

	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		tr, cfg := applyTranslatedHelp(root, newLoader)
		_ = renderMaybeColorized(cmd, cfg, tr, cmd.OutOrStdout(), func() { defaultHelp(cmd, args) })
	})
	root.SetUsageFunc(func(cmd *cobra.Command) error {
		tr, cfg := applyTranslatedHelp(root, newLoader)
		var usageErr error
		writeErr := renderMaybeColorized(cmd, cfg, tr, cmd.OutOrStderr(), func() { usageErr = defaultUsage(cmd) })
		if usageErr != nil {
			return usageErr
		}
		return writeErr
	})
}

// renderMaybeColorized runs render (cobra's own captured default Help/Usage
// func, writing to whatever cmd.OutOrStdout()/OutOrStderr() resolves to at
// call time) and, when resolveColorEnabled(cfg, os.Environ(), target)
// resolves true, intercepts that output into a buffer first and rewrites
// it through colorizeHelpText before writing the colorized result to
// target — target is cmd's REAL resolved writer, captured by the caller
// BEFORE this call (so it is correct regardless of whether a test harness
// or production code path set it). When color is disabled, render's
// output goes straight to target, completely unbuffered and untouched —
// this is what keeps every existing plain-text golden/Contains-style help
// test byte-identical to before this function existed. render itself never
// reports an error (cobra's own HelpFunc is void; its UsageFunc's error is
// captured by the caller separately, into the closure render assigns to,
// not through this return) — the only error this CAN return is target's
// own final colorized write failing.
func renderMaybeColorized(cmd *cobra.Command, cfg config.Config, tr i18n.Translator, target io.Writer, render func()) error {
	if !resolveColorEnabled(cfg, os.Environ(), target) {
		render()
		return nil
	}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	render()
	cmd.SetOut(target)
	_, err := fmt.Fprint(target, colorizeHelpText(buf.String(), tr))
	return err
}

// applyTranslatedHelp resolves the active Translator/Config (helpConfig)
// and, walking every command in root's tree: overwrites Short when
// CommandPath() is a helpShortByPath key, AND overwrites every one of
// that command's own (non-inherited) flags' Usage when the flag's name is
// a flagUsageByName key — cobra/pflag read both fields directly off the
// struct when rendering, so both need the same lazy, render-time
// override. It also overwrites root's own Example text and every
// registered *cobra.Group's Title (root.go's root.AddGroup) from the same
// catalog — cobra's default usage template renders both verbatim, so they
// need identical render-time localization to the Short/flag text above.
// Returns the resolved Translator and Config so registerTranslatedHelp's
// caller can reuse both for color resolution without a second config load.
func applyTranslatedHelp(root *cobra.Command, newLoader loaderFactory) (i18n.Translator, config.Config) {
	tr, cfg := helpTranslatorAndConfig(newLoader)

	root.Example = tr.T(i18n.MsgHelpExamplesRoot)
	root.SetUsageTemplate(usageTemplateFor(tr))
	for _, group := range root.Groups() {
		if id, ok := groupTitleByID[group.ID]; ok {
			group.Title = tr.T(id)
		}
	}

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
	return tr, cfg
}

// usageTemplateFor builds a full cobra usage-template replacement
// (root.SetUsageTemplate — inherited tree-wide, exactly like
// SetHelpFunc/SetUsageFunc) translating cobra's own eight hardcoded
// structural section labels (QA D4b: "Usage:"/"Aliases:"/"Examples:"/
// "Available Commands:"/"Additional Commands:"/"Flags:"/"Global Flags:"/
// "Additional help topics:", plus the trailing "Use \"...\" for more
// information..." line) into tr's resolved language.
//
// This is a byte-for-byte structural COPY of spf13/cobra v1.10.2's own
// unexported defaultUsageTemplate (command.go) — same fields, same
// control flow, same whitespace/newline placement — with ONLY the eight
// literal English labels swapped for tr.T(...) calls. This is a
// deliberate, KNOWN version-drift risk (Derive-or-Guard): if a future
// cobra upgrade changes defaultUsageTemplate's shape (a new section, a
// reordered field, a different padding rule), this copy will silently
// stop matching it and must be manually re-synced by diffing this
// function against cobra's own defaultUsageTemplate at that version —
// there is no way to programmatically derive this from cobra itself
// (defaultUsageTemplate is unexported, and cobra provides no
// label-override hook, only whole-template replacement). Mitigated by:
// (1) go.mod pins cobra to an EXACT version (supply-chain-pinning), so
// this can only go stale on a deliberate, reviewable version bump, never
// silently at build time; (2) TestUsageTemplateForMatchesCobraDefaultShape
// (help_test.go) renders a representative command tree through BOTH this
// template and cobra's own untouched default and asserts they produce
// IDENTICAL structure (line count, section order) once labels are
// stripped back out — a real guard that fails loudly if this copy ever
// drifts from upstream, not just a comment promising it won't.
//
// tr.T is called with ZERO args for every one of these IDs — including
// MsgHelpMoreInfo, whose catalog value embeds cobra's own literal
// "{{.CommandPath}}" template syntax — because Translator.T's own
// contract is that a zero-arg call returns the catalog string completely
// unchanged (never run through fmt.Sprintf), which is exactly what's
// needed here: this function is producing raw TEMPLATE SOURCE for cobra
// to parse and re-execute per-command, per-render, not a one-shot
// rendered string.
func usageTemplateFor(tr i18n.Translator) string {
	usage := tr.T(i18n.MsgHelpLabelUsage)
	aliases := tr.T(i18n.MsgHelpLabelAliases)
	examples := tr.T(i18n.MsgHelpLabelExamples)
	availableCommands := tr.T(i18n.MsgHelpLabelAvailableCommands)
	additionalCommands := tr.T(i18n.MsgHelpLabelAdditionalCommands)
	flags := tr.T(i18n.MsgHelpLabelFlags)
	globalFlags := tr.T(i18n.MsgHelpLabelGlobalFlags)
	additionalHelpTopics := tr.T(i18n.MsgHelpLabelAdditionalHelpTopics)
	moreInfo := tr.T(i18n.MsgHelpMoreInfo)

	return usage + `{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

` + aliases + `
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

` + examples + `
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}{{$cmds := .Commands}}{{if eq (len .Groups) 0}}

` + availableCommands + `{{range $cmds}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{else}}{{range $group := .Groups}}

{{.Title}}{{range $cmds}}{{if (and (eq .GroupID $group.ID) (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if not .AllChildCommandsHaveGroup}}

` + additionalCommands + `{{range $cmds}}{{if (and (eq .GroupID "") (or .IsAvailableCommand (eq .Name "help")))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

` + flags + `
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

` + globalFlags + `
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasHelpSubCommands}}

` + additionalHelpTopics + `{{range .Commands}}{{if .IsAdditionalHelpTopicCommand}}
  {{rpad .CommandPath .CommandPathPadding}} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableSubCommands}}

` + moreInfo + `{{end}}
`
}

// helpTranslatorAndConfig resolves both the Translator help text uses AND
// the Config resolveColorEnabled uses, from the SAME config load: config
// general.language/general.color when a config load succeeds (loader.
// Load() may create a default config file as a side effect on first run,
// exactly like every other command already does — "--help" is not
// special-cased away from that); otherwise (a construction-time loader
// failure) falls back to envOnlyTranslator (runtime.go — the same env-only
// resolution root.go's bare-invocation version banner and
// executionFlags.modeFlagValue's pre-config error both use) paired with a
// zero-value config.Config, whose General.Color is false — i.e. a broken
// config fails closed to plain, uncolored help text, never a crash.
func helpTranslatorAndConfig(newLoader loaderFactory) (i18n.Translator, config.Config) {
	loader, err := newLoader()
	if err == nil {
		if cfg, _, loadErr := loader.Load(); loadErr == nil {
			tr := i18n.NewTranslator(i18n.ResolveLanguage(cfg.General.Language, os.Getenv, i18n.SystemLocale))
			return tr, *cfg
		}
	}
	return envOnlyTranslator(), config.Config{}
}

// helpHeaderStyle/helpCommandNameStyle/helpFlagNameStyle are colorizeHelpText's
// three pastel styles (Part 2(b)): section headers/group titles in a bold
// soft lavender, command names in a pastel cyan/teal, flag names (including
// their one-letter shorthand, e.g. "-h, --help") in a pastel peach — chosen
// as fixed ANSI256 codes rather than through lipgloss's AdaptiveColor/
// compat package: charm.land/lipgloss/v2/compat's HasDarkBackground and
// Profile package-level vars are initialized by a LIVE terminal query
// (lipgloss.HasDarkBackground -> BackgroundColor, an OSC 11 query with up
// to a 2-second blocking timeout per charm.land/lipgloss/v2/terminal.go)
// that runs unconditionally at that package's import/init time — merely
// importing charm.land/lipgloss/v2/compat anywhere in this binary would
// pay that cost (and put the terminal in raw mode) on every interactive
// invocation, including a bare `comrade --help`, which this codebase's own
// cold-start-hardening history (see go.mod's atotto/clipboard replace
// comment, FAZ 11) treats as exactly the class of regression to avoid.
// These three codes are deliberately mid-tone/desaturated ("pastel") so
// they stay legible on both a light and a dark 256-color terminal without
// needing any runtime background detection at all.
var (
	helpHeaderStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(paletteLavender))
	helpCommandNameStyle = lipgloss.NewStyle().Foreground(lipgloss.Color(paletteCyan))
	helpFlagNameStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color(palettePeach))
)

// helpCommandRowPattern matches a command-list row exactly as cobra's own
// defaultUsageTemplate renders one — "  {{rpad .Name .NamePadding}}
// {{.Short}}" — capturing the two leading spaces, the (unpadded) name
// token, the padding+separator whitespace, and the rest of the line
// (Short) as four groups. It requires the first non-space character to be
// a letter specifically so it never matches a flag row (every flag row
// starts with either six spaces or "-x, ", i.e. a dash, not a letter,
// immediately after its own leading whitespace) — the two row shapes are
// structurally disjoint by design, so a single, stateful, section-aware
// pass (colorizeHelpText) can apply the right regex to the right rows
// without the two ever needing to be told apart by content.
var helpCommandRowPattern = regexp.MustCompile(`^(  )([A-Za-z][\w.-]*)(\s{2,})(.*)$`)

// helpFlagRowPattern matches a flag-list row as pflag's FlagUsages()
// renders one: either "      --name..." (long-only) or "  -x, --name..."
// (with a one-letter shorthand) — capturing the leading whitespace, the
// "-x, --name" or "--name" token itself, and the rest of the line as three
// groups.
var helpFlagRowPattern = regexp.MustCompile(`^(\s*)(-\w, --[\w-]+|--[\w-]+)(.*)$`)

// colorizeHelpText re-colors an already-fully-rendered (plain-text,
// correctly-aligned) cobra --help/usage block, wrapping only the specific
// substrings the three styles above target in ANSI escape codes —
// alignment/padding is computed ONCE by cobra/pflag against the plain
// text, before any of this runs, and is never recomputed here: adding
// zero-width ANSI codes around an already-correctly-padded substring
// cannot change a real terminal's on-screen column alignment (only its
// color), which is exactly why this operates on the fully rendered string
// rather than trying to inject color earlier, into cmd.Name()/flag names
// themselves, where the same escape codes would corrupt cobra's own
// plain-byte-length padding math.
//
// tr is the same resolved Translator applyTranslatedHelp already used, so
// this recognizes the CURRENT language's own group titles (Core:/Temel:
// etc.) AND (QA D4b) cobra's own eight structural section labels — both
// are rendered through usageTemplateFor's translated template now, so
// both must be recognized in the SAME resolved language, never a fixed
// English literal, or a TR render's headers would simply never match and
// silently stay unstyled (exactly the bug this fixes: colorizeHelpText
// used to hardcode the English label text directly, which broke the
// moment usageTemplateFor started rendering translated ones).
func colorizeHelpText(text string, tr i18n.Translator) string {
	commandSectionHeaders := map[string]bool{
		tr.T(i18n.MsgHelpLabelAvailableCommands):    true,
		tr.T(i18n.MsgHelpLabelAdditionalCommands):   true,
		tr.T(i18n.MsgHelpLabelAdditionalHelpTopics): true,
		tr.T(i18n.MsgHelpGroupCore):                 true,
		tr.T(i18n.MsgHelpGroupSetup):                true,
		tr.T(i18n.MsgHelpGroupInfo):                 true,
	}
	flagSectionHeaders := map[string]bool{
		tr.T(i18n.MsgHelpLabelFlags):       true,
		tr.T(i18n.MsgHelpLabelGlobalFlags): true,
	}
	plainHeaders := map[string]bool{
		tr.T(i18n.MsgHelpLabelUsage):    true,
		tr.T(i18n.MsgHelpLabelAliases):  true,
		tr.T(i18n.MsgHelpLabelExamples): true,
	}

	type section int
	const (
		sectionOther section = iota
		sectionCommands
		sectionFlags
	)

	lines := strings.Split(text, "\n")
	cur := sectionOther
	for i, line := range lines {
		trimmed := strings.TrimRight(line, "\r")
		switch {
		case commandSectionHeaders[trimmed]:
			cur = sectionCommands
			lines[i] = helpHeaderStyle.Render(trimmed)
			continue
		case flagSectionHeaders[trimmed]:
			cur = sectionFlags
			lines[i] = helpHeaderStyle.Render(trimmed)
			continue
		case plainHeaders[trimmed]:
			cur = sectionOther
			lines[i] = helpHeaderStyle.Render(trimmed)
			continue
		case trimmed == "":
			// A blank line never itself changes section — every section
			// transition in cobra's template is via an explicit header
			// line (handled above), never inferred from blank lines
			// alone (e.g. flag rows are never blank-separated from each
			// other within "Flags:").
			continue
		}
		switch cur {
		case sectionCommands:
			if m := helpCommandRowPattern.FindStringSubmatch(line); m != nil {
				lines[i] = m[1] + helpCommandNameStyle.Render(m[2]) + m[3] + m[4]
			}
		case sectionFlags:
			if m := helpFlagRowPattern.FindStringSubmatch(line); m != nil {
				lines[i] = m[1] + helpFlagNameStyle.Render(m[2]) + m[3]
			}
		}
	}
	return strings.Join(lines, "\n")
}
