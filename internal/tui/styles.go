// Package tui holds the bubbletea/lipgloss components used for the mode
// loop's confirmation prompts (internal/engine's ask mode) and auto mode's
// one-line status output (introduced in FAZ 6).
package tui

import (
	"charm.land/lipgloss/v2"

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
