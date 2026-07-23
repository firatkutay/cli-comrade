package context

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLastCommandPathWindowsUsesLocalAppData(t *testing.T) {
	got, err := LastCommandPath("windows", fakeEnv(map[string]string{
		"LOCALAPPDATA": `C:\Users\alice\AppData\Local`,
	}))
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\alice\AppData\Local\cli-comrade\last_command.json`, got)
}

func TestLastCommandPathWindowsErrorsWhenLocalAppDataUnset(t *testing.T) {
	_, err := LastCommandPath("windows", fakeEnv(map[string]string{}))
	assert.ErrorContains(t, err, "LOCALAPPDATA")
}

func TestLastCommandPathUnixUsesXDGStateHomeWhenSet(t *testing.T) {
	// The expected value is built with a literal "/", not filepath.Join:
	// this injects goos="linux"/"darwin" regardless of the host the test
	// binary actually runs on, so the expectation must assert the unix
	// (forward-slash) shape unconditionally rather than whatever
	// separator the test's own OS happens to use.
	for _, goos := range []string{"linux", "darwin"} {
		t.Run(goos, func(t *testing.T) {
			got, err := LastCommandPath(goos, fakeEnv(map[string]string{
				"XDG_STATE_HOME": "/home/alice/.state-custom",
				"HOME":           "/home/alice",
			}))
			require.NoError(t, err)
			assert.Equal(t, "/home/alice/.state-custom/cli-comrade/last_command.json", got)
		})
	}
}

func TestLastCommandPathUnixFallsBackToHomeLocalState(t *testing.T) {
	got, err := LastCommandPath("linux", fakeEnv(map[string]string{
		"HOME": "/home/alice",
	}))
	require.NoError(t, err)
	assert.Equal(t, "/home/alice/.local/state/cli-comrade/last_command.json", got)
}

func TestLastCommandPathUnixErrorsWhenHomeUnset(t *testing.T) {
	_, err := LastCommandPath("linux", fakeEnv(map[string]string{}))
	assert.ErrorContains(t, err, "HOME")
}

func TestReadLastCommandRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_command.json")

	ts := time.Date(2026, 7, 8, 10, 30, 0, 0, time.UTC)
	fixture := LastCommand{
		Command:    "pyton --version",
		ExitCode:   127,
		StderrTail: "bash: pyton: command not found",
		StdoutTail: "",
		Timestamp:  ts,
		Shell:      "bash",
	}
	raw, err := json.Marshal(fixture)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, raw, 0o644))

	got, ok := ReadLastCommand(path)
	require.True(t, ok)
	assert.Equal(t, "pyton --version", got.Command)
	assert.Equal(t, 127, got.ExitCode)
	assert.Equal(t, "bash: pyton: command not found", got.StderrTail)
	assert.Equal(t, "", got.StdoutTail)
	assert.True(t, ts.Equal(got.Timestamp))
	assert.Equal(t, "bash", got.Shell)

	now := ts.Add(90 * time.Second)
	assert.Equal(t, 90*time.Second, got.Age(now))
}

func TestReadLastCommandMissingFile(t *testing.T) {
	_, ok := ReadLastCommand(filepath.Join(t.TempDir(), "does-not-exist.json"))
	assert.False(t, ok)
}

func TestReadLastCommandCorruptJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_command.json")
	require.NoError(t, os.WriteFile(path, []byte("{not valid json"), 0o644))

	_, ok := ReadLastCommand(path)
	assert.False(t, ok)
}

func TestReadLastCommandEmptyPath(t *testing.T) {
	_, ok := ReadLastCommand("")
	assert.False(t, ok)
}

// TestReadLastCommandOversizedFileIsRejectedNotOOMed proves LOW#10's
// fix: a file over maxLastCommandBytes is refused outright — ok=false,
// same "not available" outcome as a missing or corrupt file — rather
// than being read in full and handed to json.Unmarshal.
func TestReadLastCommandOversizedFileIsRejectedNotOOMed(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_command.json")

	// A syntactically valid JSON object that is still over the cap: if
	// ReadLastCommand only bounded parsing (not the read itself), this
	// oversized-but-valid input would slip through and prove nothing.
	oversized := `{"command":"` + strings.Repeat("a", maxLastCommandBytes+1) + `"}`
	require.NoError(t, os.WriteFile(path, []byte(oversized), 0o644))

	_, ok := ReadLastCommand(path)
	assert.False(t, ok)
}

// TestReadLastCommandExactlyAtCapStillParses proves the cap is not
// off-by-one in the wrong direction: a file exactly at maxLastCommandBytes
// is read and parsed normally, matching "normal-sized files behave
// identically" from before this fix.
func TestReadLastCommandExactlyAtCapStillParses(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_command.json")

	fixture := LastCommand{Command: "echo hi", ExitCode: 0, Shell: "bash"}
	raw, err := json.Marshal(fixture)
	require.NoError(t, err)
	// Pad with JSON whitespace (still exactly maxLastCommandBytes total,
	// still valid JSON) so the fixture's own size reaches the cap exactly
	// regardless of how large the marshaled struct itself is.
	require.Less(t, len(raw), maxLastCommandBytes)
	padded := append([]byte(strings.Repeat(" ", maxLastCommandBytes-len(raw))), raw...)
	require.Len(t, padded, maxLastCommandBytes)
	require.NoError(t, os.WriteFile(path, padded, 0o644))

	got, ok := ReadLastCommand(path)
	require.True(t, ok)
	assert.Equal(t, "echo hi", got.Command)
}

func TestWriteLastCommandRoundTrip(t *testing.T) {
	dir := t.TempDir()
	// Nested, not-yet-existing directory: WriteLastCommand must create it.
	path := filepath.Join(dir, "nested", "last_command.json")

	ts := time.Date(2026, 7, 9, 12, 0, 0, 0, time.UTC)
	want := LastCommand{
		Command:   "echo \"héllo\nworld\" 'quo\"ted'",
		ExitCode:  127,
		Timestamp: ts,
		Shell:     "zsh",
	}

	require.NoError(t, WriteLastCommand(path, want))

	got, ok := ReadLastCommand(path)
	require.True(t, ok)
	assert.Equal(t, want.Command, got.Command)
	assert.Equal(t, want.ExitCode, got.ExitCode)
	assert.True(t, want.Timestamp.Equal(got.Timestamp))
	assert.Equal(t, want.Shell, got.Shell)
	assert.Equal(t, "", got.StderrTail)
	assert.Equal(t, "", got.StdoutTail)

	// No leftover temp file: the write must have renamed, not copied.
	entries, err := os.ReadDir(filepath.Join(dir, "nested"))
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "last_command.json", entries[0].Name())
}

func TestWriteLastCommandOverwritesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "last_command.json")

	require.NoError(t, WriteLastCommand(path, LastCommand{Command: "first", ExitCode: 0, Shell: "bash"}))
	require.NoError(t, WriteLastCommand(path, LastCommand{Command: "second", ExitCode: 1, Shell: "bash"}))

	got, ok := ReadLastCommand(path)
	require.True(t, ok)
	assert.Equal(t, "second", got.Command)
	assert.Equal(t, 1, got.ExitCode)
}
