package tui

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/safety"
)

// stdinPipe returns an io.Pipe's read end (for Confirm's "in" parameter)
// paired with its write end, so a test can hold the read side open
// indefinitely (simulating "the user hasn't pressed a key yet") without
// Confirm's underlying bubbletea Program observing an immediate EOF.
func stdinPipe() (io.Reader, io.WriteCloser) {
	return io.Pipe()
}

// letterKey builds the tea.KeyPressMsg a plain, unmodified letter keypress
// produces: Text is non-empty (so Key.String() returns it verbatim — see
// this package's confirm.go doc comment and the ultraviolet Key.String()
// implementation it delegates to), matching exactly what a real terminal
// sends for a bare "e"/"h"/"d"/"a"/"t" keypress.
func letterKey(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: string(r), Code: r}
}

func ctrlCKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}
}

func enterKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEnter}
}

func escKey() tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: tea.KeyEscape}
}

func TestMapKeyMatchesExactTRLetters(t *testing.T) {
	cases := []struct {
		key    string
		want   PromptChoice
		wantOK bool
	}{
		{"e", Yes, true},
		{"h", No, true},
		{"d", Edit, true},
		{"a", Explain, true},
		{"t", All, true},
		{"y", 0, false},
		{"n", 0, false},
		{"", 0, false},
		{"enter", 0, false},
	}
	for _, tc := range cases {
		choice, ok := mapKey(tc.key)
		assert.Equal(t, tc.wantOK, ok, "key %q", tc.key)
		if tc.wantOK {
			assert.Equal(t, tc.want, choice, "key %q", tc.key)
		}
	}
}

func TestPromptChoiceStringNames(t *testing.T) {
	assert.Equal(t, "yes", Yes.String())
	assert.Equal(t, "no", No.String())
	assert.Equal(t, "edit", Edit.String())
	assert.Equal(t, "explain", Explain.String())
	assert.Equal(t, "all", All.String())
	assert.Equal(t, "unknown", PromptChoice(99).String())
}

func newTestConfirmModel() confirmModel {
	return newConfirmModel(PromptStep{
		Command:   "apt-get install docker.io",
		Rationale: "installs docker",
		Risk:      safety.RiskElevated,
	}, false)
}

func TestConfirmModelUpdateYesQuits(t *testing.T) {
	m := newTestConfirmModel()

	updated, cmd := m.Update(letterKey('e'))
	um := updated.(confirmModel)

	assert.Equal(t, Yes, um.chosen)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestConfirmModelUpdateNoQuits(t *testing.T) {
	m := newTestConfirmModel()

	updated, cmd := m.Update(letterKey('h'))
	um := updated.(confirmModel)

	assert.Equal(t, No, um.chosen)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestConfirmModelUpdateExplainQuitsWithExplainChoice(t *testing.T) {
	m := newTestConfirmModel()

	updated, cmd := m.Update(letterKey('a'))
	um := updated.(confirmModel)

	assert.Equal(t, Explain, um.chosen)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestConfirmModelUpdateAllQuitsWithAllChoice(t *testing.T) {
	m := newTestConfirmModel()

	updated, cmd := m.Update(letterKey('t'))
	um := updated.(confirmModel)

	assert.Equal(t, All, um.chosen)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestConfirmModelUpdateUnknownKeyDoesNothing(t *testing.T) {
	m := newTestConfirmModel()

	updated, cmd := m.Update(letterKey('z'))
	um := updated.(confirmModel)

	assert.Equal(t, PromptChoice(0), um.chosen, "no choice should be set yet")
	assert.False(t, um.editing)
	assert.Nil(t, cmd)
}

func TestConfirmModelUpdateCtrlCAlwaysQuitsAsNo(t *testing.T) {
	m := newTestConfirmModel()
	m.editing = true // even mid-edit, ctrl+c must abort as No

	updated, cmd := m.Update(ctrlCKey())
	um := updated.(confirmModel)

	assert.Equal(t, No, um.chosen)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestConfirmModelUpdateEditEntersEditingMode(t *testing.T) {
	m := newTestConfirmModel()

	updated, _ := m.Update(letterKey('d'))
	um := updated.(confirmModel)

	assert.True(t, um.editing)
	assert.Equal(t, "apt-get install docker.io", um.input.Value(), "the textinput must start pre-filled with the original command")
}

func TestConfirmModelUpdateEditThenEnterReturnsEditedCommand(t *testing.T) {
	m := newTestConfirmModel()

	updated, _ := m.Update(letterKey('d'))
	um := updated.(confirmModel)
	require.True(t, um.editing)

	// Simulate the user replacing the whole value, then pressing enter.
	um.input.SetValue("apt-get install nginx")
	updated2, cmd := um.Update(enterKey())
	um2 := updated2.(confirmModel)

	assert.Equal(t, Edit, um2.chosen)
	assert.Equal(t, "apt-get install nginx", um2.editedCommand)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

func TestConfirmModelUpdateEditThenEscCancelsBackToConfirming(t *testing.T) {
	m := newTestConfirmModel()

	updated, _ := m.Update(letterKey('d'))
	um := updated.(confirmModel)
	require.True(t, um.editing)

	updated2, cmd := um.Update(escKey())
	um2 := updated2.(confirmModel)

	assert.False(t, um2.editing)
	assert.Nil(t, cmd)
	assert.Equal(t, PromptChoice(0), um2.chosen)
}

func TestConfirmModelViewShowsCommandRationaleAndOptions(t *testing.T) {
	m := newTestConfirmModel()
	view := m.View()

	assert.Contains(t, view.Content, "apt-get install docker.io")
	assert.Contains(t, view.Content, "installs docker")
	assert.Contains(t, view.Content, "[e]vet")
	assert.Contains(t, view.Content, "[h]ayır")
	assert.Contains(t, view.Content, "[d]üzenle")
	assert.Contains(t, view.Content, "[a]çıkla")
	assert.Contains(t, view.Content, "[t]ümü")
}

func TestConfirmModelViewShowsRiskBadgeUncoloredWhenColorDisabled(t *testing.T) {
	m := newConfirmModel(PromptStep{Command: "rm -rf build", Risk: safety.RiskDestructive}, false)
	view := m.View()
	assert.Contains(t, view.Content, "[destructive]")
}

func TestConfirmModelViewShowsEditPromptWhileEditing(t *testing.T) {
	m := newTestConfirmModel()
	m.editing = true

	view := m.View()
	assert.Contains(t, view.Content, "Edit command")
}

// TestConfirmRunsHeadlessProgramAndReturnsChoice drives the full
// bubbletea Program (not just Update()) end-to-end, but entirely
// headlessly: tea.WithInput/tea.WithOutput redirect the program to a
// plain in-memory io.Reader/io.Writer instead of a real terminal/PTY, so
// this still runs fine under `go test` in CI with no TTY at all. Feeding
// the raw byte for a plain, unmodified letter keystroke is exactly what a
// real terminal sends for that key (no escape sequence involved), so the
// program's own input parser turns it into the same tea.KeyPressMsg
// confirm_test.go's other cases construct by hand.
func TestConfirmRunsHeadlessProgramAndReturnsChoice(t *testing.T) {
	in := strings.NewReader("e")
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	choice, edited, err := Confirm(ctx, PromptStep{Command: "echo hi", Risk: safety.RiskRead}, false, in, &out)

	require.NoError(t, err)
	assert.Equal(t, Yes, choice)
	assert.Empty(t, edited)
}

// TestConfirmContextCanceledBeforeStartReturnsError proves Confirm does
// not hang and surfaces ctx's cancellation rather than blocking forever
// waiting for a keypress that will never come (the Ctrl-C-mid-prompt
// scenario internal/engine's Runner relies on).
func TestConfirmContextCanceledBeforeStartReturnsError(t *testing.T) {
	in, inWriter := stdinPipe()
	defer func() { _ = inWriter.Close() }()
	var out bytes.Buffer

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, _, err := Confirm(ctx, PromptStep{Command: "echo hi", Risk: safety.RiskRead}, false, in, &out)
	assert.Error(t, err)
}
