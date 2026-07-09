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
