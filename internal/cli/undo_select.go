package cli

import (
	"sort"

	"github.com/firatkutay/cli-comrade/internal/audit"
)

// undoRun groups one distinct RunID's recorded audit.Entry steps —
// internal/audit.Logger.ReadAll's own oldest-first ordering, preserved
// exactly (Steps[0] is the run's FIRST executed step) — plus that run's
// own Request and the timestamp of its most recent step (Latest), used
// to rank candidate runs newest-first.
type undoRun struct {
	RunID   string
	Request string
	Steps   []audit.Entry
	Latest  audit.Entry
}

// groupUndoRuns partitions entries (audit.Logger.ReadAll's oldest-first
// output) into one undoRun per distinct non-empty RunID, preserving each
// run's own step order, and separately counts every entry with an empty
// RunID (legacyCount) — a pre-undo-support recording (see
// audit.Entry.RunID's own doc comment) that can never be grouped into a
// run at all, and so is never eligible for automatic undo (task spec:
// "Old audit entries (empty RunID) are NEVER auto-undone").
func groupUndoRuns(entries []audit.Entry) (runs []undoRun, legacyCount int) {
	index := make(map[string]int)
	for _, e := range entries {
		if e.RunID == "" {
			legacyCount++
			continue
		}
		if i, ok := index[e.RunID]; ok {
			runs[i].Steps = append(runs[i].Steps, e)
			runs[i].Latest = e
			continue
		}
		index[e.RunID] = len(runs)
		runs = append(runs, undoRun{RunID: e.RunID, Request: e.Request, Steps: []audit.Entry{e}, Latest: e})
	}
	return runs, legacyCount
}

// alreadyUndoneRunIDs returns the set of RunIDs some entry in entries
// names as its own UndoOf — i.e. a run `comrade undo` has already acted
// on (see audit.Entry.UndoOf's own doc comment). This scans the WHOLE
// log, not just one run's own steps, since the entries recording an
// earlier undo carry a DIFFERENT RunID (the undo invocation's own) with
// UndoOf pointing back at the run it undid.
func alreadyUndoneRunIDs(entries []audit.Entry) map[string]bool {
	done := make(map[string]bool)
	for _, e := range entries {
		if e.UndoOf != "" {
			done[e.UndoOf] = true
		}
	}
	return done
}

// selectUndoTarget picks the newest eligible run — RunID non-empty, at
// least one recorded step, and not already the target of a prior undo —
// exactly the default-target rule the task spec defines. A tie in Latest
// timestamp resolves to whichever run groupUndoRuns encountered LAST
// (i.e. most recently appended to the audit log), since ReadAll's own
// ordering guarantee makes "later in entries" already mean "more recent"
// even at identical-timestamp resolution.
func selectUndoTarget(entries []audit.Entry) (undoRun, bool) {
	runs, _ := groupUndoRuns(entries)
	done := alreadyUndoneRunIDs(entries)

	var best undoRun
	found := false
	for _, r := range runs {
		if done[r.RunID] {
			continue
		}
		if !found || !r.Latest.Timestamp.Before(best.Latest.Timestamp) {
			best = r
			found = true
		}
	}
	return best, found
}

// findUndoRunByID looks up one specific run by its RunID — `comrade undo
// --run <id>`'s own target selection, which (unlike selectUndoTarget)
// deliberately does NOT check alreadyUndoneRunIDs: a user explicitly
// naming a run by id is allowed to re-derive (and re-confirm) an undo
// plan for it even if it was undone before, since --dry-run alone is
// harmless and even actually re-running it is the user's own explicit,
// informed choice, not an ambiguous "pick one for me" default.
func findUndoRunByID(entries []audit.Entry, runID string) (undoRun, bool) {
	runs, _ := groupUndoRuns(entries)
	for _, r := range runs {
		if r.RunID == runID {
			return r, true
		}
	}
	return undoRun{}, false
}

// listUndoCandidates returns the last n grouped runs (every run,
// regardless of eligibility — `comrade undo --list` is a plain read-only
// view, not a filtered eligibility check), most-recent-first, mirroring
// internal/cli/history.go's own lastN precedent. n <= 0 returns every
// run.
func listUndoCandidates(entries []audit.Entry, n int) []undoRun {
	runs, _ := groupUndoRuns(entries)
	sort.SliceStable(runs, func(i, j int) bool {
		return runs[i].Latest.Timestamp.After(runs[j].Latest.Timestamp)
	})
	if n > 0 && n < len(runs) {
		runs = runs[:n]
	}
	return runs
}
