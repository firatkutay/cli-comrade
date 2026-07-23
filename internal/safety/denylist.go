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
		// Lower-cased before comparison: `RM -rf /` must be recognized
		// exactly like `rm -rf /` on case-insensitive filesystems
		// (macOS/Windows), not merely on the flags MEDIUM finding #5
		// already fixed — the command NAME itself is not a safe place to
		// assume lowercase either.
		if strings.ToLower(path.Base(t)) != "rm" {
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

// isChmodChownRecursiveRootTarget reports whether tokens contains a
// chmod or chown invocation (matched by basename, so `/bin/chmod` counts
// too) with a recursive flag (`-R`/`-r`/`--recursive` — hasFlag's
// short-flag scan is case-insensitive, see tokenize.go) whose target
// normalizes (via normalizeRootTarget) to a rootTargets entry — i.e. the
// filesystem root or the invoking user's home directory — regardless of
// what mode/owner value is being applied.
//
// This generalizes the escalation list's existing chmod -R 777 rule
// (chmodRThenModePattern/chmodModeThenRPattern below, which fires on ANY
// target the instant mode 777 appears, and is kept unchanged) to also
// catch a restrictive-looking mode that rule would miss entirely — e.g.
// `chmod -R 000 /` (Finding 1's proof command: mode 000, not 777) — by
// keying off "recursive change targeting root" instead of "targeting
// mode 777", and extends the same protection to chown.
func isChmodChownRecursiveRootTarget(tokens []string) bool {
	for i, t := range tokens {
		// Lower-cased before comparison — see isRmRootDelete's comment on
		// why a command name is not a safe place to assume lowercase.
		name := strings.ToLower(path.Base(t))
		if name != "chmod" && name != "chown" {
			continue
		}
		args := argsAfter(tokens, i+1)
		if !hasFlag(args, 'r', "--recursive") {
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
	// group — plus the other filesystem-format-family command names that
	// are not spelled "mkfs*" at all: `mke2fs` (ext2/3/4, standalone from
	// e2fsprogs), `mkswap` (swap-space format), `mkdosfs`/`mkntfs`
	// (FAT/NTFS), and `newfs` (BSD/macOS). Each is matched as its own
	// whole word, same as "mkfs" — "mkfsutils" still does not match, and
	// neither would e.g. "newfsutils". Case-insensitive (`(?i)`) so
	// `MKFS.EXT4 /dev/sda1` on a case-insensitive filesystem (macOS/
	// Windows) is recognized identically to `mkfs.ext4`.
	mkfsPattern = regexp.MustCompile(`(?i)\b(?:mkfs|mke2fs|mkswap|mkdosfs|mkntfs|newfs)\b`)

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
	{
		// Wires hasRealDiskDeviceReference — previously used only by the
		// dd-of= and redirect rules above — into a TARGETED, tool-aware,
		// ADJACENCY-scoped rule: block only when a tool this package knows
		// is destructive is itself POINTED AT a real (non-pseudo)
		// /dev/<disk> device — i.e. the device reference appears among
		// THAT TOOL'S OWN arguments (isDestructiveDiskTool scans
		// per-invocation via argsAfter), not merely somewhere else on the
		// same command line.
		//
		// This is deliberately narrower on two axes, both found on
		// review:
		//  1. Tool-aware, not "any command referencing a real disk
		//     device" — that broadest form hard-blocked legitimate
		//     read-only disk access (`lsblk /dev/sda`, `smartctl -a
		//     /dev/sda`, `mount /dev/sda1 /mnt`, `dd if=/dev/sda
		//     of=backup.img` for imaging, ...), disproportionate under
		//     Block's unconditional (not even --yolo-overridable) tier.
		//     Read-only/unknown-tool disk access is instead caught by
		//     escalation.go's generic real-disk-device fallback rule,
		//     which only raises to Confirm.
		//  2. Adjacency-scoped, not whole-command-string co-occurrence —
		//     a whole-string AND let a destructive tool word and an
		//     unrelated /dev/<disk> reference elsewhere on the same
		//     pipeline false-Block a safe command (`cat /dev/sda | tee
		//     backup.img` reads the disk and tees to a FILE; `tee` never
		//     touches /dev/sda). Scoping the device check to each
		//     candidate tool's own argsAfter slice is what tells "tee's
		//     target is the disk" apart from "the disk appears elsewhere
		//     on this line" (still caught by the Confirm fallback either
		//     way).
		//
		// Deliberately placed last so the more specific dd-of=/redirect
		// rules above still decide the reported rule name for the
		// commands they already covered (kept for backward-compatible
		// Decision.MatchedRule text).
		name:  "destructive disk tool + real /dev/<disk> reference",
		match: func(_ string, tokens []string) bool { return isDestructiveDiskTool(tokens) },
	},
}

// alwaysDestructiveDiskToolNames is the set of command words (matched by
// basename, case-insensitively — see isDestructiveDiskTool) that are
// destructive to whatever device they're pointed at on every invocation,
// with no conditional flag/action needed: wipefs, blkdiscard, and sgdisk
// always write to the device they're given; tee and shred always write to
// (respectively overwrite) whatever path they're pointed at.
var alwaysDestructiveDiskToolNames = map[string]bool{
	"wipefs": true, "blkdiscard": true, "sgdisk": true, "tee": true, "shred": true,
}

// sfdiskDestructiveFlags is the set of sfdisk arguments this package
// treats as write/destructive (as opposed to sfdisk's default dump/list
// behavior).
var sfdiskDestructiveFlags = map[string]bool{
	"--delete": true, "-d": true, "-N": true, "--wipe": true,
}

// sfdiskArgsAreDestructive reports whether args (an sfdisk invocation's
// OWN arguments) contains one of sfdiskDestructiveFlags (or a
// `--wipe=<mode>` spelling).
func sfdiskArgsAreDestructive(args []string) bool {
	for _, a := range args {
		if sfdiskDestructiveFlags[a] || strings.HasPrefix(a, "--wipe=") {
			return true
		}
	}
	return false
}

// cryptsetupDestructiveActions is the set of cryptsetup subcommands this
// package treats as destructive to the underlying device's existing
// contents (formatting/re-encrypting/erasing), matched case-insensitively
// and lower-cased for lookup.
var cryptsetupDestructiveActions = map[string]bool{
	"luksformat": true, "reencrypt": true, "erase": true, "lukserase": true,
}

// cryptsetupArgsAreDestructive reports whether args (a cryptsetup
// invocation's OWN arguments) contains a subcommand in
// cryptsetupDestructiveActions.
func cryptsetupArgsAreDestructive(args []string) bool {
	for _, a := range args {
		if cryptsetupDestructiveActions[strings.ToLower(a)] {
			return true
		}
	}
	return false
}

// isDestructiveDiskToolInvocation reports whether name (a single command
// word, already lower-cased and basename-resolved by the caller) is a
// tool this package knows is destructive GIVEN args — that same
// invocation's OWN arguments (from argsAfter, i.e. up to the next shell
// separator) — and, critically, whether a real (non-pseudo) /dev/<disk>
// reference appears within those SAME args. Requiring the device
// reference to come from the tool's own args (not the whole command
// string) is what keeps a destructive tool word merely co-occurring with
// an unrelated disk reference elsewhere on the line (e.g. `cat /dev/sda |
// tee backup.img`) from false-Blocking: tee's own args here are just
// `backup.img`, which contains no device reference at all.
func isDestructiveDiskToolInvocation(name string, args []string) bool {
	if !hasRealDiskDeviceReference(strings.Join(args, " ")) {
		return false
	}
	switch {
	case alwaysDestructiveDiskToolNames[name]:
		return true
	case name == "sfdisk":
		return sfdiskArgsAreDestructive(args)
	case name == "badblocks":
		return hasFlag(args, 'w', "")
	case name == "cryptsetup":
		return cryptsetupArgsAreDestructive(args)
	default:
		return false
	}
}

// isDestructiveDiskTool reports whether tokens contains at least one
// invocation of a tool this package knows writes destructively to
// whatever real (non-pseudo) disk device THAT SAME INVOCATION'S OWN
// arguments point it at — see isDestructiveDiskToolInvocation. Every
// candidate tool token's name is matched by basename and lower-cased
// before comparison, so `WIPEFS`/`Wipefs`/`/sbin/wipefs` are all
// recognized identically to `wipefs` (case-insensitive on
// case-insensitive filesystems — macOS/Windows — a command name is not a
// safe place to assume lowercase).
func isDestructiveDiskTool(tokens []string) bool {
	for i, t := range tokens {
		name := strings.ToLower(path.Base(t))
		args := argsAfter(tokens, i+1)
		if isDestructiveDiskToolInvocation(name, args) {
			return true
		}
	}
	return false
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
