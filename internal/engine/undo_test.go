package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// mkdirUndoTarget builds an UndoTarget for a single recorded "mkdir demo"
// step — the fixture every happy-path test in this file starts from.
func mkdirUndoTarget() UndoTarget {
	return UndoTarget{
		RunID:   "run-1",
		Request: "make a demo folder",
		Cwd:     "/home/user/project",
		Steps: []audit.Entry{
			{Command: "mkdir demo", Risk: "write", ExitCode: 0},
		},
	}
}

const mkdirUndoPlanJSON = `{
  "summary": "Removes the demo folder that was just created.",
  "steps": [
    {"command": "rmdir demo", "rationale": "Reverses the earlier mkdir demo step.", "risk": "write", "reversible": true}
  ]
}`

func TestDeriveUndoHappyPathMapsSummaryAndSteps(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: mkdirUndoPlanJSON}}}
	undoer := NewUndoer(fake, config.Default())

	plan, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	require.NoError(t, err)

	assert.Equal(t, "Removes the demo folder that was just created.", plan.Summary)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "rmdir demo", plan.Steps[0].Command)
	assert.Equal(t, safety.RiskWrite, plan.Steps[0].Risk)
	assert.Equal(t, safety.Allow, plan.Steps[0].Decision.Action)
	assert.True(t, plan.Steps[0].Decision.Evaluated)

	// The run's own context (request, cwd, recorded step) must actually
	// reach the model via the system prompt.
	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].System, "make a demo folder")
	assert.Contains(t, fake.calls[0].System, "/home/user/project")
	assert.Contains(t, fake.calls[0].System, "mkdir demo")
	assert.Equal(t, []string{"summary"}, fake.calls[0].RequiredFields,
		"only summary is required — an empty steps array is a legitimate 'nothing reversible' response")
}

// TestDeriveUndoSerializesEachStepsOwnRecordedCwdDistinctFromCurrent
// proves the fix for the "downgraded step loses its own cwd" gap: when a
// step's recorded working directory (audit.Entry.Cwd) differs from the
// CURRENT directory this undo invocation is running in (UndoTarget.Cwd —
// exactly the scenario internal/cli's buildUndoPlan downgrades to this
// LLM tier over, since a relative-path heuristic match can't be trusted
// blindly across a cwd mismatch — see undo_plan.go), the system prompt
// must carry BOTH directories distinctly, so the model can actually
// reason about the mismatch instead of only ever seeing the current one.
func TestDeriveUndoSerializesEachStepsOwnRecordedCwdDistinctFromCurrent(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: mkdirUndoPlanJSON}}}
	undoer := NewUndoer(fake, config.Default())

	target := UndoTarget{
		RunID:   "run-cwd-mismatch",
		Request: "make a demo folder",
		Cwd:     "/home/user/project-current",
		Steps: []audit.Entry{
			{Command: "mkdir demo", Risk: "write", ExitCode: 0, Cwd: "/home/user/project-recorded"},
		},
	}

	_, err := undoer.DeriveUndo(context.Background(), target)
	require.NoError(t, err)

	require.Len(t, fake.calls, 1)
	system := fake.calls[0].System
	assert.Contains(t, system, "/home/user/project-current", "the CURRENT directory must be serialized")
	assert.Contains(t, system, "/home/user/project-recorded", "the step's OWN recorded directory must be serialized separately from the current one")
}

// TestDeriveUndoEmptyStepsIsAValidHonestRefusal proves DeriveUndo never
// runs GeneratePlan's own corrective empty-steps re-prompt: a single
// response with an empty "steps" array is accepted as-is, exactly like
// Diagnose's identical design (see that function's own doc comment).
func TestDeriveUndoEmptyStepsIsAValidHonestRefusal(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"summary": "Nothing in this run can be safely reversed: the file was deleted, not created.",
		"steps": []
	}`}}}
	undoer := NewUndoer(fake, config.Default())

	plan, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	require.NoError(t, err)

	assert.Contains(t, plan.Summary, "safely reversed")
	assert.Empty(t, plan.Steps)
	assert.Len(t, fake.calls, 1, "must never re-prompt for an empty-steps undo response")
}

func TestDeriveUndoMissingSummaryIsAnError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"steps": [{"command": "rmdir demo", "rationale": "r", "risk": "write", "reversible": true}]
	}`}}}
	undoer := NewUndoer(fake, config.Default())

	_, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	assert.Error(t, err)
}

// TestDeriveUndoUnknownRiskFailsClosedToDestructive pins DeriveUndo's
// reuse of toPlan's fail-closed risk parsing: an unrecognized "risk"
// label defaults to RiskDestructive (never silently Allow) and appends
// unknownRiskNote to Summary, exactly like GeneratePlan/Diagnose.
func TestDeriveUndoUnknownRiskFailsClosedToDestructive(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"summary": "Removes the demo folder.",
		"steps": [{"command": "rmdir demo", "rationale": "r", "risk": "bogus", "reversible": true}]
	}`}}}
	undoer := NewUndoer(fake, config.Default())

	plan, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	require.NoError(t, err)

	require.Len(t, plan.Steps, 1)
	assert.Equal(t, safety.RiskDestructive, plan.Steps[0].Risk)
	assert.Equal(t, safety.Confirm, plan.Steps[0].Decision.Action)
	assert.Contains(t, plan.Summary, "unrecognized")
}

// TestDeriveUndoStepThroughSafetyEngineEscalatesElevated proves every
// derived step is run through the SAME safety.Engine.Evaluate every other
// plan step in this codebase is (CLAUDE.md's non-negotiable second
// check): a model-proposed undo step containing "sudo" must Confirm
// regardless of the risk label the model itself gave it.
func TestDeriveUndoStepThroughSafetyEngineEscalatesElevated(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"summary": "Re-disables the service that was enabled.",
		"steps": [{"command": "sudo systemctl disable docker", "rationale": "r", "risk": "write", "reversible": true}]
	}`}}}
	undoer := NewUndoer(fake, config.Default())

	plan, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	require.NoError(t, err)

	require.Len(t, plan.Steps, 1)
	assert.Equal(t, safety.RiskElevated, plan.Steps[0].Decision.EffectiveRisk,
		"sudo must escalate to elevated even though the model declared write")
	assert.Equal(t, safety.Confirm, plan.Steps[0].Decision.Action)
}

// TestDeriveUndoDenylistedStepIsBlockedRegardlessOfDeclaredRisk proves
// the OTHER half of the same non-negotiable check: a denylisted command
// the model proposed (however implausible) must Block, never Allow, even
// if the model mislabeled its risk as merely "read".
func TestDeriveUndoDenylistedStepIsBlockedRegardlessOfDeclaredRisk(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"summary": "A decoy the model must never actually produce.",
		"steps": [{"command": "rm -rf /", "rationale": "decoy", "risk": "read", "reversible": false}]
	}`}}}
	undoer := NewUndoer(fake, config.Default())

	plan, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	require.NoError(t, err)

	require.Len(t, plan.Steps, 1)
	assert.Equal(t, safety.Block, plan.Steps[0].Decision.Action)
}

func TestDeriveUndoCompleterErrorPropagates(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{err: fmt.Errorf("network unreachable")}}}
	undoer := NewUndoer(fake, config.Default())

	_, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	assert.Error(t, err)
}

// TestDeriveUndoMarkdownWrappedJSONIsExtracted proves DeriveUndo's
// response decoding goes through the same llm.ValidateInto extraction
// every other Completer call in this codebase does (fakeCompleter itself
// runs every queued response through it — see planner_test.go's own doc
// comment) — a markdown-fenced response is handled identically to a bare
// JSON one.
func TestDeriveUndoMarkdownWrappedJSONIsExtracted(t *testing.T) {
	fenced := "```json\n" + mkdirUndoPlanJSON + "\n```"
	fake := &fakeCompleter{responses: []fakeResponse{{text: fenced}}}
	undoer := NewUndoer(fake, config.Default())

	plan, err := undoer.DeriveUndo(context.Background(), mkdirUndoTarget())
	require.NoError(t, err)
	require.Len(t, plan.Steps, 1)
	assert.Equal(t, "rmdir demo", plan.Steps[0].Command)
}
