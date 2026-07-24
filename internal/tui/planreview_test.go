package tui

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// upKey/downKey/spaceKey build the tea.KeyPressMsg values a real terminal
// sends for the Up/Down arrow keys and the space bar — the three
// language-neutral keys this file's tests exercise alongside
// confirm_test.go's own letterKey/ctrlCKey/enterKey/escKey helpers
// (same package, reused directly).
func upKey() tea.KeyPressMsg    { return tea.KeyPressMsg{Code: tea.KeyUp} }
func downKey() tea.KeyPressMsg  { return tea.KeyPressMsg{Code: tea.KeyDown} }
func spaceKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeySpace} }

// --- mapPlanReviewKey ---------------------------------------------------

func TestMapPlanReviewKeyTRLetters(t *testing.T) {
	cases := []struct {
		key    string
		want   planReviewAction
		wantOK bool
	}{
		{"y", actionMoveUp, true},
		{"a", actionMoveDown, true},
		{"d", actionEdit, true},
		{"s", actionDelete, true},
		{"t", actionApproveAll, true},
		// EN-only letters must NOT match under TR.
		{"u", actionNone, false},
		{"e", actionNone, false},
		{"r", actionNone, false},
		{"", actionNone, false},
	}
	for _, tc := range cases {
		action, ok := mapPlanReviewKey(i18n.LangTR, tc.key)
		assert.Equal(t, tc.wantOK, ok, "key %q", tc.key)
		if tc.wantOK {
			assert.Equal(t, tc.want, action, "key %q", tc.key)
		}
	}
}

// TestMapPlanReviewKeyENLetters is the regression guard for the two
// cross-language collisions this file's own mapPlanReviewKey doc comment
// documents: EN "a" must be approve-all (NOT move-down, TR's meaning),
// and EN "d" must be move-down (NOT edit, TR's meaning).
func TestMapPlanReviewKeyENLetters(t *testing.T) {
	cases := []struct {
		key    string
		want   planReviewAction
		wantOK bool
	}{
		{"u", actionMoveUp, true},
		{"d", actionMoveDown, true}, // regression guard: NOT Edit (TR's meaning for "d")
		{"e", actionEdit, true},
		{"r", actionDelete, true},
		{"a", actionApproveAll, true}, // regression guard: NOT MoveDown (TR's meaning for "a")
		// TR-only letters must NOT match under EN.
		{"y", actionNone, false},
		{"s", actionNone, false},
		{"t", actionNone, false},
		{"", actionNone, false},
	}
	for _, tc := range cases {
		action, ok := mapPlanReviewKey(i18n.LangEN, tc.key)
		assert.Equal(t, tc.wantOK, ok, "key %q", tc.key)
		if tc.wantOK {
			assert.Equal(t, tc.want, action, "key %q", tc.key)
		}
	}
}

// --- pure row-transition functions ---------------------------------------

func threeRows() []planReviewRow {
	return []planReviewRow{
		{originalIndex: 0, step: PlanReviewStep{Command: "one"}},
		{originalIndex: 1, step: PlanReviewStep{Command: "two"}},
		{originalIndex: 2, step: PlanReviewStep{Command: "three"}},
	}
}

func TestMoveCursorUpClampsAtZero(t *testing.T) {
	assert.Equal(t, 0, moveCursorUp(0))
	assert.Equal(t, 1, moveCursorUp(2))
}

func TestMoveCursorDownClampsAtLastRow(t *testing.T) {
	rows := threeRows()
	assert.Equal(t, 2, moveCursorDown(rows, 2))
	assert.Equal(t, 1, moveCursorDown(rows, 0))
}

func TestMoveRowUpSwapsAndFollowsCursor(t *testing.T) {
	rows := threeRows()
	out, cursor := moveRowUp(rows, 1)
	require.Equal(t, 0, cursor)
	assert.Equal(t, []string{"two", "one", "three"}, commandsOf(out))
	// original slice must be untouched (immutability convention).
	assert.Equal(t, []string{"one", "two", "three"}, commandsOf(rows))
}

func TestMoveRowUpAtTopIsNoOp(t *testing.T) {
	rows := threeRows()
	out, cursor := moveRowUp(rows, 0)
	assert.Equal(t, 0, cursor)
	assert.Equal(t, []string{"one", "two", "three"}, commandsOf(out))
}

func TestMoveRowDownSwapsAndFollowsCursor(t *testing.T) {
	rows := threeRows()
	out, cursor := moveRowDown(rows, 1)
	require.Equal(t, 2, cursor)
	assert.Equal(t, []string{"one", "three", "two"}, commandsOf(out))
}

func TestMoveRowDownAtBottomIsNoOp(t *testing.T) {
	rows := threeRows()
	out, cursor := moveRowDown(rows, 2)
	assert.Equal(t, 2, cursor)
	assert.Equal(t, []string{"one", "two", "three"}, commandsOf(out))
}

func TestToggleRowSkipFlipsOnlyTheCursorRow(t *testing.T) {
	rows := threeRows()
	out := toggleRowSkip(rows, 1)
	assert.False(t, out[0].skipped)
	assert.True(t, out[1].skipped)
	assert.False(t, out[2].skipped)

	out2 := toggleRowSkip(out, 1)
	assert.False(t, out2[1].skipped, "toggling twice must flip back")
}

func TestDeleteRowRemovesEntirelyAndClampsCursor(t *testing.T) {
	rows := threeRows()
	out, cursor := deleteRow(rows, 2)
	require.Len(t, out, 2)
	assert.Equal(t, 1, cursor, "cursor must clamp back into bounds after deleting the last row")
	assert.Equal(t, []string{"one", "two"}, commandsOf(out))
}

func TestDeleteRowMiddlePreservesOrderAndOriginalIndex(t *testing.T) {
	rows := threeRows()
	out, cursor := deleteRow(rows, 1)
	require.Len(t, out, 2)
	assert.Equal(t, 1, cursor)
	assert.Equal(t, []string{"one", "three"}, commandsOf(out))
	assert.Equal(t, 0, out[0].originalIndex)
	assert.Equal(t, 2, out[1].originalIndex, "the surviving row must still report ITS OWN original index, not a re-numbered one")
}

func TestCanEditRowFalseForBlockedRow(t *testing.T) {
	rows := []planReviewRow{
		{step: PlanReviewStep{Command: "rm -rf /", Blocked: true, BlockReason: "denylist"}},
	}
	assert.False(t, canEditRow(rows, 0))
}

func TestCanEditRowTrueForNonBlockedRow(t *testing.T) {
	rows := threeRows()
	assert.True(t, canEditRow(rows, 0))
}

func TestApplyRowEditReplacesOnlyTheCursorRowCommand(t *testing.T) {
	rows := threeRows()
	out := applyRowEdit(rows, 1, "two-edited")
	assert.Equal(t, []string{"one", "two-edited", "three"}, commandsOf(out))
}

func TestBuildOutcomeApprovedCarriesEveryRow(t *testing.T) {
	rows := threeRows()
	rows[1].skipped = true
	outcome := buildOutcome(rows, true)

	require.True(t, outcome.Approved)
	require.Len(t, outcome.Steps, 3)
	assert.Equal(t, ReviewedStep{OriginalIndex: 0, Command: "one", Skipped: false}, outcome.Steps[0])
	assert.Equal(t, ReviewedStep{OriginalIndex: 1, Command: "two", Skipped: true}, outcome.Steps[1])
	assert.Equal(t, ReviewedStep{OriginalIndex: 2, Command: "three", Skipped: false}, outcome.Steps[2])
}

func TestBuildOutcomeCanceledCarriesNoSteps(t *testing.T) {
	outcome := buildOutcome(threeRows(), false)
	assert.False(t, outcome.Approved)
	assert.Empty(t, outcome.Steps)
}

func commandsOf(rows []planReviewRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.step.Command
	}
	return out
}

// --- planReviewModel.Update ----------------------------------------------

func newTestPlanReviewModel() planReviewModel {
	return newPlanReviewModel([]PlanReviewStep{
		{Command: "one", Risk: safety.RiskRead},
		{Command: "two", Risk: safety.RiskWrite},
		{Command: "rm -rf /", Blocked: true, BlockReason: "denylist rule"},
	}, false, trTranslator())
}

func TestPlanReviewModelUpdateDownMovesCursorNotOrder(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, cmd := m.Update(downKey())
	um := updated.(planReviewModel)

	assert.Equal(t, 1, um.cursor)
	assert.Equal(t, []string{"one", "two", "rm -rf /"}, commandsOf(um.rows), "arrow keys must never reorder")
	assert.Nil(t, cmd)
}

func TestPlanReviewModelUpdateUpMovesCursorNotOrder(t *testing.T) {
	m := newTestPlanReviewModel()
	moved, _ := m.Update(downKey())
	mm := moved.(planReviewModel)

	updated, cmd := mm.Update(upKey())
	um := updated.(planReviewModel)

	assert.Equal(t, 0, um.cursor)
	assert.Equal(t, []string{"one", "two", "rm -rf /"}, commandsOf(um.rows), "arrow keys must never reorder")
	assert.Nil(t, cmd)
}

func TestPlanReviewModelUpdateSpaceTogglesSkip(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, _ := m.Update(spaceKey())
	um := updated.(planReviewModel)

	assert.True(t, um.rows[0].skipped)
	assert.False(t, um.rows[1].skipped)
}

func TestPlanReviewModelUpdateTRMoveDownReordersRow(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, _ := m.Update(letterKey('a')) // TR: aşağı taşı (move down)
	um := updated.(planReviewModel)

	assert.Equal(t, []string{"two", "one", "rm -rf /"}, commandsOf(um.rows))
	assert.Equal(t, 1, um.cursor)
}

func TestPlanReviewModelUpdateENMoveDownReordersRow(t *testing.T) {
	m := newPlanReviewModel([]PlanReviewStep{{Command: "one"}, {Command: "two"}}, false, enTranslator())
	updated, _ := m.Update(letterKey('d')) // EN: down
	um := updated.(planReviewModel)

	assert.Equal(t, []string{"two", "one"}, commandsOf(um.rows))
}

func TestPlanReviewModelUpdateENApproveAllQuitsApproved(t *testing.T) {
	m := newPlanReviewModel([]PlanReviewStep{{Command: "one"}}, false, enTranslator())
	updated, cmd := m.Update(letterKey('a')) // EN: all (approve all) — NOT move-down, TR's meaning
	um := updated.(planReviewModel)

	require.True(t, um.done)
	assert.True(t, um.outcome.Approved)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPlanReviewModelUpdateTRApproveAllQuitsApproved(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, cmd := m.Update(letterKey('t')) // TR: tümünü onayla
	um := updated.(planReviewModel)

	require.True(t, um.done)
	assert.True(t, um.outcome.Approved)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPlanReviewModelUpdateEscCancels(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, cmd := m.Update(escKey())
	um := updated.(planReviewModel)

	require.True(t, um.done)
	assert.False(t, um.outcome.Approved)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPlanReviewModelUpdateCtrlCAlwaysCancelsEvenMidEdit(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, _ := m.Update(letterKey('d')) // TR: düzenle — enters edit mode on row 0
	um := updated.(planReviewModel)
	require.True(t, um.editing)

	updated2, cmd := um.Update(ctrlCKey())
	um2 := updated2.(planReviewModel)

	require.True(t, um2.done)
	assert.False(t, um2.outcome.Approved)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestPlanReviewModelUpdateEditRefusedOnBlockedRow(t *testing.T) {
	m := newTestPlanReviewModel()
	m.cursor = 2 // the Blocked "rm -rf /" row

	updated, cmd := m.Update(letterKey('d')) // TR: düzenle
	um := updated.(planReviewModel)

	assert.False(t, um.editing, "editing a Blocked row must be refused")
	assert.Nil(t, cmd)
}

func TestPlanReviewModelUpdateEditThenEnterCommitsEditedCommand(t *testing.T) {
	m := newTestPlanReviewModel()

	updated, _ := m.Update(letterKey('d')) // TR: düzenle on row 0 ("one")
	um := updated.(planReviewModel)
	require.True(t, um.editing)
	assert.Equal(t, "one", um.input.Value(), "textinput must start pre-filled with the current command")

	um.input.SetValue("one-edited")
	updated2, _ := um.Update(enterKey())
	um2 := updated2.(planReviewModel)

	assert.False(t, um2.editing)
	assert.Equal(t, "one-edited", um2.rows[0].step.Command)
}

func TestPlanReviewModelUpdateEditThenEscCancelsBackToBrowsing(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, _ := m.Update(letterKey('d'))
	um := updated.(planReviewModel)
	require.True(t, um.editing)

	updated2, cmd := um.Update(escKey())
	um2 := updated2.(planReviewModel)

	assert.False(t, um2.editing)
	assert.Equal(t, "one", um2.rows[0].step.Command, "esc must discard the in-progress edit")
	assert.Nil(t, cmd)
	assert.False(t, um2.done)
}

func TestPlanReviewModelUpdateDeleteRemovesRow(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, _ := m.Update(letterKey('s')) // TR: sil, on row 0
	um := updated.(planReviewModel)

	assert.Equal(t, []string{"two", "rm -rf /"}, commandsOf(um.rows))
}

func TestPlanReviewModelUpdateUnknownKeyDoesNothing(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, cmd := m.Update(letterKey('z'))
	um := updated.(planReviewModel)

	assert.Equal(t, 0, um.cursor)
	assert.False(t, um.done)
	assert.Nil(t, cmd)
}

// --- View -----------------------------------------------------------------

func TestPlanReviewModelViewShowsBlockedMarkerForBlockedRow(t *testing.T) {
	m := newTestPlanReviewModel()
	view := m.View()

	assert.Contains(t, view.Content, "rm -rf /")
	assert.Contains(t, view.Content, "ENGELLENDİ(denylist rule)")
}

func TestPlanReviewModelViewShowsSkippedMarker(t *testing.T) {
	m := newTestPlanReviewModel()
	updated, _ := m.Update(spaceKey())
	um := updated.(planReviewModel)

	assert.Contains(t, um.View().Content, "[atlandı]")
}

func TestPlanReviewModelViewShowsTRLegend(t *testing.T) {
	m := newTestPlanReviewModel()
	view := m.View()
	assert.Contains(t, view.Content, "[y]ukarı")
	assert.Contains(t, view.Content, "[t]ümünü onayla")
}

func TestPlanReviewModelViewShowsENLegend(t *testing.T) {
	m := newPlanReviewModel([]PlanReviewStep{{Command: "one"}}, false, enTranslator())
	view := m.View()
	assert.Contains(t, view.Content, "[u]p")
	assert.Contains(t, view.Content, "[a]pprove all")
	assert.NotContains(t, view.Content, "[y]ukarı")
}

// --- ReviewPlan (headless end-to-end) --------------------------------------

func TestReviewPlanRunsHeadlessProgramAndReturnsApprovedOutcome(t *testing.T) {
	in := strings.NewReader("t") // TR: tümünü onayla
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	outcome, err := ReviewPlan(ctx, []PlanReviewStep{{Command: "echo hi", Risk: safety.RiskRead}}, false, in, &out, trTranslator())

	require.NoError(t, err)
	require.True(t, outcome.Approved)
	require.Len(t, outcome.Steps, 1)
	assert.Equal(t, "echo hi", outcome.Steps[0].Command)
}

func TestReviewPlanEscCancels(t *testing.T) {
	in := strings.NewReader("\x1b") // esc
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	outcome, err := ReviewPlan(ctx, []PlanReviewStep{{Command: "echo hi"}}, false, in, &out, trTranslator())

	require.NoError(t, err)
	assert.False(t, outcome.Approved)
}

func TestReviewPlanContextCanceledBeforeStartReturnsError(t *testing.T) {
	in, inWriter := stdinPipe()
	defer func() { _ = inWriter.Close() }()
	var out bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := ReviewPlan(ctx, []PlanReviewStep{{Command: "echo hi"}}, false, in, &out, trTranslator())
	assert.Error(t, err)
}
