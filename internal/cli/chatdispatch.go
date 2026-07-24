package cli

import (
	"context"
	"fmt"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// chatDoRunner runs one "/do <request>" invocation end-to-end (typically
// runChatDo, wrapped by the real bubbletea wiring to release/restore
// terminal control around it — see chatmodel.go's real construction);
// dispatchChatLine takes it as a parameter so it is trivially fakeable in
// tests, with no terminal/bubbletea coupling in this file at all.
type chatDoRunner func(ctx context.Context, mode engine.Mode, request string) (engine.RunSummary, error)

// chatSaveFunc persists history to path (typically saveTranscript, from
// chatsession.go); a parameter, exactly like chatDoRunner, purely so
// tests can substitute a fake and assert dispatchChatLine never calls it
// except for an actual "/save" line.
type chatSaveFunc func(path string, history []llm.Message) error

// chatController bundles every dependency dispatchChatLine needs beyond
// the session/line/ctx themselves. It holds no mutable state of its own:
// session (chatSession) is the one stateful thing dispatchChatLine
// mutates, always passed in explicitly by the caller.
type chatController struct {
	tr    i18n.Translator
	llm   chatLLM
	doRun chatDoRunner
	save  chatSaveFunc

	// maxTokens is forwarded to every plain-text chatTurn call (see
	// chatTurn's doc comment in chat.go for why this must never be left
	// at its zero value against a real Anthropic-backed client). Set from
	// cfg.LLM.MaxTokens by newChatModel — the same config field
	// engine.Planner/Explainer/Diagnoser already use for every other
	// Complete call in this codebase.
	maxTokens int

	// showUsage is runChat's cfg.General.ShowUsage||--usage resolution
	// (usage.go) — gates both appendTurnUsage and appendSessionTotal
	// below; when false neither ever appends anything, regardless of
	// what sessionTally/turnTally recorded.
	showUsage bool

	// sessionTally accumulates every Complete call for the whole chat
	// session; appendSessionTotal reads it on "/exit"/"/quit".
	// turnTally is reset by dispatchChatLine immediately before every
	// dispatch and read by appendTurnUsage immediately after — a second,
	// independently-resettable tally (rather than diffing two
	// sessionTally snapshots) is what makes an exact per-turn figure
	// possible: llm.Client only accepts ONE usageObserver, so both
	// tallies are fed by the same callback (see runChat's own wiring),
	// and turnTally's reset/read window is scoped to exactly one
	// dispatchChatLine call — safe because chatModel.Update only ever
	// has one dispatch in flight at a time (see its own "waiting" gate).
	// Both are nil in any chatController a test builds without opting
	// into usage tracking; every method below treats a nil tally exactly
	// like showUsage == false.
	sessionTally *usageTally
	turnTally    *usageTally
}

// dispatchChatLine parses one line of chat input (parseChatInput) and
// acts on it: a slash command is handled entirely here (mode switch,
// clear, save, do, help, exit, or an unrecognized-command notice); plain
// text is sent to the LLM as the next chat turn (chatTurn), with both the
// user's message and the assistant's reply appended to session.history
// on success. It returns the text to display (a chat-turn reply, a
// command's confirmation/error/usage message, or empty for "/clear" —
// which prints nothing but MsgChatCleared) and whether the session must
// end ("/exit"/"/quit").
//
// dispatchChatLine has no bubbletea/terminal dependency at all — every
// branch is driven by dc's injected llm/doRun/save, which is exactly what
// makes it unit-testable end to end (chat_test.go) without a TTY, per
// docs/history/UYGULAMA_PLANI.md FAZ 9's own "pure functions, bubbletea is the shell"
// requirement.
//
// Usage-display wiring wraps the switch below rather than touching each
// branch individually: dc.turnTally is reset up front (a no-op for any
// branch that never reaches the LLM — mode/clear/save/help/unknown —
// since it simply stays at zero), and the switch's own result is run
// through appendSessionTotal (for "/exit"/"/quit") or appendTurnUsage
// (every other branch) before returning — both are no-ops when
// dc.showUsage is false or the relevant tally recorded zero requests, so
// this is safe to apply unconditionally.
func (dc *chatController) dispatchChatLine(ctx context.Context, session *chatSession, line string) (output string, exit bool) {
	if dc.turnTally != nil {
		dc.turnTally.reset()
	}

	output, exit = dc.dispatch(ctx, session, line)
	if exit {
		return dc.appendSessionTotal(output), true
	}
	return dc.appendTurnUsage(output), false
}

// dispatch is dispatchChatLine's actual slash-command/chat-turn routing,
// factored out so dispatchChatLine itself can wrap it uniformly with the
// turnTally reset/usage-append bookkeeping above without duplicating it
// into every case below.
func (dc *chatController) dispatch(ctx context.Context, session *chatSession, line string) (output string, exit bool) {
	cmd := parseChatInput(line)

	switch cmd.kind {
	case chatText:
		return dc.handleText(ctx, session, cmd.arg), false

	case chatCmdMode:
		if cmd.arg == "" {
			return dc.tr.T(i18n.MsgChatModeUsage), false
		}
		mode, err := session.setMode(cmd.arg)
		if err != nil {
			return dc.tr.T(i18n.MsgChatModeUsage), false
		}
		return dc.tr.T(i18n.MsgChatModeChanged, mode.String()), false

	case chatCmdClear:
		session.clear()
		return dc.tr.T(i18n.MsgChatCleared), false

	case chatCmdSave:
		if cmd.arg == "" {
			return dc.tr.T(i18n.MsgChatSaveUsage), false
		}
		if err := dc.save(cmd.arg, session.history); err != nil {
			return dc.tr.T(i18n.MsgChatSaveFailed, cmd.arg, err), false
		}
		return dc.tr.T(i18n.MsgChatSaved, cmd.arg), false

	case chatCmdDo:
		if cmd.arg == "" {
			return dc.tr.T(i18n.MsgChatDoUsage), false
		}
		return dc.handleDo(ctx, session, cmd.arg), false

	case chatCmdHelp:
		return dc.tr.T(i18n.MsgChatHelp), false

	case chatCmdExit:
		return dc.tr.T(i18n.MsgChatExiting), true

	default: // chatCmdUnknown
		return dc.tr.T(i18n.MsgChatUnknownCommand, cmd.arg), false
	}
}

// appendTurnUsage appends a blank line plus dc.turnTally's own
// formatUsageLine to output, when dc.showUsage is set and this turn's
// dispatch recorded at least one request — a no-op (returns output
// unchanged) otherwise, which covers both usage display being off and
// every slash command that never reaches the LLM at all.
func (dc *chatController) appendTurnUsage(output string) string {
	if !dc.showUsage || dc.turnTally == nil {
		return output
	}
	snap := dc.turnTally.snapshot()
	if snap.requests == 0 {
		return output
	}
	return output + "\n" + formatUsageLine(dc.tr, snap)
}

// appendSessionTotal appends dc.sessionTally's cumulative
// MsgChatUsageSessionTotal line to output — dispatchChatLine's own
// "/exit"/"/quit" branch, called once the session's about to end. A
// no-op under the same conditions as appendTurnUsage (usage display off,
// or zero requests recorded across the whole session).
func (dc *chatController) appendSessionTotal(output string) string {
	if !dc.showUsage || dc.sessionTally == nil {
		return output
	}
	snap := dc.sessionTally.snapshot()
	if snap.requests == 0 {
		return output
	}
	return output + "\n" + dc.tr.T(i18n.MsgChatUsageSessionTotal, formatUsageLine(dc.tr, snap))
}

// renderChatLLMError renders err (a failed chat-turn or "/do" pipeline
// error) through MsgChatLLMError, substituting classifyLLMError's
// friendly, translated message in place of err's own raw wrap-chain
// whenever it's classifiable — currently: *llm.KeyMissingError (QA
// MAJOR-1), applied to chat exactly like every other LLM-reaching
// command. dispatchChatLine and its handlers have no cmd/stderr to dump
// a COMRADE_DEBUG detail to (see dispatchChatLine's own doc comment:
// deliberately no bubbletea/terminal dependency at all — every branch is
// driven by dc's injected dependencies), so this calls classifyLLMError
// directly rather than translateLLMError (runtime.go), which needs an
// io.Writer for that dump.
func renderChatLLMError(tr i18n.Translator, err error) string {
	if ok, translated := classifyLLMError(tr, err); ok {
		err = translated
	}
	return tr.T(i18n.MsgChatLLMError, err)
}

// handleText drives one plain-text chat turn: chatTurn (chat.go) sends
// session.history plus the new message to dc.llm. Both the user's
// message and the assistant's reply are appended to session.history only
// on success — a failed turn leaves history exactly as it was before the
// attempt, so the user can simply retry without a phantom half-turn
// polluting subsequent requests.
func (dc *chatController) handleText(ctx context.Context, session *chatSession, text string) string {
	reply, err := chatTurn(ctx, dc.llm, dc.tr.Lang(), session.history, text, dc.maxTokens)
	if err != nil {
		return renderChatLLMError(dc.tr, err)
	}
	session.appendUser(text)
	session.appendAssistant(reply)
	return reply
}

// handleDo drives "/do <request>": dc.doRun (typically runChatDo,
// wrapped with real terminal release/restore by the bubbletea wiring —
// see chatmodel.go) is the SAME safety-gated plan+execute pipeline
// `comrade do` uses, run under session's current mode. The "/do" request
// and a one-line rendered summary are both appended to session.history
// (as a user/assistant-shaped pair) purely so `/save` captures what
// actually happened, exactly like every other turn.
func (dc *chatController) handleDo(ctx context.Context, session *chatSession, request string) string {
	summary, err := dc.doRun(ctx, session.mode, request)
	session.appendUser("/do " + request)
	if err != nil {
		reply := renderChatLLMError(dc.tr, err)
		session.appendAssistant(reply)
		return reply
	}

	reply := dc.tr.T(i18n.MsgChatDoSummary) + " " + formatRunSummaryLine(summary)
	session.appendAssistant(reply)
	return reply
}

// formatRunSummaryLine renders summary as the same "N executed, M
// skipped, K blocked[, aborted: reason]" shape printRunSummary
// (do.go) prints for `comrade do`/`comrade fix`, reused here verbatim so
// the two surfaces never describe a run differently.
func formatRunSummaryLine(summary engine.RunSummary) string {
	var executed, skipped, blocked int
	for _, r := range summary.Results {
		switch r.Outcome {
		case engine.OutcomeExecuted:
			executed++
		case engine.OutcomeSkipped:
			skipped++
		case engine.OutcomeBlocked:
			blocked++
		}
	}
	line := fmt.Sprintf("%d executed, %d skipped, %d blocked", executed, skipped, blocked)
	if summary.Aborted {
		line += fmt.Sprintf("; aborted: %s", summary.AbortReason)
	}
	return line
}
