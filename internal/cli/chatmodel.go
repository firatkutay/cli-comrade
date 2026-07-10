package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/textinput"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// chatModel is the bubbletea (v2) model backing `comrade chat` — a
// scrollback viewport (charm.land/bubbles/v2/viewport) plus a single-line
// input (charm.land/bubbles/v2/textinput), matching internal/tui/
// confirm.go's v2 style. Every actual decision (slash-command parsing,
// session-state transitions, the chat-turn/"/do" dispatch itself) lives
// in the pure chatController.dispatchChatLine (chatdispatch.go) and its
// helpers — this type only wires that pure logic to bubbletea's Cmd/quit
// protocol and the two visual components, exactly like confirmModel wires
// mapKey.
type chatModel struct {
	ctx        context.Context
	controller *chatController
	session    *chatSession

	viewport viewport.Model
	input    textinput.Model
	spinner  spinner.Model

	// waiting is true from the moment Enter dispatches a line (chat
	// turn or "/do") until its chatTurnDoneMsg comes back. It gates
	// spinner animation/rendering and blocks a second line from being
	// dispatched concurrently — see the "enter" case in Update and
	// runChatTurnCmd's doc comment for why this call is async at all.
	waiting bool

	// ioIn/ioOut are the real streams runChatProgram wired this session's
	// *tea.Program to; "/do"'s real doRunner (newRealChatDoRunner) reads/
	// writes them directly once ReleaseTerminal has handed raw terminal
	// control back, for its own nested confirm prompt.
	ioIn  io.Reader
	ioOut io.Writer

	// program is set by runChatProgram once both the model and the
	// *tea.Program exist, so "/do"'s real doRunner can call
	// ReleaseTerminal/RestoreTerminal around the do-pipeline's own nested
	// bubbletea confirm program (see newRealChatDoRunner's doc comment).
	program *tea.Program

	quitting bool
}

// newChatModel builds a chatModel wired to a real *llm.Client and the
// real runChatDo pipeline (via newRealChatDoRunner) for "/do". ctx and the
// I/O streams/*tea.Program are filled in by runChatProgram once they
// exist (see its own doc comment) — newChatModel itself never touches a
// terminal.
func newChatModel(cfg config.Config, tr i18n.Translator, client *llm.Client, session *chatSession) *chatModel {
	vp := viewport.New(viewport.WithWidth(80), viewport.WithHeight(20))
	vp.SetContent(tr.T(i18n.MsgChatWelcome))

	ti := textinput.New()
	ti.Prompt = "> "
	ti.Focus()

	// waitSpinnerStyle/waitSpinnerFrames' frame set (spinner.go) is reused
	// here verbatim — same braille frames, same pastel-183 color as every
	// other "waiting on the LLM" indicator in this codebase — so the
	// in-bubbletea spinner reads as the same visual language, not a
	// second, independently-drifting one.
	sp := spinner.New(spinner.WithSpinner(waitSpinnerFrames), spinner.WithStyle(waitSpinnerStyle))

	m := &chatModel{session: session, viewport: vp, input: ti, spinner: sp}
	m.controller = &chatController{
		tr:        tr,
		llm:       client,
		save:      saveTranscript,
		maxTokens: cfg.LLM.MaxTokens,
	}
	m.controller.doRun = m.newRealChatDoRunner(cfg, client)
	return m
}

// newRealChatDoRunner returns the chatDoRunner the real bubbletea session
// uses: it releases the running *tea.Program's terminal control before
// runChatDo (chat.go) runs — since runChatDo's own ask-mode confirm
// prompts (internal/tui.Confirm, via tuiPromptUI) spin up ANOTHER,
// independent bubbletea program against the same terminal, which only
// works once this outer program has let go of it — and restores it
// afterward, regardless of runChatDo's outcome. m.program is nil in every
// test that constructs a chatModel without calling runChatProgram, so
// this guards against calling Release/RestoreTerminal on a nil program.
func (m *chatModel) newRealChatDoRunner(cfg config.Config, client *llm.Client) chatDoRunner {
	return func(ctx context.Context, mode engine.Mode, request string) (engine.RunSummary, error) {
		if m.program != nil {
			_ = m.program.ReleaseTerminal()
			defer func() { _ = m.program.RestoreTerminal() }()
		}
		return runChatDo(ctx, cfg, client, mode, request, m.ioIn, m.ioOut, m.ioOut, resolveColorEnabled(cfg, os.Environ(), m.ioOut))
	}
}

func (m chatModel) Init() tea.Cmd { return nil }

// chatTurnDoneMsg carries dispatchChatLine's result back into Update once
// runChatTurnCmd's goroutine finishes — see its doc comment for why this
// round-trip through bubbletea's own Msg/Cmd machinery exists at all
// instead of calling dispatchChatLine inline.
type chatTurnDoneMsg struct {
	output string
	exit   bool
}

// runChatTurnCmd returns the tea.Cmd Update dispatches on Enter: it runs
// controller.dispatchChatLine(ctx, session, line) — the LLM call for a
// plain-text turn, or the entire safety-gated plan+execute pipeline for
// "/do" — on bubbletea's own command goroutine, never inside Update
// itself. Previously this call was made synchronously inline in Update,
// which blocks bubbletea's whole event loop (no render, no spinner, no
// key handling at all — including Ctrl-C) for the call's entire duration:
// up to llm.timeout_seconds (60s by default) with zero visual feedback,
// which is indistinguishable from "the tool hung" to a user watching a
// frozen terminal. Routing the same call through a Cmd instead lets the
// render loop keep animating m.spinner (started alongside this Cmd, see
// Update's "enter" case) while the call is in flight, and keeps Ctrl-C
// responsive throughout.
func runChatTurnCmd(ctx context.Context, controller *chatController, session *chatSession, line string) tea.Cmd {
	return func() tea.Msg {
		output, exit := controller.dispatchChatLine(ctx, session, line)
		return chatTurnDoneMsg{output: output, exit: exit}
	}
}

// Update implements tea.Model. Enter is the one key that does anything
// beyond passthrough to the textinput: while a previous turn is still
// in flight (m.waiting) it is ignored outright — one line dispatched at a
// time, never overlapping requests against the same session.history.
// Otherwise it reads the current input line, clears it, echoes it into
// the viewport, and hands it to runChatTurnCmd, which the render loop
// keeps servicing (including the spinner's own tick messages) until
// chatTurnDoneMsg arrives with the reply/error and, for "/exit"/"/quit",
// the signal to quit.
func (m chatModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.viewport.SetWidth(msg.Width)
		if msg.Height > 3 {
			m.viewport.SetHeight(msg.Height - 3)
		}
		m.input.SetWidth(msg.Width)
		return m, nil

	case tea.KeyPressMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "enter":
			if m.waiting {
				return m, nil
			}
			line := m.input.Value()
			m.input.SetValue("")
			if line == "" {
				return m, nil
			}
			m.appendViewportLine("> " + line)
			m.waiting = true
			m.input.Blur()
			return m, tea.Batch(m.spinner.Tick, runChatTurnCmd(m.ctx, m.controller, m.session, line))
		}

	case chatTurnDoneMsg:
		m.waiting = false
		m.input.Focus()
		if msg.output != "" {
			m.appendViewportLine(msg.output)
		}
		if msg.exit {
			m.quitting = true
			return m, tea.Quit
		}
		return m, nil

	case spinner.TickMsg:
		if !m.waiting {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd
	}

	if m.waiting {
		return m, nil
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// appendViewportLine appends line to the scrollback and scrolls to the
// bottom, so the most recent turn is always what's visible. m is
// addressable (Update's own parameter, or a caller's local variable), so
// this pointer-receiver call is valid even though Update/View/Init use a
// value receiver, matching confirmModel's own value-semantics style.
func (m *chatModel) appendViewportLine(line string) {
	content := m.viewport.GetContent()
	if content != "" {
		content += "\n"
	}
	m.viewport.SetContent(content + line)
	m.viewport.GotoBottom()
}

// View renders the scrollback, an in-flight "thinking…" spinner line
// while m.waiting (see runChatTurnCmd), and the input line — always in
// that order, so the spinner never displaces the scrollback's own last
// line and always sits directly above the input the user is about to
// reuse for their next line.
func (m chatModel) View() tea.View {
	view := m.viewport.View() + "\n"
	if m.waiting {
		view += m.spinner.View() + " " + m.controller.tr.T(i18n.MsgSpinnerThinking) + "\n"
	}
	view += m.input.View()
	return tea.NewView(view)
}

// runChatProgram wires m's ctx and I/O streams, builds the *tea.Program
// (WithContext for Ctrl-C/signal-driven cancellation propagation, exactly
// like `comrade do`/`comrade fix`'s own signal.NotifyContext wiring —
// WithInput/WithOutput so tests never need a real PTY, matching
// internal/tui.Confirm's identical pattern), stores it on m (for "/do"'s
// ReleaseTerminal/RestoreTerminal — see newRealChatDoRunner), and runs it
// to completion.
func runChatProgram(ctx context.Context, m *chatModel, in io.Reader, out io.Writer) error {
	m.ctx = ctx
	m.ioIn = in
	m.ioOut = out

	p := tea.NewProgram(m, tea.WithContext(ctx), tea.WithInput(in), tea.WithOutput(out))
	m.program = p

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("chat: run session: %w", err)
	}
	return nil
}
