package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/audit"
)

// readAuditEntries reads back every entry currently in the (isolated, per
// withIsolatedConfigDir) audit log — the independent verification path
// end-to-end tests use to confirm exactly which steps internal/engine.
// Runner actually executed, without trusting stdout alone.
func readAuditEntries(t *testing.T) []audit.Entry {
	t.Helper()
	path, err := audit.DefaultPath()
	require.NoError(t, err)
	logger, err := audit.NewLogger(path)
	require.NoError(t, err)
	entries, err := logger.ReadAll()
	require.NoError(t, err)
	return entries
}

// seedAuditEntries appends n entries directly to the isolated audit log,
// with strictly increasing timestamps, so history_test.go's tests don't
// need a real `comrade do` run just to have something to list.
func seedAuditEntries(t *testing.T, n int) {
	t.Helper()
	path, err := audit.DefaultPath()
	require.NoError(t, err)
	logger, err := audit.NewLogger(path)
	require.NoError(t, err)

	base := time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < n; i++ {
		require.NoError(t, logger.Append(audit.Entry{
			Timestamp:  base.Add(time.Duration(i) * time.Minute),
			Request:    "test request",
			Command:    fmt.Sprintf("echo entry-%d", i),
			Risk:       "read",
			Mode:       "auto",
			ExitCode:   0,
			DurationMs: 5,
		}))
	}
}

func TestHistoryTableShowsRecentEntriesNewestOrderPreserved(t *testing.T) {
	withIsolatedConfigDir(t)
	seedAuditEntries(t, 3)

	out, stderr, err := execRootSplit(t, "dev", "history")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, out, "TIME")
	assert.Contains(t, out, "MODE")
	assert.Contains(t, out, "RISK")
	assert.Contains(t, out, "EXIT")
	assert.Contains(t, out, "COMMAND")
	assert.Contains(t, out, "echo entry-0")
	assert.Contains(t, out, "echo entry-1")
	assert.Contains(t, out, "echo entry-2")
}

func TestHistoryJSONPrintsOneEntryPerLine(t *testing.T) {
	withIsolatedConfigDir(t)
	seedAuditEntries(t, 2)

	out, stderr, err := execRootSplit(t, "dev", "history", "--json")
	require.NoError(t, err, "stderr: %s", stderr)

	var decoded []audit.Entry
	scanner := bufio.NewScanner(strings.NewReader(out))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e audit.Entry
		require.NoError(t, json.Unmarshal([]byte(line), &e))
		decoded = append(decoded, e)
	}
	require.Len(t, decoded, 2)
	assert.Equal(t, "echo entry-0", decoded[0].Command)
	assert.Equal(t, "echo entry-1", decoded[1].Command)
}

func TestHistoryLimitFlagCapsToMostRecentEntries(t *testing.T) {
	withIsolatedConfigDir(t)
	seedAuditEntries(t, 5)

	out, stderr, err := execRootSplit(t, "dev", "history", "--limit", "2")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.NotContains(t, out, "echo entry-0")
	assert.NotContains(t, out, "echo entry-2")
	assert.Contains(t, out, "echo entry-3")
	assert.Contains(t, out, "echo entry-4")
}

// TestHistoryOnEmptyLogPrintsFriendlyEmptyMessage proves an empty audit
// log prints a friendly "nothing recorded yet" message instead of a bare
// header row with nothing underneath it (which earlier phases printed —
// see docs/phases/FAZ-09.md). execRootSplit (not execRoot) is used
// because this is the isolated dir's first invocation, and the shared
// first-run config notice (unrelated to this test) lands on stderr.
func TestHistoryOnEmptyLogPrintsFriendlyEmptyMessage(t *testing.T) {
	withIsolatedConfigDir(t)

	out, stderr, err := execRootSplit(t, "dev", "history")
	require.NoError(t, err, "stderr: %s", stderr)

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 1, "an empty audit log must print exactly one line: the empty message")
	assert.NotContains(t, lines[0], "COMMAND", "the table header must not appear when there is nothing to show under it")
	assert.Contains(t, lines[0], "No commands recorded yet.")
}

// TestHistoryIsReadOnlyNeverRewritesAuditFile proves `comrade history`
// never mutates the audit log it reads — no lazy retention cleanup runs
// here (that is `comrade do`'s concern, on invocations that actually
// execute something); viewing history is always safe.
func TestHistoryIsReadOnlyNeverRewritesAuditFile(t *testing.T) {
	withIsolatedConfigDir(t)
	seedAuditEntries(t, 1)

	before := readAuditEntries(t)
	_, _, err := execRootSplit(t, "dev", "history")
	require.NoError(t, err)
	after := readAuditEntries(t)

	require.Len(t, before, 1)
	require.Len(t, after, 1)
	assert.Equal(t, before[0], after[0])
}

// TestHistoryStrayArgShowsTranslatedUsageError proves `comrade history`'s
// Args (translatedNoArgs, argvalidation.go) renders a friendly, i18n'd
// usage error naming the command's own full path, instead of cobra's raw
// English "accepts 0 arg(s), received 1", when given a stray positional
// argument — representative of every other translatedNoArgs command in
// this tree (chat, config list/edit/path/models/test-llm, upgrade, auth
// status, hook, hook record), which all share this exact same
// implementation.
func TestHistoryStrayArgShowsTranslatedUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "history", "unexpected")

	require.Error(t, err)
	assert.Equal(t, "comrade history does not take any arguments", err.Error())
}

// TestHistoryStrayArgShowsTranslatedUsageErrorInTurkish is the same proof
// under COMRADE_LANG=tr.
func TestHistoryStrayArgShowsTranslatedUsageErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "history", "unexpected")

	require.Error(t, err)
	assert.Equal(t, "comrade history hiçbir argüman almaz", err.Error())
}
