package safety

import "fmt"

// RiskClass is one of the five command risk categories from CLAUDE.md's
// "Komut Risk Sınıflandırması". Its integer value is an ascending severity
// ordinal (RiskRead lowest, RiskDestructive highest) — Engine.Evaluate
// relies on plain integer comparison/max to implement the upward-only
// escalation rule (CLAUDE.md/UYGULAMA_PLANI.md FAZ 5: a rule may only
// raise a command's effective risk, never lower what the LLM declared).
type RiskClass int

const (
	// RiskRead is a read-only command: no state is changed (ls, cat,
	// Get-ChildItem, df).
	RiskRead RiskClass = iota
	// RiskWrite changes files or local settings (mkdir, chmod, package
	// install) but nothing irreversible or requiring elevation.
	RiskWrite
	// RiskNetwork performs network access (curl, apt update,
	// Invoke-WebRequest).
	RiskNetwork
	// RiskElevated requires sudo/admin elevation to run.
	RiskElevated
	// RiskDestructive is an irreversible delete/format/registry/disk
	// operation. Highest severity: Engine.Evaluate always resolves to
	// Confirm (or Block, for a denylisted command) at this level.
	RiskDestructive
)

// riskNames is the canonical string form of each RiskClass, in the exact
// spelling CLAUDE.md and the LLM plan-generation JSON schema use. It is
// also the single source both String() and ParseRiskClass draw from, so
// the two can never drift out of sync with each other.
var riskNames = [...]string{
	RiskRead:        "read",
	RiskWrite:       "write",
	RiskNetwork:     "network",
	RiskElevated:    "elevated",
	RiskDestructive: "destructive",
}

// String renders r using the same lowercase spelling ParseRiskClass
// accepts, so round-tripping a RiskClass through String/ParseRiskClass is
// lossless. An out-of-range value (never produced by this package, but
// possible via an explicit RiskClass(n) conversion) renders as
// "unknown(<n>)" rather than panicking or silently picking a name.
func (r RiskClass) String() string {
	if int(r) < 0 || int(r) >= len(riskNames) {
		return fmt.Sprintf("unknown(%d)", int(r))
	}
	return riskNames[r]
}

// ParseRiskClass parses one of the five canonical risk class names
// ("read", "write", "network", "elevated", "destructive") into a
// RiskClass. Any other input — including an empty string, different
// casing, or a class name the LLM invented — is an error: callers outside
// this package (internal/engine's plan parser) are expected to treat that
// error as "unknown risk label" and fail closed to RiskDestructive
// themselves, per UYGULAMA_PLANI.md FAZ 5; ParseRiskClass itself never
// guesses a default.
func ParseRiskClass(s string) (RiskClass, error) {
	for i, name := range riskNames {
		if name == s {
			return RiskClass(i), nil
		}
	}
	return 0, fmt.Errorf("safety: unknown risk class %q", s)
}
