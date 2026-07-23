package safety

import "regexp"

// escalationRule is one upward-only risk-escalation rule from CLAUDE.md's
// "Komut Risk Sınıflandırması": when match(command) is true, the
// command's effective risk is raised to at least risk — never lowered,
// and never applied when the command already matched a denylist rule
// (Engine.Evaluate checks the denylist first and returns Block
// immediately in that case, before escalation rules ever run).
type escalationRule struct {
	name  string
	risk  RiskClass
	match func(command string) bool
}

var (
	chmodRThenModePattern = regexp.MustCompile(`\bchmod\b[^;|&\n]*?-R\b[^;|&\n]*?\b777\b`)
	chmodModeThenRPattern = regexp.MustCompile(`\bchmod\b[^;|&\n]*?\b777\b[^;|&\n]*?-R\b`)

	// findDeletePattern matches any `find` invocation whose arguments
	// contain the `-delete` action — a mass, non-`rm` deletion the old
	// classifier had no signature for at all (Finding 1: `find / -delete`,
	// `find ~ -type f -delete`).
	findDeletePattern = regexp.MustCompile(`(?i)\bfind\b[^;|&\n]*-delete\b`)

	// shredUnlinkPattern matches `shred` invoked with its `-u`/`--remove`
	// unlink-after-overwrite flag (short form tolerant of a combined
	// cluster like `-uvz`, mirroring the `rm -r/-f` escalation rule's own
	// combined-short-flag style below) — a non-`rm` secure-delete the old
	// classifier never recognized.
	shredUnlinkPattern = regexp.MustCompile(`(?i)\bshred\s+(?:-[a-zA-Z]*u[a-zA-Z]*|--remove)\b`)

	// truncateZeroPattern matches `truncate` invoked with a `-s 0`/`-s0`/
	// `--size 0`/`--size=0` argument — truncating a file to zero bytes is
	// as destructive as deleting its content.
	truncateZeroPattern = regexp.MustCompile(`(?i)\btruncate\b[^;|&\n]*?(?:-s\s?0\b|--size[= ]0\b)`)

	// mvToDevNullPattern matches `mv` whose arguments reach `/dev/null` —
	// moving a file into the null device discards it exactly like a
	// delete, but no `rm`/`Remove-Item`-family rule ever sees it.
	mvToDevNullPattern = regexp.MustCompile(`(?i)\bmv\b[^;|&\n]*/dev/null\b`)

	// fetchPipeInterpreterPattern matches a network-fetch command (curl,
	// wget, Invoke-WebRequest, Invoke-RestMethod) whose output is piped
	// into a shell/script interpreter — the classic "curl | sh" remote
	// code execution shape, previously escalated no further than the
	// "network access verb" rule's RiskNetwork (still Allow in auto mode).
	fetchPipeInterpreterPattern = regexp.MustCompile(`(?i)\b(?:curl|wget|Invoke-WebRequest|Invoke-RestMethod)\b[^\n]*?\|\s*(?:sh|bash|zsh|python[0-9.]*|pwsh)\b`)

	// base64DecodePipePattern matches `base64 -d`/`base64 --decode` piped
	// into a shell/script interpreter — a decode-and-execute pipeline that
	// hides the executed payload from every other rule in this package.
	base64DecodePipePattern = regexp.MustCompile(`(?i)\bbase64\b[^;|\n]*(?:-d\b|--decode\b)[^\n]*\|\s*(?:sh|bash|zsh|python[0-9.]*|pwsh)\b`)

	// bareEvalPattern matches the `eval` shell builtin anywhere in the
	// command — eval executes a dynamically-built string as code, so its
	// actual effect is invisible to every other rule in this package.
	bareEvalPattern = regexp.MustCompile(`(?i)\beval\b`)

	// windowsStorageCmdletPattern matches the PowerShell storage cmdlets
	// that reformat a volume, wipe a disk, or delete a partition —
	// `format <drive>:`'s modern PowerShell equivalents, none of which the
	// old classifier's isFormatDrive (which only matches the legacy
	// `format` command word) ever recognized.
	windowsStorageCmdletPattern = regexp.MustCompile(`(?i)\b(?:Format-Volume|Clear-Disk|Initialize-Disk|Remove-Partition)\b`)

	// regDeleteForcePattern matches cmd.exe's `reg delete ... /f` —
	// unconditional (no-prompt) registry-key deletion.
	regDeleteForcePattern = regexp.MustCompile(`(?i)\breg\b[^;|&\n]*?\bdelete\b[^;|&\n]*?/f\b`)

	// diskpartScriptFilePattern matches `diskpart` invoked with a `/s`
	// script-file argument — the existing "diskpart clean" denylist rule
	// only fires when the literal word "clean" also appears in the
	// command string, but a diskpart script file's contents (which can
	// just as easily contain `clean`) are opaque to that string match.
	diskpartScriptFilePattern = regexp.MustCompile(`(?i)\bdiskpart\b[^;|&\n]*?/s\b`)
)

// isRemoveItemAliasEscalation implements the escalation list's
// "Remove-Item -Recurse/-Force" rule, generalized exactly like the
// denylist's isRemoveItemAliasDriveRootDelete over every word in
// removeItemAliasWords (Remove-Item/Remove-ItemProperty and the
// ri/rd/rmdir/del/erase/rm aliases): a recurse-ish OR force-ish flag
// anywhere after the command word, targeting anything at all (not just a
// drive root — that stricter case is the denylist's job and already
// returns Block before escalation rules ever run).
func isRemoveItemAliasEscalation(command string) bool {
	tokens := tokenizeCommand(command)
	for i, t := range tokens {
		if !isRemoveItemAliasWord(t) {
			continue
		}
		args := argsAfter(tokens, i+1)
		if hasRecurseIshFlag(args) || hasForceIshFlag(args) {
			return true
		}
	}
	return false
}

// escalationRules is the fixed, ordered list of escalation rules every
// Engine applies (in addition to the built-in and user denylists) on
// every Evaluate call. Order only matters for which rule's name ends up
// recorded in Decision.MatchedRule when several rules independently imply
// the same maximum risk — Evaluate keeps the first one it reaches; it
// never affects the resulting RiskClass itself, since escalation always
// takes the max across every matching rule.
var escalationRules = []escalationRule{
	{
		name:  "rm -r/-f (recursive or force delete)",
		risk:  RiskDestructive,
		match: regexp.MustCompile(`\brm\s+(?:-[a-zA-Z]*[rf][a-zA-Z]*|--recursive|--force)\b`).MatchString,
	},
	{
		name:  "Remove-Item (or ri/rd/rmdir/del/erase/rm alias) -Recurse/-Force (PowerShell)",
		risk:  RiskDestructive,
		match: isRemoveItemAliasEscalation,
	},
	{
		name: "chmod -R 777",
		risk: RiskDestructive,
		match: func(command string) bool {
			return chmodRThenModePattern.MatchString(command) || chmodModeThenRPattern.MatchString(command)
		},
	},
	{
		// Broadened to the exact same "any real disk device" concept as
		// the denylist's isDiskRedirect/isDdRealDiskWrite (see
		// denylist.go's hasRealDiskDeviceReference) — in practice, any
		// command matching this rule was already Blocked by the denylist
		// before Evaluate's escalation loop ever runs; kept for
		// defense-in-depth and behavioral consistency, not because it is
		// expected to be the deciding rule.
		name: "> /dev/<disk> or of=/dev/<disk> (disk write)",
		risk: RiskDestructive,
		match: func(command string) bool {
			return isDiskRedirect(command) || isDdRealDiskWrite(command)
		},
	},
	{
		name:  "Remove-Item/-ItemProperty on HKLM:/HKCU: (registry)",
		risk:  RiskDestructive,
		match: regexp.MustCompile(`(?i)\bRemove-Item(?:Property)?\b[^;|&\n]*?\b(?:HKLM|HKCU):`).MatchString,
	},
	{
		name:  "killall / taskkill /F",
		risk:  RiskElevated,
		match: regexp.MustCompile(`(?:\bkillall\b|(?i:\btaskkill\b[^;|&\n]*?/F\b))`).MatchString,
	},
	{
		name:  "iptables -F / netsh advfirewall reset",
		risk:  RiskElevated,
		match: regexp.MustCompile(`(?:\biptables\s+-F\b|(?i:\bnetsh\s+advfirewall\s+reset\b))`).MatchString,
	},
	{
		name:  "git push --force/-f",
		risk:  RiskDestructive,
		match: regexp.MustCompile(`\bgit\s+push\b[^;|&\n]*?(?:--force\b|\s-f\b)`).MatchString,
	},
	{
		name:  "sudo / runas / elevation verb",
		risk:  RiskElevated,
		match: regexp.MustCompile(`(?:\bsudo\b|(?i:\brunas\b)|(?i:-Verb\s+RunAs\b))`).MatchString,
	},
	{
		name:  "package manager install",
		risk:  RiskWrite,
		match: regexp.MustCompile(`(?i)\b(?:apt|apt-get|dnf|yum|pacman|brew|winget|choco|scoop)\b[^;|&\n]*?\b(?:install|add)\b`).MatchString,
	},
	{
		name:  "network access verb",
		risk:  RiskNetwork,
		match: regexp.MustCompile(`(?:\b(?:curl|wget)\b|(?i:\bInvoke-WebRequest\b)|(?i:\bInvoke-RestMethod\b)|\bapt(?:-get)?\s+(?:update|upgrade)\b)`).MatchString,
	},

	// --- Finding 1 hardening: signature-allowlist gaps below this line ---

	{
		name:  "find -delete (mass, non-rm deletion)",
		risk:  RiskDestructive,
		match: findDeletePattern.MatchString,
	},
	{
		name: "chmod/chown -R (any mode) on root-ish target",
		risk: RiskDestructive,
		match: func(command string) bool {
			return isChmodChownRecursiveRootTarget(tokenizeCommand(command))
		},
	},
	{
		name:  "shred -u/--remove (non-rm secure delete)",
		risk:  RiskDestructive,
		match: shredUnlinkPattern.MatchString,
	},
	{
		name:  "truncate -s 0 (zeroes a file's content)",
		risk:  RiskDestructive,
		match: truncateZeroPattern.MatchString,
	},
	{
		name:  "mv <target> /dev/null (discard via move)",
		risk:  RiskDestructive,
		match: mvToDevNullPattern.MatchString,
	},
	{
		name:  "fetch piped into a shell/script interpreter",
		risk:  RiskElevated,
		match: fetchPipeInterpreterPattern.MatchString,
	},
	{
		name:  "base64 decode piped into a shell/script interpreter",
		risk:  RiskElevated,
		match: base64DecodePipePattern.MatchString,
	},
	{
		name:  "bare eval (dynamically executes a string)",
		risk:  RiskElevated,
		match: bareEvalPattern.MatchString,
	},
	{
		name:  "Windows storage cmdlet (Format-Volume/Clear-Disk/Initialize-Disk/Remove-Partition)",
		risk:  RiskDestructive,
		match: windowsStorageCmdletPattern.MatchString,
	},
	{
		name:  "reg delete ... /f (unconditional registry-key delete)",
		risk:  RiskDestructive,
		match: regDeleteForcePattern.MatchString,
	},
	{
		name:  "diskpart /s <script> (opaque script file)",
		risk:  RiskDestructive,
		match: diskpartScriptFilePattern.MatchString,
	},
	{
		// Generic fallback for real /dev/<disk> references: the denylist's
		// "destructive disk tool + real /dev/<disk> reference" rule
		// (denylist.go) only Blocks when the command also names a tool
		// this package specifically knows is destructive
		// (isDestructiveDiskTool). Anything else that still references a
		// real disk device — read-only inspection (`lsblk /dev/sda`,
		// `smartctl -a /dev/sda`, `fdisk -l /dev/sda`, `blkid /dev/sda`),
		// a mount, a non-destructive `badblocks -sv` test, a `dd
		// if=/dev/sda of=backup.img` disk image, or any tool this package
		// has never heard of — reaches this rule instead and is only
		// escalated to Confirm, never unconditionally Blocked. This is
		// what keeps auto mode from silently running unrecognized disk
		// tooling while not hard-blocking legitimate read-only access.
		name:  "real /dev/<disk> reference (not a recognized destructive disk tool)",
		risk:  RiskDestructive,
		match: hasRealDiskDeviceReference,
	},
}
