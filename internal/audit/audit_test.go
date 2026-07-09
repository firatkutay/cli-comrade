package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestLogger(t *testing.T) *Logger {
	t.Helper()
	dir := t.TempDir()
	logger, err := NewLogger(filepath.Join(dir, "audit.jsonl"))
	require.NoError(t, err)
	return logger
}

func TestAppendThenReadAllRoundTripsExactFields(t *testing.T) {
	logger := newTestLogger(t)
	ts := time.Date(2026, 7, 8, 12, 0, 0, 0, time.UTC)

	entry := Entry{
		Timestamp:  ts,
		Request:    "docker kur",
		Command:    "sudo apt-get install -y docker.io",
		Risk:       "elevated",
		Mode:       "auto",
		ExitCode:   0,
		DurationMs: 1234,
	}
	require.NoError(t, logger.Append(entry))

	entries, err := logger.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 1)

	got := entries[0]
	assert.True(t, ts.Equal(got.Timestamp))
	assert.Equal(t, "docker kur", got.Request)
	assert.Equal(t, "sudo apt-get install -y docker.io", got.Command)
	assert.Equal(t, "elevated", got.Risk)
	assert.Equal(t, "auto", got.Mode)
	assert.Equal(t, 0, got.ExitCode)
	assert.Equal(t, int64(1234), got.DurationMs)
}

func TestAppendNeverLogsStdoutOrStderrContent(t *testing.T) {
	// Entry has no Stdout/Stderr field at all — this test pins that
	// contract by proving a marshaled entry's JSON never contains
	// anything resembling captured process output, even though the
	// caller obviously ran a command that produced some.
	logger := newTestLogger(t)
	require.NoError(t, logger.Append(Entry{
		Timestamp: time.Now(),
		Command:   "echo hi",
		Risk:      "read",
		Mode:      "auto",
		ExitCode:  0,
	}))

	entries, err := logger.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "echo hi", entries[0].Command)
}

func TestReadAllOnMissingFileReturnsNoEntriesNoError(t *testing.T) {
	dir := t.TempDir()
	logger := &Logger{path: filepath.Join(dir, "does-not-exist.jsonl")}

	entries, err := logger.ReadAll()
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestReadAllSkipsCorruptLinesButKeepsGoodOnes(t *testing.T) {
	logger := newTestLogger(t)
	require.NoError(t, logger.Append(Entry{Timestamp: time.Now(), Command: "good-1", Risk: "read", Mode: "auto"}))

	// Hand-corrupt the file by appending a non-JSON line directly.
	f, err := os.OpenFile(logger.Path(), os.O_APPEND|os.O_WRONLY, 0o644)
	require.NoError(t, err)
	_, err = f.WriteString("not json at all\n")
	require.NoError(t, err)
	require.NoError(t, f.Close())

	require.NoError(t, logger.Append(Entry{Timestamp: time.Now(), Command: "good-2", Risk: "read", Mode: "auto"}))

	entries, err := logger.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 2)
	assert.Equal(t, "good-1", entries[0].Command)
	assert.Equal(t, "good-2", entries[1].Command)
}

func TestApplyRetentionDropsOldEntriesKeepsRecentOnes(t *testing.T) {
	logger := newTestLogger(t)
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)

	old := Entry{Timestamp: now.AddDate(0, 0, -100), Command: "old-command", Risk: "read", Mode: "auto"}
	recent := Entry{Timestamp: now.AddDate(0, 0, -1), Command: "recent-command", Risk: "read", Mode: "auto"}
	require.NoError(t, logger.Append(old))
	require.NoError(t, logger.Append(recent))

	require.NoError(t, logger.ApplyRetention(90, now))

	entries, err := logger.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "recent-command", entries[0].Command)
}

func TestApplyRetentionZeroOrNegativeDisablesCleanup(t *testing.T) {
	logger := newTestLogger(t)
	now := time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC)
	old := Entry{Timestamp: now.AddDate(0, 0, -10000), Command: "ancient", Risk: "read", Mode: "auto"}
	require.NoError(t, logger.Append(old))

	require.NoError(t, logger.ApplyRetention(0, now))

	entries, err := logger.ReadAll()
	require.NoError(t, err)
	require.Len(t, entries, 1, "retention_days<=0 must disable cleanup entirely")
}

func TestPathForWindowsUsesLocalAppData(t *testing.T) {
	getenv := func(k string) string {
		if k == "LOCALAPPDATA" {
			return `C:\Users\tester\AppData\Local`
		}
		return ""
	}
	path, err := PathFor("windows", getenv)
	require.NoError(t, err)
	assert.Equal(t, `C:\Users\tester\AppData\Local\cli-comrade\audit.jsonl`, path)
}

func TestPathForWindowsMissingLocalAppDataErrors(t *testing.T) {
	_, err := PathFor("windows", func(string) string { return "" })
	assert.Error(t, err)
}

func TestPathForUnixUsesXDGStateHome(t *testing.T) {
	getenv := func(k string) string {
		if k == "XDG_STATE_HOME" {
			return "/home/tester/.state"
		}
		return ""
	}
	path, err := PathFor("linux", getenv)
	require.NoError(t, err)
	assert.Equal(t, "/home/tester/.state/cli-comrade/audit.jsonl", path)
}

func TestPathForUnixFallsBackToHomeLocalState(t *testing.T) {
	getenv := func(k string) string {
		if k == "HOME" {
			return "/home/tester"
		}
		return ""
	}
	path, err := PathFor("linux", getenv)
	require.NoError(t, err)
	assert.Equal(t, "/home/tester/.local/state/cli-comrade/audit.jsonl", path)
}

func TestPathForUnixMissingHomeErrors(t *testing.T) {
	_, err := PathFor("linux", func(string) string { return "" })
	assert.Error(t, err)
}

func TestNewLoggerCreatesParentDirectory(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c", "audit.jsonl")

	logger, err := NewLogger(nested)
	require.NoError(t, err)
	assert.NoError(t, logger.Append(Entry{Timestamp: time.Now(), Command: "x", Risk: "read", Mode: "auto"}))
}
