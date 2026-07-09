package cli

import (
	"context"
	"fmt"
	"io"

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

	m := &chatModel{session: session, viewport: vp, input: ti}
	m.controller = &chatController{
		tr:   tr,
		llm:  client,
		save: saveTranscript,
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
		return runChatDo(ctx, cfg, client, mode, request, m.ioIn, m.ioOut, m.ioOut, cfg.General.Color)
	}
}

func (m chatModel) Init() tea.Cmd { return nil }

// Update implements tea.Model. Exactly one key ever does anything beyond
// passthrough to the textinput: Enter, which reads the current input
// line, clears it, echoes it into the viewport, and hands it to
// chatController.dispatchChatLine — the pure dispatch logic — appending
// its reply and quitting when dispatchChatLine says to ("/exit"/"/quit").
// This blocks the bubbletea event loop for the duration of an LLM call or
// a "/do" run; see docs/phases/FAZ-09.md for why that tradeoff was made
// (a real terminal is released to the do-pipeline during "/do" anyway, so
// nothing else could animate concurrently in that case regardless).
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
			line := m.input.Value()
			m.input.SetValue("")
			if line == "" {
				return m, nil
			}
			m.appendViewportLine("> " + line)
			output, exit := m.controller.dispatchChatLine(m.ctx, m.session, line)
			if output != "" {
				m.appendViewportLine(output)
			}
			if exit {
				m.quitting = true
				return m, tea.Quit
			}
			return m, nil
		}
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

func (m chatModel) View() tea.View {
	return tea.NewView(m.viewport.View() + "\n" + m.input.View())
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
