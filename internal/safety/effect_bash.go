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
// MAY-NOT-EXECUTE bodies — the correctness-critical part of this design,
// tightened after a follow-up security audit found a CRITICAL false-Allow
// regression in an earlier version of this walk. A while/until body may
// run zero iterations (`while false; do R=echo; done` never runs Do at
// all); a for-loop's Items may resolve to an empty list at runtime
// (`for i in $UNSET`, `for i in $(false)`); an "elif" is only reached if
// every earlier Cond in the chain was false; an if/elif/else's Then/Else,
// and each case item's Stmts, are mutually exclusive alternatives where
// at most one (possibly none) actually runs. None of these facts is
// something this single-pass, non-executing analyzer can determine — it
// never evaluates whether a Cond is true, whether an Items list is
// non-empty, or which branch "really" runs.
//
// Given that, a variable such a body assigns is genuinely AMBIGUOUS
// afterward: its real value is either whatever it was before (if the
// body never ran) or the newly assigned one (if it did) — and BOTH of
// the naive alternatives are unsound:
//
//   - Sharing r.env directly with the body (this analyzer's ORIGINAL,
//     regressed behavior) treats the body as ALWAYS having run: `R=rm;
//     while false; do R=echo; done; $R -rf /` would silently overwrite
//     the already-known-dangerous R=rm with R=echo, then resolve `$R` to
//     the WRONG, safe-looking value — a false Allow on a command that
//     genuinely runs `rm -rf /` in real bash, since the while body never
//     executes at all.
//   - Discarding the body's assignment entirely (this analyzer's
//     Subshell treatment — correct THERE, because a real subshell's
//     assignments provably never leak, full stop) is the MIRROR false
//     negative here: `R=echo; for i in 1; do R=rm; done; $R -rf /` DOES
//     run its body (Items is the literal, non-empty list `1`), so `$R`
//     really is `rm` afterward — silently keeping the stale R=echo would
//     resolve `$R -rf /` to a safe-looking value again.
//
// The sound fix (resolveMayNotExecute, below) is two steps: resolve the
// body against a FRESH CLONE of the env (so an assignment-and-use INSIDE
// the SAME body, e.g. `for i in 1; do R=rm; $R -rf /; done`, still
// escalates correctly — this is issue #15's original ask), then
// INVALIDATE (delete) every variable name the clone's value disagrees
// with, back in the PARENT env. A later COMMAND-WORD/assignment-VALUE
// reference to an invalidated name is therefore genuinely
// strictly-unresolvable — fails closed to Confirm, never silently
// resolves to either the stale OR the new value. A later ARGUMENT-position
// reference resolves to "" and stays inert (see the ARGUMENT-position
// discussion below), so this costs essentially no false-positive rate on
// the ordinary, non-security-relevant shape of "loop variable read only
// as an argument after the loop".
//
// Exactly which parts of each construct are "always runs" (shared,
// plain r.resolveStmts) versus "may not execute" (resolveMayNotExecute):
// the very FIRST if's Cond and a while/until's own Cond always run at
// least once when control reaches the statement at all, so both stay
// shared; a for/while/until's Do body, an "elif"'s own Cond, an
// if/elif/else's Then/Else, and each case item's Stmts are all
// may-not-execute. A subshell (`( )`) keeps its own, DIFFERENT treatment
// (resolveScopedStmts: clone, then unconditionally discard, no
// invalidation) — a subshell's assignments are not "ambiguous", they are
// UNCONDITIONALLY isolated by real bash semantics, so there is nothing to
// invalidate.
//
// This does mean two mutually exclusive branches assigning the SAME
// variable to DIFFERENT literal values can never both be resolved with
// confidence afterward — correct: no static analysis of this kind can
// know which one ran, so the honest answer is "unknown", not "whichever
// was visited last" (a real false-negative risk the ORIGINAL if/case
// implementation carried too, before this fix: it left a pre-existing
// value in place unchanged rather than invalidating it, which is unsound
// in the same direction as the Subshell-discard mirror case above, for
// any if/elif/else Then/Else or case item that is actually guaranteed to
// run).
//
// Resource guard: every scope-forking clone this may-not-execute
// treatment (and Subshell) requires goes through safeCloneEnv, which
// fails closed once a shared, whole-analysis resolverBudget is exhausted
// or an env grows past maxEnvSize — see resolverBudget's doc comment for
// the memory-amplification concern this closes (nested/many-branch
// constructs fork one clone per branch per level, which multiplies, not
// merely adds, with nesting depth).
//
// A for-loop's own iteration variable (*syntax.WordIter's Name) is,
// separately, deliberately NEVER added to env at all (not merely
// invalidated after the fact): its real value is whichever of the loop's
// Items is bound on a given iteration, which this analyzer cannot
// enumerate. Leaving it absent from env means a reference to it in
// ARGUMENT position (the common, idiomatic shape — `for f in *.go; do
// gofmt -l $f; done`) resolves to "" and stays inert, while a reference
// to it in COMMAND-WORD position fails closed via the same
// unresolved-command-word rule as any other unknown variable — never
// silently resolved to a guessed item.
func analyzeBashEffect(command string) effectVerdict {
	parser := syntax.NewParser(syntax.Variant(syntax.LangBash))
	file, err := parser.Parse(strings.NewReader(command), "")
	if err != nil {
		return indeterminateVerdict("parse error: " + err.Error())
	}

	resolver := &bashResolver{env: map[string]string{}, budget: &resolverBudget{forksRemaining: maxScopeForks}}
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
// signature reuse cannot express as a regex. budget is shared BY POINTER
// with every child resolver spawned from the same analyzeBashEffect call
// (resolveMayNotExecute, resolveScopedStmts) — see resolverBudget's doc
// comment.
type bashResolver struct {
	env      map[string]string
	findings []effectVerdict
	budget   *resolverBudget
}

// resolverBudget bounds the TOTAL number of scope-forking clones
// (safeCloneEnv) a single analyzeBashEffect call may perform, shared BY
// POINTER across every bashResolver spawned from that one call (the root
// resolver in analyzeBashEffect creates it; every may-not-execute/
// subshell child carries the SAME pointer forward, never a fresh one).
//
// A per-node NESTING-DEPTH cap alone would under-bound this: a case
// statement forks one clone PER ITEM, and each of those items can itself
// nest further constructs, so nesting several many-branch case/if/loop
// constructs MULTIPLIES the total clone count with depth rather than
// merely adding to it. A follow-up security audit measured a ~19x/494MB
// memory blow-up (vs ~26MB on origin/main) from a single ~48KB crafted
// input exploiting exactly this fan-out-times-depth shape — a realistic
// threat, since this analyzer parses LLM-generated, untrusted text, not
// hand-vetted scripts. Capping the CUMULATIVE fork count instead bounds
// worst-case memory/CPU to maxScopeForks*maxEnvSize map entries
// regardless of whether the blow-up comes from deep nesting, wide
// branching, or both at once.
type resolverBudget struct {
	forksRemaining int
}

const (
	// maxScopeForks is the total number of scope-forking clones a single
	// analyzeBashEffect call may perform before failing closed — see
	// resolverBudget's doc comment. Chosen generously above anything a
	// legitimate one-liner would ever need (even an elaborately nested
	// real script rarely forks more than a handful of scopes), while
	// still bounding worst-case total clone volume to a small, fixed
	// multiple of maxEnvSize.
	maxScopeForks = 512

	// maxEnvSize caps how many distinct variable names a single env map
	// may hold before this analyzer refuses to clone it any further.
	// Combined with maxScopeForks, this bounds the absolute worst-case
	// total clone volume across an entire analyzeBashEffect call to
	// maxScopeForks*maxEnvSize map entries, however the input is shaped.
	maxEnvSize = 256
)

// scopeGuardReason is the fail-closed audit reason safeCloneEnv reports
// when either guard fires. Deliberately generic (not "too deep" vs "too
// many variables") — every caller already folds it into an "effect:
// indeterminate (...)" verdict via indeterminateVerdict (RiskElevated,
// Confirm), and both thresholds exist for the identical reason (bounding
// total clone memory/CPU), so no caller has any use for telling the two
// apart.
const scopeGuardReason = "control-structure nesting/branching exceeds this analyzer's resource guard"

// safeCloneEnv clones env, or refuses (nil, false) if budget is
// exhausted or env already holds more than maxEnvSize entries — the
// SINGLE choke point every scope-forking clone in this file goes through
// (resolveMayNotExecute, resolveScopedStmts, and resolveCallExpr's
// per-invocation-assign clone), so the guard cannot be bypassed by a new
// call site added later. A successful clone decrements
// budget.forksRemaining, since budget is shared by pointer across every
// resolver spawned from the same analyzeBashEffect call — see
// resolverBudget's doc comment.
func safeCloneEnv(env map[string]string, budget *resolverBudget) (map[string]string, bool) {
	if budget.forksRemaining <= 0 || len(env) > maxEnvSize {
		return nil, false
	}
	budget.forksRemaining--
	return cloneEnv(env), true
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
// analyzeBashEffect's doc comment for exactly which shapes are walked,
// which are deliberately left unmodeled, and the MAY-NOT-EXECUTE
// clone+invalidate treatment resolveMayNotExecute implements for every
// construct below whose body is not guaranteed to run.
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
		return r.resolveForClause(c)
	case *syntax.CaseClause:
		return r.resolveCaseClause(c)
	case *syntax.Block:
		// `{ }` runs in the CURRENT scope in real bash — no fork, unlike a
		// subshell — so a plain same-resolver resolveStmts is correct.
		return r.resolveStmts(c.Stmts)
	case *syntax.Subshell:
		return r.resolveScopedStmts(c.Stmts)
	case *syntax.DeclClause:
		return r.resolveDeclClause(c)
	default:
		return "", false, ""
	}
}

// resolveScopedStmts resolves stmts (a SUBSHELL's own statement list)
// against a fresh clone of r.env, then UNCONDITIONALLY DISCARDS that
// clone once resolution completes — never invalidating anything in
// r.env, unlike resolveMayNotExecute. A subshell forks a real child
// process in bash, so its assignments never propagate back to the parent
// shell: not "ambiguous", not "may or may not have happened" — provably,
// unconditionally isolated on real bash semantics. There is therefore
// nothing to invalidate: r.env already correctly has no idea the
// subshell's assignments ever happened, which is exactly right regardless
// of whether the subshell "actually" ran. Findings (checkFetcherPipeline)
// the nested resolution accumulates are still merged back into r, since
// those are always worth surfacing regardless of which scope discovered
// them.
func (r *bashResolver) resolveScopedStmts(stmts []*syntax.Stmt) (text string, indeterminate bool, reason string) {
	clone, ok := safeCloneEnv(r.env, r.budget)
	if !ok {
		return "", true, scopeGuardReason
	}
	child := &bashResolver{env: clone, budget: r.budget}
	text, indeterminate, reason = child.resolveStmts(stmts)
	r.findings = append(r.findings, child.findings...)
	return text, indeterminate, reason
}

// resolveMayNotExecute resolves stmts as a MAY-NOT-EXECUTE body — see
// analyzeBashEffect's doc comment for the full false-Allow/false-negative
// analysis this fixes. Two steps:
//
//  1. Resolve stmts against a FRESH CLONE of r.env (safeCloneEnv), via a
//     child resolver. This still correctly catches an assignment-and-use
//     INSIDE the same body (`for i in 1; do R=rm; $R -rf /; done` still
//     escalates: the child resolver's own R=rm is visible when it
//     resolves $R, exactly as within any other statement list).
//  2. INVALIDATE, in r.env itself, every variable name whose value in the
//     clone differs from r.env's: such a variable's value after the
//     construct is genuinely ambiguous — the body may not have run at all
//     (old value, if any, still stands), or it may have run and produced
//     the new one — and this analyzer must never silently resolve to
//     either. Invalidating means DELETING the key entirely, so a later
//     STRICT (command-word/assignment-value) reference to it fails closed
//     exactly like any other genuinely unknown variable, while a later
//     ARGUMENT-position reference merely resolves to "" and stays inert
//     (see resolveWord's non-strict default) — the ordinary shape of
//     "loop variable used only as an argument" does not start
//     over-prompting.
//
// THIS STEP MUST ITERATE r.env (the PARENT's keys), NOT clone (the
// child's) — a follow-up security audit found and fixed a HIGH-severity
// composition failure here: iterating clone only ever sees a key the
// child ADDED or MODIFIED, never one an INNER resolveMayNotExecute
// (deeper in the same body) already DELETED from ITS OWN parent (this
// child) as ambiguous. At nesting depth >= 2 — `R=echo; for i in 1; do
// if true; then R=rm; fi; done; $R -rf /` — the inner if's Then
// invalidates R inside the outer for-loop's clone (deleting it there),
// but the outer loop's old (buggy) `for name, newVal := range clone`
// loop would then simply never see "R" at all (it is absent from clone,
// not merely changed), so the OUTER invalidation never reran, and
// r.env's original, now-stale "R=echo" survived unchanged — silently
// keeping the WRONG value even though the ambiguity is exactly as real
// two levels up as one. Iterating r.env instead makes a MISSING key in
// clone (deleted at any inner depth) trigger invalidation exactly like a
// CHANGED one — deletion IS the signal "this name is ambiguous", and it
// must propagate outward through every enclosing resolveMayNotExecute
// call, however many levels deep it originated. This composes at ANY
// nesting depth by induction: each level's own invalidation pass only
// ever needs to look at ITS OWN immediate child's clone versus its own
// r.env, and correctly treats "value differs OR is now absent" as
// ambiguous either way — so an inner level's deletion is indistinguishable
// from (and handled identically to) a same-level modification, one level
// up, all the way to the top.
//
// (A key the body ADDS that r.env never had is a different, simpler
// case: since this loop ranges over r.env — not clone — a brand-new name
// is never even visited here at all, let alone compared or deleted. That
// is still correct, not a gap: the name was never in r.env to begin
// with, so a later reference to it is already strictly-unresolvable via
// the ordinary "unknown variable" path, with nothing for this function
// to invalidate or for a caller outside it to observe changing.)
//
// If the child resolution is itself indeterminate, that propagates
// directly (no invalidation step needed — the whole command already
// fails closed).
func (r *bashResolver) resolveMayNotExecute(stmts []*syntax.Stmt) (text string, indeterminate bool, reason string) {
	clone, ok := safeCloneEnv(r.env, r.budget)
	if !ok {
		return "", true, scopeGuardReason
	}
	child := &bashResolver{env: clone, budget: r.budget}
	text, indeterminate, reason = child.resolveStmts(stmts)
	r.findings = append(r.findings, child.findings...)
	if indeterminate {
		return "", true, reason
	}
	for name, oldVal := range r.env {
		if newVal, stillPresent := clone[name]; !stillPresent || newVal != oldVal {
			delete(r.env, name)
		}
	}
	return text, false, ""
}

// resolveIfClause resolves a TOP-LEVEL if/elif/else chain: this is
// analyzeBashEffect's only entry point into an *syntax.IfClause, and it
// always starts resolveIfClauseChain with condAlwaysRuns=true for c's OWN
// Cond — see resolveIfClauseChain's doc comment for why that is true only
// for this very first Cond, never for a chained "elif"'s.
func (r *bashResolver) resolveIfClause(c *syntax.IfClause) (text string, indeterminate bool, reason string) {
	return r.resolveIfClauseChain(c, true)
}

// resolveIfClauseChain resolves one link of an if/elif/else chain.
// condAlwaysRuns is true ONLY for the very first "if": reaching the if
// statement at all guarantees at least that first condition test runs,
// so its own assignments persist directly into r.env via plain
// r.resolveStmts — exactly like a while/until's own Cond. Every
// SUBSEQUENT "elif" in the chain is reached ONLY if every earlier Cond
// was false — itself a may-not-execute body from this analyzer's
// perspective (it never evaluates whether a Cond is true or false), so an
// elif's own Cond is resolved via resolveMayNotExecute exactly like a
// loop body (this is the audit's "recursive elif Cond" fix). c's Then —
// and, transitively, every later elif's Then — is ALWAYS a
// may-not-execute body: an if/elif/else chain is a set of mutually
// exclusive alternatives, and this analyzer cannot determine which one
// (if any) actually runs. A plain "else" is represented as a further
// IfClause with Cond empty, so resolveMayNotExecute(nil) trivially
// contributes nothing and invalidates nothing.
func (r *bashResolver) resolveIfClauseChain(c *syntax.IfClause, condAlwaysRuns bool) (text string, indeterminate bool, reason string) {
	var condText string
	var indet bool
	var why string
	if condAlwaysRuns {
		condText, indet, why = r.resolveStmts(c.Cond)
	} else {
		condText, indet, why = r.resolveMayNotExecute(c.Cond)
	}
	if indet {
		return "", true, why
	}

	thenText, indet, why := r.resolveMayNotExecute(c.Then)
	if indet {
		return "", true, why
	}

	var parts []string
	parts = appendNonEmpty(parts, condText)
	parts = appendNonEmpty(parts, thenText)
	if c.Else != nil {
		elseText, indet, why := r.resolveIfClauseChain(c.Else, false)
		if indet {
			return "", true, why
		}
		parts = appendNonEmpty(parts, elseText)
	}
	return strings.Join(parts, " ; "), false, ""
}

// resolveWhileClause resolves one while/until clause: Cond ALWAYS runs at
// least once when control reaches the while statement at all (the first
// test), so it is resolved via plain r.resolveStmts against the shared,
// persistent env — exactly like the very first "if"'s Cond. Do is a
// MAY-NOT-EXECUTE body: the loop may run zero iterations (the audit's
// exact repro — `while false; do R=echo; done` never runs Do at all), so
// it is resolved via resolveMayNotExecute exactly like a for-loop body or
// an if/case branch.
func (r *bashResolver) resolveWhileClause(w *syntax.WhileClause) (text string, indeterminate bool, reason string) {
	condText, indet, why := r.resolveStmts(w.Cond)
	if indet {
		return "", true, why
	}
	doText, indet, why := r.resolveMayNotExecute(w.Do)
	if indet {
		return "", true, why
	}
	var parts []string
	parts = appendNonEmpty(parts, condText)
	parts = appendNonEmpty(parts, doText)
	return strings.Join(parts, " ; "), false, ""
}

// resolveForClause resolves a for/select clause's Do body as a
// MAY-NOT-EXECUTE construct: the loop may run zero iterations (an empty,
// unset, or dynamically-empty Items list — `for i in $UNSET`,
// `for i in $(false)` — the audit's exact repro), so Do is resolved via
// resolveMayNotExecute exactly like a while/until body. See
// analyzeBashEffect's doc comment for why the iteration variable itself
// (*syntax.WordIter's Name) is separately, deliberately never added to
// env at all — a different, narrower limitation from the
// may-not-execute treatment of the body as a whole.
func (r *bashResolver) resolveForClause(f *syntax.ForClause) (text string, indeterminate bool, reason string) {
	return r.resolveMayNotExecute(f.Do)
}

// resolveCaseClause resolves a case/switch clause: exactly one CaseItem's
// Stmts actually runs at runtime (or possibly none, if no pattern
// matches), so each item is resolved as its own MAY-NOT-EXECUTE body
// (resolveMayNotExecute) — never chained from one item into the next, and
// any variable an item assigns is invalidated in the surrounding scope
// rather than either leaked into a sibling item or silently discarded.
func (r *bashResolver) resolveCaseClause(c *syntax.CaseClause) (text string, indeterminate bool, reason string) {
	var parts []string
	for _, item := range c.Items {
		t, indet, why := r.resolveMayNotExecute(item.Stmts)
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
		clone, ok := safeCloneEnv(r.env, r.budget)
		if !ok {
			return "", true, scopeGuardReason
		}
		workEnv = clone
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
// real host syscall/NSS lookup that makes this resolution depend on which
// accounts exist on the machine currently running comrade, not merely on
// (command, dialect).
//
// This function closes that host dependency in STRICT
// (command-word/assignment-value) position ENTIRELY — the one place a
// correctness review flagged it — by rejecting every leading unescaped
// tilde BEFORE expand.Literal is ever invoked there (resolveWord's strict
// gate checks wordIsStrictlyResolvable, which delegates to
// strictUnresolvableReason, which calls this function, first): so
// os/user.Lookup is never reached in strict position at all, "dropping
// the lookup" outright rather than merely tolerating its result.
// Rejecting EVERY leading unescaped tilde — not only the "~name" spelling
// that triggers the Lookup — keeps the rule simple (one gate, not "~name
// only, but ~ and ~/foo are fine").
//
// This function is called ONLY from strictUnresolvableReason, which is
// itself consulted ONLY by resolveWord's STRICT branch — so it has no
// effect on, and makes no claim about, ARGUMENT (non-strict) position:
// resolveWord's non-strict branch (redirect targets, every argument after
// Args[0]) calls expand.Literal directly with no pre-check at all, so a
// leading unescaped tilde THERE still reaches real tilde expansion and
// can still trigger a genuine os/user.Lookup host call. That is a known,
// accepted, narrower residual gap, not an oversight: the tilde-host-I/O
// fix this function implements was explicitly scoped to strict position
// only (the fix a security review specifically asked for), the same way
// analyzeBashEffect's own doc comment explains why an ordinary argument's
// unresolved substitution is otherwise safe to default to "" — see
// KNOWN_LIMITATIONS.md for this residual documented for users.
//
// A QUOTED leading tilde ('~name', "~name") parses to a
// *syntax.SglQuoted/*syntax.DblQuoted node instead of *syntax.Lit and so
// is never matched here — it is inert literal text to a real shell too,
// not a tilde expansion. A BACKSLASH-escaped leading tilde (\~name) also
// fails to match: mvdan.cc/sh's parser keeps the backslash IN the Lit's
// own Value ("\~name"), which does not have a "~" PREFIX, so
// expand.Literal's own strings.CutPrefix check already leaves it
// unexpanded — this function's prefix check agrees with that, rather than
// fighting it.
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
