package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// chatSession holds `comrade chat`'s entire in-memory, per-invocation
// state: the conversation history (sent to the LLM as context on every
// turn) and the session's active mode (what `/do <request>` runs the
// plan+execute pipeline under — see runChatDo in chat.go). Every method
// here is a pure state transition with no I/O of its own — the bubbletea
// model (chatmodel.go) is the only caller, and every one of these is also
// exercised directly in chatsession_test.go with no TTY at all.
//
// CLAUDE.md's privacy rule ("chat history is never written to disk except
// explicit /save <file>"): chatSession itself never touches a filesystem
// — saveTranscript (below) is the ONE function in this whole file that
// does, and it is only ever called from the "/save" branch of the chat
// command dispatch (chat.go's handleChatCommand).
type chatSession struct {
	mode    engine.Mode
	history []llm.Message
}

// newChatSession builds a chatSession starting in initialMode (the
// resolved mode at session start — see resolveChatInitialMode in
// chat.go, which applies the exact same flag/env/config precedence
// engine.ResolveMode already gives `comrade do`/`comrade fix`).
func newChatSession(initialMode engine.Mode) *chatSession {
	return &chatSession{mode: initialMode}
}

// setMode parses arg as a mode name and, on success, switches s.mode to
// it and returns the new Mode; an invalid/empty arg leaves s.mode
// unchanged and returns the parse error, which the caller renders via
// MsgChatModeUsage.
func (s *chatSession) setMode(arg string) (engine.Mode, error) {
	mode, err := engine.ParseMode(arg)
	if err != nil {
		return s.mode, err
	}
	s.mode = mode
	return mode, nil
}

// clear resets the conversation history to empty. The session's active
// mode is untouched — "/mode" and "/clear" are independent axes.
func (s *chatSession) clear() {
	s.history = nil
}

// appendUser appends one user-turn message to the history.
func (s *chatSession) appendUser(text string) {
	s.history = append(s.history, llm.Message{Role: "user", Content: text})
}

// appendAssistant appends one assistant-turn message to the history.
func (s *chatSession) appendAssistant(text string) {
	s.history = append(s.history, llm.Message{Role: "assistant", Content: text})
}

// renderTranscript renders history as plain text, one "ROLE: content"
// paragraph per message, in order — the exact (and only) format /save
// ever writes to disk.
func renderTranscript(history []llm.Message) string {
	var b strings.Builder
	for _, m := range history {
		fmt.Fprintf(&b, "%s: %s\n\n", strings.ToUpper(m.Role), m.Content)
	}
	return b.String()
}

// saveTranscript writes history's rendered transcript to path with 0600
// permissions (it may contain anything the user discussed, including
// command output) — CLAUDE.md's one, explicit, opt-in exception to
// "session history is never written to disk": this function is reachable
// ONLY from the chat command dispatch's literal "/save" branch, never
// from any autosave/periodic-checkpoint path (there is none anywhere in
// this package).
func saveTranscript(path string, history []llm.Message) error {
	return os.WriteFile(path, []byte(renderTranscript(history)), 0o600) // #nosec G306 -- a transcript the user explicitly asked to export; 0600 restricts it to the owner
}
