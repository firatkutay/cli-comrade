package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

func TestNewChatSessionStartsWithGivenModeAndEmptyHistory(t *testing.T) {
	s := newChatSession(engine.ModeAsk)
	assert.Equal(t, engine.ModeAsk, s.mode)
	assert.Empty(t, s.history)
}

func TestChatSessionSetModeSwitchesModeOnValidName(t *testing.T) {
	s := newChatSession(engine.ModeAsk)
	mode, err := s.setMode("auto")
	require.NoError(t, err)
	assert.Equal(t, engine.ModeAuto, mode)
	assert.Equal(t, engine.ModeAuto, s.mode)
}

func TestChatSessionSetModeLeavesModeUnchangedOnInvalidName(t *testing.T) {
	s := newChatSession(engine.ModeAsk)
	_, err := s.setMode("bogus")
	require.Error(t, err)
	assert.Equal(t, engine.ModeAsk, s.mode, "an invalid /mode argument must never change the active mode")
}

func TestChatSessionClearResetsHistoryButNotMode(t *testing.T) {
	s := newChatSession(engine.ModeAuto)
	s.appendUser("hi")
	s.appendAssistant("hello")
	require.Len(t, s.history, 2)

	s.clear()
	assert.Empty(t, s.history)
	assert.Equal(t, engine.ModeAuto, s.mode, "/clear must not touch the active mode")
}

func TestChatSessionAppendPreservesOrderAndRoles(t *testing.T) {
	s := newChatSession(engine.ModeAsk)
	s.appendUser("what does ls do")
	s.appendAssistant("it lists files")

	require.Len(t, s.history, 2)
	assert.Equal(t, llm.Message{Role: "user", Content: "what does ls do"}, s.history[0])
	assert.Equal(t, llm.Message{Role: "assistant", Content: "it lists files"}, s.history[1])
}

func TestRenderTranscriptFormatsEveryMessage(t *testing.T) {
	history := []llm.Message{
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello there"},
	}
	got := renderTranscript(history)
	assert.Equal(t, "USER: hi\n\nASSISTANT: hello there\n\n", got)
}

func TestSaveTranscriptWritesExactRenderedContentWithRestrictivePermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "transcript.txt")
	history := []llm.Message{{Role: "user", Content: "hi"}}

	require.NoError(t, saveTranscript(path, history))

	data, err := os.ReadFile(path) //nolint:gosec // test-controlled temp path
	require.NoError(t, err)
	assert.Equal(t, renderTranscript(history), string(data))

	info, err := os.Stat(path)
	require.NoError(t, err)
	// Windows' os.Chmod/FileMode does not model Unix permission bits, so
	// 0600 is not a meaningful assertion there (this was previously
	// guarded by the nonexistent "GOOS" env var, which is never actually
	// set at runtime — a no-op guard that always evaluated true,
	// including on Windows itself; runtime.GOOS is the real check). The
	// content assertion above already runs unconditionally on every OS.
	if runtime.GOOS != "windows" {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}
