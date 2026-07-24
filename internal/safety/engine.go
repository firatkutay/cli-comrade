package safety

import (
	"runtime"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// Engine is the LLM-independent second check every generated command
// passes through before its Confirm/Block verdict reaches FAZ 6's mode
// logic (auto/ask/info). It never simply trusts the risk label the LLM
// produced: Evaluate always re-derives whether the command matches the
// hardcoded denylist, any escalation rule, or (on a dialect the AST
// effect layer supports — see dialectForGOOS in effect.go) what the
// command's parsed argv actually resolves to, using the caller-declared
// risk only as the floor any of those may raise but never lower.
type Engine struct {
	userDenylist []denylistRule
	dialect      effectDialect
}

// NewEngine builds an Engine from cfg.Safety.DenylistExtra, compiling each
// user-supplied regex once at construction time via compileUserDenylist,
// and from the host OS (runtime.GOOS), which fixes the AST effect layer's
// dialect for this Engine's whole lifetime — see newEngineForGOOS. An
// entry that fails to compile is skipped with a single stderr warning
// rather than failing construction or panicking — see
// compileUserDenylist's doc comment. The built-in denylist and escalation
// rule sets are fixed package-level data and never depend on cfg.
func NewEngine(cfg config.Config) *Engine {
	return newEngineForGOOS(cfg, runtime.GOOS)
}

// newEngineForGOOS is NewEngine's OS-injectable constructor, used
// directly by this package's own tests to exercise both the Unix/AST
// dialect and the Windows/signatures-only dialect without requiring an
// actual Windows host — mirrors internal/executor's newForGOOS test seam
// (executor.go).
func newEngineForGOOS(cfg config.Config, goos string) *Engine {
	return &Engine{
		userDenylist: compileUserDenylist(cfg.Safety.DenylistExtra),
		dialect:      dialectForGOOS(goos),
	}
}

// Evaluate classifies command against declared (the risk class the LLM
// assigned it) and returns the safety engine's independent verdict:
//
//  1. Denylist: if command matches any built-in or user-supplied denylist
//     rule, the result is Block, unconditionally — declared is not even
//     consulted, and nothing in this package lets a mode or override flag
//     change a Block (that authority belongs to FAZ 6, not here). The AST
//     effect layer (step 2b below) is NEVER consulted for this decision —
//     it can only ever raise EffectiveRisk into the Confirm tier, never
//     produce Block; Block stays exclusively signature-owned.
//  2. Escalation: EffectiveRisk starts at declared and is raised — never
//     lowered — to the highest risk implied by (a) any matching
//     signature escalation rule, or (b) — on a dialect the AST effect
//     layer supports (see dialectForGOOS) — analyzeEffect's independent
//     opinion of what the command's parsed argv actually resolves to
//     (effect.go/effect_bash.go), e.g. seeing through `R=rm; $R -rf /`'s
//     variable indirection to the `rm -rf /` it actually runs. A command
//     the LLM already declared destructive stays destructive even when
//     it matches no escalation rule and no AST finding at all.
//  3. An EffectiveRisk of destructive or elevated maps to Confirm;
//     anything lower (read/write/network) maps to Allow.
func (e *Engine) Evaluate(command string, declared RiskClass) Decision {
	// Every matcher — built-in denylist, user denylist_extra, and every
	// escalation rule — runs against the normalized form, never the raw
	// command string. This is what closes the quote-fragility hole a
	// single stray quote would otherwise open (e.g.
	// `dd if=/dev/zero of='/dev/sda'` contains no literal "of=/dev/sda"
	// substring once the quotes are counted): normalizing once, here,
	// hardens every rule below at once instead of requiring each one to
	// grow its own quote-tolerant pattern. See normalizeCommand's doc
	// comment in tokenize.go.
	normalized := normalizeCommand(command)
	tokens := tokenizeCommand(normalized)

	for _, rule := range builtinDenylist {
		if rule.match(normalized, tokens) {
			return blockDecision(rule.name)
		}
	}
	for _, rule := range e.userDenylist {
		if rule.match(normalized, tokens) {
			return blockDecision(rule.name)
		}
	}

	effective := declared
	matchedRule := ""
	for _, rule := range escalationRules {
		if rule.risk > effective && rule.match(normalized) {
			effective = rule.risk
			matchedRule = rule.name
		}
	}

	// AST effect layer: an additional, independent escalation source —
	// see analyzeEffect's and dialectForGOOS's doc comments. Deliberately
	// evaluated against the ORIGINAL command, not normalized: the AST
	// parser needs real quote semantics (a single-quoted '$R' must NOT
	// expand) that normalizeCommand's blanket quote-stripping would
	// destroy — see effect_bash.go's analyzeBashEffect doc comment.
	if ev := analyzeEffect(command, e.dialect); ev.risk > effective {
		effective = ev.risk
		matchedRule = ev.reason
	}

	if effective >= RiskElevated {
		reason := "declared risk is already " + effective.String()
		if matchedRule != "" {
			reason = "escalated to " + effective.String() + " by rule: " + matchedRule
		}
		return Decision{
			Action:        Confirm,
			Reason:        reason,
			EffectiveRisk: effective,
			MatchedRule:   matchedRule,
			Evaluated:     true,
		}
	}

	return Decision{
		Action:        Allow,
		EffectiveRisk: effective,
		MatchedRule:   matchedRule,
		Evaluated:     true,
	}
}

func blockDecision(ruleName string) Decision {
	return Decision{
		Action:        Block,
		Reason:        "matches denylist rule: " + ruleName,
		EffectiveRisk: RiskDestructive,
		MatchedRule:   ruleName,
		Evaluated:     true,
	}
}
