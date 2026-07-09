package engine

import "github.com/firatkutay/cli-comrade/internal/safety"

// Step is one command in a Plan: what to run, why (in the user's
// language, per the plan-generation system prompt), the risk class the
// LLM declared, and whether undoing it is straightforward. Decision is
// filled in by GeneratePlan after the LLM responds — it is
// internal/safety's independent, LLM-distrusting verdict for Command,
// computed from Risk via safety.Engine.Evaluate, and is what
// `comrade do --dry-run` (and, in FAZ 6, the real executor) actually acts
// on instead of trusting Risk directly.
type Step struct {
	Command    string
	Rationale  string
	Risk       safety.RiskClass
	Reversible bool
	Decision   safety.Decision
}

// Plan is GeneratePlan's result: a short overview plus the ordered steps
// that carry it out. Summary may carry an appended, bracketed warning
// marker — see truncationMarker/unknownRiskNote in planner.go — when the
// plan was truncated to safety.max_auto_steps or contained a step whose
// risk label the LLM used did not parse as one of the five known
// safety.RiskClass values.
type Plan struct {
	Summary string
	Steps   []Step
}
