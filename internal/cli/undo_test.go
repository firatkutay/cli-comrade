package cli

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/executor"
)

// scriptedUndoPrompt is a minimal, scripted engine.PromptUI test double —
// mirroring internal/engine's own fakePrompt (unexported outside that
// package, so redeclared here) — used so this file's end-to-end tests
// exercise the REAL internal/executor against harmless, real filesystem
// operations in a t.TempDir(), without any bubbletea/terminal involved.
type scriptedUndoPrompt struct {
	choice engine.Choice
	shown  []engine.Step
}

func (p *scriptedUndoPrompt) Confirm(_ context.Context, step engine.Step) (engine.Choice, string, error) {
	p.shown = append(p.shown, step)
	return p.choice, "", nil
}

func (p *scriptedUndoPrompt) Explain(_ context.Context, _ engine.Step) (string, error) {
	return "", nil
}

// chdir changes the working directory to dir for the duration of the
// test, restoring the original one on cleanup — needed because
// internal/executor.Options has no per-call working-directory override
// (a command always inherits this PROCESS's own cwd), and this file's
// end-to-end tests need a real mkdir/rmdir to land in an isolated
// t.TempDir().
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(old) })
}

// mkdirDemoCommand/rmdirDemoCommand return this file's own recorded/
// expected "demo" directory command text in whichever dialect the REAL
// runtime.GOOS actually requires — internal/executor.New always builds a
// command for the host platform (sh on Unix, PowerShell on Windows), and
// internal/undo's own heuristic table is GOOS-gated identically, so an
// end-to-end test that must pass on every OS in CLAUDE.md's own CI
// matrix (ubuntu, macos, windows) has to speak whichever dialect is
// actually running.
func mkdirDemoCommand() string {
	if runtime.GOOS == "windows" {
		return "New-Item -ItemType Directory demo"
	}
	return "mkdir demo"
}

func rmdirDemoCommand() string {
	if runtime.GOOS == "windows" {
		return "Remove-Item demo"
	}
	return "rmdir demo"
}

// TestRunUndoCoreExecutesPureHeuristicPlanAgainstRealExecutor is this
// feature's own end-to-end proof: a recorded `mkdir demo` step (in
// whichever dialect the host OS actually uses), run through
// runUndoCore's REAL derive-then-execute pipeline — internal/undo's
// heuristic table, engine.Execute under ModeAsk, and the REAL
// internal/executor — actually removes the directory it created, with
// NO LLM call at all (the pure-heuristic fast path), confirmed via a
// scripted PromptUI fake answering [e]vet to the one confirm prompt.
func TestRunUndoCoreExecutesPureHeuristicPlanAgainstRealExecutor(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	demoPath := filepath.Join(dir, "demo")
	require.NoError(t, os.Mkdir(demoPath, 0o755))

	target := undoRun{
		RunID: "run-e2e",
		Steps: []audit.Entry{
			{Command: mkdirDemoCommand(), Risk: "write", ExitCode: 0, Cwd: dir},
		},
	}

	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	var stdout, stderr bytes.Buffer
	prompt := &scriptedUndoPrompt{choice: engine.ChoiceYes}
	deps := undoPipelineDeps{
		Prompt:     prompt,
		Executor:   executor.New(&stdout, &stderr),
		Stdout:     &stdout,
		Stderr:     &stderr,
		GOOS:       runtime.GOOS,
		CurrentCwd: dir,
	}

	outcome, err := runUndoCore(context.Background(), cfg, fake, target, false, testEnTranslator(), deps)
	require.NoError(t, err)

	assert.Equal(t, 0, fake.calls, "a fully heuristic-resolved run must never call the LLM tier")
	require.True(t, outcome.Executed)
	require.Len(t, outcome.Summary.Results, 1)
	assert.Equal(t, engine.OutcomeExecuted, outcome.Summary.Results[0].Outcome)
	assert.Equal(t, 0, outcome.Summary.Results[0].ExitCode)
	assert.Equal(t, rmdirDemoCommand(), outcome.Summary.Results[0].Command)
	require.Len(t, prompt.shown, 1, "the one derived step must be confirmed before running, exactly like any other plan step")

	_, statErr := os.Stat(demoPath)
	assert.True(t, os.IsNotExist(statErr), "the derived rmdir/Remove-Item must have actually removed the directory")
}

// TestRunUndoCoreDeclinedConfirmNeverExecutes proves ask mode's own
// non-negotiable confirm gate applies to an undo plan exactly like any
// other: answering [h]ayır (ChoiceNo) to the one derived step must leave
// the directory untouched.
func TestRunUndoCoreDeclinedConfirmNeverExecutes(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	demoPath := filepath.Join(dir, "demo")
	require.NoError(t, os.Mkdir(demoPath, 0o755))

	target := undoRun{
		RunID: "run-e2e-declined",
		Steps: []audit.Entry{
			{Command: mkdirDemoCommand(), Risk: "write", ExitCode: 0, Cwd: dir},
		},
	}

	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	var stdout, stderr bytes.Buffer
	deps := undoPipelineDeps{
		Prompt:     &scriptedUndoPrompt{choice: engine.ChoiceNo},
		Executor:   executor.New(&stdout, &stderr),
		Stdout:     &stdout,
		Stderr:     &stderr,
		GOOS:       runtime.GOOS,
		CurrentCwd: dir,
	}

	outcome, err := runUndoCore(context.Background(), cfg, fake, target, false, testEnTranslator(), deps)
	require.NoError(t, err)

	require.True(t, outcome.Executed)
	require.Len(t, outcome.Summary.Results, 1)
	assert.Equal(t, engine.OutcomeSkipped, outcome.Summary.Results[0].Outcome)

	_, statErr := os.Stat(demoPath)
	assert.NoError(t, statErr, "declining the confirm must leave the directory untouched")
}

// TestRunUndoCoreDryRunNeverExecutes proves --dry-run's own contract:
// runUndoCore must derive and return the Plan without ever reaching
// engine.Execute at all (Executed stays false, and the prompt is never
// shown), leaving the directory untouched.
func TestRunUndoCoreDryRunNeverExecutes(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)

	demoPath := filepath.Join(dir, "demo")
	require.NoError(t, os.Mkdir(demoPath, 0o755))

	target := undoRun{
		RunID: "run-e2e-dry-run",
		Steps: []audit.Entry{
			{Command: mkdirDemoCommand(), Risk: "write", ExitCode: 0, Cwd: dir},
		},
	}

	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	var stdout, stderr bytes.Buffer
	prompt := &scriptedUndoPrompt{choice: engine.ChoiceYes}
	deps := undoPipelineDeps{
		Prompt:     prompt,
		Executor:   executor.New(&stdout, &stderr),
		Stdout:     &stdout,
		Stderr:     &stderr,
		GOOS:       runtime.GOOS,
		CurrentCwd: dir,
	}

	outcome, err := runUndoCore(context.Background(), cfg, fake, target, true, testEnTranslator(), deps)
	require.NoError(t, err)

	assert.False(t, outcome.Executed)
	assert.Empty(t, prompt.shown, "dry-run must never even reach the confirm prompt")
	require.Len(t, outcome.Plan.Steps, 1)
	assert.Equal(t, rmdirDemoCommand(), outcome.Plan.Steps[0].Command)

	_, statErr := os.Stat(demoPath)
	assert.NoError(t, statErr, "dry-run must never touch the filesystem")
}

// TestRunUndoCoreNothingReversiblePropagatesSentinel proves runUndoCore
// surfaces buildUndoPlan's own errUndoNothingReversible unchanged, so
// runUndo's cobra-level RunE can render it as the translated
// MsgUndoNothingReversibleError.
func TestRunUndoCoreNothingReversiblePropagatesSentinel(t *testing.T) {
	target := undoRun{RunID: "run-nothing", Steps: []audit.Entry{
		{Command: "apt install nginx", ExitCode: 1},
	}}

	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	var stdout, stderr bytes.Buffer
	deps := undoPipelineDeps{
		Prompt:     &scriptedUndoPrompt{choice: engine.ChoiceYes},
		Executor:   executor.New(&stdout, &stderr),
		Stdout:     &stdout,
		Stderr:     &stderr,
		GOOS:       runtime.GOOS,
		CurrentCwd: t.TempDir(),
	}

	outcome, err := runUndoCore(context.Background(), cfg, fake, target, false, testEnTranslator(), deps)

	require.Error(t, err)
	assert.False(t, outcome.Executed)
	require.Len(t, outcome.Notes, 1)
	assert.Contains(t, outcome.Notes[0], "apt install nginx")
}

// TestPrintUndoCandidatesRendersHeaderAndRows proves --list's own
// rendering: every run appears with its id, step count, and request.
func TestPrintUndoCandidatesRendersHeaderAndRows(t *testing.T) {
	runs := []undoRun{
		{RunID: "run-a", Request: "make docs folder", Steps: []audit.Entry{{}, {}}},
	}

	var buf bytes.Buffer
	require.NoError(t, printUndoCandidates(&buf, runs, testEnTranslator()))

	out := buf.String()
	assert.Contains(t, out, "RUN ID")
	assert.Contains(t, out, "run-a")
	assert.Contains(t, out, "make docs folder")
}

// TestPrintUndoCandidatesOnEmptyPrintsFriendlyMessage mirrors
// `comrade history`'s own identical empty-log precedent.
func TestPrintUndoCandidatesOnEmptyPrintsFriendlyMessage(t *testing.T) {
	var buf bytes.Buffer
	require.NoError(t, printUndoCandidates(&buf, nil, testEnTranslator()))
	assert.Contains(t, buf.String(), "No recorded runs")
}

// --- full `comrade undo` cobra command end-to-end tests ------------------

// seedUndoRun appends run's steps directly to the isolated (per
// withIsolatedConfigDir) audit log, exactly like history_test.go's own
// seedAuditEntries — so these tests exercise the full cobra command
// against a realistic, pre-populated audit.jsonl rather than a live
// `comrade do` invocation.
func seedUndoRun(t *testing.T, runID string, steps ...audit.Entry) {
	t.Helper()
	path, err := audit.DefaultPath()
	require.NoError(t, err)
	logger, err := audit.NewLogger(path)
	require.NoError(t, err)
	for _, e := range steps {
		e.RunID = runID
		require.NoError(t, logger.Append(e))
	}
}

// TestUndoCommandNoTargetShowsTranslatedError proves `comrade undo` on a
// completely empty audit log fails with the friendly, translated
// MsgUndoNoTargetError rather than any raw internal error.
func TestUndoCommandNoTargetShowsTranslatedError(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, dockerKurPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	_, stderr, err := execRootSplit(t, "dev", "undo")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "No reversible run")
	_ = stderr
}

// TestUndoCommandRunNotFoundShowsTranslatedError proves `comrade undo
// --run <bad-id>` fails with MsgUndoRunNotFoundError naming the given id.
func TestUndoCommandRunNotFoundShowsTranslatedError(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, dockerKurPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedUndoRun(t, "run-real", audit.Entry{Timestamp: ts(0), Command: "echo hi", Risk: "read", Mode: "auto", ExitCode: 0})

	_, _, err := execRootSplit(t, "dev", "undo", "--run", "no-such-run")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no-such-run")
}

// TestUndoCommandDryRunRendersHeuristicDerivedPlanTable is this
// command's own end-to-end proof at the cobra layer: a seeded run whose
// one step matches internal/undo's heuristic table renders through the
// SAME renderPlan table `comrade do --dry-run` uses, with no LLM call
// (the mock plan server here only proves it was never even contacted —
// httptest.Server has no request-count assertion built in, so this test
// instead relies on buildUndoPlan's own unit-level proof
// (TestBuildUndoPlanPureHeuristicNeverCallsLLM) for the "never calls the
// LLM" guarantee, and checks here only that the RIGHT command renders).
func TestUndoCommandDryRunRendersHeuristicDerivedPlanTable(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, dockerKurPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	// Resolve dir through any symlinks BEFORE seeding it as the entry's
	// Cwd: production's own currentCwd (internal/cli/undo.go's os.Getwd()
	// call, after this same os.Chdir) always comes back symlink-resolved
	// (e.g. macOS returns t.TempDir()'s /var/... as /private/var/... once
	// os.Chdir has landed in it), so an unresolved dir here would spuriously
	// mismatch e.Cwd against currentCwd on such platforms, wrongly
	// triggering the cwd-mismatch downgrade this test does not intend to
	// exercise (see deriveStepsForRun's UsesRelativePath branch above).
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	seedUndoRun(t, "run-heuristic", audit.Entry{
		Timestamp: ts(0), Command: mkdirDemoCommand(), Risk: "write", Mode: "ask", ExitCode: 0, Cwd: dir,
	})

	oldWd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	t.Cleanup(func() { _ = os.Chdir(oldWd) })

	stdout, stderr, err := execRootSplit(t, "dev", "undo", "--run", "run-heuristic", "--dry-run")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, rmdirDemoCommand())
}

// TestUndoCommandIsVisibleInHelp proves undo is registered as a real,
// user-facing Core-group command (see root.go).
func TestUndoCommandIsVisibleInHelp(t *testing.T) {
	withIsolatedConfigDir(t)
	out := execRoot(t, "dev", "--help")
	assert.Contains(t, out, "undo")
}
