package cli

import "strings"

// chatCommandKind identifies which slash command (if any) a chat input
// line named. Every value here is parsed by parseChatInput — a pure
// function with no bubbletea/LLM/I-O dependency at all, so every branch
// is unit-testable without a TTY (see chatparse_test.go).
type chatCommandKind int

const (
	// chatText is not a slash command at all: arg carries the plain text
	// to send to the LLM as the next chat turn.
	chatText chatCommandKind = iota
	// chatCmdMode is "/mode <name>"; arg carries the (unvalidated) mode
	// name argument, which may be empty.
	chatCmdMode
	// chatCmdClear is "/clear": reset the in-memory history.
	chatCmdClear
	// chatCmdExit is "/exit" or "/quit": end the session.
	chatCmdExit
	// chatCmdSave is "/save <file>"; arg carries the (unvalidated) file
	// path argument, which may be empty.
	chatCmdSave
	// chatCmdDo is "/do <request>"; arg carries the (unvalidated) request
	// text, which may be empty.
	chatCmdDo
	// chatCmdHelp is "/help".
	chatCmdHelp
	// chatCmdUnknown is any "/xyz" this list does not recognize; arg
	// carries the raw, unrecognized command text (including its leading
	// slash) for MsgChatUnknownCommand's %s.
	chatCmdUnknown
)

// chatCommand is parseChatInput's result: which slash command (if any)
// line named, plus its single trailing argument (trimmed, may be empty).
type chatCommand struct {
	kind chatCommandKind
	arg  string
}

// parseChatInput classifies one line of raw chat input. A line not
// starting with "/" (after trimming leading/trailing whitespace) is
// always chatText, verbatim (not re-trimmed — a message's own leading/
// trailing spaces are the user's business, not this parser's). A line
// starting with "/" splits on the first run of whitespace into the
// command word and everything after it (arg, trimmed); the command word
// is matched case-sensitively against the fixed slash-command set above,
// falling back to chatCmdUnknown for anything else — including a bare
// "/" with nothing after it.
func parseChatInput(line string) chatCommand {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "/") {
		return chatCommand{kind: chatText, arg: line}
	}

	fields := strings.SplitN(trimmed, " ", 2)
	word := fields[0]
	arg := ""
	if len(fields) > 1 {
		arg = strings.TrimSpace(fields[1])
	}

	switch word {
	case "/mode":
		return chatCommand{kind: chatCmdMode, arg: arg}
	case "/clear":
		return chatCommand{kind: chatCmdClear}
	case "/exit", "/quit":
		return chatCommand{kind: chatCmdExit}
	case "/save":
		return chatCommand{kind: chatCmdSave, arg: arg}
	case "/do":
		return chatCommand{kind: chatCmdDo, arg: arg}
	case "/help":
		return chatCommand{kind: chatCmdHelp}
	default:
		return chatCommand{kind: chatCmdUnknown, arg: trimmed}
	}
}
