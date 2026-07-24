package cli

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// fakeUndoCompleter is a minimal engine.Completer test double for this
// file: it records every call and answers from one fixed response,
// exactly like internal/engine's own fakeCompleter (redeclared here,
// package-local, since engine's is unexported outside that package).
type fakeUndoCompleter struct {
	calls    int
	response string
	err      error
}

func (f *fakeUndoCompleter) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	f.calls++
	if f.err != nil {
		return llm.CompletionResponse{}, f.err
	}
	doc, err := llm.ValidateInto(f.response, req.RequiredFields, nil)
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	return llm.CompletionResponse{Text: f.response, JSON: doc}, nil
}

func testEnTranslator() i18n.Translator {
	return i18n.NewTranslator(i18n.LangEN)
}

func mkdirRun() undoRun {
	return undoRun{
		RunID:   "run-1",
		Request: "make a demo folder",
		Steps: []audit.Entry{
			{Command: "mkdir demo", Risk: "write", ExitCode: 0, Cwd: "/home/user/project"},
		},
	}
}

func TestDeriveStepsForRunSkipsNonzeroExit(t *testing.T) {
	run := undoRun{Steps: []audit.Entry{
		{Command: "apt install nginx", ExitCode: 1},
	}}

	steps := deriveStepsForRun(run, "linux", "/home/user/project")

	require.Len(t, steps, 1)
	assert.True(t, steps[0].skipped)
	assert.Empty(t, steps[0].commands)
}

func TestDeriveStepsForRunUsesHeuristicWhenCwdMatches(t *testing.T) {
	run := undoRun{Steps: []audit.Entry{
		{Command: "mkdir demo", ExitCode: 0, Cwd: "/home/user/project"},
	}}

	steps := deriveStepsForRun(run, "linux", "/home/user/project")

	require.Len(t, steps, 1)
	assert.False(t, steps[0].skipped)
	assert.False(t, steps[0].downgraded)
	assert.Equal(t, []string{"rmdir demo"}, steps[0].commands)
}

// TestDeriveStepsForRunDowngradesRelativePathCwdMismatch proves the
// cwd-mismatch safety rule: a relative-path heuristic match recorded in a
// DIFFERENT working directory than the one undo is currently running in
// must never be trusted blindly.
func TestDeriveStepsForRunDowngradesRelativePathCwdMismatch(t *testing.T) {
	run := undoRun{Steps: []audit.Entry{
		{Command: "mkdir demo", ExitCode: 0, Cwd: "/home/user/project-a"},
	}}

	steps := deriveStepsForRun(run, "linux", "/home/user/project-b")

	require.Len(t, steps, 1)
	assert.True(t, steps[0].downgraded)
	assert.Empty(t, steps[0].commands, "a downgraded step must never carry a trusted command")
}

// TestDeriveStepsForRunAbsolutePathIsNeverDowngraded proves the OTHER
// half of the same rule: an absolute-path heuristic match is trusted
// regardless of any cwd mismatch, since it does not depend on the
// current directory at all.
func TestDeriveStepsForRunAbsolutePathIsNeverDowngraded(t *testing.T) {
	run := undoRun{Steps: []audit.Entry{
		{Command: "mkdir /tmp/demo", ExitCode: 0, Cwd: "/home/user/project-a"},
	}}

	steps := deriveStepsForRun(run, "linux", "/home/user/project-b")

	require.Len(t, steps, 1)
	assert.False(t, steps[0].downgraded)
	assert.Equal(t, []string{"rmdir /tmp/demo"}, steps[0].commands)
}

// TestBuildUndoPlanPureHeuristicNeverCallsLLM proves the fast/deterministic
// path: when every eligible step resolves via internal/undo's heuristic
// table, buildUndoPlan never calls the Completer at all — nothing about
// the run leaves the machine.
func TestBuildUndoPlanPureHeuristicNeverCallsLLM(t *testing.T) {
	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	plan, notes, err := buildUndoPlan(context.Background(), undoer, safetyEngine, mkdirRun(), "linux", "/home/user/project", testEnTranslator())
	require.NoError(t, err)

	assert.Equal(t, 0, fake.calls, "a fully heuristic-resolved run must never call the LLM tier")
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "rmdir demo", plan.Steps[0].Command)
	assert.Equal(t, safety.Allow, plan.Steps[0].Decision.Action)
	assert.Empty(t, notes)
}

// TestBuildUndoPlanFallsBackToLLMWhenHeuristicMisses proves the second
// trust tier: a step internal/undo does not recognize at all falls
// through to Undoer.DeriveUndo for the WHOLE run (see buildUndoPlan's own
// doc comment on the deliberate all-or-nothing-per-run design), with a
// translated fallback note.
func TestBuildUndoPlanFallsBackToLLMWhenHeuristicMisses(t *testing.T) {
	fake := &fakeUndoCompleter{response: `{
		"summary": "Deletes the file that curl created.",
		"steps": [{"command": "rm downloaded.txt", "rationale": "undo the curl download", "risk": "write", "reversible": true}]
	}`}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	run := undoRun{RunID: "run-2", Request: "download a file", Steps: []audit.Entry{
		{Command: "curl -o downloaded.txt https://example.com/f", ExitCode: 0, Cwd: "/home/user/project"},
	}}

	plan, notes, err := buildUndoPlan(context.Background(), undoer, safetyEngine, run, "linux", "/home/user/project", testEnTranslator())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.calls)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "rm downloaded.txt", plan.Steps[0].Command)
	require.NotEmpty(t, notes)
	assert.Contains(t, notes[len(notes)-1], "model")
}

// TestBuildUndoPlanDowngradedStepTriggersLLMFallbackWithNote proves the
// cwd-mismatch downgrade actually reaches buildUndoPlan's own tier
// decision (not just deriveStepsForRun in isolation): a downgraded step
// forces the whole run to the LLM tier, and the downgrade note is
// present alongside the generic fallback note.
func TestBuildUndoPlanDowngradedStepTriggersLLMFallbackWithNote(t *testing.T) {
	fake := &fakeUndoCompleter{response: `{
		"summary": "Cannot safely determine the right path to remove.",
		"steps": []
	}`}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	run := undoRun{RunID: "run-3", Steps: []audit.Entry{
		{Command: "mkdir demo", ExitCode: 0, Cwd: "/home/user/project-a"},
	}}

	_, notes, err := buildUndoPlan(context.Background(), undoer, safetyEngine, run, "linux", "/home/user/project-b", testEnTranslator())
	require.NoError(t, err)

	assert.Equal(t, 1, fake.calls)
	require.Len(t, notes, 2)
	assert.Contains(t, notes[0], "/home/user/project-a")
	assert.Contains(t, notes[0], "/home/user/project-b")
}

// TestBuildUndoPlanNothingReversibleReturnsSentinelWithoutCallingLLM
// proves the honest-refusal shortcut: when every step in the run failed
// (nonzero exit code), buildUndoPlan returns errUndoNothingReversible
// without ever asking the LLM tier about a run with nothing eligible
// left in it.
func TestBuildUndoPlanNothingReversibleReturnsSentinelWithoutCallingLLM(t *testing.T) {
	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	run := undoRun{RunID: "run-4", Steps: []audit.Entry{
		{Command: "apt install nginx", ExitCode: 1},
	}}

	_, notes, err := buildUndoPlan(context.Background(), undoer, safetyEngine, run, "linux", "/home/user/project", testEnTranslator())

	assert.True(t, errors.Is(err, errUndoNothingReversible))
	assert.Equal(t, 0, fake.calls)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "apt install nginx")
}

// TestBuildUndoPlanSkippedStepStillAllowsHeuristicPlanForTheRest proves a
// partially-failed run (one nonzero-exit step, one genuinely reversible
// one) still produces a pure heuristic plan for the step that DID take
// effect, with a skip note for the one that didn't.
func TestBuildUndoPlanSkippedStepStillAllowsHeuristicPlanForTheRest(t *testing.T) {
	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	run := undoRun{RunID: "run-5", Steps: []audit.Entry{
		{Command: "mkdir demo", ExitCode: 0, Cwd: "/home/user/project"},
		{Command: "apt install nginx", ExitCode: 1},
	}}

	plan, notes, err := buildUndoPlan(context.Background(), undoer, safetyEngine, run, "linux", "/home/user/project", testEnTranslator())
	require.NoError(t, err)

	assert.Equal(t, 0, fake.calls)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "rmdir demo", plan.Steps[0].Command)
	require.Len(t, notes, 1)
	assert.Contains(t, notes[0], "apt install nginx")
}

// TestBuildUndoPlanSurfacesHeuristicCaveatInRationale proves
// internal/undo.Derived.Caveat is actually surfaced to the user rather
// than silently dropped: the Windows New-Item->Remove-Item rule's own
// "only removes an empty directory" caveat must appear in the derived
// step's Rationale (rendered by both `--dry-run`'s table and the
// ask-mode confirm prompt — see buildUndoPlan's own doc comment on why
// Rationale is the right place for this).
func TestBuildUndoPlanSurfacesHeuristicCaveatInRationale(t *testing.T) {
	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	run := undoRun{RunID: "run-caveat", Steps: []audit.Entry{
		{Command: `New-Item -ItemType Directory demo`, ExitCode: 0, Cwd: `C:\Users\me\project`},
	}}

	plan, _, err := buildUndoPlan(context.Background(), undoer, safetyEngine, run, "windows", `C:\Users\me\project`, testEnTranslator())
	require.NoError(t, err)

	assert.Equal(t, 0, fake.calls, "the Windows New-Item rule must still resolve purely via the heuristic table")
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "Remove-Item demo", plan.Steps[0].Command)
	assert.Contains(t, plan.Steps[0].Rationale, "Reverses: New-Item -ItemType Directory demo")
	assert.Contains(t, plan.Steps[0].Rationale, "only removes the directory if it is still empty")
}

// TestBuildUndoPlanReversesStepsNewestFirst proves buildUndoPlan's own
// reverse-order contract for the pure-heuristic path: a two-step run
// (mkdir a, mkdir b) must reverse b before a.
func TestBuildUndoPlanReversesStepsNewestFirst(t *testing.T) {
	fake := &fakeUndoCompleter{}
	cfg := config.Default()
	undoer := engine.NewUndoer(fake, cfg)
	safetyEngine := safety.NewEngine(cfg)

	run := undoRun{RunID: "run-6", Steps: []audit.Entry{
		{Command: "mkdir a", ExitCode: 0, Cwd: "/home/user/project"},
		{Command: "mkdir b", ExitCode: 0, Cwd: "/home/user/project"},
	}}

	plan, _, err := buildUndoPlan(context.Background(), undoer, safetyEngine, run, "linux", "/home/user/project", testEnTranslator())
	require.NoError(t, err)

	require.Len(t, plan.Steps, 2)
	assert.Equal(t, "rmdir b", plan.Steps[0].Command, "the LAST mkdir must be undone FIRST")
	assert.Equal(t, "rmdir a", plan.Steps[1].Command)
}
