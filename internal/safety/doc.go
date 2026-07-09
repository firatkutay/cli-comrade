// Package safety implements the local, LLM-independent rule engine
// (denylist and risk-escalation checks, CLAUDE.md "Komut Risk
// Sınıflandırması") that double-checks every command internal/engine's
// Planner generates before it may run. Engine.Evaluate never trusts the
// LLM's declared risk class beyond treating it as a floor: a built-in or
// user-configured denylist match always yields Block, and every
// escalation rule may only raise — never lower — the effective risk.
// This package imports only internal/config and the standard library, so
// it can be exercised (and audited) in complete isolation from the LLM
// layer; see internal/engine for how Planner wires it into plan
// generation.
package safety
