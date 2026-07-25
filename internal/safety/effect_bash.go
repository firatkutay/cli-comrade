package safety

import (
	"errors"
	"path"
	"strings"

	"mvdan.cc/sh/v3/expand"
	"mvdan.cc/sh/v3/syntax"
)

// fetcherCommandWords is the set of command words this package's AST
// layer treats as network fetchers ONLY when they appear as a
// *syntax.CallExpr's resolved Args[0] — its command word — never merely
// as a substring anywhere on the line. This is the disambiguation the
// string-regex signature layer's fetchPipeInterpreterPattern deliberately
// cannot do (see escalation.go's comment on that pattern): "http"
// (httpie) collides with the "http://"/"https://" URL-scheme substring,
// and "fetch" (BSD) is an ordinary English word, so a whole-string regex
// naming either would false-escalate `echo see http://example.com`. The
// AST gives us the one piece of information a regex never has — which
// word is structurally in COMMAND-WORD position — closing the gap
// without reopening that collision. See checkFetcherPipeline.
var fetcherCommandWords = map[string]bool{
	"http":  true,
	"fetch": true,
}

// isInterpreterCommandWord reports whether word (a resolved command word,
// matched case-insensitively) names a script/shell interpreter — the
// same interpreter family fetchPipeInterpreterPattern's own alternation
// already accepts (sh/bash/zsh/python*/pwsh), just tested structurally
// against a resolved *syntax.CallExpr's own command word instead of via a
// whole-string regex.
func isInterpreterCommandWord(word string) bool {
	switch strings.ToLower(word) {
	case "sh", "bash", "zsh", "dash", "ksh", "pwsh", "powershell":
		return true
	}
	return strings.HasPrefix(strings.ToLower(word), "python")
}

// errUnexpectedProcSubst is resolveWord's cfg.ProcSubst stand-in error for
// a NON-STRICT (argument-position) word: it turns a live *syntax.ProcSubst
// node into a clean, fail-closed ("", false) result — see resolveWord's
// doc comment. A STRICT-position (command-word or assignment-value)
// ProcSubst never reaches this stand-in at all: wordIsStrictlyResolvable
// already rejects it before expand.Literal is ever called.
var errUnexpectedProcSubst = errors.New("safety: unexpected process substitution")

// analyzeBashEffect is this package's bash/POSIX AST effect analyzer: it
// parses command with mvdan.cc/sh/v3/syntax, then sequentially resolves
// simple variable indirection through the command's own literal
// assignments (`R=rm; $R -rf /`), then re-runs the EXACT SAME builtin
// denylist/escalation matchers engine.go already trusts against the
// RESOLVED text, so this layer never needs its own parallel copy of the
// risk taxonomy (derive from the existing rules, don't hand-mirror them).
// A dedicated structural check on top of that (checkFetcherPipeline)
// catches the one shape resolved-text reuse cannot: an `http`/`fetch`
// fetcher in COMMAND-WORD position piped into an interpreter — see
// fetcherCommandWords' doc comment.
//
// Fails closed (returns an indeterminateVerdict) on: a parse error; a
// CmdSubst, ProcSubst, ArithmExp, bash extglob, or non-simple/unresolved
// parameter expansion in COMMAND-WORD position specifically (the one
// place "we don't know what this actually runs" is most dangerous — see
// resolveWord's strict-mode doc comment); or an unsupported assignment
// shape (array/associative/naked/`+=`/indexed — see resolveCallExpr,
// which resolves every assignment VALUE in the same strict, fail-closed
// mode as a command word, since a mis-resolved variable can poison every
// later reference to it).
//
// A CmdSubst/ProcSubst/ArithmExp/complex-ParamExp in ARGUMENT position
// (anywhere other than the two strict positions above) is deliberately
// NOT fail-closed by DEFAULT: it resolves to whatever expand.Literal's
// default unset-variable/no-CmdSubst-handler behavior produces (empty
// string for a $(...) or <(...) — see resolveWord's non-strict branch —
// or the real computed value for arithmetic and default-value forms),
// exactly like a real shell. Treating an unresolvable argument as ""
// is safe ONLY when the danger a rule is looking for would have to be
// textually present in the substitution's own body to matter (that is
// what makes `diff <(ls a) <(ls b)` and `echo $(date)` correctly stay
// unescalated: the pre-existing signature layer's normalizeCommand step
// already textually unwraps `$(...)` before either layer's rules run —
// tokenize.go — and already regex-matches a fetch verb INSIDE a `<(...)`
// directly against the raw command — processSubstitutionFetchPattern,
// escalation.go — both independent of this analyzer, so nothing is lost
// by not ALSO rejecting the whole command here). It is UNSAFE, and
// exactly the false-Allow this package's own security review caught,
// when the danger is instead a DEVICE/FILE TARGET computed from running
// a command or reading a file (`dd of=$(cat dev.txt)`,
// `wipefs -a $(cat dev.txt)`) rather than a literal string sitting inside
// the substitution: neither layer can see "which disk" without actually
// running the substitution, so "" would silently drop the target
// entirely instead of flagging the command as unknown. resolveCallExpr's
// isDiskTargetDestructiveWord check is the exception that keeps that case
// fail-closed while leaving diff/echo/every non-disk-destructive tool's
// substitution arguments at their safe, accurate "" default.
//
// Scope boundary, deliberate and documented rather than an oversight:
// *syntax.CallExpr (simple commands), *syntax.BinaryCmd (&&, ||, |, |&
// chains), *syntax.IfClause, *syntax.WhileClause, *syntax.ForClause,
// *syntax.CaseClause, *syntax.Block (`{ }`), *syntax.Subshell (`( )`), and
// *syntax.DeclClause (declare/local/export/readonly/typeset) are all
// walked — see resolveCommand's dispatch and resolveIfClause/
// resolveWhileClause/resolveCaseClause/resolveDeclClause below — so an
// assignment made INSIDE one of these constructs is visible to the same
// resolution logic a top-level assignment already gets, closing the
// control-structure-plus-indirection gap the security audit flagged
// (`if true; then R=rm; $R -rf /; fi` used to Allow: R was never added to
// this analyzer's env at all, since nothing walked into the `if`'s Then
// branch). Every other Command shape — function bodies (*syntax.FuncDecl),
// *syntax.ArithmCmd, *syntax.TestClause, *syntax.LetClause,
// *syntax.TimeClause, *syntax.CoprocClause — still contributes nothing to
// this analyzer's OWN verdict rather than forcing the whole command
// indeterminate. This remains safe, not just convenient: a dangerous
// command wrapped in one of those genuinely unmodeled shapes is still
// caught by the pre-existing signature layer's own structure-agnostic,
// substring/token-based pass over the RAW command, completely
// independently of this analyzer — see engine.go's Evaluate, which runs
// both layers unconditionally and takes their max. And a variable
// assigned only inside one of those still-unmodeled shapes is simply
// never added to this analyzer's env map, so a later command-word
// reference to it (`$R`) still fails closed via the unresolved-
// command-word-position rule above — "not modeled" degrades to
// "indeterminate", never to "silently resolved wrong".
//
// Two narrower, deliberate limits inside the now-walked constructs, kept
// intentionally simple rather than modeling full control-flow:
//
//  1. An if/elif/else's Then/Else branches, and each case item's Stmts,
//     are mutually exclusive at runtime — at most one of them actually
//     executes — so each is resolved against its OWN CLONE of the env as
//     of entering the construct (resolveScopedStmts), never chained
//     sequentially into one another and never leaked back to the
//     surrounding scope. This is what keeps two mutually exclusive
//     branches assigning the SAME variable to DIFFERENT literal values
//     from silently picking whichever branch happened to be visited last
//     as if it were the only one that could run (a real false-negative
//     risk a naive shared-env walk would introduce), while still letting
//     `if true; then R=rm; $R -rf /; fi` resolve correctly: the
//     assignment and its use are both inside the SAME branch's own clone.
//     A subshell (`( )`) is forked the same way, since it always runs in
//     its own child environment and never leaks assignments back out —
//     real bash semantics, not just a conservative approximation. An
//     if/while's own Cond, and a while/for's own Do body, are NOT forked
//     (plain r.resolveStmts): they are not mutually-exclusive
//     alternatives, they always run in the CURRENT scope in real bash,
//     exactly like a top-level statement list.
//  2. A for-loop's own iteration variable (*syntax.WordIter's Name) is
//     deliberately NEVER added to env: its real value is whichever of
//     the loop's Items is bound on a given iteration, which this
//     single-pass analyzer cannot enumerate. Leaving it absent from env
//     means a reference to it in ARGUMENT position (the common,
//     idiomatic shape — `for f in *.go; do gofmt -l $f; done`) resolves
//     to "" and stays inert (safe, see the ARGUMENT-position discussion
//     below), while a reference to it in COMMAND-WORD position fails
//     closed via the same unresolved-command-word rule as any other
//     unknown variable — never silently resolved to a guessed item.
func analyzeBashEffect(command string) effectVerdict {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return indeterminateVerdict("parse error: " + err.Error())
	}

	resolver := &bashResolver{env: map[string]string{}}
	text, indeterminate, reason := resolver.resolveStmts(file.Stmts)
	if indeterminate {
		return indeterminateVerdict(reason)
	}

	verdict := effectVerdict{}
	normalized := normalizeCommand(text)
	tokens := tokenizeCommand(normalized)
	for _, rule := range builtinDenylist {
		if rule.match(normalized, tokens) {
			verdict = maxVerdict(verdict, effectVerdict{
				risk:   RiskDestructive,
				reason: "effect: resolved argv matches denylist signature '" + rule.name + "'",
			})
		}
	}
	for _, rule := range escalationRules {
		if rule.risk > verdict.risk && rule.match(normalized) {
			verdict = maxVerdict(verdict, effectVerdict{
				risk:   rule.risk,
				reason: "effect: resolved argv matches escalation signature '" + rule.name + "'",
			})
		}
	}
	for _, finding := range resolver.findings {
		verdict = maxVerdict(verdict, finding)
	}

	return verdict
}

// bashResolver sequentially resolves a bash/POSIX statement list into a
// single reconstructed text, threading a plain map[string]string of
// resolved variable assignments forward exactly as a real shell would —
// see analyzeBashEffect's doc comment for what it deliberately does and
// does not model. findings accumulates structural results
// (checkFetcherPipeline) discovered along the way that resolved-text
// signature reuse cannot express as a regex.
type bashResolver struct {
	env      map[string]string
	findings []effectVerdict
}

// resolveStmts resolves a statement list in order, joining each
// statement's own reconstructed text with " ; " so the reused denylist/
// escalation matchers see equivalent structure (a separator between
// independent statements) to what the original command had.
func (r *bashResolver) resolveStmts(stmts []*syntax.Stmt) (text string, indeterminate bool, reason string) {
	var parts []string
	for _, stmt := range stmts {
		t, indet, why := r.resolveStmt(stmt)
		if indet {
			return "", true, why
		}
		if t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " ; "), false, ""
}

// resolveStmt resolves one statement's command plus its own redirections.
func (r *bashResolver) resolveStmt(stmt *syntax.Stmt) (text string, indeterminate bool, reason string) {
	cmdText, indet, why := r.resolveCommand(stmt.Cmd)
	if indet {
		return "", true, why
	}
	parts := []string{cmdText}
	for _, rd := range stmt.Redirs {
		val, ok := resolveWord(rd.Word, r.env, false)
		if !ok {
			return "", true, "unresolved redirect target"
		}
		parts = append(parts, rd.Op.String()+val)
	}
	return strings.Join(parts, " "), false, ""
}

// resolveCommand dispatches on the Command's concrete node type — see
// analyzeBashEffect's "Scope boundary" doc-comment section for exactly
// which shapes are walked, which are deliberately left unmodeled (and why
// that degrades safely to indeterminate rather than to a silently wrong
// resolution), and the two narrower documented limits within the walked
// set (mutually-exclusive-branch scoping, the for-loop iteration
// variable).
func (r *bashResolver) resolveCommand(cmd syntax.Command) (text string, indeterminate bool, reason string) {
	switch c := cmd.(type) {
	case *syntax.CallExpr:
		return r.resolveCallExpr(c)
	case *syntax.BinaryCmd:
		if c.Op == syntax.Pipe || c.Op == syntax.PipeAll {
			r.checkFetcherPipeline(c)
		}
		leftText, indet, why := r.resolveStmt(c.X)
		if indet {
			return "", true, why
		}
		rightText, indet, why := r.resolveStmt(c.Y)
		if indet {
			return "", true, why
		}
		return leftText + " " + c.Op.String() + " " + rightText, false, ""
	case *syntax.IfClause:
		return r.resolveIfClause(c)
	case *syntax.WhileClause:
		return r.resolveWhileClause(c)
	case *syntax.ForClause:
		// The loop body always runs in the CURRENT scope in real bash (no
		// subshell fork) — see analyzeBashEffect's doc comment on why the
		// iteration variable itself is deliberately never added to env.
		return r.resolveStmts(c.Do)
	case *syntax.CaseClause:
		return r.resolveCaseClause(c)
	case *syntax.Block:
		// `{ }` runs in the CURRENT scope in real bash — no fork, unlike a
		// subshell — so a plain same-resolver resolveStmts is correct.
		return r.resolveStmts(c.Stmts)
	case *syntax.Subshell:
		return r.resolveScopedStmts(cloneEnv(r.env), c.Stmts)
	case *syntax.DeclClause:
		return r.resolveDeclClause(c)
	default:
		return "", false, ""
	}
}

// resolveScopedStmts resolves stmts against env — which may be r.env
// itself (a same-scope construct: an if/while's Cond, a while/for's Do
// body, or a `{ }` block, none of which fork in real bash) or a fresh
// clone (a construct that forks its own scope: a subshell, or one
// mutually-exclusive branch of an if/elif/else or case, where at most one
// of several alternatives actually runs and none may leak an assignment
// into a sibling alternative or the surrounding scope) — merging any
// findings (checkFetcherPipeline) the nested resolution accumulates back
// into r, since those are always worth surfacing regardless of which
// scope discovered them.
func (r *bashResolver) resolveScopedStmts(env map[string]string, stmts []*syntax.Stmt) (text string, indeterminate bool, reason string) {
	child := &bashResolver{env: env}
	text, indeterminate, reason = child.resolveStmts(stmts)
	r.findings = append(r.findings, child.findings...)
	return text, indeterminate, reason
}

// resolveIfClause resolves one if/elif/else chain: Cond always runs in
// the CURRENT scope (a real if condition is not a mutually-exclusive
// alternative — it unconditionally executes when control reaches it), so
// its own assignments persist into r.env directly via plain
// r.resolveStmts; Then is resolved against a FRESH CLONE of r.env (see
// resolveScopedStmts) since at most one of Then/Else ever actually runs.
// c.Else (either a further "elif" with its own Cond, or a plain "else"
// with Cond empty) is recursed into with the SAME base r.env — not
// chained through Then's own clone — so an "elif"/"else" branch forks
// from the state as of entering the WHOLE chain, never from a sibling
// branch that (at runtime) would never have run alongside it.
func (r *bashResolver) resolveIfClause(c *syntax.IfClause) (text string, indeterminate bool, reason string) {
	condText, indet, why := r.resolveStmts(c.Cond)
	if indet {
		return "", true, why
	}
	thenText, indet, why := r.resolveScopedStmts(cloneEnv(r.env), c.Then)
	if indet {
		return "", true, why
	}
	var parts []string
	parts = appendNonEmpty(parts, condText)
	parts = appendNonEmpty(parts, thenText)
	if c.Else != nil {
		elseText, indet, why := r.resolveIfClause(c.Else)
		if indet {
			return "", true, why
		}
		parts = appendNonEmpty(parts, elseText)
	}
	return strings.Join(parts, " ; "), false, ""
}

// resolveWhileClause resolves one while/until clause: neither Cond nor Do
// forks a new scope in real bash (the loop body may run zero or more
// times, but it is the SAME code path each time, not a mutually-exclusive
// alternative the way an if/case branch is), so both are resolved via
// plain r.resolveStmts against the shared, persistent env.
func (r *bashResolver) resolveWhileClause(w *syntax.WhileClause) (text string, indeterminate bool, reason string) {
	condText, indet, why := r.resolveStmts(w.Cond)
	if indet {
		return "", true, why
	}
	doText, indet, why := r.resolveStmts(w.Do)
	if indet {
		return "", true, why
	}
	var parts []string
	parts = appendNonEmpty(parts, condText)
	parts = appendNonEmpty(parts, doText)
	return strings.Join(parts, " ; "), false, ""
}

// resolveCaseClause resolves a case/switch clause: exactly one CaseItem's
// Stmts actually runs at runtime, so — exactly like resolveIfClause's
// Then/Else — each item is resolved against its OWN fresh clone of the
// env as of entering the whole case statement (resolveScopedStmts), never
// chained from one item into the next and never leaked back to the
// surrounding scope.
func (r *bashResolver) resolveCaseClause(c *syntax.CaseClause) (text string, indeterminate bool, reason string) {
	var parts []string
	for _, item := range c.Items {
		t, indet, why := r.resolveScopedStmts(cloneEnv(r.env), item.Stmts)
		if indet {
			return "", true, why
		}
		parts = appendNonEmpty(parts, t)
	}
	return strings.Join(parts, " ; "), false, ""
}

// resolveDeclClause resolves one declare/local/export/readonly/typeset
// clause. Each Arg is one of three shapes (see syntax.Assign's own doc
// comment on Naked): a bare option/flag (Naked, Name == nil — "-r", "-x",
// ...), a bare "NAME" declaration with no assigned value (Naked, Name !=
// nil, Value == nil), or a normal "NAME=value" assignment (not Naked).
// The first two contribute nothing this analyzer can resolve — skipping
// them is safe, not a silently-wrong guess, exactly like any other
// unmodeled shape: it simply leaves the name (if any) absent from env, so
// a later reference degrades to strictly-unresolvable/fail-closed rather
// than resolving to a wrong value. A normal assignment is resolved and
// persisted into r.env with the exact same strict, fail-closed contract
// resolveCallExpr's own per-invocation assignments use (unsupported
// shapes — array, indexed, `+=` — fail closed rather than silently
// dropping information; see resolveCallExpr's own doc comment for why).
// declare/local/export/readonly/typeset are all treated identically here:
// this analyzer does not model function-local scoping (function bodies —
// *syntax.FuncDecl — are not walked at all, see analyzeBashEffect's
// "Scope boundary" section), so there is no narrower scope for "local" to
// mean in the shapes this package actually analyzes (single command
// lines, not multi-function scripts).
func (r *bashResolver) resolveDeclClause(d *syntax.DeclClause) (text string, indeterminate bool, reason string) {
	var parts []string
	for _, a := range d.Args {
		if a.Naked {
			continue
		}
		if a.Name == nil || a.Value == nil || a.Index != nil || a.Append || a.Array != nil {
			return "", true, "unsupported declare assignment shape (array, associative, indexed, or += append)"
		}
		val, ok := resolveWord(a.Value, r.env, true)
		if !ok {
			why, _ := strictUnresolvableReason(a.Value, r.env)
			if why == "" {
				why = "unresolved value"
			}
			return "", true, why + " assigned to " + a.Name.Value + " in declare/local/export/readonly/typeset"
		}
		r.env[a.Name.Value] = val
		parts = append(parts, a.Name.Value+"="+val)
	}
	return strings.Join(parts, " "), false, ""
}

// appendNonEmpty appends s to parts only when s is non-empty — the same
// "a statement with nothing to say contributes no text" filtering
// resolveStmts already applies to a whole statement list, reused by every
// compound-command resolver above so an empty branch (e.g. an if with no
// Else) never leaves a stray, empty joined segment in the reconstructed
// text.
func appendNonEmpty(parts []string, s string) []string {
	if s == "" {
		return parts
	}
	return append(parts, s)
}

// resolveCallExpr resolves one simple command: its own assignments (both
// the persistent "R=rm" form and the per-invocation "FOO=bar cmd" form —
// see syntax.CallExpr's own doc comment on the distinction) and, if it
// has any Args, the command word (Args[0], STRICT resolution — see
// resolveWord) and every remaining argument (non-strict resolution).
func (r *bashResolver) resolveCallExpr(c *syntax.CallExpr) (text string, indeterminate bool, reason string) {
	// Per-invocation assigns ("FOO=bar cmd") apply only to this call and
	// must never leak into r.env; a bare assignment-only statement
	// ("FOO=bar", no Args) instead sets a persistent shell variable later
	// statements can see. Cloning env here (instead of mutating r.env
	// directly) is what keeps the two cases apart.
	workEnv := r.env
	if len(c.Args) > 0 && len(c.Assigns) > 0 {
		workEnv = cloneEnv(r.env)
	}

	var assignTexts []string
	for _, a := range c.Assigns {
		// Anything besides a plain "NAME=value" is out of scope and
		// fails closed rather than resolving to a silently WRONG value:
		// an array/associative assign (a.Value == nil, a.Array set
		// instead), a naked DeclClause-only assign (a.Name == nil), an
		// indexed-element assign ("arr[i]=x" — a.Index != nil, which
		// this package does not attempt to model as anything other than
		// a plain scalar), or "+=" append (a.Append — silently treating
		// it as overwrite would DROP the prior value, which can only
		// ever make a later resolved argument LESS accurate, e.g.
		// missing the "/dev/" prefix of an accumulated "/dev/"+"sda").
		if a.Name == nil || a.Value == nil || a.Index != nil || a.Append {
			return "", true, "unsupported assignment shape (array, associative, indexed, naked, or += append)"
		}
		val, ok := resolveWord(a.Value, workEnv, true)
		if !ok {
			why, _ := strictUnresolvableReason(a.Value, workEnv)
			if why == "" {
				why = "unresolved value"
			}
			return "", true, why + " assigned to " + a.Name.Value
		}
		workEnv[a.Name.Value] = val
		assignTexts = append(assignTexts, a.Name.Value+"="+val)
	}

	if len(c.Args) == 0 {
		// Assignment-only statement: env was already mutated in place
		// above (workEnv is r.env itself in this branch).
		return strings.Join(assignTexts, " "), false, ""
	}

	cmdWord, cmdWordOK := resolveWord(c.Args[0], workEnv, true)
	if !cmdWordOK {
		why, _ := strictUnresolvableReason(c.Args[0], workEnv)
		if why == "" {
			why = "unresolved command word"
		}
		return "", true, why + " in command-word position"
	}

	argTexts := make([]string, len(c.Args))
	argTexts[0] = cmdWord
	for i := 1; i < len(c.Args); i++ {
		val, ok := resolveWord(c.Args[i], workEnv, false)
		if !ok {
			// Only a CmdSubst/ProcSubst makes a non-strict word
			// unresolvable (see resolveWord's doc comment) — i.e. this
			// argument's real value comes from running a command or
			// reading a file, not from a variable this analyzer already
			// tracks. For most tools that is safe to treat as "" (see
			// analyzeBashEffect's doc comment): an empty resolved
			// argument can only cause a rule to NOT match. But for a
			// tool whose device/file TARGET is exactly this kind of
			// argument (dd's of=, wipefs/blkdiscard/shred/sgdisk/
			// badblocks/cryptsetup/sfdisk/tee's positional target), "" is
			// not a safe stand-in for "unknown, possibly /dev/sda" — a
			// dynamically-computed disk target must fail closed instead
			// of silently vanishing from the reconstructed argv (see
			// isDiskTargetDestructiveWord).
			if isDiskTargetDestructiveWord(cmdWord) {
				return "", true, "unresolved substitution in argument to destructive disk tool " + cmdWord
			}
			val = "" // unset-variable-expands-to-empty, same as a real shell.
		}
		argTexts[i] = val
	}
	return strings.Join(append(assignTexts, argTexts...), " "), false, ""
}

// diskTargetDestructiveWords is the set of command words (matched by
// basename, case-insensitively — see isDiskTargetDestructiveWord) whose
// own arguments name the device/file they destructively write to, wipe,
// or repartition/reformat, covering both the exact tool set denylist.go's
// alwaysDestructiveDiskToolNames/sfdiskDestructiveFlags/
// cryptsetupDestructiveActions already treat as destructive-to-their-
// target (dd (of=), wipefs, blkdiscard, sgdisk, shred, badblocks (-w),
// cryptsetup (destructive subcommands), sfdisk (destructive flags), tee)
// AND the partition-table/low-level-format/secure-erase family the
// original set missed (parted, fdisk, gdisk, cfdisk, cgdisk, partx,
// partprobe, hdparm (secure-erase), nvme (format), dmsetup). This
// analyzer does not re-derive any of those tools' own destructive-flag
// gating here — any of these words appearing as the resolved command word
// of an invocation with an UNRESOLVABLE target argument fails closed
// regardless of flags (or subcommand, for nvme/dmsetup), which is
// intentionally broader (never narrower) than the denylist's own
// flag-gated definition of "destructive": an argument this analyzer
// cannot see into is exactly the case where trusting a flag-based
// allowlist read from the SAME unresolved command line would be
// misplaced confidence. Accepted, deliberate consequence: a read-only
// dynamic-target invocation of one of these tools (e.g. `fdisk -l
// $(cat d)`) now also Confirms — over-confirming a read is the safe
// direction of this fail-closed posture, not a bug.
//
// Two honest limits of this set, recorded rather than silently assumed:
//
//  1. This is a HAND-MAINTAINED ALLOWLIST, and is therefore categorically
//     weaker than the literal-"/dev/<disk>" net this package's signature
//     layer already casts (hasRealDiskDeviceReference, denylist.go/
//     escalation.go), which is deliberately NOT an allowlist — it matches
//     any real device path regardless of which tool references it. A
//     destructive disk tool whose name is not (yet) in this set, invoked
//     with a DYNAMIC (substitution) target, is a Confirm-vs-Allow gap
//     until it is added here — this is a known, accepted limitation of
//     the AST effect layer specifically, not of the classifier as a
//     whole: the signature Block floor still catches every LITERAL
//     "/dev/<disk>" target and the mkfs*/diskpart-family shapes
//     unconditionally, regardless of which tool name appears, and regardless
//     of whether this set is complete.
//  2. "tee" is INTENTIONALLY in this set, even though tee is an ordinary,
//     widely-useful command. This closes `tee $(cat dev)` — a dynamic
//     raw-device write — at the direct cost of over-confirming the
//     entirely benign `tee $(mktemp)` / `... | tee $(date +%s).log`
//     idiom in auto mode. That tradeoff is a deliberate
//     safety-over-convenience choice, not an oversight: Confirm is
//     monotonic and safe (it only ever adds a prompt, never skips one,
//     and only auto mode is affected — ask mode already prompts every
//     step), so the cost is user friction, never a missed destructive
//     write.
var diskTargetDestructiveWords = map[string]bool{
	"dd":         true,
	"wipefs":     true,
	"blkdiscard": true,
	"sgdisk":     true,
	"shred":      true,
	"badblocks":  true,
	"cryptsetup": true,
	"sfdisk":     true,
	"tee":        true,
	"parted":     true,
	"fdisk":      true,
	"gdisk":      true,
	"cfdisk":     true,
	"cgdisk":     true,
	"partx":      true,
	"partprobe":  true,
	"hdparm":     true,
	"nvme":       true,
	"dmsetup":    true,
}

// isDiskTargetDestructiveWord reports whether cmdWord (a resolved command
// word) names a tool in diskTargetDestructiveWords, matched by basename
// and case-insensitively — the same matching convention
// isDestructiveDiskTool (denylist.go) and isInterpreterCommandWord (this
// file) already use for a resolved/tokenized command name.
func isDiskTargetDestructiveWord(cmdWord string) bool {
	return diskTargetDestructiveWords[strings.ToLower(path.Base(cmdWord))]
}

// checkFetcherPipeline appends a structural finding to r.findings if bc's
// pipeline (flattened via flattenPipeline) opens with an http/fetch
// command word (fetcherCommandWords) followed, in any later stage, by a
// known interpreter command word (isInterpreterCommandWord). Best-effort:
// a stage whose command word cannot be resolved with the current env is
// silently skipped rather than forcing indeterminate — this is a
// supplementary finding layered on top of, never a substitute for,
// resolveStmts' own fail-closed contract above.
func (r *bashResolver) checkFetcherPipeline(bc *syntax.BinaryCmd) {
	stages := flattenPipeline(bc)
	if len(stages) < 2 {
		return
	}
	firstWord, ok := stageCommandWord(stages[0], r.env)
	if !ok || !fetcherCommandWords[strings.ToLower(firstWord)] {
		return
	}
	for _, stage := range stages[1:] {
		w, ok := stageCommandWord(stage, r.env)
		if ok && isInterpreterCommandWord(w) {
			r.findings = append(r.findings, effectVerdict{
				risk: RiskElevated,
				reason: "effect: fetcher '" + firstWord +
					"' in command-word position piped into interpreter '" + w + "'",
			})
			return
		}
	}
}

// flattenPipeline returns bc's pipeline stages left to right: bash parses
// `A | B | C` as a LEFT-nested BinaryCmd (`(A | B) | C`), so the first
// stage's own Stmt may itself be a further Pipe/PipeAll BinaryCmd that
// needs flattening; any other stage is a leaf.
func flattenPipeline(bc *syntax.BinaryCmd) []*syntax.Stmt {
	var stages []*syntax.Stmt
	if left, isBinary := bc.X.Cmd.(*syntax.BinaryCmd); isBinary && (left.Op == syntax.Pipe || left.Op == syntax.PipeAll) {
		stages = append(stages, flattenPipeline(left)...)
	} else {
		stages = append(stages, bc.X)
	}
	return append(stages, bc.Y)
}

// stageCommandWord returns a pipeline stage's resolved command word (its
// CallExpr's Args[0], strictly resolved — see resolveWord), or ("",
// false) if the stage is not a plain CallExpr, has no Args, or its
// command word cannot be resolved.
func stageCommandWord(stage *syntax.Stmt, env map[string]string) (string, bool) {
	c, isCall := stage.Cmd.(*syntax.CallExpr)
	if !isCall || len(c.Args) == 0 {
		return "", false
	}
	return resolveWord(c.Args[0], env, true)
}

// isSimpleParamExp reports whether p is a bare "$NAME" or brace "${NAME}"
// parameter reference and nothing more elaborate — no indirection
// (${!name}), length (${#name}), default/alternate-value operators
// (${name:-x}), replacement (${name/x/y}), slicing, name-matching, or
// Zsh's nested-parameter form. Every one of those is a construct this
// analyzer does not attempt to emulate; see resolveWord's strict-mode doc
// comment for why that is a deliberate fail-closed choice for
// command-word position, not a gap.
func isSimpleParamExp(p *syntax.ParamExp) bool {
	return p.Param != nil &&
		p.Index == nil &&
		p.Modifiers == nil &&
		p.Slice == nil &&
		p.Repl == nil &&
		p.Names == 0 &&
		p.Exp == nil &&
		p.NestedParam == nil &&
		!p.Excl && !p.Length && !p.Width && !p.IsSet
}

// wordHasLeadingUnescapedTilde reports whether w's FIRST word part is a
// plain, unquoted *syntax.Lit whose value begins with "~" — exactly the
// shape mvdan.cc/sh/v3/expand's own tilde expansion acts on (see
// (*expand.Config).expandUser, gated on "i == 0 && ql == quoteNone" in
// mvdan.cc/sh/v3/expand/expand.go): a bare/current-user tilde ("~",
// "~/foo") reads the "HOME" entry of whatever expand.Environ it is given,
// and a named-user tilde ("~name") additionally calls os/user.Lookup — a
// real host syscall/NSS lookup this package's own resolution has no
// business making, since analyzeBashEffect's whole contract is being a
// PURE function of (command, dialect), never of what accounts happen to
// exist on the machine currently running comrade. Rejecting every leading
// unescaped tilde in strict position — not only the "~name" spelling that
// triggers the Lookup — keeps this gate simple (one rule, not "~name only
// but ~ and ~/foo are fine") and entirely removes the host-dependent
// branch from the strict path: resolveWord's strict gate is checked
// BEFORE expand.Literal is ever invoked, so os/user.Lookup is never
// reached in strict position at all, "dropping the lookup" outright
// rather than merely tolerating its result.
//
// A QUOTED leading tilde ('~name', "~name") parses to a
// *syntax.SglQuoted/*syntax.DblQuoted node instead of *syntax.Lit and so
// is never matched here — it is inert literal text to a real shell too,
// not a tilde expansion. A BACKSLASH-escaped leading tilde (\~name) also
// fails to match: mvdan.cc/sh's parser keeps the backslash IN the Lit's
// own Value ("\~name"), which does not have a "~" PREFIX, so
// expand.Literal's own strings.CutPrefix check already leaves it
// unexpanded — this function's prefix check agrees with that, rather than
// fighting it. Deliberately scoped to non-strict (argument) position:
// item 3's fix is explicitly the STRICT path only — see
// analyzeBashEffect's doc comment on why an ordinary argument's
// unresolved substitution is safe to default to "".
func wordHasLeadingUnescapedTilde(w *syntax.Word) bool {
	if len(w.Parts) == 0 {
		return false
	}
	lit, ok := w.Parts[0].(*syntax.Lit)
	return ok && strings.HasPrefix(lit.Value, "~")
}

// strictUnresolvableReason walks w (exactly as wordIsStrictlyResolvable
// does — it is this function's sole caller) and reports whether w is
// strictly resolvable and, when it is not, a short, user-meaningful
// description of the FIRST construct responsible: a leading unescaped
// tilde (wordHasLeadingUnescapedTilde), a command/process substitution,
// an arithmetic expansion, an extended glob, or a non-simple/unknown
// parameter expansion. This is the single source both
// wordIsStrictlyResolvable (which only needs the bool) and every STRICT
// resolveWord failure's audit reason (resolveCallExpr's command-word and
// assignment-value checks, resolveDeclClause's assignment check) draw
// from, replacing what used to be one fixed "unresolved parameter
// expansion in command-word position" string reported for every one of
// these shapes alike, including the ones that were never actually a
// parameter expansion at all (CmdSubst/ArithmExp/ExtGlob).
func strictUnresolvableReason(w *syntax.Word, env map[string]string) (reason string, resolvable bool) {
	if wordHasLeadingUnescapedTilde(w) {
		return "unresolved tilde expansion", false
	}
	resolvable = true
	syntax.Walk(w, func(n syntax.Node) bool {
		if !resolvable {
			return false
		}
		switch p := n.(type) {
		case *syntax.CmdSubst:
			reason, resolvable = "unresolved command substitution", false
		case *syntax.ProcSubst:
			reason, resolvable = "unresolved process substitution", false
		case *syntax.ArithmExp:
			reason, resolvable = "unresolved arithmetic expansion", false
		case *syntax.ExtGlob:
			reason, resolvable = "unresolved extended glob", false
		case *syntax.ParamExp:
			if !isSimpleParamExp(p) {
				reason, resolvable = "unresolved parameter expansion", false
				return false
			}
			if _, known := env[p.Param.Value]; !known {
				reason, resolvable = "unresolved parameter expansion", false
				return false
			}
			return true
		default:
			return true
		}
		return false
	})
	return reason, resolvable
}

// wordIsStrictlyResolvable reports whether w contains ONLY constructs this
// analyzer can confidently resolve to a known literal value: plain text
// (*syntax.Lit/*syntax.SglQuoted/*syntax.DblQuoted) and simple, already-
// known parameter references (isSimpleParamExp, name present in env) — and
// does NOT begin with an unescaped tilde (wordHasLeadingUnescapedTilde).
// Anything else — a *syntax.CmdSubst, *syntax.ProcSubst, *syntax.ArithmExp,
// *syntax.ExtGlob, or a non-simple/unknown *syntax.ParamExp, anywhere
// within w, including nested inside a *syntax.DblQuoted — makes w
// unresolvable for STRICT-mode purposes. Used only to gate STRICT
// (command-word-position and assignment-value) resolution — see
// resolveWord's doc comment for why non-strict (ordinary argument)
// resolution does not use this gate at all. Delegates to
// strictUnresolvableReason (this function's only caller that discards the
// reason) so the bool and the audit-reason classification can never drift
// apart.
func wordIsStrictlyResolvable(w *syntax.Word, env map[string]string) bool {
	_, resolvable := strictUnresolvableReason(w, env)
	return resolvable
}

// resolveWord resolves w to its literal value using expand.Literal,
// seeded with env via mapEnviron. strict gates COMMAND-WORD-POSITION and
// assignment-VALUE resolution specifically (see resolveCallExpr and
// stageCommandWord): when strict is true, resolution is refused (false,
// "") unless w is wordIsStrictlyResolvable — this package's "unresolved
// parameter expansion in command-word position" fail-closed rule,
// broadened to also cover a CmdSubst/ProcSubst/ArithmExp/ExtGlob
// appearing in that same position. When strict is false (redirect
// targets and every argument position after Args[0]), w is resolved via
// expand.Literal directly with no pre-check at all: an unset or complex
// parameter reference resolves to "" or its real computed value
// respectively (exactly like a real shell), and a CmdSubst/ProcSubst
// resolves to "" via the fail-closed stand-ins below (expand.Literal
// errors, which this function turns into ("", false), which
// resolveCallExpr's non-strict branch turns into "") — see
// analyzeBashEffect's doc comment for why that is both accurate and
// safe, instead of forcing every command containing any command/process
// substitution to indeterminate regardless of context.
//
// cfg.ProcSubst is wired to a fail-closed stand-in (errUnexpectedProcSubst)
// rather than left nil: expand.Literal calls cfg.ProcSubst directly, with
// no nil check, on any ProcSubst node it reaches, so leaving it nil would
// turn a non-strict `<(...)` argument into a panic instead of a clean,
// fail-closed error. cfg.CmdSubst is left nil deliberately: expand.Literal
// already raises expand.UnexpectedCommandError — no panic — on any
// CmdSubst node when CmdSubst is nil, which is exactly the fail-closed
// signal this function wants for a non-strict `$(...)` argument too.
func resolveWord(w *syntax.Word, env map[string]string, strict bool) (string, bool) {
	if w == nil {
		return "", true
	}
	if strict && !wordIsStrictlyResolvable(w, env) {
		return "", false
	}
	cfg := &expand.Config{
		Env: mapEnviron(env),
		ProcSubst: func(*syntax.ProcSubst) (string, error) {
			return "", errUnexpectedProcSubst
		},
	}
	val, err := expand.Literal(cfg, w)
	if err != nil {
		return "", false
	}
	return val, true
}

// cloneEnv returns a shallow copy of env, used to scope a per-invocation
// ("FOO=bar cmd") assignment to a single CallExpr without mutating the
// caller's persistent environment — see resolveCallExpr.
func cloneEnv(env map[string]string) map[string]string {
	clone := make(map[string]string, len(env))
	for k, v := range env {
		clone[k] = v
	}
	return clone
}

// mapEnviron is a minimal expand.Environ backed directly by this
// package's own resolved-variable map — deliberately NOT
// expand.ListEnviron, whose own case-folding is conditioned on the HOST
// machine's runtime.GOOS (see its doc comment: "On Windows ... resulting
// variable names will all be uppercase") and would make this analyzer's
// behavior depend on which OS the comrade binary happens to be running
// on, rather than being a pure function of (command, dialect) — the same
// property Engine.Evaluate itself relies on via its OS-injectable
// newEngineForGOOS test seam (engine.go).
type mapEnviron map[string]string

// Get implements expand.Environ.
func (m mapEnviron) Get(name string) expand.Variable {
	v, ok := m[name]
	if !ok {
		return expand.Variable{}
	}
	return expand.Variable{Set: true, Kind: expand.String, Str: v}
}

// Each implements expand.Environ.
func (m mapEnviron) Each(f func(name string, vr expand.Variable) bool) {
	for name, v := range m {
		if !f(name, expand.Variable{Set: true, Kind: expand.String, Str: v}) {
			return
		}
	}
}
