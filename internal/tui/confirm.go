package tui

import (
	"context"
	"fmt"
	"io"
	"strings"

	"charm.land/bubbles/v2/textinput"
	tea "charm.land/bubbletea/v2"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// PromptChoice is the user's response to one ask-mode confirm prompt. The
// accepted keypress for each choice is language-specific (see mapKey) —
// TR: [e]vet [h]ayır [d]üzenle [a]çıkla [t]ümü; EN: [y]es [n]o [e]dit
// [x]plain [a]ll. Note the deliberate non-overlap: TR's "e" is Yes while
// EN's "e" is Edit, and TR's "a" is Explain while EN's "a" is All — mapKey
// resolves strictly by the active language, never as a union of both key
// sets, specifically to avoid a user in one language pressing a key that
// means something dangerously different in the other.
type PromptChoice int

const (
	// Yes (TR "e"vet, EN "y"es) runs the step as shown.
	Yes PromptChoice = iota
	// No (TR "h"ayır, EN "n"o) skips this step.
	No
	// Edit (TR "d"üzenle, EN "e"dit) opens an inline textinput to edit
	// the command before re-confirming; Confirm's caller must re-run the
	// edited command through internal/safety before acting on it — this
	// package never re-evaluates safety itself.
	Edit
	// Explain (TR "a"çıkla, EN e"x"plain) asks the caller to fetch and
	// display a detailed explanation for this step, then re-show the
	// prompt.
	Explain
	// All (TR "t"ümü, EN "a"ll) approves this step and every remaining
	// read/write/network step without asking again; destructive/elevated
	// steps still prompt individually — Confirm itself has no notion of
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
// selects, under the exact accepted key set of lang. ok is false for any
// key that doesn't select a choice in that language (the caller should
// keep waiting for another keypress). This is a pure function specifically
// so it can be unit-tested directly, with no bubbletea program/PTY
// involved at all.
//
// The two switches below are resolved STRICTLY by lang — never merged
// into one combined switch — by design: TR's "e"=Yes and EN's "e"=Edit
// collide, as do TR's "a"=Explain and EN's "a"=All. A union would let a
// TR-trained user press "e" under an EN prompt expecting Yes and silently
// get Edit instead (or the reverse for "a"/All vs "a"/Explain) — exactly
// the confirm-prompt hazard this per-language split exists to prevent.
func mapKey(lang i18n.Lang, key string) (choice PromptChoice, ok bool) {
	if lang == i18n.LangTR {
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

	switch key {
	case "y":
		return Yes, true
	case "n":
		return No, true
	case "e":
		return Edit, true
	case "x":
		return Explain, true
	case "a":
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
	tr           i18n.Translator

	editing bool
	input   textinput.Model

	chosen        PromptChoice
	editedCommand string
}

func newConfirmModel(step PromptStep, colorEnabled bool, tr i18n.Translator) confirmModel {
	ti := textinput.New()
	ti.Prompt = "> "
	ti.SetValue(step.Command)
	return confirmModel{step: step, colorEnabled: colorEnabled, tr: tr, input: ti}
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

	choice, matched := mapKey(m.tr.Lang(), key)
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
		b.WriteString(m.tr.T(i18n.MsgConfirmEditHeader))
		b.WriteString(m.input.View())
		return tea.NewView(b.String())
	}

	badge := RiskBadge(m.step.Risk, m.colorEnabled)
	fmt.Fprintf(&b, "%s %s\n", badge, commandStyle(m.colorEnabled).Render(m.step.Command))
	if m.step.Rationale != "" {
		fmt.Fprintf(&b, "  %s\n", m.step.Rationale)
	}
	b.WriteString(m.tr.T(i18n.MsgConfirmLegend))
	return tea.NewView(b.String())
}

// Confirm runs the interactive confirm prompt for step and blocks until
// the user picks a choice, ctx is canceled, or the underlying bubbletea
// program errors. in/out wire the program to specific streams (a real
// terminal in production; the caller decides — internal/cli's wiring is
// the only production caller, so tests never need a real PTY).
//
// tr is the injected Translator (per this project's dependency-injection
// rule — no global/package-level language state anywhere in this
// package) that resolves both the rendered legend/edit-header text AND,
// via tr.Lang(), which of the two disjoint per-language key sets mapKey
// accepts. The caller (internal/cli's tuiPromptUI) builds tr from the
// exact same general.language/COMRADE_LANG/LANG/LC_ALL resolution chain
// every other command's output uses (see internal/i18n.ResolveLanguage
// and internal/cli's newTranslator) — there is no separate, confirm-
// prompt-specific language decision anywhere.
//
// When choice == Edit, editedCommand is the user's edited command text —
// NOT yet re-evaluated by internal/safety; the caller (internal/engine's
// Runner) is responsible for re-running it through safety.Engine.Evaluate
// before acting on it, per UYGULAMA_PLANI.md FAZ 6's ask-mode edit rule.
func Confirm(ctx context.Context, step PromptStep, colorEnabled bool, in io.Reader, out io.Writer, tr i18n.Translator) (choice PromptChoice, editedCommand string, err error) {
	m := newConfirmModel(step, colorEnabled, tr)
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
