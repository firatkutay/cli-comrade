package safety

import "strings"

// normalizeCommand is the single normalization every matcher in this
// package — built-in denylist, user denylist_extra, and every escalation
// rule alike — runs against, instead of the raw command string Evaluate
// received. It strips every `"`, `'`, and “ ` “ character anywhere in
// the string (not just at token edges — a quote embedded mid-token, as in
// `of='/dev/sda'`, defeats a naive edge-trim but not a full-string strip),
// unwraps `$(...)` command-substitution boundaries (see
// unwrapCommandSubstitution), and collapses all whitespace runs to single
// spaces.
//
// This exists to close two real evasion holes. First, a matcher written
// against the raw string can be defeated by a single stray quote
// (`dd if=/dev/zero of='/dev/sda'` no longer contains the literal
// substring `of=/dev/sda` once the quotes are counted). Second, a command
// substitution like `$(rm -rf /)` hides `rm -rf /` from every token-based
// rule until a shell actually expands it — normalizing the `$(`/`)`
// boundary away up front means `isRmRootDelete` sees the same tokens it
// would for a bare `rm -rf /`, instead of only escalation catching it.
// Normalizing once, centrally, in Engine.Evaluate before any rule ever
// runs (see engine.go), means every rule — regex-based or token-based —
// gets both hardenings for free instead of each one needing its own
// quote-/substitution-tolerant pattern.
//
// It is deliberately not full shell-grammar aware — no quote-aware
// grouping, no escape handling, no variable expansion — so a quoted
// argument like `echo "rm -rf /"` normalizes to `echo rm -rf /`,
// tokenizing identically to an unquoted `rm -rf /`. That is an
// intentional, documented conservative-match choice (see
// docs/history/phases/FAZ-05.md "Decisions"): this package's rules would
// rather flag a string that merely *contains* a dangerous-looking
// invocation (a false positive, e.g. `echo "rm -rf /"`) than miss one
// actually executed via `sh -c "..."` / `bash -c '...'` (a false
// negative) — per CLAUDE.md's fail-closed safety mandate. Applying
// normalizeCommand to an already-normalized string is idempotent: every
// quote/`$(`/matched-`)` it would have stripped is already gone, so a
// second pass is a no-op beyond the (already-collapsed) whitespace join.
func normalizeCommand(command string) string {
	stripped := stripQuoteChars(command)
	unwrapped := unwrapCommandSubstitution(stripped)
	return strings.Join(strings.Fields(unwrapped), " ")
}

// stripQuoteChars removes every `"`, `'`, and “ ` “ character anywhere in
// command, unconditionally (not paired-aware) — see normalizeCommand's
// doc comment.
func stripQuoteChars(command string) string {
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
	return b.String()
}

// unwrapCommandSubstitution removes the `$(` / `)` delimiters of every
// `$(...)` command-substitution boundary, nesting-aware, flattening its
// contents into the surrounding text so a denylist/escalation rule sees
// `$(rm -rf /)` exactly as it would see a bare `rm -rf /`. A bare `$` not
// followed by `(` (as in `$HOME`) is left untouched — rootTargets' `$HOME`
// entry depends on that.
//
// It deliberately leaves every OTHER, non-substitution parenthesis
// untouched: a stack tracks, for each `(` encountered, whether it opened
// via `$(` (true) or on its own (false), and a `)` is only dropped from
// the output when it closes a `$(` — otherwise it is kept exactly as
// written. This is what lets the fork-bomb denylist rule's literal
// `:(){ :|:& };:` signature — whose `()` are bare, not a substitution —
// survive normalization completely unmodified.
func unwrapCommandSubstitution(s string) string {
	runes := []rune(s)
	var b strings.Builder
	b.Grow(len(s))
	var subOpen []bool
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case r == '$' && i+1 < len(runes) && runes[i+1] == '(':
			subOpen = append(subOpen, true)
			i++ // also consume the '(' — both delimiter runes are dropped
		case r == '(':
			subOpen = append(subOpen, false)
			b.WriteRune(r)
		case r == ')' && len(subOpen) > 0:
			last := subOpen[len(subOpen)-1]
			subOpen = subOpen[:len(subOpen)-1]
			if !last {
				b.WriteRune(r)
			}
			// last == true: this ')' closed a "$(" — drop it too.
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
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
// "-rf"/"-fr"/"-r"/"-Rf"/"-RF"/"-rF"). Pass short as 0 to disable the
// short-flag check for a command family that only has a long form.
//
// The short-flag scan lower-cases the token before searching (short
// itself is always passed as an ASCII lowercase letter by every call site
// in this package): `rm`'s `-r`/`-R` and `-f`/`-F` are both valid,
// case-insensitive spellings of the same flag, and matching only the
// lowercase form let a capitalized `rm -Rf /` slip past isRmRootDelete's
// denylist check entirely (MEDIUM finding #5) — it still reached
// Confirm via the case-tolerant escalation regex, but never the
// unconditional Block a root delete must get.
func hasFlag(args []string, short byte, long string) bool {
	for _, t := range args {
		if long != "" && strings.EqualFold(t, long) {
			return true
		}
		if short != 0 && len(t) > 1 && t[0] == '-' && t[1] != '-' && strings.IndexByte(strings.ToLower(t), short) >= 0 {
			return true
		}
	}
	return false
}
