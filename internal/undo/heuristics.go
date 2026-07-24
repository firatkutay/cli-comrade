// Package undo implements `comrade undo`'s deterministic, LLM-independent
// heuristic tier: a narrow, table-driven set of exact-shape command
// reversals (mirroring internal/safety's own denylist: a small, explicit
// table rather than any attempt at general shell understanding). Every
// rule here is pure, stdlib-only, and side-effect-free — Derive never
// touches the filesystem, network, or clock; it only maps one recorded
// command string to the shell command(s) that would undo it, or reports
// that it does not recognize the shape at all (internal/cli's own
// LLM-assisted tier — internal/engine.Undoer — is what handles anything
// this package does not).
//
// DECIDED (see the task's own design note): this package deliberately
// does NOT include a `cp A B` -> `rm B` heuristic. Unlike every rule
// below, a copy's target is not verifiably safe to delete after the fact
// — B might have existed before the copy, might have been modified since,
// or might not even be the same file `cp` actually wrote (a directory
// copy, a glob, `-r`/`-a`...). That class of undo is left to the LLM/
// manual tier, which can reason about the specific case instead of a
// blind, unconditional table rule.
package undo

import "strings"

// Recorded is the one audit.Entry's worth of information Derive needs
// about a step that already executed, translated into this package's own
// vocabulary so it never has to import internal/audit itself (a
// package-local, consumer-side shape — the same "accept the minimal
// interface/struct a caller can already build" pattern
// internal/engine.Completer uses for its own dependency).
type Recorded struct {
	// Command is the exact command text that ran.
	Command string
	// Cwd is the working directory the command ran in (may be empty for
	// a pre-undo-support recording — see audit.Entry.Cwd).
	Cwd string
	// GOOS selects which command dialect Derive expects Command to be
	// written in: "windows" for PowerShell syntax (comrade's own
	// windows executor always runs `powershell -Command`), anything else
	// for POSIX shell syntax.
	GOOS string
	// ExitCode is the command's recorded exit code — callers are expected
	// to skip a nonzero-exit step before ever calling Derive (it never
	// took effect, so there is nothing to reverse), but Derive itself
	// does not re-check this: it is a pure mapping over Command/GOOS only.
	ExitCode int
}

// Derived is Derive's successful result: the shell command(s) that
// reverse Recorded.Command, in the dialect Recorded.GOOS implies.
type Derived struct {
	// Commands is one or more commands that, run in order, reverse the
	// recorded command's effect. Every current rule produces exactly one,
	// but the shape allows a future rule to need more than one without a
	// breaking change.
	Commands []string
	// Caveat is an optional, plain-English (deliberately untranslated —
	// see doctor.Result.Fix's identical precedent: a shell-adjacent
	// technical note, not prose to route through i18n) qualification a
	// caller should surface alongside Commands — e.g. Windows'
	// Remove-Item only actually deleting an empty directory.
	Caveat string
	// UsesRelativePath reports whether at least one filesystem path this
	// derivation operated on is relative (not rooted) — internal/cli's
	// own undo pipeline downgrades a step to its LLM/manual tier instead
	// of blindly trusting Commands when this is true AND the step's
	// Recorded.Cwd differs from the directory undo itself is about to run
	// in (see that package's own doc comment on why this must never be
	// silently rewritten). Always false for a derivation with no
	// filesystem path at all (a service/package-manager reversal).
	UsesRelativePath bool
}

// unixPackageInstallVerbs maps a Unix package manager's binary name to
// its own removal verb — the reverse of "install". apt and apt-get are
// both listed (distinct binaries in practice, but either may appear in a
// recorded command) so the reconstructed command always uses whichever
// one the user's original command actually invoked.
var unixPackageInstallVerbs = map[string]string{
	"apt":     "remove",
	"apt-get": "remove",
	"dnf":     "remove",
	"brew":    "uninstall",
}

// windowsPackageInstallVerbs is unixPackageInstallVerbs' Windows sibling:
// winget and scoop, both fronting an "uninstall" removal verb.
var windowsPackageInstallVerbs = map[string]string{
	"winget": "uninstall",
	"scoop":  "uninstall",
}

// systemctlOpposites maps a systemctl action this table recognizes to
// its own reverse action.
var systemctlOpposites = map[string]string{
	"enable": "disable",
	"start":  "stop",
}

// Derive attempts to map r.Command to the command(s) that would reverse
// it, dispatching to the Windows (PowerShell) or Unix (POSIX shell) rule
// set purely by r.GOOS — comrade's own executor always runs a
// Windows-recorded command through PowerShell and a non-Windows one
// through the platform shell (see CLAUDE.md's "Dizin Yapısı" /
// internal/executor), so the two dialects never mix within one recorded
// command. ok is false whenever no rule in the applicable table
// recognizes command's exact shape — including an empty command, and
// including a recognized command WORD used with any flag/argument
// combination this narrow table does not specifically enumerate (e.g.
// `mkdir -p x`, `mkdir a b`) — a deliberate fail-closed default: an
// unrecognized shape falls through to internal/cli's LLM/manual tier
// rather than this package guessing.
func Derive(r Recorded) (Derived, bool) {
	if strings.TrimSpace(r.Command) == "" {
		return Derived{}, false
	}
	if r.GOOS == "windows" {
		return deriveWindows(r)
	}
	return deriveUnix(r)
}

// deriveUnix dispatches every POSIX-shell rule this table recognizes:
// mkdir/rmdir, mv, the Unix package managers' install/remove pair, and
// systemctl enable|start/disable|stop. A leading "sudo" is recognized and
// preserved (re-prefixed onto the derived command) but otherwise never
// consulted here — internal/safety.Engine already escalates any command
// containing "sudo" to RiskElevated on its own (see its own escalation
// rule), so this package does not need to track/report elevation itself.
func deriveUnix(r Recorded) (Derived, bool) {
	tokens := strings.Fields(r.Command)
	sudo := false
	if len(tokens) > 0 && tokens[0] == "sudo" {
		sudo = true
		tokens = tokens[1:]
	}
	if len(tokens) == 0 {
		return Derived{}, false
	}

	switch tokens[0] {
	case "mkdir":
		return deriveMkdir(tokens, sudo, r.GOOS)
	case "mv":
		return deriveMv(tokens, sudo, r.GOOS)
	case "systemctl":
		return deriveSystemctl(tokens, sudo)
	}

	if verb, ok := unixPackageInstallVerbs[tokens[0]]; ok {
		return derivePackageInstall(tokens, sudo, verb)
	}
	return Derived{}, false
}

// deriveWindows dispatches every PowerShell rule this table recognizes:
// New-Item -ItemType Directory (-> Remove-Item), and the Windows package
// managers' install/uninstall pair.
func deriveWindows(r Recorded) (Derived, bool) {
	tokens := strings.Fields(r.Command)
	if len(tokens) == 0 {
		return Derived{}, false
	}

	if strings.EqualFold(tokens[0], "New-Item") {
		return deriveNewItemDirectory(tokens)
	}

	if verb, ok := windowsPackageInstallVerbs[strings.ToLower(tokens[0])]; ok {
		return derivePackageInstall(tokens, false, verb)
	}
	return Derived{}, false
}

// deriveMkdir matches `[sudo] mkdir <path>` — EXACTLY one argument, no
// flags at all (rejecting `-p`, `-m 0755`, or a second path — see the
// package doc comment's "narrow" design: any of those shapes falls
// through to the LLM/manual tier instead of this rule guessing which
// path(s), if any, are still safe to rmdir).
func deriveMkdir(tokens []string, sudo bool, goos string) (Derived, bool) {
	if len(tokens) != 2 || tokens[0] != "mkdir" {
		return Derived{}, false
	}
	path := tokens[1]
	if strings.HasPrefix(path, "-") {
		return Derived{}, false
	}

	return Derived{
		Commands:         []string{prefixSudo("rmdir "+path, sudo)},
		UsesRelativePath: !isAbsolutePath(path, goos),
	}, true
}

// deriveMv matches `[sudo] mv <A> <B>` — EXACTLY two plain path
// arguments (no flags, no glob/wildcard characters in either), reversed
// as `mv <B> <A>`. Any flag (`-f`, `-n`, ...), any extra argument (three
// or more sources plus a destination), or a wildcard in either path falls
// through to the LLM/manual tier — none of those shapes is safe to
// reverse with a single, unconditional mv.
func deriveMv(tokens []string, sudo bool, goos string) (Derived, bool) {
	if len(tokens) != 3 || tokens[0] != "mv" {
		return Derived{}, false
	}
	a, b := tokens[1], tokens[2]
	if hasFlagOrWildcard(a) || hasFlagOrWildcard(b) {
		return Derived{}, false
	}

	return Derived{
		Commands:         []string{prefixSudo("mv "+b+" "+a, sudo)},
		UsesRelativePath: !isAbsolutePath(a, goos) || !isAbsolutePath(b, goos),
	}, true
}

// derivePackageInstall matches `[sudo] <manager> install <args...>` for
// any manager already present in the caller's verb map (Unix's
// apt/apt-get/dnf/brew, or Windows' winget/scoop), reversing it as
// `<manager> <removalVerb> <args...>` — every argument after "install" is
// carried through completely unchanged (package name(s), and any flag
// the original install used), so this rule never has to parse or
// second-guess package-manager-specific flag syntax.
func derivePackageInstall(tokens []string, sudo bool, removalVerb string) (Derived, bool) {
	if len(tokens) < 3 || tokens[1] != "install" {
		return Derived{}, false
	}
	rest := strings.Join(tokens[2:], " ")
	cmd := tokens[0] + " " + removalVerb + " " + rest
	return Derived{Commands: []string{prefixSudo(cmd, sudo)}}, true
}

// deriveSystemctl matches `[sudo] systemctl <enable|start> [flags...] <unit>`,
// reversing the action (enable->disable, start->stop) while preserving
// every flag verbatim (e.g. `--now`) and requiring EXACTLY one remaining,
// non-flag token as the unit name — any other action word (`restart`,
// `reload`, `status`, ...), a missing unit, or more than one non-flag
// token falls through to the LLM/manual tier.
func deriveSystemctl(tokens []string, sudo bool) (Derived, bool) {
	if len(tokens) < 3 || tokens[0] != "systemctl" {
		return Derived{}, false
	}
	opposite, ok := systemctlOpposites[tokens[1]]
	if !ok {
		return Derived{}, false
	}

	var flags, units []string
	for _, tok := range tokens[2:] {
		if strings.HasPrefix(tok, "-") {
			flags = append(flags, tok)
		} else {
			units = append(units, tok)
		}
	}
	if len(units) != 1 {
		return Derived{}, false
	}

	parts := append([]string{"systemctl", opposite}, flags...)
	parts = append(parts, units[0])
	return Derived{Commands: []string{prefixSudo(strings.Join(parts, " "), sudo)}}, true
}

// psFlagValue is one parsed `-FlagName Value` pair from a PowerShell
// cmdlet invocation, as deriveNewItemDirectory's tokenizer produces it.
type psFlagValue struct {
	name  string
	value string
}

// deriveNewItemDirectory matches `New-Item` invoked to create a
// directory, in any of PowerShell's own argument orderings — flags may
// appear in any order, and the path may be given either via `-Path` or as
// a bare positional argument. It recognizes exactly two flags
// (`-ItemType`/`-Path`, case-insensitively, PowerShell's own convention);
// any OTHER flag at all (`-Force`, `-Value`, ...) is treated as an
// unrecognized shape and falls through, since this table cannot know
// whether that flag changes what actually needs reversing. Reverses to
// `Remove-Item <path>`, with a caveat noting Remove-Item only actually
// removes an empty directory (New-Item -ItemType Directory never had any
// content-creating side effect of its own, so this is always safe when
// it matches at all).
func deriveNewItemDirectory(tokens []string) (Derived, bool) {
	rest := tokens[1:]
	if len(rest) == 0 {
		return Derived{}, false
	}

	var flags []psFlagValue
	var positional []string
	for i := 0; i < len(rest); i++ {
		tok := rest[i]
		if !strings.HasPrefix(tok, "-") {
			positional = append(positional, tok)
			continue
		}
		if i+1 >= len(rest) {
			return Derived{}, false
		}
		switch {
		case strings.EqualFold(tok, "-ItemType"), strings.EqualFold(tok, "-Path"):
			flags = append(flags, psFlagValue{name: strings.ToLower(tok), value: rest[i+1]})
			i++
		default:
			// An unrecognized flag (-Force, -Value, ...) means this shape
			// is not one this table can safely reverse.
			return Derived{}, false
		}
	}

	itemTypeIsDirectory := false
	path := ""
	hasPath := false
	for _, f := range flags {
		switch f.name {
		case "-itemtype":
			if !strings.EqualFold(f.value, "Directory") {
				return Derived{}, false
			}
			itemTypeIsDirectory = true
		case "-path":
			if hasPath {
				return Derived{}, false
			}
			path = f.value
			hasPath = true
		}
	}
	if !itemTypeIsDirectory {
		return Derived{}, false
	}

	switch {
	case hasPath && len(positional) != 0:
		return Derived{}, false
	case !hasPath && len(positional) != 1:
		return Derived{}, false
	case !hasPath:
		path = positional[0]
	}

	return Derived{
		Commands:         []string{"Remove-Item " + path},
		Caveat:           "only removes the directory if it is still empty",
		UsesRelativePath: !isAbsolutePath(path, "windows"),
	}, true
}

// prefixSudo re-prefixes cmd with "sudo " when sudo is true, preserving
// the original command's own elevation exactly as recorded — see
// deriveUnix's own doc comment on why this package never needs to
// classify risk itself.
func prefixSudo(cmd string, sudo bool) string {
	if sudo {
		return "sudo " + cmd
	}
	return cmd
}

// hasFlagOrWildcard reports whether s looks like a command-line flag (a
// leading "-") or contains a shell glob/wildcard character — either of
// which makes a path argument unsafe for deriveMv's unconditional
// single-file reversal.
func hasFlagOrWildcard(s string) bool {
	if strings.HasPrefix(s, "-") {
		return true
	}
	return strings.ContainsAny(s, "*?[]")
}

// windowsDriveRoot matches a Windows drive-letter root ("C:\", "C:/",
// "C:") at the start of a path, case-insensitively.
func windowsDriveRoot(p string) bool {
	if len(p) < 2 {
		return false
	}
	c := p[0]
	isLetter := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
	return isLetter && p[1] == ':'
}

// isAbsolutePath reports whether p is an absolute filesystem path in
// goos's own dialect: a leading "/" for Unix, a drive-letter root or a
// UNC ("\\server\share") prefix for Windows. Used only to compute
// Derived.UsesRelativePath — internal/cli's own cwd-mismatch safety check
// reads that field rather than re-deriving it itself.
func isAbsolutePath(p, goos string) bool {
	if goos == "windows" {
		return windowsDriveRoot(p) || strings.HasPrefix(p, `\\`)
	}
	return strings.HasPrefix(p, "/")
}
