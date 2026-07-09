package safety

import (
	"fmt"
	"os"
	"path"
	"regexp"
	"strings"
)

// denylistRule is one entry in the built-in or user-supplied denylist:
// name identifies it for Decision.Reason/MatchedRule, match reports
// whether a command is a match, given both its normalizeCommand'd text
// and that same text's tokenizeCommand tokens (whichever a rule finds
// more convenient). A denylist match always yields Action Block from
// Engine.Evaluate, unconditionally — see engine.go. Every caller passes
// the *normalized* command, never the raw one Evaluate received — see
// normalizeCommand's doc comment in tokenize.go for why.
type denylistRule struct {
	name  string
	match func(command string, tokens []string) bool
}

// rootTargets is the exact set of tokenizeCommand tokens — after
// normalizeRootTarget's canonicalization — that count as "the filesystem
// root or the invoking user's home directory" for the rm-root-delete rule
// below. Anything else — `/tmp/x`, `./build`, `/home/user` — is a
// near-miss that must NOT match.
var rootTargets = map[string]bool{
	"/": true, "/*": true,
	"~": true, "~/*": true,
	"$HOME": true, "${HOME}": true, "$HOME/*": true, "${HOME}/*": true,
}

// normalizeRootTarget canonicalizes a filesystem-path-shaped token before
// it is looked up in rootTargets, so equivalents of the same target don't
// slip past as near-misses.
//
// A "/"-rooted target is canonicalized with the standard library's
// path.Clean, which — being real path-cleaning logic rather than an
// ad-hoc set of hand-picked cases — correctly collapses every "stay at
// root" spelling at once: repeated slashes ("//", "///" -> "/"), a
// trailing/embedded "." segment ("/.", "/./", "/.//" -> "/"), and — the
// residual this replaced an ad-hoc version over — ".." segments that
// can't climb above root ("/..", "/../.." -> "/"). It equally leaves a
// genuine near-miss alone (`/tmp/x` unchanged) and never turns a
// glob-suffixed kept target into something that misses (`path.Clean("/*")
// == "/*"`, since "*" is an ordinary path segment, not "." or "..").
//
// "~" and "$HOME"/"${HOME}" are not path syntax, so path.Clean does not
// understand them at all — their trailing-slash/trailing-"/."
// normalization ("~/" -> "~", "$HOME/." -> "$HOME") is handled by hand,
// exactly as before, and deliberately never touches a genuine near-miss
// like "~/project".
func normalizeRootTarget(target string) string {
	if strings.HasPrefix(target, "/") {
		return path.Clean(target)
	}

	if trimmed := strings.TrimSuffix(target, "/."); trimmed != target {
		target = trimmed
	} else if len(target) > 1 && strings.HasSuffix(target, "/") {
		target = strings.TrimSuffix(target, "/")
	}
	return target
}

// driveRootPattern matches a PowerShell drive-root token exactly — "C:\"
// or "C:" — but not "C:\Users\foo".
var driveRootPattern = regexp.MustCompile(`(?i)^[a-z]:\\?$`)

// removeItemAliasWords is every command word this package treats as
// equivalent to PowerShell's Remove-Item for the drive-root-delete
// denylist rule and the Remove-Item escalation rule below: the cmdlet
// itself, its built-in alias `ri`, the legacy cmd.exe-heritage
// aliases/commands PowerShell also recognizes (`rd`, `rmdir`, `del`,
// `erase`), Remove-ItemProperty (registry-property deletion), and `rm` —
// itself a built-in PowerShell alias for Remove-Item (as well as the Unix
// command handled separately by isRmRootDelete's unix-flag/unix-target
// logic). Matched case-insensitively and by basename (so
// `/bin/rm`/`/usr/bin/rm` still count — see isRmRootDelete).
var removeItemAliasWords = map[string]bool{
	"remove-item":         true,
	"remove-itemproperty": true,
	"ri":                  true,
	"rd":                  true,
	"rmdir":               true,
	"del":                 true,
	"erase":               true,
	"rm":                  true,
}

// isRemoveItemAliasWord reports whether t (a single token) names one of
// removeItemAliasWords, matched case-insensitively and by path basename.
func isRemoveItemAliasWord(t string) bool {
	return removeItemAliasWords[strings.ToLower(path.Base(t))]
}

// hasRecurseIshFlag reports whether args contains a flag this package
// accepts as meaning "recursive" for the Remove-Item alias family: an
// unambiguous, case-insensitive prefix abbreviation of "recurse"
// (PowerShell allows abbreviating parameter names — "-r", "-rec",
// "-Recurse" all bind to -Recurse), or the cmd.exe rd/rmdir legacy flag
// "/s" (subdirectories).
func hasRecurseIshFlag(args []string) bool {
	return hasAbbreviatedFlag(args, "recurse") || hasLegacySlashFlag(args, "/s")
}

// hasForceIshFlag reports whether args contains a flag this package
// accepts as meaning "force" for the Remove-Item alias family: an
// unambiguous, case-insensitive prefix abbreviation of "force" ("-f",
// "-fo", "-Force"), or the cmd.exe del/erase legacy flag "/q" (quiet —
// suppresses the delete confirmation prompt, functionally the same
// "don't ask" intent as -Force).
func hasForceIshFlag(args []string) bool {
	return hasAbbreviatedFlag(args, "force") || hasLegacySlashFlag(args, "/q")
}

// hasAbbreviatedFlag reports whether args contains a single- or
// double-dash token whose dash-stripped, lowercased name is a non-empty
// prefix of full (e.g. "-r"/"-rec"/"-Recurse"/"--recurse" are all
// prefixes of "recurse"). This intentionally mirrors PowerShell's own
// parameter-name-abbreviation behavior rather than requiring the exact
// flag spelling.
func hasAbbreviatedFlag(args []string, full string) bool {
	for _, t := range args {
		if len(t) < 2 || t[0] != '-' {
			continue
		}
		name := strings.ToLower(strings.TrimLeft(t, "-"))
		if name != "" && strings.HasPrefix(full, name) {
			return true
		}
	}
	return false
}

// hasLegacySlashFlag reports whether args contains slashFlag (e.g. "/s",
// "/q"), matched case-insensitively, as cmd.exe's rd/rmdir/del/erase use.
func hasLegacySlashFlag(args []string, slashFlag string) bool {
	for _, t := range args {
		if strings.EqualFold(t, slashFlag) {
			return true
		}
	}
	return false
}

// isRmRootDelete implements the denylist's `rm -rf /` family: `rm -rf /`,
// `rm -fr /`, `rm -rf /*`, and the `~`/$HOME equivalents (with
// `--no-preserve-root` covered incidentally, since it only ever
// accompanies a root target in the first place), matched by rm's
// basename (so `/bin/rm`/`/usr/bin/rm` count too) and with the target
// canonicalized by normalizeRootTarget first (so `rm -rf //`, `rm -rf
// /.`, `rm -rf ~/`, `rm -rf $HOME/` all match exactly like their bare
// equivalents). Matched conservatively — see normalizeCommand's doc
// comment for the accepted quoted-argument false positive
// (`echo "rm -rf /"`).
func isRmRootDelete(tokens []string) bool {
	for i, t := range tokens {
		if path.Base(t) != "rm" {
			continue
		}
		args := argsAfter(tokens, i+1)
		if !hasFlag(args, 'r', "--recursive") || !hasFlag(args, 'f', "--force") {
			continue
		}
		for _, a := range args {
			if rootTargets[normalizeRootTarget(a)] {
				return true
			}
		}
	}
	return false
}

// isRemoveItemAliasDriveRootDelete implements the denylist's
// `Remove-Item -Recurse C:\` rule, generalized over every word in
// removeItemAliasWords (including the cmd.exe-heritage `rd`/`rmdir`/
// `del`/`erase` and the PowerShell `rm`/`ri` aliases) and every
// recurse-ish flag spelling hasRecurseIshFlag accepts: a recursive
// delete targeting an entire drive root, not a subdirectory of one.
// -Force is not required — CLAUDE.md's denylist entry only names
// -Recurse.
func isRemoveItemAliasDriveRootDelete(tokens []string) bool {
	for i, t := range tokens {
		if !isRemoveItemAliasWord(t) {
			continue
		}
		args := argsAfter(tokens, i+1)
		if !hasRecurseIshFlag(args) {
			continue
		}
		for _, a := range args {
			if driveRootPattern.MatchString(a) {
				return true
			}
		}
	}
	return false
}

// isFormatDrive implements the denylist's `format <drive>:` rule.
func isFormatDrive(tokens []string) bool {
	for i, t := range tokens {
		if !strings.EqualFold(t, "format") {
			continue
		}
		for _, a := range argsAfter(tokens, i+1) {
			if driveRootPattern.MatchString(a) {
				return true
			}
		}
	}
	return false
}

// safeDeviceNames are /dev/<name> pseudo-devices this package's
// disk-write denylist/escalation rules must never treat as "a real disk":
// infinite/throwaway data sources and sinks, terminals, and the standard
// stream aliases. Writing to any of these can never overwrite persistent
// storage the way writing to a block/character disk device can.
var safeDeviceNames = map[string]bool{
	"null": true, "zero": true, "tty": true, "random": true,
	"urandom": true, "full": true, "stdin": true, "stdout": true, "stderr": true,
}

// devReferencePattern captures the path segment immediately following
// "/dev/" in a "/dev/<name>" reference.
var devReferencePattern = regexp.MustCompile(`/dev/([a-zA-Z0-9_]+)`)

// isSafeDeviceReference reports whether name — the segment right after
// "/dev/" — is a safe pseudo-device: one of safeDeviceNames, "ttyN" (any
// tty number, not just the bare "tty"), or the "fd"/"pts" passthrough
// namespaces (whose real target is a file descriptor or pseudo-terminal
// slot, never a block device, regardless of what numeric suffix follows
// the next "/").
func isSafeDeviceReference(name string) bool {
	lower := strings.ToLower(name)
	if safeDeviceNames[lower] {
		return true
	}
	if strings.HasPrefix(lower, "tty") {
		return true
	}
	return lower == "fd" || lower == "pts"
}

// hasRealDiskDeviceReference reports whether text contains at least one
// "/dev/<name>" reference where name is NOT a safe pseudo-device — i.e. a
// genuine disk device (sda, nvme0n1, vda, xvda, mmcblk0, disk0, loop0,
// ...). This is deliberately broad (CLAUDE.md's `dd of=/dev/` denylist
// entry names no specific device family) rather than an allowlist of
// known disk-name prefixes, which would always be one new device-naming
// convention away from a false negative.
func hasRealDiskDeviceReference(text string) bool {
	for _, m := range devReferencePattern.FindAllStringSubmatch(text, -1) {
		if !isSafeDeviceReference(m[1]) {
			return true
		}
	}
	return false
}

// isDiskRedirect implements the denylist's `> /dev/<disk>` rule: stdout
// redirected into any real (non-pseudo) disk device.
func isDiskRedirect(command string) bool {
	for _, m := range redirectDevPattern.FindAllStringSubmatch(command, -1) {
		if hasRealDiskDeviceReference(m[1]) {
			return true
		}
	}
	return false
}

// isDdRealDiskWrite implements the denylist's `dd ... of=/dev/<disk>`
// rule: any `dd` invocation whose `of=` target is a real disk device.
func isDdRealDiskWrite(command string) bool {
	for _, m := range ddOfPattern.FindAllStringSubmatch(command, -1) {
		if hasRealDiskDeviceReference(m[1]) {
			return true
		}
	}
	return false
}

var (
	// mkfsPattern matches `mkfs`, `mkfs.ext4`, etc. anywhere in the
	// command — `\b...\b` already stops at the transition from "mkfs" to
	// "." or whitespace, so it does not need a separate dotted-suffix
	// group.
	mkfsPattern = regexp.MustCompile(`\bmkfs\b`)

	// redirectDevPattern captures the "/dev/<name>" target of a `>`
	// redirection.
	redirectDevPattern = regexp.MustCompile(`>\s*(/dev/[a-zA-Z0-9_]+)`)

	// ddOfPattern captures the "/dev/<name>" value of a `dd ... of=`
	// argument.
	ddOfPattern = regexp.MustCompile(`\bdd\b[^;|&\n]*?\bof=(/dev/[a-zA-Z0-9_]+)`)

	// diskpartPattern / cleanPattern together implement the denylist's
	// `diskpart` + `clean` rule (both words must appear, in either
	// order — a diskpart script's `clean` command wipes a disk's
	// partition table).
	diskpartPattern = regexp.MustCompile(`(?i)\bdiskpart\b`)
	cleanPattern    = regexp.MustCompile(`(?i)\bclean\b`)

	// forkBombPattern matches the classic `:(){ :|:& };:` fork bomb,
	// tolerant of extra whitespace around every token.
	forkBombPattern = regexp.MustCompile(`:\s*\(\s*\)\s*\{\s*:\s*\|\s*:\s*&\s*\}\s*;\s*:`)
)

// builtinDenylist is the hardcoded, LLM-independent denylist from
// CLAUDE.md's "Komut Risk Sınıflandırması": every entry always yields
// Block, regardless of mode, override flags, or what risk class the LLM
// declared.
var builtinDenylist = []denylistRule{
	{
		name:  "rm -rf / (or ~ / $HOME root delete)",
		match: func(_ string, tokens []string) bool { return isRmRootDelete(tokens) },
	},
	{
		name:  "mkfs (format a filesystem)",
		match: func(command string, _ []string) bool { return mkfsPattern.MatchString(command) },
	},
	{
		name:  "dd of=/dev/<disk> (raw disk overwrite)",
		match: func(command string, _ []string) bool { return isDdRealDiskWrite(command) },
	},
	{
		name: "diskpart clean (wipes a disk's partition table)",
		match: func(command string, _ []string) bool {
			return diskpartPattern.MatchString(command) && cleanPattern.MatchString(command)
		},
	},
	{
		name:  "Remove-Item (or ri/rd/rmdir/del/erase/rm alias) -Recurse <drive root> (PowerShell)",
		match: func(_ string, tokens []string) bool { return isRemoveItemAliasDriveRootDelete(tokens) },
	},
	{
		name:  "format <drive>: (Windows format)",
		match: func(_ string, tokens []string) bool { return isFormatDrive(tokens) },
	},
	{
		name:  "fork bomb",
		match: func(command string, _ []string) bool { return forkBombPattern.MatchString(command) },
	},
	{
		name:  "> /dev/<disk> (redirect into a real disk device)",
		match: func(command string, _ []string) bool { return isDiskRedirect(command) },
	},
}

// compileUserDenylist compiles each entry of safety.denylist_extra into a
// denylistRule. An entry that fails to compile as a Go regexp is skipped
// — never a construction error, never a panic — with exactly one warning
// line to stderr naming the bad pattern (see NewEngine's doc comment for
// why this is the documented behavior, not a defect).
func compileUserDenylist(patterns []string) []denylistRule {
	rules := make([]denylistRule, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cli-comrade: warning: safety.denylist_extra pattern %q is invalid, skipping: %v\n", p, err)
			continue
		}
		compiled := re
		name := fmt.Sprintf("user denylist_extra: %s", p)
		rules = append(rules, denylistRule{
			name:  name,
			match: func(command string, _ []string) bool { return compiled.MatchString(command) },
		})
	}
	return rules
}
