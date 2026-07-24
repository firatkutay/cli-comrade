package cli

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// newTestChatModelColored is newTestChatModel with colorEnabled forced on
// and the input's own prompt style set to match — newTestChatModel builds
// its textinput.Model by hand (not via newChatModel), so it never calls
// setChatInputPromptStyle itself; this replicates that one call, exactly
// like newChatModel does, for tests that assert on the colorized
// rendering path specifically. No doRun parameter: none of this file's
// tests exercise "/do" (newTestChatModel's own doRun-exercising case
// already covers that, uncolored — colorizing is orthogonal to which
// chatDoRunner is wired), so every call site here would only ever pass
// nil.
func newTestChatModelColored(llmClient chatLLM) *chatModel {
	m := newTestChatModel(llmClient, nil)
	m.colorEnabled = true
	setChatInputPromptStyle(&m.input, true)
	return m
}

// --- styleEchoedUserLine (item 1: user's own transcript messages, item 4: "> ") ---

// TestStyleEchoedUserLinePlainWhenColorDisabled proves the byte-identical-
// when-disabled contract for the transcript's own echoed user line: exactly
// "> " + line, no ANSI at all — matching this view's behavior before any
// of this task's styling existed.
func TestStyleEchoedUserLinePlainWhenColorDisabled(t *testing.T) {
	m := newTestChatModel(&fakeChatLLM{}, nil)
	got := m.styleEchoedUserLine("what does ls do")
	assert.Equal(t, "> what does ls do", got)
}

// TestStyleEchoedUserLineColoredWhenEnabled pins the exact ANSI output
// when color is enabled: a pastel-yellow "> " prompt-symbol prefix
// (matching the live input's own prompt color) followed by the message
// text in a muted pastel gray — two independently-styled-and-reset
// segments, not one combined style.
func TestStyleEchoedUserLineColoredWhenEnabled(t *testing.T) {
	m := newTestChatModelColored(&fakeChatLLM{})
	got := m.styleEchoedUserLine("what does ls do")
	want := "\x1b[38;5;222m> \x1b[m\x1b[38;5;245mwhat does ls do\x1b[m"
	assert.Equal(t, want, got)
}

// --- colorizeSlashCommandList / styleChatOutput (item 3: commands) --------

// TestColorizeSlashCommandListColorsEachLeadingToken pins
// colorizeSlashCommandList's exact output against the REAL EN
// MsgChatHelp catalog text — every one of its six rows' leading "/xxx"
// token wrapped in the pastel-blue command style, nothing else in any
// row touched (the usage-syntax placeholders, descriptions, and the
// "Slash commands:" header line all pass through byte-for-byte).
func TestColorizeSlashCommandListColorsEachLeadingToken(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	input := tr.T(i18n.MsgChatHelp)

	got := colorizeSlashCommandList(input)

	blue := func(s string) string { return "\x1b[38;5;111m" + s + "\x1b[m" }
	want := "Slash commands:\n" +
		"  " + blue("/mode") + " auto|ask|info   switch this session's active mode\n" +
		"  " + blue("/do") + " <request>         run a request through the plan+execute pipeline (safety-gated, per the active mode)\n" +
		"  " + blue("/clear") + "                reset the conversation history\n" +
		"  " + blue("/save") + " <file>          export the transcript to <file> (the only way this session is ever written to disk)\n" +
		"  " + blue("/help") + "                 show this help\n" +
		"  " + blue("/exit") + "                 end the session"
	assert.Equal(t, want, got)
}

// TestColorizeSlashCommandListInTurkishColorsEachLeadingToken is the same
// proof against the TR catalog text, since MsgChatHelp's row SHAPE (two
// spaces + "/xxx" token + space) is identical in both languages even
// though the descriptions themselves are translated.
func TestColorizeSlashCommandListInTurkishColorsEachLeadingToken(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangTR)
	input := tr.T(i18n.MsgChatHelp)

	got := colorizeSlashCommandList(input)

	for _, token := range []string{"/mode", "/do", "/clear", "/save", "/help", "/exit"} {
		assert.Contains(t, got, "\x1b[38;5;111m"+token+"\x1b[m")
	}
}

// TestStyleChatOutputColorsExactHelpTextWhenEnabled proves styleChatOutput
// recognizes "/help"'s own output specifically (an exact match against
// tr.T(i18n.MsgChatHelp)) and colorizes it when color is enabled.
func TestStyleChatOutputColorsExactHelpTextWhenEnabled(t *testing.T) {
	m := newTestChatModelColored(&fakeChatLLM{})
	helpText := m.controller.tr.T(i18n.MsgChatHelp)

	got := m.styleChatOutput(helpText)

	assert.Contains(t, got, "\x1b[38;5;111m/mode\x1b[m")
	assert.NotEqual(t, helpText, got, "the colorized help text must differ from the plain catalog string")
}

// TestStyleChatOutputLeavesHelpTextPlainWhenDisabled is the disabled-path
// counterpart: even though it IS the recognized "/help" text, colorEnabled
// false means zero bytes change.
func TestStyleChatOutputLeavesHelpTextPlainWhenDisabled(t *testing.T) {
	m := newTestChatModel(&fakeChatLLM{}, nil)
	helpText := m.controller.tr.T(i18n.MsgChatHelp)

	got := m.styleChatOutput(helpText)

	assert.Equal(t, helpText, got)
}

// TestStyleChatOutputNeverTouchesNonHelpOutputEvenWhenColorEnabled is the
// critical "assistant replies (and every other dispatch output) stay
// completely untouched" regression guard: styleChatOutput must be a pure
// passthrough for anything that is NOT an exact match for "/help"'s own
// text, regardless of colorEnabled — a chat-turn reply, a "/do" summary,
// a "/mode changed" confirmation, an error message, and (importantly) any
// text that merely CONTAINS something that looks like a slash command
// (e.g. quoting "/mode" inside a sentence) must all pass through
// byte-for-byte identical.
func TestStyleChatOutputNeverTouchesNonHelpOutputEvenWhenColorEnabled(t *testing.T) {
	m := newTestChatModelColored(&fakeChatLLM{})

	cases := []string{
		"ls lists files in the current directory",
		"mode set to auto",
		"2 executed, 0 skipped, 0 blocked",
		"You can type /mode to switch modes, by the way.",
		"",
	}
	for _, output := range cases {
		got := m.styleChatOutput(output)
		assert.Equal(t, output, got, "non-/help output must never be touched, got: %q", got)
	}
}

// --- setChatInputPromptStyle (item 4: input prompt symbol) ----------------

// TestSetChatInputPromptStyleDisabledIsFullyPlain is the regression guard
// for a genuine PRE-EXISTING gap this task's own investigation found:
// bubbles/v2/textinput.New()'s own default styles (DefaultDarkStyles())
// set the prompt's color UNCONDITIONALLY (Foreground(Color("7")), i.e.
// ANSI "white") with no NO_COLOR/TTY/colorEnabled awareness at all —
// confirmed empirically (textinput.New() + Focus() + View() emits
// "\x1b[37m> \x1b[m..." with zero gating). setChatInputPromptStyle(ti,
// false) must leave the rendered prompt with ZERO ANSI escape bytes,
// closing that gap, not merely "not yellow yet still colored 7".
func TestSetChatInputPromptStyleDisabledIsFullyPlain(t *testing.T) {
	m := newTestChatModel(&fakeChatLLM{}, nil)
	setChatInputPromptStyle(&m.input, false)
	m.input.Focus()
	m.input.SetVirtualCursor(false) // exclude the cursor's OWN, separate, out-of-scope styling from this assertion

	view := m.input.View()

	assert.NotContains(t, view, "\x1b[", "the prompt must render with zero ANSI escape bytes when color is disabled")
	assert.Contains(t, view, "> ")
}

// TestSetChatInputPromptStyleEnabledUsesPastelYellow proves the enabled
// path applies the pastel-yellow prompt color, in both the Focused and
// Blurred style states (Update toggles between them on every Enter/reply
// round-trip).
func TestSetChatInputPromptStyleEnabledUsesPastelYellow(t *testing.T) {
	m := newTestChatModel(&fakeChatLLM{}, nil)
	setChatInputPromptStyle(&m.input, true)

	got := m.input.Styles()

	assert.Equal(t, "\x1b[38;5;222mx\x1b[m", got.Focused.Prompt.Render("x"))
	assert.Equal(t, "\x1b[38;5;222mx\x1b[m", got.Blurred.Prompt.Render("x"))
}

// --- end-to-end: newChatModel actually wires colorEnabled through -------

// TestNewChatModelWiresColorEnabledIntoPromptStyle is the constructor-level
// proof that newChatModel itself (not just the two helper functions in
// isolation) actually calls setChatInputPromptStyle with the colorEnabled
// value it was given.
func TestNewChatModelWiresColorEnabledIntoPromptStyle(t *testing.T) {
	cfg := config.Config{}
	tr := i18n.NewTranslator(i18n.LangEN)

	colored := newChatModel(cfg, tr, nil, newChatSession(engine.ModeAsk), true, false, nil, nil)
	require.Equal(t, true, colored.colorEnabled)
	assert.Equal(t, "\x1b[38;5;222mx\x1b[m", colored.input.Styles().Focused.Prompt.Render("x"))

	plain := newChatModel(cfg, tr, nil, newChatSession(engine.ModeAsk), false, false, nil, nil)
	require.Equal(t, false, plain.colorEnabled)
	assert.Equal(t, "x", plain.input.Styles().Focused.Prompt.Render("x"))
}

// TestChatModelHeadlessProgramColorsEchoedUserLineButNotReply is the
// full end-to-end proof, through the real headless bubbletea Program
// (exactly like TestChatModelHeadlessProgramRoundTripsReplyIntoTranscript,
// with color forced on): the user's own echoed line reaches the
// transcript wrapped in the exact pastel prompt/message ANSI, while the
// assistant's reply lands completely unstyled, byte-for-byte — proving
// the "assistant replies stay untouched" scope decision holds even
// through the real bubbletea round trip, not just in styleChatOutput's
// own unit tests.
func TestChatModelHeadlessProgramColorsEchoedUserLineButNotReply(t *testing.T) {
	fake := &fakeChatLLM{reply: "it lists files"}
	m := newTestChatModelColored(fake)

	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p, modelCh, errCh := runHeadlessChatProgram(ctx, t, m, inR, &out)

	_, writeErr := inW.Write([]byte("what does ls do\r"))
	require.NoError(t, writeErr)

	fm, runErr := quitOnceTurnSettles(t, p, fake, modelCh, errCh)
	require.NoError(t, runErr)

	final, ok := fm.(chatModel)
	require.True(t, ok)
	content := final.viewport.GetContent()
	assert.Contains(t, content, "\x1b[38;5;222m> \x1b[m\x1b[38;5;245mwhat does ls do\x1b[m",
		"the echoed user line must carry the exact pastel prompt+message ANSI")
	assert.Contains(t, content, "it lists files", "the reply must still reach the transcript")
	assert.NotContains(t, content, "\x1b[38;5;245mit lists files",
		"the assistant reply must NEVER be wrapped in the user-message gray style")
	assert.NotContains(t, content, "\x1b[38;5;111m",
		"no command-blue ANSI is expected anywhere in this turn (not a /help exchange)")
}

// TestChatModelHeadlessProgramColorsSlashCommandsInHelpOutput is the
// end-to-end proof for item 3 (commands → pastel blue) through the real
// bubbletea round trip: typing "/help" produces a transcript entry with
// every one of its six slash-command tokens wrapped in the command-blue
// style.
func TestChatModelHeadlessProgramColorsSlashCommandsInHelpOutput(t *testing.T) {
	fake := &fakeChatLLM{}
	m := newTestChatModelColored(fake)

	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p, modelCh, errCh := runHeadlessChatProgram(ctx, t, m, inR, &out)

	_, writeErr := inW.Write([]byte("/help\r"))
	require.NoError(t, writeErr)

	// "/help" never calls dc.llm at all (dispatchChatLine's chatCmdHelp
	// case answers synchronously within its own Cmd goroutine, no
	// network) — so this can't reuse quitOnceTurnSettles's
	// fake.callCount()==1 gate the LLM-turn tests wait on; it just polls
	// p.Quit() on a short ticker until the settled model comes back.
	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	var fm tea.Model
	var runErr error
loop:
	for {
		select {
		case fm = <-modelCh:
			runErr = <-errCh
			break loop
		case <-ticker.C:
			p.Quit()
		case <-deadline:
			t.Fatal("program did not quit in time")
		}
	}
	require.NoError(t, runErr)

	final, ok := fm.(chatModel)
	require.True(t, ok)
	content := final.viewport.GetContent()
	for _, token := range []string{"/mode", "/do", "/clear", "/save", "/help", "/exit"} {
		assert.Contains(t, content, "\x1b[38;5;111m"+token+"\x1b[m")
	}
}
