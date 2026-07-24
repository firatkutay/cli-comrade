package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/audit"
)

func ts(minute int) time.Time {
	return time.Date(2026, 7, 1, 0, minute, 0, 0, time.UTC)
}

// mixedFormatEntries builds a fixture mixing pre-undo-support (empty
// RunID) entries with two distinct, properly RunID-tagged runs — the
// exact "old-format + new-format lines mixed" fixture the task spec asks
// for.
func mixedFormatEntries() []audit.Entry {
	return []audit.Entry{
		// Legacy entries recorded before comrade-undo existed at all.
		{Timestamp: ts(0), Command: "echo legacy-1", Risk: "read", Mode: "auto", ExitCode: 0},
		{Timestamp: ts(1), Command: "echo legacy-2", Risk: "read", Mode: "auto", ExitCode: 0},
		// run-a: two steps.
		{Timestamp: ts(2), RunID: "run-a", Request: "make docs folder", Command: "mkdir docs", Risk: "write", Mode: "ask", ExitCode: 0},
		{Timestamp: ts(3), RunID: "run-a", Request: "make docs folder", Command: "echo done", Risk: "read", Mode: "ask", ExitCode: 0},
		// run-b: one step, strictly newer than run-a.
		{Timestamp: ts(4), RunID: "run-b", Request: "install nginx", Command: "apt install nginx", Risk: "elevated", Mode: "ask", ExitCode: 0},
	}
}

func TestGroupUndoRunsSeparatesLegacyFromRunIDEntries(t *testing.T) {
	entries := mixedFormatEntries()

	runs, legacyCount := groupUndoRuns(entries)

	assert.Equal(t, 2, legacyCount)
	require.Len(t, runs, 2)
	assert.Equal(t, "run-a", runs[0].RunID)
	require.Len(t, runs[0].Steps, 2)
	assert.Equal(t, "mkdir docs", runs[0].Steps[0].Command, "step order must be preserved oldest-first")
	assert.Equal(t, "echo done", runs[0].Steps[1].Command)
	assert.Equal(t, "run-b", runs[1].RunID)
	require.Len(t, runs[1].Steps, 1)
}

func TestSelectUndoTargetPicksNewestEligibleRun(t *testing.T) {
	entries := mixedFormatEntries()

	target, ok := selectUndoTarget(entries)

	require.True(t, ok)
	assert.Equal(t, "run-b", target.RunID, "run-b is strictly newer than run-a")
}

// olderThanEverything is a timestamp before every entry mixedFormatEntries
// produces — used to construct an "already undone" marker run.Steps in
// this file whose OWN chronological position must not matter (a
// selectUndoTarget test cares only about which run got EXCLUDED, not
// about the excluding run's own recency).
var olderThanEverything = time.Date(2026, 6, 30, 23, 59, 0, 0, time.UTC)

// TestSelectUndoTargetSkipsAlreadyUndoneRun proves the "not already
// referenced by a later entry's UndoOf" eligibility rule: once run-b has
// been undone (an undo-run entry naming UndoOf: "run-b"), the default
// target selection must fall back to run-a instead of offering to undo
// run-b a second time — even though the marker entry recording that undo
// is itself a distinct, eligible run (comrade undo's own steps always get
// a fresh RunID — see audit.Entry.RunID's own doc comment).
func TestSelectUndoTargetSkipsAlreadyUndoneRun(t *testing.T) {
	entries := mixedFormatEntries()
	entries = append(entries, audit.Entry{
		Timestamp: olderThanEverything, RunID: "undo-of-run-b", Command: "apt remove nginx", Risk: "elevated", Mode: "ask", ExitCode: 0, UndoOf: "run-b",
	})

	target, ok := selectUndoTarget(entries)

	require.True(t, ok)
	assert.Equal(t, "run-a", target.RunID)
}

// TestSelectUndoTargetFindsNoneOnEmptyLog proves the honest-refusal path
// at the selection layer for a freshly-installed comrade: an empty audit
// log has no target at all.
func TestSelectUndoTargetFindsNoneOnEmptyLog(t *testing.T) {
	_, ok := selectUndoTarget(nil)
	assert.False(t, ok)
}

// TestSelectUndoTargetFindsNoneWhenOnlyLegacyEntriesExist proves the
// OTHER honest-refusal case: a log containing only pre-undo-support
// (empty RunID) entries has nothing groupable into a run at all, so
// there is no default target — task spec: "Old audit entries (empty
// RunID) are NEVER auto-undone". Note there is no analogous "every REAL
// run has been undone" case to test here: marking a run as undone always
// requires appending a NEW entry with its own fresh RunID (comrade
// undo's own steps are never legacy), and that new run is itself
// eligible — by construction, at least one eligible run always exists
// once any non-legacy entry exists in the log at all.
func TestSelectUndoTargetFindsNoneWhenOnlyLegacyEntriesExist(t *testing.T) {
	entries := []audit.Entry{
		{Timestamp: ts(0), Command: "echo legacy-1", Risk: "read", Mode: "auto", ExitCode: 0},
		{Timestamp: ts(1), Command: "echo legacy-2", Risk: "read", Mode: "auto", ExitCode: 0},
	}

	_, ok := selectUndoTarget(entries)

	assert.False(t, ok)
}

func TestFindUndoRunByIDReturnsExactMatch(t *testing.T) {
	entries := mixedFormatEntries()

	run, ok := findUndoRunByID(entries, "run-a")

	require.True(t, ok)
	assert.Equal(t, "run-a", run.RunID)
	assert.Equal(t, "make docs folder", run.Request)
}

func TestFindUndoRunByIDMissingReturnsFalse(t *testing.T) {
	entries := mixedFormatEntries()

	_, ok := findUndoRunByID(entries, "does-not-exist")

	assert.False(t, ok)
}

// TestFindUndoRunByIDAllowsAnAlreadyUndoneRun proves --run's own
// documented deliberate exception: an explicit --run <id> target is
// findable even after it has already been undone once (unlike the
// default selection rule) — the user named it explicitly.
func TestFindUndoRunByIDAllowsAnAlreadyUndoneRun(t *testing.T) {
	entries := mixedFormatEntries()
	entries = append(entries, audit.Entry{
		Timestamp: ts(5), RunID: "undo-of-run-b", Command: "apt remove nginx", Risk: "elevated", Mode: "ask", ExitCode: 0, UndoOf: "run-b",
	})

	run, ok := findUndoRunByID(entries, "run-b")

	require.True(t, ok)
	assert.Equal(t, "run-b", run.RunID)
}

func TestListUndoCandidatesOrdersNewestFirstAndRespectsLimit(t *testing.T) {
	entries := mixedFormatEntries()

	all := listUndoCandidates(entries, 0)
	require.Len(t, all, 2)
	assert.Equal(t, "run-b", all[0].RunID, "newest run first")
	assert.Equal(t, "run-a", all[1].RunID)

	limited := listUndoCandidates(entries, 1)
	require.Len(t, limited, 1)
	assert.Equal(t, "run-b", limited[0].RunID)
}

func TestListUndoCandidatesOnEmptyLogReturnsEmpty(t *testing.T) {
	candidates := listUndoCandidates(nil, 20)
	assert.Empty(t, candidates)
}
