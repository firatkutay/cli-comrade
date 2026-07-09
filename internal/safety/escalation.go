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
}
