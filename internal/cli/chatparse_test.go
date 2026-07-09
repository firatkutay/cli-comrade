package cli

import "testing"

func TestParseChatInputPlainTextIsNotASlashCommand(t *testing.T) {
	cases := []string{"hello there", "", "   ", "not/a/command", "-slash-like"}
	for _, line := range cases {
		got := parseChatInput(line)
		if got.kind != chatText {
			t.Errorf("parseChatInput(%q).kind = %v, want chatText", line, got.kind)
		}
	}
}

// TestParseChatInputBareSlashIsUnknownCommand documents the one edge
// case a naive "does it start with /" reading might get wrong: a bare "/"
// (nothing after it) IS treated as a slash command (an unrecognized one),
// not as plain chat text — parseChatInput's HasPrefix check alone decides
// that, before the switch on the command word ever runs.
func TestParseChatInputBareSlashIsUnknownCommand(t *testing.T) {
	got := parseChatInput("/")
	if got.kind != chatCmdUnknown {
		t.Fatalf("parseChatInput(\"/\").kind = %v, want chatCmdUnknown", got.kind)
	}
}

func TestParseChatInputPlainTextArgIsVerbatimNotTrimmed(t *testing.T) {
	got := parseChatInput("  hello  ")
	if got.arg != "  hello  " {
		t.Fatalf("plain text arg must be verbatim (not trimmed), got %q", got.arg)
	}
}

func TestParseChatInputSlashCommands(t *testing.T) {
	cases := []struct {
		line     string
		wantKind chatCommandKind
		wantArg  string
	}{
		{"/mode auto", chatCmdMode, "auto"},
		{"/mode", chatCmdMode, ""},
		{"/mode   ask  ", chatCmdMode, "ask"},
		{"/clear", chatCmdClear, ""},
		{"/exit", chatCmdExit, ""},
		{"/quit", chatCmdExit, ""},
		{"/save transcript.txt", chatCmdSave, "transcript.txt"},
		{"/save", chatCmdSave, ""},
		{"/do docker kur", chatCmdDo, "docker kur"},
		{"/do", chatCmdDo, ""},
		{"/help", chatCmdHelp, ""},
		{"/bogus", chatCmdUnknown, "/bogus"},
		{"/bogus with args", chatCmdUnknown, "/bogus with args"},
	}
	for _, tc := range cases {
		t.Run(tc.line, func(t *testing.T) {
			got := parseChatInput(tc.line)
			if got.kind != tc.wantKind {
				t.Fatalf("parseChatInput(%q).kind = %v, want %v", tc.line, got.kind, tc.wantKind)
			}
			if got.arg != tc.wantArg {
				t.Fatalf("parseChatInput(%q).arg = %q, want %q", tc.line, got.arg, tc.wantArg)
			}
		})
	}
}

func TestParseChatInputTrimsLeadingWhitespaceBeforeSlash(t *testing.T) {
	got := parseChatInput("   /clear")
	if got.kind != chatCmdClear {
		t.Fatalf("leading whitespace before a slash command must still be recognized, got kind %v", got.kind)
	}
}
