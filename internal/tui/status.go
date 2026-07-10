package tui

import (
	"fmt"
	"io"

	"charm.land/lipgloss/v2"
)

// statusStyle renders auto mode's one-line "-> running: <cmd>" status
// lines. Plain, unstyled when colorEnabled is false.
func statusStyle(colorEnabled bool) lipgloss.Style {
	if !colorEnabled {
		return lipgloss.NewStyle()
	}
	return lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
}

// PrintStatus writes one auto-mode status line to w — docs/history/UYGULAMA_PLANI.md
// FAZ 6's "her adımda tek satır durum yaz" requirement. Auto mode
// deliberately does not need a full bubbletea program per step (see this
// package's doc comment); a single styled, newline-terminated Fprintln is
// the whole of its UI.
func PrintStatus(w io.Writer, line string, colorEnabled bool) error {
	_, err := fmt.Fprintln(w, statusStyle(colorEnabled).Render(line))
	return err
}

// PrintWarning writes line to w as the mandatory red warning banner —
// CLAUDE.md security rule #6 (every --yolo use prints a red warning) and
// the auto-mode destructive/elevated bypass notice docs/history/UYGULAMA_PLANI.md FAZ 6
// requires each time that bypass actually fires.
func PrintWarning(w io.Writer, line string, colorEnabled bool) error {
	_, err := fmt.Fprintln(w, RenderWarning(line, colorEnabled))
	return err
}

// PrintExplanation writes an LLM-provided detailed explanation for a
// single step to w. The explanation text itself is fetched by the caller
// (internal/engine's Runner, via its Completer) — this package only
// renders it; it has no LLM dependency of its own.
func PrintExplanation(w io.Writer, explanation string) error {
	_, err := fmt.Fprintln(w, explanation)
	return err
}
