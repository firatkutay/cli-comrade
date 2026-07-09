package engine

import "fmt"

// Mode is one of the three behavior modes CLAUDE.md's "Davranış Modları"
// section defines. Runner.Execute's entire dispatch is a switch on this
// type.
type Mode int

const (
	// ModeAuto runs every step automatically, dropping to a confirm
	// prompt only for destructive/elevated steps (or never running a
	// Block at all).
	ModeAuto Mode = iota
	// ModeAsk confirms every non-Blocked step individually before
	// running it. This is the schema default (general.mode = "ask").
	ModeAsk
	// ModeInfo never executes anything: it only prints the plan.
	ModeInfo
)

// modeNames is the canonical string form of each Mode, matching the exact
// config/flag spelling ("auto"/"ask"/"info") CLAUDE.md and config.toml
// use. String() and ParseMode both draw from this single slice so they
// cannot drift apart.
var modeNames = [...]string{
	ModeAuto: "auto",
	ModeAsk:  "ask",
	ModeInfo: "info",
}

func (m Mode) String() string {
	if int(m) < 0 || int(m) >= len(modeNames) {
		return fmt.Sprintf("unknown(%d)", int(m))
	}
	return modeNames[m]
}

// ParseMode parses one of the three canonical mode names. Any other input
// (including empty) is an error.
func ParseMode(s string) (Mode, error) {
	for i, name := range modeNames {
		if name == s {
			return Mode(i), nil
		}
	}
	return 0, fmt.Errorf("engine: unknown mode %q", s)
}

// ResolveMode implements UYGULAMA_PLANI.md FAZ 6 item 2's exact mode
// precedence: an explicit --auto/--ask/--info flag wins outright, then
// COMRADE_MODE, then config general.mode. flagValue/envValue/configValue
// are plain strings ("", "auto", "ask", or "info") so this function stays
// a pure, cobra-independent unit — internal/cli is the only caller, and it
// is responsible for collapsing its three mutually exclusive bool flags
// into flagValue before calling this. An empty flagValue/envValue is
// treated as "not set" and falls through to the next source; configValue
// is expected to always be one of the three valid names (the schema
// default is "ask") but is still validated here rather than assumed.
func ResolveMode(flagValue, envValue, configValue string) (Mode, error) {
	for _, candidate := range []string{flagValue, envValue, configValue} {
		if candidate == "" {
			continue
		}
		mode, err := ParseMode(candidate)
		if err != nil {
			return 0, fmt.Errorf("engine: resolve mode: %w", err)
		}
		return mode, nil
	}
	return 0, fmt.Errorf("engine: resolve mode: no mode source (flag/env/config) provided a value")
}
