// Package tui holds the bubbletea/lipgloss components used for the mode
// loop's confirmation prompts (internal/engine's ask mode) and auto mode's
// one-line status output (introduced in FAZ 6).
package tui

import (
	"charm.land/lipgloss/v2"

	"github.com/firatkutay/cli-comrade/internal/doctor"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// riskColors maps each safety.RiskClass to the color its badge renders in,
// per CLAUDE.md's tech-stack note ("risk rozeti; read=yeşil, write=cyan,
// network=mavi, elevated=sarı, destructive=kırmızı"). Index by
// safety.RiskClass's integer ordinal.
var riskColors = [...]string{
	safety.RiskRead:        "10", // green
	safety.RiskWrite:       "6",  // cyan
	safety.RiskNetwork:     "4",  // blue
	safety.RiskElevated:    "3",  // yellow
	safety.RiskDestructive: "1",  // red
}

// riskBadgeStyle returns the lipgloss.Style used to render risk's badge.
// When colorEnabled is false (config general.color=false) it returns a
// completely unstyled Style, so Render degrades to plain, uncolored text —
// the "color=false must fully disable lipgloss color" requirement.
func riskBadgeStyle(risk safety.RiskClass, colorEnabled bool) lipgloss.Style {
	if !colorEnabled {
		return lipgloss.NewStyle()
	}
	color := "7"
	if int(risk) >= 0 && int(risk) < len(riskColors) {
		color = riskColors[risk]
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color(color)).Padding(0, 1)
}

// RiskBadge renders risk's uppercase name as a colored badge (or plain
// bracketed text when colorEnabled is false).
func RiskBadge(risk safety.RiskClass, colorEnabled bool) string {
	label := risk.String()
	if !colorEnabled {
		return "[" + label + "]"
	}
	return riskBadgeStyle(risk, colorEnabled).Render(label)
}

// warningStyle renders the mandatory red "--yolo bypass" warning banner
// (CLAUDE.md security rule #6 / FAZ 6's yolo-bypass requirement). Plain,
// unstyled text when colorEnabled is false.
func warningStyle(colorEnabled bool) lipgloss.Style {
	if !colorEnabled {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("15")).Background(lipgloss.Color("1"))
}

// RenderWarning renders text as the red warning banner used for --yolo
// bypass notices and the --yolo flag's own mandatory per-use warning.
func RenderWarning(text string, colorEnabled bool) string {
	return warningStyle(colorEnabled).Render(text)
}

// commandStyle renders the command line itself in the confirm prompt.
func commandStyle(colorEnabled bool) lipgloss.Style {
	if !colorEnabled {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Bold(true)
}

// PromptYellow is the pastel-yellow ANSI256 code the edit-mode textinput's
// "> " prompt symbol renders in (editPromptStyle, below) — deliberately
// the SAME value as internal/cli/color.go's unexported paletteYellow
// constant, for one consistent prompt color across both of this
// codebase's "> "-prompted textinputs (this package's own ask-mode edit
// prompt, and internal/cli/chatmodel.go's chat input). internal/tui
// cannot import internal/cli (internal/cli already imports internal/tui
// — the correct, one-way dependency direction; importing the other way
// would be a cycle), so this is a deliberate, minimal, hand-maintained
// mirror of a single scalar value rather than a shared color package for
// one constant. It is NOT an unguarded mirror: internal/cli's own test
// suite (color_test.go's TestPromptYellowMatchesTUIPackage) asserts
// paletteYellow == tui.PromptYellow — internal/cli is already permitted
// to import internal/tui, so that guard lives on the cli side, and it
// fails the moment either constant changes without the other.
const PromptYellow = "222"

// editPromptStyle returns the lipgloss.Style the ask-mode confirm
// prompt's edit-mode textinput applies to its own "> " prompt symbol —
// PromptYellow when colorEnabled, or a completely empty, unstyled Style
// (matching every other style function in this file) when not, so a
// disabled render is genuinely byte-clean, not merely "not yellow yet
// still colored" (see newConfirmModel's own doc comment for the
// pre-existing bug this closes: bubbles/v2/textinput.New()'s own default
// styles color the prompt unconditionally, with no colorEnabled/NO_COLOR/
// TTY awareness at all).
func editPromptStyle(colorEnabled bool) lipgloss.Style {
	if !colorEnabled {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color(PromptYellow))
}

// doctorSeverityGlyphs maps each internal/doctor.Severity to the
// checkmark-family symbol `comrade doctor` renders it as when color is
// enabled, and doctorSeverityColors maps the same Severity to that
// symbol's ANSI256 color — index by doctor.Severity's own integer
// ordinal, exactly like riskColors indexes by safety.RiskClass above.
var (
	doctorSeverityGlyphs = [...]string{
		doctor.SeverityOK:   "✓",
		doctor.SeverityWarn: "⚠",
		doctor.SeverityFail: "✗",
		doctor.SeveritySkip: "-",
	}
	doctorSeverityColors = [...]string{
		doctor.SeverityOK:   "10", // green
		doctor.SeverityWarn: "3",  // yellow
		doctor.SeverityFail: "1",  // red
		doctor.SeveritySkip: "8",  // gray
	}
	// doctorSeverityWords is the word-fallback vocabulary
	// DoctorSeverityLabel renders when colorEnabled is false — bracketed
	// plain text, exactly like RiskBadge's own colorEnabled=false
	// fallback ("[" + label + "]") a few lines above. Deliberately NOT
	// routed through internal/i18n: this is internal, stable vocabulary
	// (four fixed states), not prose — the same precedent RiskBadge and
	// secrets.Source's un-translated "keychain"/"file" values already
	// established in this codebase.
	doctorSeverityWords = [...]string{
		doctor.SeverityOK:   "[OK]",
		doctor.SeverityWarn: "[WARN]",
		doctor.SeverityFail: "[FAIL]",
		doctor.SeveritySkip: "[SKIP]",
	}
)

// doctorSeverityGlyph returns sev's checkmark-family symbol, or "?" for
// an out-of-range Severity value (defensive only — every Severity this
// package's own callers ever construct is one of the four named
// constants).
func doctorSeverityGlyph(sev doctor.Severity) string {
	if int(sev) >= 0 && int(sev) < len(doctorSeverityGlyphs) {
		return doctorSeverityGlyphs[sev]
	}
	return "?"
}

// doctorSeverityStyle returns the lipgloss.Style DoctorSeverityLabel
// renders sev's glyph in. A completely unstyled Style when colorEnabled
// is false, matching every other style function in this file.
func doctorSeverityStyle(sev doctor.Severity, colorEnabled bool) lipgloss.Style {
	if !colorEnabled {
		return lipgloss.NewStyle()
	}
	color := "7"
	if int(sev) >= 0 && int(sev) < len(doctorSeverityColors) {
		color = doctorSeverityColors[sev]
	}
	return lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color(color))
}

// DoctorSeverityLabel renders sev as a colored ✓/⚠/✗/- symbol when
// colorEnabled, or the bracketed word fallback ([OK]/[WARN]/[FAIL]/
// [SKIP]) otherwise — `comrade doctor`'s per-check severity marker (see
// doctorSeverityWords' own doc comment for why this is deliberately not
// i18n'd).
func DoctorSeverityLabel(sev doctor.Severity, colorEnabled bool) string {
	if !colorEnabled {
		if int(sev) >= 0 && int(sev) < len(doctorSeverityWords) {
			return doctorSeverityWords[sev]
		}
		return "[?]"
	}
	return doctorSeverityStyle(sev, colorEnabled).Render(doctorSeverityGlyph(sev))
}
