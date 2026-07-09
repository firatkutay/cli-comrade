package safety

// Action is Engine.Evaluate's verdict for a single command.
type Action int

const (
	// Allow means the command may run without extra confirmation beyond
	// whatever the active mode (auto/ask/info — FAZ 6) already requires.
	Allow Action = iota
	// Confirm means the command's effective risk is elevated or
	// destructive: CLAUDE.md's non-negotiable safety exception requires
	// user confirmation even in auto mode (short of
	// safety.confirm_destructive=false plus --yolo, which is FAZ 6's
	// concern, not this package's).
	Confirm
	// Block means the command matched the denylist and must never run,
	// regardless of mode or any override flag.
	Block
)

func (a Action) String() string {
	switch a {
	case Allow:
		return "allow"
	case Confirm:
		return "confirm"
	case Block:
		return "block"
	default:
		return "unknown"
	}
}

// Decision is Engine.Evaluate's result for one command: what to do
// (Action), why (Reason, always populated for Confirm/Block, empty for a
// plain Allow with no escalation), the risk class the decision was based
// on after any escalation (EffectiveRisk — never lower than the risk the
// caller declared, see Engine.Evaluate), and which rule (if any) drove the
// decision (MatchedRule — empty when no denylist/escalation rule fired),
// kept for audit/debug logging.
type Decision struct {
	Action        Action
	Reason        string
	EffectiveRisk RiskClass
	MatchedRule   string
}
