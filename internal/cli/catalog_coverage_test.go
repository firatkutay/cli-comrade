package cli

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"testing"
	"unicode"
)

// fmtVerbPattern matches a single Go fmt verb (e.g. "%s", "%d", "%-6.2f",
// "%%") so containsProseLetter can strip every verb out of a format
// string before checking what's left for actual prose — otherwise the
// verb letter itself (the "s" in "%s", the "v" in "%v", ...) would be
// indistinguishable from real text and defeat the whole heuristic.
var fmtVerbPattern = regexp.MustCompile(`%[-+ #0]*[0-9]*(\.[0-9]+)?[a-zA-Z%]`)

// catalogCoverageAllowlist names every non-test .go file under
// internal/cli and internal/tui that still has at least one raw,
// letter-containing string literal passed straight to an fmt.Print*/
// Fprint*/Println/Printf call OR a pflag flag-registration's description
// argument, rather than routed through an i18n.Translator's T() method
// (Print*/Fprint*) or enUsageDefault (flag descriptions). As of FAZ 9's
// full sweep (see docs/phases/FAZ-09.md), this is a MINIMAL,
// explicitly-justified list. Only ONE entry remains:
//
//   - hook.go: recordLastCommand's COMRADE_DEBUG-gated diagnostic line
//     runs on every shell prompt (FAZ 4's hot path), AND `hook record`'s
//     three flag descriptions (--shell/--exit/--command). Both are
//     developer-facing (an explicit debug env var; an internal command
//     invoked only by generated shell snippets, never typed by an end
//     user), and loading config there just to resolve a display language
//     would add overhead to code that fires on every single shell
//     prompt — a deliberate performance tradeoff, not an oversight. See
//     docs/phases/FAZ-09.md's "KEEP as-is" section.
//
// This allowlist is intentionally a DENYLIST-OF-EXISTING-DEBT, not a
// blanket exemption: every file NOT listed here is fully covered by
// TestCatalogCoverageNoNewHardcodedUserFacingStrings, and
// TestCatalogCoverageAllowlistHasNoStaleEntries fails the moment any
// listed file's last flagged literal is migrated away, forcing its entry
// to be deleted rather than left as unjustified dead weight.
var catalogCoverageAllowlist = map[string]string{
	"hook.go":    "recordLastCommand's COMRADE_DEBUG-gated diagnostic line, and hook record's --shell/--exit/--command flag descriptions, are developer-facing (debug-gated / internal-only, invoked by generated shell snippets, never read by an end user via --help); loading config there just to resolve a display language is a deliberate perf tradeoff against a hot path, not an oversight.",
	"spinner.go": `startWaitSpinner's line-clear write is the literal "\r\x1b[K" — a raw ANSI cursor-return + erase-in-line control sequence, not language text; containsProseLetter's letter-detection heuristic (this file) has no way to distinguish the "K" that ends that escape code from real prose, so it flags this one non-prose literal. There is nothing here for any i18n.Translator to translate.`,
}

// catalogCoverageScanDirs are the only packages this drift guard covers —
// UYGULAMA_PLANI.md FAZ 9 scopes the coverage test to "cli/tui packages"
// specifically; internal/engine's own pre-existing (FAZ 5-8) printed
// output is a separate, larger, more heavily-tested surface this phase
// did not attempt to fully migrate (see docs/phases/FAZ-09.md).
var catalogCoverageScanDirs = []string{".", "../tui"}

// fmtPrintSelectors is the exact set of fmt functions this scan
// recognizes as "prints text a user might see" — Sprintf/Sprintln/Sprint
// and Errorf are deliberately excluded: they build strings/errors that
// may or may not ever reach a terminal (an error can be swallowed,
// wrapped again, logged, or compared in a test), so treating every
// Sprintf/Errorf literal as "must be i18n" would both over-flag internal
// error-wrapping text (CLAUDE.md's own `fmt.Errorf("...: %w", err)`
// convention) and under-cover nothing extra Print*/Fprint* doesn't
// already catch for what's actually shown to the user directly. See
// docs/phases/FAZ-09.md's "full-sentence fmt.Errorf" section for the
// separate, manually-applied rule that DID migrate a bounded set of
// standalone user-facing fmt.Errorf/errors.New messages — deliberately
// NOT automated here, because a robust wrap-vs-standalone heuristic for
// arbitrary Go source is fragile (see this file's own doc comment on
// TestCatalogCoverageNoNewHardcodedUserFacingStrings for what a fragile
// heuristic would risk: false positives on every "doing X: %w" wrap
// chain, which would need per-case suppression and defeat the point of
// an enforcing gate).
var fmtPrintSelectors = map[string]bool{
	"Print": true, "Println": true, "Printf": true,
	"Fprint": true, "Fprintln": true, "Fprintf": true,
}

// flagRegistrationSelectors is the set of pflag.FlagSet registration
// method names whose LAST argument is always the flag's description
// ("usage" in pflag's own vocabulary) — Bool/BoolP/BoolVar/BoolVarP,
// String.../Int.../Duration.../StringSlice... all share that shape
// (pointer-or-not, name, [shorthand], default value, usage), so "last
// argument" is a reliable, shape-independent rule across every variant.
// Matched purely by method name (not by resolving the receiver's static
// type via go/types) — a pragmatic scope limit acceptable because this
// scan only ever runs over internal/cli/internal/tui, where every such
// method is called on a *pflag.FlagSet obtained from cmd.Flags().
var flagRegistrationSelectors = map[string]bool{
	"Bool": true, "BoolP": true, "BoolVar": true, "BoolVarP": true,
	"String": true, "StringP": true, "StringVar": true, "StringVarP": true,
	"Int": true, "IntP": true, "IntVar": true, "IntVarP": true,
	"Duration": true, "DurationVar": true,
	"StringSlice": true, "StringSliceVar": true,
}

// TestCatalogCoverageNoNewHardcodedUserFacingStrings is UYGULAMA_PLANI.md
// FAZ 9's "katalog dışı string linter'ı" acceptance test: it statically
// scans every non-test .go file directly under internal/cli and
// internal/tui (catalogCoverageScanDirs) for:
//
//  1. a call to one of fmtPrintSelectors whose format/text argument is a
//     raw, letter-containing string literal (as opposed to a variable, a
//     tr.T(...) call result, or a format string built entirely of verbs/
//     whitespace/punctuation, e.g. "%s\n" or "%d\t%s\n" — those need no
//     translation and are exempt by construction, not by allowlist);
//  2. a call to one of flagRegistrationSelectors (a pflag flag
//     registration) whose LAST argument — the flag's description — is
//     likewise a raw, letter-containing string literal, rather than an
//     enUsageDefault(id) call (help.go); and
//  3. a call to any WriteString(...) method (matched by method name only,
//     regardless of receiver — this catches *strings.Builder.WriteString
//     just as well as an io.StringWriter) whose single argument is a raw,
//     letter-containing string literal. Added when internal/tui/
//     confirm.go's ask-mode prompt (previously this scan's one documented
//     blind spot — see the historical note below) was migrated to render
//     through an i18n.Translator: the fix is only as durable as the guard
//     that keeps it from regressing, so the guard was extended at the
//     same time the literals it now needs to see were removed.
//
// What this DOES catch: a NEW raw literal of ANY of the three shapes
// added to any file not already in catalogCoverageAllowlist — the drift
// guard's whole point, so a future contributor who hardcodes a new
// user-facing message, a new hardcoded flag description, or a new
// WriteString'd prompt line in (say) explain.go, confirm.go, or a new
// command fails this test immediately.
//
// What this CANNOT catch (documented, not silently assumed away):
//   - A literal built via string concatenation/Sprintf-of-a-Sprintf
//     before reaching Print*/WriteString (this scan only inspects the
//     immediate call argument's AST shape).
//   - The ~12 standalone, full-sentence fmt.Errorf/errors.New user-facing
//     error messages this phase migrated (see docs/phases/FAZ-09.md) —
//     Errorf/errors.New are deliberately excluded from fmtPrintSelectors
//     (see its own doc comment) because a reliable AST-level rule to
//     distinguish "a complete sentence the user reads as the terminal
//     error" from "a `doing X: %w` wrap chain" does not exist without
//     false positives on the dozens of legitimate wrap chains throughout
//     this codebase; that migration was applied manually, one call site
//     at a time, against the exact rule stated in
//     docs/phases/FAZ-09.md — this test does not (and, per the
//     coordinator's own instruction, deliberately does not try to)
//     enforce that rule going forward. A future added Errorf/errors.New
//     is NOT caught by this test either way.
//   - Text embedded in a cobra `Use` command-token string (deliberately
//     untranslated by design — see docs/phases/FAZ-09.md).
//   - A file already in catalogCoverageAllowlist growing MORE
//     letter-containing literals — that pre-existing debt is exempted by
//     file, not by count; see TestCatalogCoverageAllowlistHasNoStaleEntries
//     for the guard that at least keeps the allowlist itself honest.
//
// Historical note: internal/tui/confirm.go's prompt used to be exactly
// the blind spot rule 3 above closes — it rendered via
// strings.Builder.WriteString (invisible to a scan that only recognized
// fmt.Print*/Fprint*), and was hardcoded Turkish regardless of
// general.language. That was fixed (confirm.go now renders through
// i18n.MsgConfirmLegend/MsgConfirmEditHeader, resolved per the active
// Translator, with mapKey accepting a disjoint per-language key set —
// see confirm.go's own doc comments for the TR/EN key-collision
// rationale) in the same change that added rule 3, so this scan now
// actually enforces confirm.go going forward instead of merely
// documenting that it couldn't.
func TestCatalogCoverageNoNewHardcodedUserFacingStrings(t *testing.T) {
	for _, dir := range catalogCoverageScanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if entry.IsDir() || filepath.Ext(name) != ".go" || len(name) > 8 && name[len(name)-8:] == "_test.go" {
				continue
			}
			if reason, ok := catalogCoverageAllowlist[name]; ok {
				_ = reason
				continue
			}

			path := filepath.Join(dir, name)
			for _, lit := range findRawPrintLiterals(t, path) {
				t.Errorf("%s: raw string literal %q passed to a fmt.Print*/Fprint* call — route it through an i18n.Translator's T() method instead", path, lit)
			}
			for _, lit := range findRawFlagDescriptions(t, path) {
				t.Errorf("%s: raw string literal %q used as a flag description — route it through enUsageDefault(id) (help.go) instead", path, lit)
			}
			for _, lit := range findRawWriteStringLiterals(t, path) {
				t.Errorf("%s: raw string literal %q passed to a WriteString(...) call — route it through an i18n.Translator's T() method instead", path, lit)
			}
		}
	}
}

// TestCatalogCoverageAllowlistHasNoStaleEntries keeps
// catalogCoverageAllowlist itself honest: every listed file must still
// exist AND still actually contain at least one flagged literal (of any
// of the three shapes — Print/Fprint text, a flag description, or a
// WriteString call) — an entry for a file that was since fully migrated
// (zero remaining flagged literals) must be removed, not left as dead
// weight silently widening the exemption.
func TestCatalogCoverageAllowlistHasNoStaleEntries(t *testing.T) {
	for _, dir := range catalogCoverageScanDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("read dir %s: %v", dir, err)
		}
		for _, entry := range entries {
			name := entry.Name()
			if _, ok := catalogCoverageAllowlist[name]; !ok {
				continue
			}
			path := filepath.Join(dir, name)
			total := len(findRawPrintLiterals(t, path)) + len(findRawFlagDescriptions(t, path)) + len(findRawWriteStringLiterals(t, path))
			if total == 0 {
				t.Errorf("catalogCoverageAllowlist[%q] is stale: this file no longer has any flagged literal — remove its entry", name)
			}
		}
	}
}

// findRawPrintLiterals parses the Go source file at path and returns
// every raw string literal (unquoted) passed as the format/text argument
// of an fmt.Print*/Fprint* call (fmtPrintSelectors) that contains at
// least one Unicode letter — a pragmatic proxy for "this looks like
// user-facing prose", as opposed to a bare format string like "%s\n" or a
// bracket/punctuation-only literal.
func findRawPrintLiterals(t *testing.T, path string) []string {
	t.Helper()
	file := parseGoFile(t, path)

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		pkgIdent, ok := sel.X.(*ast.Ident)
		if !ok || pkgIdent.Name != "fmt" || !fmtPrintSelectors[sel.Sel.Name] {
			return true
		}

		args := call.Args
		// Fprint/Fprintln/Fprintf's first argument is the io.Writer, not
		// the text — skip it. Print/Println/Printf have no writer arg.
		if len(sel.Sel.Name) > 1 && sel.Sel.Name[0] == 'F' {
			if len(args) < 2 {
				return true
			}
			args = args[1:]
		}
		if len(args) == 0 {
			return true
		}

		if text, ok := proseLiteral(args[0]); ok {
			found = append(found, text)
		}
		return true
	})
	return found
}

// findRawFlagDescriptions parses the Go source file at path and returns
// every raw, letter-containing string literal used as a flag
// registration's (flagRegistrationSelectors) description argument — the
// LAST argument of the call, regardless of which variant (Bool/BoolVar/
// BoolVarP/...) is used, per flagRegistrationSelectors' own doc comment.
func findRawFlagDescriptions(t *testing.T, path string) []string {
	t.Helper()
	file := parseGoFile(t, path)

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !flagRegistrationSelectors[sel.Sel.Name] {
			return true
		}
		if len(call.Args) == 0 {
			return true
		}

		if text, ok := proseLiteral(call.Args[len(call.Args)-1]); ok {
			found = append(found, text)
		}
		return true
	})
	return found
}

// findRawWriteStringLiterals parses the Go source file at path and
// returns every raw, letter-containing string literal passed as the sole
// argument of a WriteString(...) call. Matched purely by method name
// (like flagRegistrationSelectors' own pragmatic scope limit) rather than
// resolving the receiver's static type via go/types — deliberately not
// restricted to *strings.Builder specifically, so this also catches, say,
// a bytes.Buffer or a hand-rolled io.StringWriter used the same way. A
// WriteString call with any argument count other than exactly one (which
// would not even compile against the real WriteString(string) signature,
// but this is a syntax-only AST scan, not a type-checked one) is skipped
// rather than guessed at.
func findRawWriteStringLiterals(t *testing.T, path string) []string {
	t.Helper()
	file := parseGoFile(t, path)

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "WriteString" {
			return true
		}
		if len(call.Args) != 1 {
			return true
		}

		if text, ok := proseLiteral(call.Args[0]); ok {
			found = append(found, text)
		}
		return true
	})
	return found
}

// parseGoFile parses the Go source file at path, failing the test on any
// parse error — shared by findRawPrintLiterals/findRawFlagDescriptions/
// findRawWriteStringLiterals so none of them has to duplicate fileset/
// parser setup.
func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

// proseLiteral reports whether expr is a raw string literal containing
// prose (containsProseLetter) and, if so, returns its unquoted text.
func proseLiteral(expr ast.Expr) (string, bool) {
	lit, ok := expr.(*ast.BasicLit)
	if !ok || lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	if !containsProseLetter(text) {
		return "", false
	}
	return text, true
}

// containsProseLetter reports whether s, AFTER stripping every fmt verb
// (fmtVerbPattern — "%s", "%d", "%-6.2f", "%%", ...), still has at least
// one Unicode letter — the heuristic that separates a pure format string
// with NO letters outside its verbs (e.g. "%s\n", "%d) %s\n", "%s = %s\n",
// which need no translation) from one that also carries actual prose
// (e.g. "%d executed, %d skipped, %d blocked\n" — "executed"/"skipped"/
// "blocked" survive the strip and correctly get flagged).
func containsProseLetter(s string) bool {
	stripped := fmtVerbPattern.ReplaceAllString(s, "")
	for _, r := range stripped {
		if unicode.IsLetter(r) {
			return true
		}
	}
	return false
}
