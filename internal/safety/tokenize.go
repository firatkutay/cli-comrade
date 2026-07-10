package safety

import "strings"

// normalizeCommand is the single normalization every matcher in this
// package — built-in denylist, user denylist_extra, and every escalation
// rule alike — runs against, instead of the raw command string Evaluate
// received. It strips every `"`, `'`, and “ ` “ character anywhere in
// the string (not just at token edges — a quote embedded mid-token, as in
// `of='/dev/sda'`, defeats a naive edge-trim but not a full-string strip)
// and collapses all whitespace runs to single spaces.
//
// This exists to close a real quote-fragility hole: a matcher written
// against the raw string can be defeated by a single stray quote
// (`dd if=/dev/zero of='/dev/sda'` no longer contains the literal
// substring `of=/dev/sda` once the quotes are counted). Normalizing once,
// centrally, in Engine.Evaluate before any rule ever runs (see
// engine.go), means every rule — regex-based or token-based — gets this
// hardening for free instead of each one needing its own quote-tolerant
// pattern.
//
// It is deliberately not full shell-grammar aware — no quote-aware
// grouping, no escape handling — so a quoted argument like
// `echo "rm -rf /"` normalizes to `echo rm -rf /`, tokenizing identically
// to an unquoted `rm -rf /`. That is an intentional, documented
// conservative-match choice (see docs/history/phases/FAZ-05.md "Decisions"): this
// package's rules would rather flag a string that merely *contains* a
// dangerous-looking invocation (a false positive, e.g. `echo "rm -rf /"`)
// than miss one actually executed via `sh -c "..."` / `bash -c '...'` (a
// false negative) — per CLAUDE.md's fail-closed safety mandate.
func normalizeCommand(command string) string {
	var b strings.Builder
	b.Grow(len(command))
	for _, r := range command {
		switch r {
		case '"', '\'', '`':
			continue
		default:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

// tokenizeCommand splits command into whitespace-separated tokens. Callers
// within this package always pass an already-normalizeCommand'd string
// (Engine.Evaluate normalizes once, up front — see engine.go), so no
// quote-stripping happens here anymore; tokenizeCommand is kept as a
// separate, still-useful step (whitespace splitting) in its own right, and
// remains safe to call on a non-normalized string too, since
// strings.Fields never fails on stray quote characters — they simply
// remain part of whatever token they're attached to.
func tokenizeCommand(command string) []string {
	return strings.Fields(command)
}

// isSeparatorToken reports whether tok is a shell command-separator token
// (as its own whitespace-delimited word) that stops a forward scan
// through a command's arguments — so `rm -rf ./build ; echo /` does not
// let the unrelated trailing `/` count as rm's target.
func isSeparatorToken(tok string) bool {
	switch tok {
	case ";", "&&", "||", "|", "&":
		return true
	default:
		return false
	}
}

// argsAfter returns the tokens in tokens[start:] up to (but excluding) the
// next separator token, or the whole remainder if none is found.
func argsAfter(tokens []string, start int) []string {
	for i := start; i < len(tokens); i++ {
		if isSeparatorToken(tokens[i]) {
			return tokens[start:i]
		}
	}
	return tokens[start:]
}

// hasFlag reports whether args (the tokens following a command name)
// contain the long flag name (matched case-insensitively, e.g.
// "-Recurse") or a short combined flag containing short (e.g. 'r' inside
// "-rf"/"-fr"/"-r"). Pass short as 0 to disable the short-flag check for a
// command family that only has a long form.
func hasFlag(args []string, short byte, long string) bool {
	for _, t := range args {
		if long != "" && strings.EqualFold(t, long) {
			return true
		}
		if short != 0 && len(t) > 1 && t[0] == '-' && t[1] != '-' && strings.IndexByte(t, short) >= 0 {
			return true
		}
	}
	return false
}
