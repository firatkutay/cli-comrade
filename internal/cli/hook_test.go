package cli

import (
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/context"
)

// setHookStateDir points "comrade hook record"'s last_command.json write
// at dir, for whichever real OS the test binary is actually running on
// (recordLastCommand resolves the path via context.LastCommandPath(
// runtime.GOOS, os.Getenv) — real runtime.GOOS, not an injectable one —
// so the test must set the environment variable that OS's branch
// actually reads: LOCALAPPDATA on Windows, XDG_STATE_HOME everywhere
// else). Returns the resulting last_command.json path.
func setHookStateDir(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Setenv("LOCALAPPDATA", dir)
	} else {
		t.Setenv("XDG_STATE_HOME", dir)
	}
	return filepath.Join(dir, "cli-comrade", "last_command.json")
}

func TestHookRecordWritesLastCommandJSONRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	path := setHookStateDir(t, stateDir)

	// Quotes, unicode, and an embedded newline: exactly the shapes a
	// hand-assembled-JSON shell script would have gotten wrong, and the
	// reason FAZ 4's hooks exec this Go command instead.
	cmdText := "echo \"héllo\nworld\" 'quo\"ted'"

	out := execRoot(t, "dev", "hook", "record",
		"--shell", "bash",
		"--exit", "127",
		"--command", cmdText,
	)
	assert.Equal(t, "", out, "hook record must print nothing on success")

	got, ok := context.ReadLastCommand(path)
	require.True(t, ok)
	assert.Equal(t, cmdText, got.Command)
	assert.Equal(t, 127, got.ExitCode)
	assert.Equal(t, "bash", got.Shell)
	assert.Equal(t, "", got.StderrTail)
	assert.Equal(t, "", got.StdoutTail)
	assert.WithinDuration(t, time.Now().UTC(), got.Timestamp, 10*time.Second)
}

func TestHookRecordOverwritesPreviousEntry(t *testing.T) {
	stateDir := t.TempDir()
	path := setHookStateDir(t, stateDir)

	execRoot(t, "dev", "hook", "record", "--shell", "bash", "--exit", "0", "--command", "first")
	execRoot(t, "dev", "hook", "record", "--shell", "zsh", "--exit", "2", "--command", "second")

	got, ok := context.ReadLastCommand(path)
	require.True(t, ok)
	assert.Equal(t, "second", got.Command)
	assert.Equal(t, 2, got.ExitCode)
	assert.Equal(t, "zsh", got.Shell)
}

func TestHookRecordExitsZeroSilentlyWhenPathCannotBeResolved(t *testing.T) {
	// Unset both XDG_STATE_HOME and HOME so LastCommandPath fails; the
	// hook must still exit 0 and print nothing (never break a prompt).
	t.Setenv("XDG_STATE_HOME", "")
	t.Setenv("HOME", "")

	out := execRoot(t, "dev", "hook", "record", "--shell", "bash", "--exit", "1", "--command", "x")
	assert.Equal(t, "", out)
}

func TestHookRecordIsHiddenFromHelp(t *testing.T) {
	root := NewRootCmd("dev")
	hookCmd, _, err := root.Find([]string{"hook"})
	require.NoError(t, err)
	assert.True(t, hookCmd.Hidden)

	recordCmd, _, err := root.Find([]string{"hook", "record"})
	require.NoError(t, err)
	assert.True(t, recordCmd.Hidden)
}
