package safety

// effectDialect selects which language-specific effect analyzer (if any)
// Engine.Evaluate's AST layer runs a command through. It is chosen once,
// at Engine construction time, from the target OS (see dialectForGOOS) —
// never re-derived per call, and never influenced by anything in the
// command string itself.
type effectDialect int

const (
	// dialectNone means no AST effect analysis runs at all: analyzeEffect
	// always returns the zero effectVerdict (no escalation). This is the
	// PowerShell/Windows path — no pure-Go PowerShell AST parser exists,
	// so the AST layer contributes nothing there and Engine.Evaluate
	// relies entirely on the (dialect-independent) signature denylist/
	// escalation layer, exactly as it did before this package gained an
	// AST layer.
	dialectNone effectDialect = iota
	// dialectBash routes the command through analyzeBashEffect
	// (effect_bash.go): mvdan.cc/sh/v3's bash/POSIX parser and expander.
	dialectBash
)

// dialectForGOOS derives the effect-analysis dialect from a target OS
// string, mirroring internal/executor's own runtime.GOOS-branching
// convention (never a build tag — see executor.go and CLAUDE.md's "Kod
// Kuralları"): internal/executor runs every non-Windows target's
// commands via a POSIX shell (`sh -c`) and Windows via PowerShell: every
// GOOS Engine.NewEngine ever sees maps to one of those same two
// executor.go families, so this switch is exhaustive over the same
// two-way split, not a growing per-OS list.
func dialectForGOOS(goos string) effectDialect {
	if goos == "windows" {
		return dialectNone
	}
	return dialectBash
}

// effectVerdict is the AST effect analyzer's independent opinion of one
// command's risk: risk is RiskRead (the zero value) when the analyzer
// found nothing to escalate; reason is a free-text audit string, always
// non-empty when risk > RiskRead, suitable for Decision.MatchedRule.
//
// effectVerdict never carries an Action and is never, by itself, capable
// of producing Block: Engine.Evaluate folds it into the SAME upward-only
// max() escalation the built-in escalationRules already implement (see
// engine.go), so the worst an effectVerdict can ever do is raise a
// command's EffectiveRisk into the Confirm tier — never Block, which
// stays exclusively owned by the denylist loop that already runs, and
// already returns, before this layer is ever consulted.
type effectVerdict struct {
	risk   RiskClass
	reason string
}

// maxVerdict returns whichever of a, b carries the higher risk,
// preferring a on a tie — so a caller folding together several
// independent findings (denylist-signature reuse, escalation-signature
// reuse, dialect-specific structural findings) never has an earlier,
// equally-severe finding's reason silently overwritten by a later one.
func maxVerdict(a, b effectVerdict) effectVerdict {
	if b.risk > a.risk {
		return b
	}
	return a
}

// indeterminateVerdict builds the effectVerdict this package's fail-closed
// contract requires whenever the analyzer cannot confidently resolve a
// command's actual effect at all — a parse error, or a command/process
// substitution, arithmetic expansion, or unresolved parameter expansion
// in COMMAND-WORD or ASSIGNMENT-VALUE position specifically (see
// effect_bash.go's analyzeBashEffect and resolveWord doc comments for the
// exact triggers and why ARGUMENT position is handled differently). why
// is folded into reason for audit purposes. Its risk is RiskElevated —
// the minimum level Engine.Evaluate's `effective >= RiskElevated` check
// maps to Confirm — never RiskRead: an indeterminate command must never
// be silently treated as safe.
func indeterminateVerdict(why string) effectVerdict {
	return effectVerdict{
		risk:   RiskElevated,
		reason: "effect: indeterminate (" + why + "), failing closed to confirm",
	}
}

// analyzeBashEffectFunc is analyzeEffect's indirection onto
// analyzeBashEffect (effect_bash.go) — a package-level var, not a direct
// call, purely so this package's own tests can swap in a panicking
// stand-in to exercise analyzeEffect's panic-recovery path against a
// REAL panic (see effect_test.go's TestAnalyzeEffectRecoversFromPanic)
// instead of only trusting that mvdan.cc/sh itself never panics. Never
// reassigned outside tests.
var analyzeBashEffectFunc = analyzeBashEffect

// analyzeEffect is Engine.Evaluate's single entry point into the AST
// effect-analysis layer: it dispatches on dialect and otherwise never
// looks at anything but command itself. dialectNone (PowerShell/Windows —
// no pure-Go AST parser exists for it) always returns the zero
// effectVerdict; dialectBash additionally recovers from any panic
// escaping the underlying parser/expander — defense in depth for a
// security-critical path that (via the fuzz corpus, and in production,
// via whatever the LLM happened to generate) is fed adversarial,
// untrusted input: a bug or edge case inside a third-party parser must
// never let a dangerous command slip through as an unhandled crash
// instead of a fail-closed Confirm.
func analyzeEffect(command string, dialect effectDialect) (verdict effectVerdict) {
	switch dialect {
	case dialectBash:
		defer func() {
			if r := recover(); r != nil {
				verdict = indeterminateVerdict("panic during AST effect analysis")
			}
		}()
		return analyzeBashEffectFunc(command)
	default:
		return effectVerdict{}
	}
}
