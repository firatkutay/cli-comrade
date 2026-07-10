package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

// This file tests chatmodel.go's own bubbletea WIRING — Update/View,
// runChatProgram, the Enter-key dispatch path, and the async
// runChatTurnCmd/chatTurnDoneMsg round trip — as opposed to
// chatdispatch_test.go, which tests chatController.dispatchChatLine's
// pure logic without ever touching a chatModel. Before this file, nothing
// in this codebase ever drove chatModel.Update/View at all: the "no
// response" bug report's mechanics (does Enter actually dispatch a turn?
// does a reply/error ever reach the screen? is there any feedback while
// waiting?) had zero test coverage.

// enterKey/ctrlCKey build the same tea.KeyPressMsg shapes
// internal/tui/confirm_test.go's identical helpers do — a real terminal
// produces the exact same message for these keys regardless of which
// bubbletea model is receiving it.
func enterKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: tea.KeyEnter} }
func ctrlCKey() tea.KeyPressMsg { return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl} }

// newTestChatModel builds a *chatModel wired to a fake chatLLM/chatDoRunner
// (never a real *llm.Client — newChatModel's own signature requires one,
// which is exactly why this file builds a chatModel by hand instead:
// chatLLM/chatDoRunner are chatController's real seams for a fake,
// precisely mirroring newTestController in chatdispatch_test.go), so this
// file tests only the bubbletea plumbing newChatModel itself wires up,
// never re-testing dispatchChatLine's own logic.
func newTestChatModel(llmClient chatLLM, doRun chatDoRunner) *chatModel {
	tr := i18n.NewTranslator(i18n.LangEN)
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SetContent(tr.T(i18n.MsgChatWelcome))
	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()
	sp := spinner.New(spinner.WithSpinner(waitSpinnerFrames), spinner.WithStyle(waitSpinnerStyle))

	m := &chatModel{
		ctx:      context.Background(),
		session:  newChatSession(engine.ModeAsk),
		viewport: vp,
		input:    ti,
		spinner:  sp,
	}
	m.controller = &chatController{tr: tr, llm: llmClient, doRun: doRun, save: saveTranscript, maxTokens: testChatMaxTokens}
	return m
}

// runCmdToMsg executes cmd (as bubbletea's own runtime would, on its
// command goroutine) and returns the tea.Msg it produced — the manual
// stand-in for bubbletea's scheduler this file uses to drive Update
// deterministically, one message at a time, with no goroutine timing
// dependency at all.
func runCmdToMsg(t *testing.T, cmd tea.Cmd) tea.Msg {
	t.Helper()
	require.NotNil(t, cmd, "expected a non-nil Cmd to run")
	return cmd()
}

// --- (a) a chat turn round-trips a mock reply into the transcript --------

// TestChatModelEnterDispatchesTurnAsynchronouslyNotInsideUpdate is the
// direct proof that Enter no longer blocks Update on the LLM call (the
// mechanical half of the "no response" bug: previously chatTurn ran
// synchronously inline in Update, freezing the whole render loop with no
// feedback for the call's entire duration). The fake records a call only
// once runChatTurnCmd's returned Cmd is actually executed — never as a
// side effect of Update itself.
func TestChatModelEnterDispatchesTurnAsynchronouslyNotInsideUpdate(t *testing.T) {
	fake := &fakeChatLLM{reply: "it lists files"}
	m := newTestChatModel(fake, nil)
	m.input.SetValue("what does ls do")

	updated, cmd := m.Update(enterKey())
	m2, ok := updated.(chatModel)
	require.True(t, ok)

	assert.Empty(t, fake.calls, "Update itself must never call the LLM synchronously")
	assert.True(t, m2.waiting, "waiting must be set the instant a turn is dispatched")
	assert.Contains(t, m2.viewport.GetContent(), "> what does ls do", "the user's own line echoes immediately, before the reply arrives")
	assert.Empty(t, m2.input.Value(), "the input line is cleared immediately on submit")

	msg := runCmdToMsg(t, cmd)
	// tea.Batch bundles the spinner tick and the turn dispatch into one
	// tea.Cmd that returns a tea.BatchMsg (a slice of Cmds to run, per
	// bubbletea's own Batch contract) — extract the chatTurnDoneMsg from it.
	done := extractChatTurnDoneMsg(t, msg)
	require.Len(t, fake.calls, 1, "the LLM call happens once the batched Cmd actually runs")
	assert.Equal(t, "it lists files", done.output)
	assert.False(t, done.exit)
}

// extractChatTurnDoneMsg runs every tea.Cmd in msg (a tea.BatchMsg, from
// tea.Batch) until it finds the one that produced a chatTurnDoneMsg —
// tea.Batch's own contract makes no ordering guarantee, so this searches
// rather than assuming a fixed index.
func extractChatTurnDoneMsg(t *testing.T, msg tea.Msg) chatTurnDoneMsg {
	t.Helper()
	batch, ok := msg.(tea.BatchMsg)
	require.True(t, ok, "expected tea.Batch's BatchMsg from a batched Enter dispatch")
	for _, cmd := range batch {
		if cmd == nil {
			continue
		}
		if done, ok := cmd().(chatTurnDoneMsg); ok {
			return done
		}
	}
	t.Fatal("no chatTurnDoneMsg produced by the batched commands")
	return chatTurnDoneMsg{}
}

// TestChatModelChatTurnDoneMsgAppendsReplyAndClearsWaiting completes the
// round trip TestChatModelEnterDispatchesTurnAsynchronouslyNotInsideUpdate
// starts: once chatTurnDoneMsg reaches Update, the reply must land in the
// transcript and waiting must clear.
func TestChatModelChatTurnDoneMsgAppendsReplyAndClearsWaiting(t *testing.T) {
	m := newTestChatModel(&fakeChatLLM{}, nil)
	m.waiting = true
	m.input.Blur()

	updated, cmd := m.Update(chatTurnDoneMsg{output: "it lists files", exit: false})
	m2, ok := updated.(chatModel)
	require.True(t, ok)

	assert.Nil(t, cmd)
	assert.False(t, m2.waiting, "waiting must clear once the reply arrives")
	assert.Contains(t, m2.viewport.GetContent(), "it lists files")
	assert.True(t, m2.input.Focused(), "the input must be refocused once the turn completes")
}

// runHeadlessChatProgram builds and runs the exact same *tea.Program
// construction runChatProgram (chatmodel.go) uses — WithContext/WithInput/
// WithOutput, m.program wired for "/do"'s ReleaseTerminal — but, unlike
// runChatProgram itself, keeps the final Model p.Run() returns instead of
// discarding it (runChatProgram discards it because nothing in production
// ever reads chat state after the session quits; internal/tui.Confirm's
// own Run() call is the precedent for capturing it when a caller does need
// to, per its doc comment). in is an *io.PipeReader so the caller can
// control exactly when bytes become available, rather than handing the
// program a fixed byte string up front — required to sequence a second
// keypress after an async turn has genuinely completed instead of racing
// it.
func runHeadlessChatProgram(ctx context.Context, t *testing.T, m *chatModel, in io.Reader, out io.Writer) (*tea.Program, <-chan tea.Model, <-chan error) {
	t.Helper()
	m.ctx = ctx
	m.ioIn = in
	m.ioOut = out
	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out))
	m.program = p

	modelCh := make(chan tea.Model, 1)
	errCh := make(chan error, 1)
	go func() {
		fm, err := p.Run()
		modelCh <- fm
		errCh <- err
	}()
	return p, modelCh, errCh
}

// quitOnceTurnSettles sends p.Quit() repeatedly (a harmless no-op once
// already queued/processed — see tea.Program.Quit's own doc comment) until
// the program actually exits, starting only once fake.calls confirms the
// real, full pipeline actually dispatched the LLM call. This is the
// deterministic way to end a headless-program test of the ASYNC turn path
// without racing raw input bytes (e.g. a second "/exit\r") against
// chatTurnDoneMsg: Quit and every command's result message flow through
// the exact same p.Send-backed internal channel (bubbletea's own
// handleCommands, tea.go), so once chatTurnDoneMsg is confirmed generated,
// repeatedly nudging Quit converges on "after the reply was processed"
// rather than risking "before".
func quitOnceTurnSettles(t *testing.T, p *tea.Program, fake *fakeChatLLM, modelCh <-chan tea.Model, errCh <-chan error) (tea.Model, error) {
	t.Helper()
	require.Eventually(t, func() bool { return fake.callCount() == 1 }, 2*time.Second, 5*time.Millisecond,
		"the real program must have actually dispatched the LLM call")

	deadline := time.After(5 * time.Second)
	ticker := time.NewTicker(20 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case fm := <-modelCh:
			return fm, <-errCh
		case <-ticker.C:
			p.Quit()
		case <-deadline:
			t.Fatal("program did not quit in time")
			return nil, nil
		}
	}
}

// TestChatModelHeadlessProgramRoundTripsReplyIntoTranscript drives the
// full, real *tea.Program end-to-end (the exact construction
// runChatProgram uses) headlessly — same technique as internal/tui/
// confirm_test.go's TestConfirmRunsHeadlessProgramAndReturnsChoice:
// WithInput/WithOutput redirect to in-memory buffers, so this needs no
// TTY/PTY and still exercises the exact same input-parsing/async-dispatch/
// render path a real terminal session would. This is the closest thing to
// literally reproducing "type a message in `comrade chat`, press Enter"
// available without spending a real API call.
func TestChatModelHeadlessProgramRoundTripsReplyIntoTranscript(t *testing.T) {
	fake := &fakeChatLLM{reply: "it lists files"}
	m := newTestChatModel(fake, nil)

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
	assert.Contains(t, final.viewport.GetContent(), "what does ls do", "the user's own line must have been rendered")
	assert.Contains(t, final.viewport.GetContent(), "it lists files", "the reply must have reached the transcript, not been swallowed")
}

// TestChatModelEnterDispatchesDoCommandAsynchronouslyToo proves "/do"
// goes through the exact same async runChatTurnCmd/chatTurnDoneMsg path
// as a plain-text turn — dispatchChatLine (chatdispatch.go) routes both
// through the same pure function, and chatModel.Update never
// distinguishes between them — so "/do"'s whole safety-gated plan+execute
// pipeline gets the same in-flight spinner and never-blocks-Ctrl-C
// treatment a plain-text turn does, not a second, parallel mechanism.
func TestChatModelEnterDispatchesDoCommandAsynchronouslyToo(t *testing.T) {
	doRun := func(context.Context, engine.Mode, string) (engine.RunSummary, error) {
		return engine.RunSummary{Results: []engine.StepResult{{Outcome: engine.OutcomeExecuted}}}, nil
	}
	m := newTestChatModel(&fakeChatLLM{}, doRun)
	m.input.SetValue("/do install docker")

	updated, cmd := m.Update(enterKey())
	m2, ok := updated.(chatModel)
	require.True(t, ok)
	assert.True(t, m2.waiting, "\"/do\" must also set waiting — it is dispatched through the same async Cmd")

	done := extractChatTurnDoneMsg(t, runCmdToMsg(t, cmd))
	assert.Contains(t, done.output, "1 executed, 0 skipped, 0 blocked")
	assert.False(t, done.exit)
}

// --- (b) an LLM error renders a visible i18n'd error line -----------------

// TestChatModelChatTurnDoneMsgRendersLLMErrorNotSilence pins the "output
// must never be silently empty" contract end to end through Update: a
// failed turn's dispatchChatLine output (chatdispatch.go's
// dc.tr.T(i18n.MsgChatLLMError, err), never "") must reach the transcript.
func TestChatModelChatTurnDoneMsgRendersLLMErrorNotSilence(t *testing.T) {
	fake := &fakeChatLLM{err: errors.New("network down")}
	m := newTestChatModel(fake, nil)
	m.input.SetValue("hello")

	_, cmd := m.Update(enterKey())
	done := extractChatTurnDoneMsg(t, runCmdToMsg(t, cmd))
	require.NotEmpty(t, done.output, "an LLM failure must produce a visible error line, never an empty output")
	assert.Contains(t, done.output, "network down")

	updated, _ := m.Update(done)
	m2, ok := updated.(chatModel)
	require.True(t, ok)
	assert.Contains(t, m2.viewport.GetContent(), "network down")
	assert.False(t, m2.waiting)
}

// TestChatModelHeadlessProgramRendersLLMErrorInTranscript is
// TestChatModelChatTurnDoneMsgRendersLLMErrorNotSilence's full end-to-end
// counterpart: an LLM failure must reach the real, running program's
// transcript, not just a directly-driven Update() call's return value.
func TestChatModelHeadlessProgramRendersLLMErrorInTranscript(t *testing.T) {
	fake := &fakeChatLLM{err: errors.New("network down")}
	m := newTestChatModel(fake, nil)

	inR, inW := io.Pipe()
	defer func() { _ = inW.Close() }()
	var out bytes.Buffer

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	p, modelCh, errCh := runHeadlessChatProgram(ctx, t, m, inR, &out)

	_, writeErr := inW.Write([]byte("hello\r"))
	require.NoError(t, writeErr)

	fm, runErr := quitOnceTurnSettles(t, p, fake, modelCh, errCh)
	require.NoError(t, runErr)

	final, ok := fm.(chatModel)
	require.True(t, ok)
	assert.Contains(t, final.viewport.GetContent(), "network down", "a failed turn must never render as total silence")
}

// --- (d) an in-flight indicator appears while waiting, disappears after -

// TestChatModelViewShowsSpinnerWhileWaitingAndHidesItOnceDone is the
// direct, timing-independent proof of the in-flight indicator's whole
// contract: invisible before a turn starts, visible the instant Enter
// dispatches one (this is exactly the window a real ~60s-timeout LLM call
// would occupy — proven without actually waiting any real time, since
// this drives Update/View directly rather than racing a goroutine), and
// gone again once chatTurnDoneMsg arrives.
func TestChatModelViewShowsSpinnerWhileWaitingAndHidesItOnceDone(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	thinking := tr.T(i18n.MsgSpinnerThinking)

	m := newTestChatModel(&fakeChatLLM{reply: "ok"}, nil)
	assert.NotContains(t, m.View().Content, thinking, "no spinner before any turn was ever dispatched")

	m.input.SetValue("hello")
	updated, _ := m.Update(enterKey())
	m2, ok := updated.(chatModel)
	require.True(t, ok)

	assert.Contains(t, m2.View().Content, thinking, "the spinner must be visible the instant a turn is in flight")

	updated2, _ := m2.Update(chatTurnDoneMsg{output: "ok"})
	m3, ok := updated2.(chatModel)
	require.True(t, ok)
	assert.NotContains(t, m3.View().Content, thinking, "the spinner must disappear once the reply arrives")
}

// TestChatModelViewHidesSpinnerAfterLLMErrorToo mirrors the success case
// above for the failure path — the spinner must never survive an errored
// turn either.
func TestChatModelViewHidesSpinnerAfterLLMErrorToo(t *testing.T) {
	tr := i18n.NewTranslator(i18n.LangEN)
	thinking := tr.T(i18n.MsgSpinnerThinking)

	m := newTestChatModel(&fakeChatLLM{}, nil)
	m.waiting = true
	require.Contains(t, m.View().Content, thinking)

	updated, _ := m.Update(chatTurnDoneMsg{output: tr.T(i18n.MsgChatLLMError, errors.New("boom"))})
	m2, ok := updated.(chatModel)
	require.True(t, ok)
	assert.NotContains(t, m2.View().Content, thinking)
	assert.Contains(t, m2.viewport.GetContent(), "boom")
}

// --- Enter is ignored while a turn is already in flight -------------------

// TestChatModelEnterIsIgnoredWhileWaiting proves a second Enter press
// cannot dispatch an overlapping turn against the same session.history
// while one is already in flight.
func TestChatModelEnterIsIgnoredWhileWaiting(t *testing.T) {
	fake := &fakeChatLLM{reply: "ok"}
	m := newTestChatModel(fake, nil)
	m.waiting = true
	m.input.SetValue("second message")

	updated, cmd := m.Update(enterKey())
	m2, ok := updated.(chatModel)
	require.True(t, ok)

	assert.Nil(t, cmd, "Enter must be a no-op while a turn is already in flight")
	assert.True(t, m2.waiting)
	assert.Equal(t, "second message", m2.input.Value(), "the in-progress input line must be left untouched")
}

// --- Ctrl-C still quits even mid-turn --------------------------------------

// TestChatModelCtrlCQuitsEvenWhileWaiting proves Ctrl-C is never blocked
// by an in-flight turn — runChatTurnCmd.go's doc comment on why the LLM
// call moved off Update's own goroutine.
func TestChatModelCtrlCQuitsEvenWhileWaiting(t *testing.T) {
	m := newTestChatModel(&fakeChatLLM{}, nil)
	m.waiting = true

	updated, cmd := m.Update(ctrlCKey())
	m2, ok := updated.(chatModel)
	require.True(t, ok)

	assert.True(t, m2.quitting)
	require.NotNil(t, cmd)
	assert.IsType(t, tea.QuitMsg{}, cmd())
}
