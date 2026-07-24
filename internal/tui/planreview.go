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

// PlanReviewStep is the minimal, presentation-only view of one plan step
// the plan-preview/edit screen renders — mirrors PromptStep's own
// engine-free shape (see PromptStep's doc comment): internal/cli converts
// an engine.Step into this before calling ReviewPlan, and converts the
// returned ReviewOutcome back into an engine.Plan afterward. This package
// never imports internal/engine and never evaluates safety itself —
// Blocked/BlockReason are supplied by the caller (internal/cli, from
// safety.Engine.Evaluate's own Decision) and are purely rendered, never
// recomputed here.
type PlanReviewStep struct {
	Command     string
	Rationale   string
	Risk        safety.RiskClass
	Blocked     bool
	BlockReason string
}

// ReviewedStep is one step of ReviewOutcome.Steps: the position it
// originally occupied in ReviewPlan's own steps argument (OriginalIndex —
// stable across reordering, so a caller re-entering ReviewPlan after a
// post-edit safety re-evaluation can map a row back to the plan step it
// came from), its current (possibly edited) command text, and whether
// the user toggled it to be skipped. A step the user DELETED entirely is
// simply absent from this slice — there is no tombstone entry for it.
type ReviewedStep struct {
	OriginalIndex int
	Command       string
	Skipped       bool
}

// ReviewOutcome is ReviewPlan's result. Approved is false when the user
// canceled the whole review (ctrl+c, or esc while not mid-edit) — Steps
// is empty and must not be acted on in that case, exactly like
// tui.Confirm's No/ctx-canceled outcomes carry no usable edited text.
type ReviewOutcome struct {
	Approved bool
	Steps    []ReviewedStep
}

// planReviewAction is one letter-bound action mapPlanReviewKey resolves a
// keypress to, under the exact accepted key set of the active language —
// see mapPlanReviewKey's own doc comment for the full TR/EN key tables
// and confirm.go's mapKey (~74-113) for the collision-avoidance
// discipline this reuses.
type planReviewAction int

const (
	actionNone planReviewAction = iota
	// actionMoveUp/actionMoveDown reorder the row under the cursor within
	// the list (the cursor follows the moved row) — distinct from the
	// language-neutral Up/Down arrow keys, which only move the cursor
	// between rows without reordering anything.
	actionMoveUp
	actionMoveDown
	// actionEdit starts inline-editing the row under the cursor —
	// refused (a no-op) when that row is Blocked; see canEditRow.
	actionEdit
	// actionDelete removes the row under the cursor from the list
	// entirely (no tombstone, unlike toggle-skip).
	actionDelete
	// actionApproveAll finishes the review with Approved=true.
	actionApproveAll
)

// mapPlanReviewKey maps a single bubbletea key string (tea.KeyPressMsg.
// String()) to the planReviewAction it selects under lang's exact
// accepted key set, mirroring confirm.go's mapKey precedent (~74-113):
// the TR and EN switches are resolved STRICTLY and separately — never
// merged into one unioned switch — so a key that means one action under
// TR and a different one under EN can never be reached by the wrong
// language's dispatch. Arrow keys, space, enter and esc are handled
// directly by planReviewModel.Update (language-neutral — see this file's
// package doc comment); only the five letter-bound actions above go
// through this function.
//
// TR: y=yukarı taşı(move up) a=aşağı taşı(move down) d=düzenle(edit)
// s=sil(delete) t=tümünü onayla(approve all).
// EN: u=up d=down e=edit r=remove a=all.
//
// Note the deliberate cross-language overlaps this tolerates (TR's "a" is
// move-down while EN's "a" is approve-all; TR's "d" is edit while EN's
// "d" is move-down) — exactly the same shape of collision confirm.go's
// own mapKey already accepts (TR "e"=Yes/EN "e"=Edit, TR "a"=Explain/EN
// "a"=All) and defends against the same way: never dispatching from a
// unioned key set, only ever from the single active language's own
// switch.
func mapPlanReviewKey(lang i18n.Lang, key string) (action planReviewAction, ok bool) {
	if lang == i18n.LangTR {
		switch key {
		case "y":
			return actionMoveUp, true
		case "a":
			return actionMoveDown, true
		case "d":
			return actionEdit, true
		case "s":
			return actionDelete, true
		case "t":
			return actionApproveAll, true
		default:
			return actionNone, false
		}
	}

	switch key {
	case "u":
		return actionMoveUp, true
	case "d":
		return actionMoveDown, true
	case "e":
		return actionEdit, true
	case "r":
		return actionDelete, true
	case "a":
		return actionApproveAll, true
	default:
		return actionNone, false
	}
}

// planReviewRow is one row of planReviewModel's working list: the
// PlanReviewStep as currently shown (Command may have been edited away
// from the original), whether the user has toggled it to be skipped, and
// which index in ReviewPlan's original steps argument it came from
// (originalIndex — used to populate ReviewedStep.OriginalIndex, stable
// across reordering/deletion of OTHER rows).
type planReviewRow struct {
	originalIndex int
	step          PlanReviewStep
	skipped       bool
}

// newPlanReviewRows builds the initial, identity-ordered working list
// from ReviewPlan's steps argument.
func newPlanReviewRows(steps []PlanReviewStep) []planReviewRow {
	rows := make([]planReviewRow, len(steps))
	for i, s := range steps {
		rows[i] = planReviewRow{originalIndex: i, step: s}
	}
	return rows
}

// moveCursorUp/moveCursorDown clamp cursor navigation to the list's
// bounds (no wraparound) — these move ONLY the cursor, never a row's
// position; see moveRowUp/moveRowDown for the reorder actions.
func moveCursorUp(cursor int) int {
	if cursor <= 0 {
		return cursor
	}
	return cursor - 1
}

func moveCursorDown(rows []planReviewRow, cursor int) int {
	if cursor >= len(rows)-1 {
		return cursor
	}
	return cursor + 1
}

// moveRowUp/moveRowDown are the pure, testable core of the "move up/down
// (reorder)" action: they swap the row under cursor with its neighbor
// and return the cursor position that keeps following the moved row. A
// cursor already at the relevant edge (or out of the rows' bounds
// entirely — defensive, never produced by Update's own bounds-respecting
// cursor navigation) is a no-op, returning rows/cursor unchanged. Neither
// function mutates rows in place — each returns a fresh copy, per this
// project's immutability convention.
func moveRowUp(rows []planReviewRow, cursor int) ([]planReviewRow, int) {
	if cursor <= 0 || cursor >= len(rows) {
		return rows, cursor
	}
	out := make([]planReviewRow, len(rows))
	copy(out, rows)
	out[cursor-1], out[cursor] = out[cursor], out[cursor-1]
	return out, cursor - 1
}

func moveRowDown(rows []planReviewRow, cursor int) ([]planReviewRow, int) {
	if cursor < 0 || cursor >= len(rows)-1 {
		return rows, cursor
	}
	out := make([]planReviewRow, len(rows))
	copy(out, rows)
	out[cursor+1], out[cursor] = out[cursor], out[cursor+1]
	return out, cursor + 1
}

// toggleRowSkip flips the skipped flag of the row under cursor. Allowed
// for a Blocked row (spec: a Blocked row can only be deleted/skipped,
// never edited/un-blocked) — see canEditRow for the one action this
// package actually refuses on a Blocked row.
func toggleRowSkip(rows []planReviewRow, cursor int) []planReviewRow {
	if cursor < 0 || cursor >= len(rows) {
		return rows
	}
	out := make([]planReviewRow, len(rows))
	copy(out, rows)
	out[cursor].skipped = !out[cursor].skipped
	return out
}

// deleteRow removes the row under cursor entirely, clamping the returned
// cursor back into the shrunk list's bounds (the row after the deleted
// one slides into the same visual position; deleting the last row moves
// the cursor to the new last row).
func deleteRow(rows []planReviewRow, cursor int) ([]planReviewRow, int) {
	if cursor < 0 || cursor >= len(rows) {
		return rows, cursor
	}
	out := make([]planReviewRow, 0, len(rows)-1)
	out = append(out, rows[:cursor]...)
	out = append(out, rows[cursor+1:]...)
	newCursor := cursor
	if newCursor >= len(out) {
		newCursor = len(out) - 1
	}
	return out, newCursor
}

// canEditRow reports whether the row under cursor may enter edit mode —
// false for a Blocked row (the non-negotiable "CANNOT be un-blocked or
// edited into execution" rule) or an out-of-bounds cursor (defensive
// only — Update never calls this with one).
func canEditRow(rows []planReviewRow, cursor int) bool {
	if cursor < 0 || cursor >= len(rows) {
		return false
	}
	return !rows[cursor].step.Blocked
}

// applyRowEdit replaces the command text of the row under cursor. Update
// only ever calls this after canEditRow already confirmed the row isn't
// Blocked (edit mode can only be entered via actionEdit, which is itself
// gated the same way), so this function does not re-check Blocked
// itself — it is the single, unconditional "commit the edited text" step
// of the edit flow, exactly mirroring confirmModel's own edit-then-enter
// path (confirm.go's Update, the "enter" case under m.editing).
func applyRowEdit(rows []planReviewRow, cursor int, newCommand string) []planReviewRow {
	if cursor < 0 || cursor >= len(rows) {
		return rows
	}
	out := make([]planReviewRow, len(rows))
	copy(out, rows)
	out[cursor].step.Command = newCommand
	return out
}

// buildOutcome converts rows into the ReviewOutcome ReviewPlan returns. A
// canceled review (approved=false) carries no steps at all — mirroring
// tui.Confirm's No/ctx-canceled outcomes, which never carry usable edited
// text either.
func buildOutcome(rows []planReviewRow, approved bool) ReviewOutcome {
	if !approved {
		return ReviewOutcome{Approved: false}
	}
	steps := make([]ReviewedStep, len(rows))
	for i, r := range rows {
		steps[i] = ReviewedStep{OriginalIndex: r.originalIndex, Command: r.step.Command, Skipped: r.skipped}
	}
	return ReviewOutcome{Approved: true, Steps: steps}
}

// planReviewModel is the bubbletea (v2) model backing ReviewPlan. Its
// Update method is a thin shell around the pure functions above — every
// interesting reorder/skip/edit/delete/approve decision lives in one of
// them, exactly like confirmModel delegates its own decision logic to
// mapKey (see confirm.go's doc comment) — so planreview_test.go exercises
// those functions directly, with no bubbletea program/PTY involved at
// all.
type planReviewModel struct {
	rows         []planReviewRow
	cursor       int
	colorEnabled bool
	tr           i18n.Translator

	editing bool
	input   textinput.Model

	outcome ReviewOutcome
	done    bool
}

// newPlanReviewModel builds the planReviewModel ReviewPlan drives,
// closing the same bubbles/v2/textinput unconditional-prompt-color leak
// newConfirmModel's own doc comment documents (confirm.go) — both models'
// edit-mode textinput share the identical PromptYellow/editPromptStyle
// treatment for their "> " prompt symbol.
func newPlanReviewModel(steps []PlanReviewStep, colorEnabled bool, tr i18n.Translator) planReviewModel {
	ti := textinput.New()
	ti.Prompt = "> "

	promptStyle := editPromptStyle(colorEnabled)
	tiStyles := ti.Styles()
	tiStyles.Focused.Prompt = promptStyle
	tiStyles.Blurred.Prompt = promptStyle
	ti.SetStyles(tiStyles)

	return planReviewModel{rows: newPlanReviewRows(steps), colorEnabled: colorEnabled, tr: tr, input: ti}
}

func (m planReviewModel) Init() tea.Cmd {
	return nil
}

// Update implements tea.Model. Key handling order mirrors confirmModel's
// Update exactly: ctrl+c always aborts first (even mid-edit), then
// edit-mode keys are handled (enter commits, esc cancels back to
// browsing, everything else passes through to the textinput), then —
// only in browsing mode — the language-neutral Up/Down/space/esc keys,
// and finally mapPlanReviewKey's five letter-bound actions.
func (m planReviewModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		m.outcome = buildOutcome(m.rows, false)
		m.done = true
		return m, tea.Quit
	}

	if m.editing {
		switch key {
		case "enter":
			m.rows = applyRowEdit(m.rows, m.cursor, m.input.Value())
			m.editing = false
			return m, nil
		case "esc":
			m.editing = false
			return m, nil
		}
		var cmd tea.Cmd
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}

	switch key {
	case "up":
		m.cursor = moveCursorUp(m.cursor)
		return m, nil
	case "down":
		m.cursor = moveCursorDown(m.rows, m.cursor)
		return m, nil
	case "space":
		m.rows = toggleRowSkip(m.rows, m.cursor)
		return m, nil
	case "esc":
		m.outcome = buildOutcome(m.rows, false)
		m.done = true
		return m, tea.Quit
	}

	action, matched := mapPlanReviewKey(m.tr.Lang(), key)
	if !matched {
		return m, nil
	}

	switch action {
	case actionMoveUp:
		m.rows, m.cursor = moveRowUp(m.rows, m.cursor)
	case actionMoveDown:
		m.rows, m.cursor = moveRowDown(m.rows, m.cursor)
	case actionEdit:
		if canEditRow(m.rows, m.cursor) {
			m.editing = true
			m.input.SetValue(m.rows[m.cursor].step.Command)
			cmd := m.input.Focus()
			return m, cmd
		}
	case actionDelete:
		m.rows, m.cursor = deleteRow(m.rows, m.cursor)
	case actionApproveAll:
		m.outcome = buildOutcome(m.rows, true)
		m.done = true
		return m, tea.Quit
	}

	return m, nil
}

// View implements tea.Model: a numbered, cursor-highlighted list, each
// row showing its risk badge (or a BLOCKED(<reason>) marker in place of
// the badge for a Blocked row — never both), the command text, and a
// trailing skipped marker when toggled; the rationale renders dim below
// each row. The trailing legend line lists every available action in the
// active language.
func (m planReviewModel) View() tea.View {
	var b strings.Builder

	if m.editing {
		b.WriteString(m.tr.T(i18n.MsgConfirmEditHeader))
		b.WriteString(m.input.View())
		return tea.NewView(b.String())
	}

	b.WriteString(m.tr.T(i18n.MsgPlanReviewHeader))

	for i, row := range m.rows {
		cursor := "  "
		if i == m.cursor {
			cursor = "> "
		}

		var marker string
		if row.step.Blocked {
			marker = m.tr.T(i18n.MsgPlanReviewBlockedMarker, row.step.BlockReason)
		} else {
			marker = RiskBadge(row.step.Risk, m.colorEnabled)
		}

		fmt.Fprintf(&b, "%s%d. %s %s", cursor, i+1, marker, commandStyle(m.colorEnabled).Render(row.step.Command))
		if row.skipped {
			fmt.Fprintf(&b, " %s", m.tr.T(i18n.MsgPlanReviewSkippedMarker))
		}
		b.WriteString("\n")
		if row.step.Rationale != "" {
			fmt.Fprintf(&b, "     %s\n", row.step.Rationale)
		}
	}

	b.WriteString(m.tr.T(i18n.MsgPlanReviewLegend))
	return tea.NewView(b.String())
}

// ReviewPlan runs the interactive plan-preview/edit screen for steps and
// blocks until the user approves (all remaining rows) or cancels
// (ctrl+c, or esc while not mid-edit), ctx is canceled, or the underlying
// bubbletea program errors — the same contract shape as tui.Confirm (see
// its own doc comment): in/out wire the program to specific streams, and
// tr resolves both the rendered text AND, via tr.Lang(), which of the two
// disjoint per-language letter-action sets mapPlanReviewKey accepts.
//
// This package NEVER evaluates safety.Engine itself: a Blocked row is
// rendered exactly as steps[i].Blocked/BlockReason says, and an edited
// command's Blocked status never changes within a single ReviewPlan
// call — internal/cli is responsible for re-running safety.Engine.
// Evaluate on every edited command after ReviewPlan returns, and for
// calling ReviewPlan again (with an updated steps argument reflecting the
// newly-Blocked row) when that re-evaluation finds one. See
// internal/cli/planreview.go's reviewPlan for that loop.
func ReviewPlan(ctx context.Context, steps []PlanReviewStep, colorEnabled bool, in io.Reader, out io.Writer, tr i18n.Translator) (ReviewOutcome, error) {
	m := newPlanReviewModel(steps, colorEnabled, tr)
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out))

	finalModel, runErr := p.Run()
	if runErr != nil {
		return ReviewOutcome{}, fmt.Errorf("tui: run plan review: %w", runErr)
	}
	if ctx.Err() != nil {
		return ReviewOutcome{}, ctx.Err()
	}

	fm, ok := finalModel.(planReviewModel)
	if !ok {
		return ReviewOutcome{}, fmt.Errorf("tui: plan review returned an unexpected model type %T", finalModel)
	}
	return fm.outcome, nil
}
