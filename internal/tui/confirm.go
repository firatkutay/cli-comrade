package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/firatkutay/cli-comrade/internal/safety"
)

// PromptChoice is the user's response to one ask-mode confirm prompt, per
// CLAUDE.md's exact TR letters: [e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü.
type PromptChoice int

const (
	// Yes ("e"vet) runs the step as shown.
	Yes PromptChoice = iota
	// No ("h"ayır) skips this step.
	No
	// Edit ("d"üzenle) opens an inline textinput to edit the command
	// before re-confirming; Confirm's caller must re-run the edited
	// command through internal/safety before acting on it — this
	// package never re-evaluates safety itself.
	Edit
	// Explain ("a"çıkla) asks the caller to fetch and display a detailed
	// explanation for this step, then re-show the prompt.
	Explain
	// All ("t"ümü) approves this step and every remaining read/write/
	// network step without asking again; destructive/elevated steps
	// still prompt individually — Confirm itself has no notion of
	// "remaining steps", so this is purely a signal the mode-loop caller
	// interprets.
	All
)

// String renders c using the same lowercase English name used throughout
// this package's own doc comments and tests.
func (c PromptChoice) String() string {
	switch c {
	case Yes:
		return "yes"
	case No:
		return "no"
	case Edit:
		return "edit"
	case Explain:
		return "explain"
	case All:
		return "all"
	default:
		return "unknown"
	}
}

// mapKey maps a single bubbletea key string (tea.KeyPressMsg.String(), or
// any other Stringer-shaped key representation) to the PromptChoice it
// selects. ok is false for any key that doesn't select a choice (the
// caller should keep waiting for another keypress). This is a pure
// function specifically so it can be unit-tested directly, with no
// bubbletea program/PTY involved at all.
func mapKey(key string) (choice PromptChoice, ok bool) {
	switch key {
	case "e":
		return Yes, true
	case "h":
		return No, true
	case "d":
		return Edit, true
	case "a":
		return Explain, true
	case "t":
		return All, true
	default:
		return 0, false
	}
}

// PromptStep is the minimal, presentation-only view of a plan step that
// confirmModel renders: just enough to show the command, its one-line
// rationale, and its risk badge. It deliberately does not depend on
// internal/engine's Step type — see docs/phases/FAZ-06.md's layering note:
// this package (internal/tui) stays independent of internal/engine, and
// internal/cli's adapter converts an engine.Step into this shape when it
// wires the real bubbletea implementation into engine.Runner's PromptUI
// interface.
type PromptStep struct {
	Command   string
	Rationale string
	Risk      safety.RiskClass
}

// confirmModel is the bubbletea (v2) model backing Confirm. Its Update
// method is a thin shell around mapKey: all the interesting
// keypress-to-choice decision logic lives in that pure function, which is
// what confirm_test.go exercises directly; this model only wires mapKey's
// result to bubbletea's Cmd/quit protocol and manages the one piece of
// genuinely stateful UI (the inline edit textinput).
type confirmModel struct {
	step         PromptStep
	colorEnabled bool

	editing bool
	input   textinput.Model

	chosen        PromptChoice
	editedCommand string
}

func newConfirmModel(step PromptStep, colorEnabled bool) confirmModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.SetValue(step.Command)
	return confirmModel{step: step, colorEnabled: colorEnabled, input: ti}
}

func (m confirmModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. See confirmModel's doc comment: the actual
// decision logic is mapKey; this method only handles the edit-mode
// textinput passthrough and bubbletea's own Cmd/quit plumbing.
func (m confirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	keyMsg, ok := msg.(tea.KeyPressMsg)
	if !ok {
		if m.editing {
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
		return m, nil
	}

	key := keyMsg.String()

	if key == "ctrl+c" {
		m.chosen = No
		return m, tea.Quit
	}

	if m.editing {
		switch key {
		case "enter":
			m.chosen = Edit
			m.editedCommand = m.input.Value()
			return m, tea.Quit
		case "esc":
			m.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	choice, matched := mapKey(key)
	if !matched {
		return m, nil
	}

	if choice == Edit {
		m.editing = true
		cmd := m.input.Focus()
		return m, cmd
	}

	m.chosen = choice
	return m, tea.Quit
}

func (m confirmModel) View() tea.View {
	var b strings.Builder

	if m.editing {
		b.WriteString("Edit command (enter to confirm, esc to cancel):\n")
		b.WriteString(m.input.View())
		return tea.NewView(b.String())
	}

	badge := RiskBadge(m.step.Risk, m.colorEnabled)
	fmt.Fprintf(&b, "%s %s\n", badge, commandStyle(m.colorEnabled).Render(m.step.Command))
	if m.step.Rationale != "" {
		fmt.Fprintf(&b, "  %s\n", m.step.Rationale)
	}
	b.WriteString("[e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü: ")
	return tea.NewView(b.String())
}

// Confirm runs the interactive confirm prompt for step and blocks until
// the user picks a choice, ctx is canceled, or the underlying bubbletea
// program errors. in/out wire the program to specific streams (a real
// terminal in production; the caller decides — internal/cli's wiring is
// the only production caller, so tests never need a real PTY).
//
// When choice == Edit, editedCommand is the user's edited command text —
// NOT yet re-evaluated by internal/safety; the caller (internal/engine's
// Runner) is responsible for re-running it through safety.Engine.Evaluate
// before acting on it, per UYGULAMA_PLANI.md FAZ 6's ask-mode edit rule.
func Confirm(ctx context.Context, step PromptStep, colorEnabled bool, in io.Reader, out io.Writer) (choice PromptChoice, editedCommand string, err error) {
	m := newConfirmModel(step, colorEnabled)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out))

	finalModel, runErr := p.Run()
	if runErr != nil {
		return No, "", fmt.Errorf("tui: run confirm prompt: %w", runErr)
	}
	if ctx.Err() != nil {
		return No, "", ctx.Err()
	}

	fm, ok := finalModel.(confirmModel)
	if !ok {
		return No, "", fmt.Errorf("tui: confirm prompt returned an unexpected model type %T", finalModel)
	}
	return fm.chosen, fm.editedCommand, nil
}
