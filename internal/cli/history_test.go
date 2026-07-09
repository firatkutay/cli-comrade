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

	out := execRoot(t, "dev", "history")

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

	out := execRoot(t, "dev", "history", "--json")

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

	out := execRoot(t, "dev", "history", "--limit", "2")

	assert.NotContains(t, out, "echo entry-0")
	assert.NotContains(t, out, "echo entry-2")
	assert.Contains(t, out, "echo entry-3")
	assert.Contains(t, out, "echo entry-4")
}

func TestHistoryOnEmptyLogPrintsHeaderOnly(t *testing.T) {
	withIsolatedConfigDir(t)

	out := execRoot(t, "dev", "history")

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 1, "an empty audit log must print only the header row")
	assert.Contains(t, lines[0], "COMMAND")
}

// TestHistoryIsReadOnlyNeverRewritesAuditFile proves `comrade history`
// never mutates the audit log it reads — no lazy retention cleanup runs
// here (that is `comrade do`'s concern, on invocations that actually
// execute something); viewing history is always safe.
func TestHistoryIsReadOnlyNeverRewritesAuditFile(t *testing.T) {
	withIsolatedConfigDir(t)
	seedAuditEntries(t, 1)

	before := readAuditEntries(t)
	_ = execRoot(t, "dev", "history")
	after := readAuditEntries(t)

	require.Len(t, before, 1)
	require.Len(t, after, 1)
	assert.Equal(t, before[0], after[0])
}
