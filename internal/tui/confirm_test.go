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

	"github.com/firatkutay/cli-comrade/internal/i18n"
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
// sends for a bare "e"/"h"/"d"/"a"/"t"/"y"/"n"/"x" keypress.
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

// trTranslator and enTranslator are the two Translators every table/model
// test below builds a confirmModel with, matching exactly how
// internal/cli's newTranslator resolves general.language in production —
// there is no separate confirm-prompt-specific language path to test.
func trTranslator() i18n.Translator { return i18n.NewTranslator(i18n.LangTR) }
func enTranslator() i18n.Translator { return i18n.NewTranslator(i18n.LangEN) }

// TestMapKeyTRLetters pins mapKey's Turkish key set — unchanged from this
// project's original (pre-i18n) confirm prompt behavior.
func TestMapKeyTRLetters(t *testing.T) {
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
		// EN-only letters must NOT match under TR.
		{"y", 0, false},
		{"n", 0, false},
		{"x", 0, false},
		{"", 0, false},
		{"enter", 0, false},
	}
	for _, tc := range cases {
		choice, ok := mapKey(i18n.LangTR, tc.key)
		assert.Equal(t, tc.wantOK, ok, "key %q", tc.key)
		if tc.wantOK {
			assert.Equal(t, tc.want, choice, "key %q", tc.key)
		}
	}
}

// TestMapKeyENLetters pins mapKey's English key set: y/n/e/x/a. The "e"
// and "a" cases are the load-bearing regression guards for the TR/EN
// inversion hazard this whole change closes — "e" must be Edit (NOT Yes,
// which is what TR's "e" means) and "a" must be All (NOT Explain, which
// is what TR's "a" means). The TR-only letters (h/d/t) must be rejected
// under EN — proving mapKey never falls back to a union of both key sets.
func TestMapKeyENLetters(t *testing.T) {
	cases := []struct {
		key    string
		want   PromptChoice
		wantOK bool
	}{
		{"y", Yes, true},
		{"n", No, true},
		{"e", Edit, true}, // regression guard: NOT Yes (TR's meaning for "e")
		{"x", Explain, true},
		{"a", All, true}, // regression guard: NOT Explain (TR's meaning for "a")
		// TR-only letters must NOT match under EN.
		{"h", 0, false},
		{"d", 0, false},
		{"t", 0, false},
		{"", 0, false},
		{"enter", 0, false},
	}
	for _, tc := range cases {
		choice, ok := mapKey(i18n.LangEN, tc.key)
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
	}, false, trTranslator())
}

func newTestConfirmModelEN() confirmModel {
	return newConfirmModel(PromptStep{
		Command:   "apt-get install docker.io",
		Rationale: "installs docker",
		Risk:      safety.RiskElevated,
	}, false, enTranslator())
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

// TestConfirmModelUpdateENPressEEntersEditNotYes is the Update-level
// regression guard for the TR/EN inversion hazard: under an EN-language
// model, pressing "e" must enter edit mode (EN's Edit key) — it must NOT
// immediately quit with Yes, which is what "e" means under TR.
func TestConfirmModelUpdateENPressEEntersEditNotYes(t *testing.T) {
	m := newTestConfirmModelEN()

	updated, cmd := m.Update(letterKey('e'))
	um := updated.(confirmModel)

	assert.True(t, um.editing, `EN "e" must enter edit mode, not immediately choose Yes`)
	assert.Equal(t, PromptChoice(0), um.chosen, "no terminal choice should be set yet — edit mode waits for enter/esc")
	assert.NotNil(t, cmd) // input.Focus()'s returned Cmd
}

// TestConfirmModelUpdateENPressAQuitsWithAllNotExplain is the Update-level
// regression guard for the other TR/EN inversion: under EN, "a" must
// select All — it must NOT select Explain, which is what "a" means under
// TR.
func TestConfirmModelUpdateENPressAQuitsWithAllNotExplain(t *testing.T) {
	m := newTestConfirmModelEN()

	updated, cmd := m.Update(letterKey('a'))
	um := updated.(confirmModel)

	assert.Equal(t, All, um.chosen)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}

// TestConfirmModelUpdateENTRLettersDoNothing proves EN mode rejects TR's
// h/d/t exactly like an unmatched key — no union of the two key sets.
func TestConfirmModelUpdateENTRLettersDoNothing(t *testing.T) {
	for _, r := range []rune{'h', 'd', 't'} {
		m := newTestConfirmModelEN()
		updated, cmd := m.Update(letterKey(r))
		um := updated.(confirmModel)

		assert.Equal(t, PromptChoice(0), um.chosen, "key %q must not select a choice under EN", string(r))
		assert.False(t, um.editing, "key %q must not enter edit mode under EN", string(r))
		assert.Nil(t, cmd, "key %q must not produce a Cmd under EN", string(r))
	}
}

func TestConfirmModelViewShowsTRLegend(t *testing.T) {
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

// TestConfirmModelViewShowsENLegend proves the rendered legend actually
// switches with the active language — the whole point of this change —
// and does NOT leak any TR wording when EN is active.
func TestConfirmModelViewShowsENLegend(t *testing.T) {
	m := newTestConfirmModelEN()
	view := m.View()

	assert.Contains(t, view.Content, "apt-get install docker.io")
	assert.Contains(t, view.Content, "installs docker")
	assert.Contains(t, view.Content, "[y]es")
	assert.Contains(t, view.Content, "[n]o")
	assert.Contains(t, view.Content, "[e]dit")
	assert.Contains(t, view.Content, "[x]plain")
	assert.Contains(t, view.Content, "[a]ll")

	assert.NotContains(t, view.Content, "[e]vet")
	assert.NotContains(t, view.Content, "[h]ayır")
	assert.NotContains(t, view.Content, "[d]üzenle")
	assert.NotContains(t, view.Content, "[a]çıkla")
	assert.NotContains(t, view.Content, "[t]ümü")
}

func TestConfirmModelViewShowsRiskBadgeUncoloredWhenColorDisabled(t *testing.T) {
	m := newConfirmModel(PromptStep{Command: "rm -rf build", Risk: safety.RiskDestructive}, false, trTranslator())
	view := m.View()
	assert.Contains(t, view.Content, "[destructive]")
}

func TestConfirmModelViewShowsEditPromptWhileEditingTR(t *testing.T) {
	m := newTestConfirmModel()
	m.editing = true

	view := m.View()
	assert.Contains(t, view.Content, "Komutu düzenle")
}

// TestConfirmModelViewShowsEditPromptWhileEditingEN proves the edit-mode
// header also follows the active language, not just the legend.
func TestConfirmModelViewShowsEditPromptWhileEditingEN(t *testing.T) {
	m := newTestConfirmModelEN()
	m.editing = true

	view := m.View()
	assert.Contains(t, view.Content, "Edit command")
	assert.NotContains(t, view.Content, "Komutu düzenle")
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

	choice, edited, err := Confirm(ctx, PromptStep{Command: "echo hi", Risk: safety.RiskRead}, false, in, &out, trTranslator())

	require.NoError(t, err)
	assert.Equal(t, Yes, choice)
	assert.Empty(t, edited)
}

// TestConfirmRunsHeadlessProgramENPressAIsAllNotExplain is the full,
// end-to-end (not just Update()) proof that an EN-language Confirm call
// resolves "a" to All — never Explain, which is what "a" means under TR —
// exercising the real language-to-key-set wiring all the way from
// Confirm's tr parameter through mapKey.
func TestConfirmRunsHeadlessProgramENPressAIsAllNotExplain(t *testing.T) {
	in := strings.NewReader("a")
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	choice, edited, err := Confirm(ctx, PromptStep{Command: "echo hi", Risk: safety.RiskRead}, false, in, &out, enTranslator())

	require.NoError(t, err)
	assert.Equal(t, All, choice)
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

	_, _, err := Confirm(ctx, PromptStep{Command: "echo hi", Risk: safety.RiskRead}, false, in, &out, trTranslator())
	assert.Error(t, err)
}
