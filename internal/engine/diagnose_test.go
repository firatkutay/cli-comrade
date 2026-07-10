package engine

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// pytonDiagnosisJSON is the canned model response for a "command not
// found" scenario (a typo'd "pyton" instead of "python3") — the exact
// acceptance scenario docs/history/UYGULAMA_PLANI.md FAZ 7 names ("pyton --version").
const pytonDiagnosisJSON = `{
  "root_cause": "The command \"pyton\" does not exist; it is a typo for python3, which is also not installed.",
  "explanation": "Your computer doesn't recognize pyton. It's probably a typo for python3, and that isn't installed yet either.",
  "plan": {
    "summary": "Install python3, then check its version.",
    "steps": [
      {"command": "sudo apt-get install -y python3", "rationale": "Installs Python 3 using the detected package manager.", "risk": "elevated", "reversible": false},
      {"command": "python3 --version", "rationale": "Confirms python3 now works.", "risk": "read", "reversible": true}
    ]
  }
}`

func pytonErrorContext() ErrorContext {
	return ErrorContext{
		Command:  "pyton --version",
		ExitCode: 127,
		Stderr:   "sh: 1: pyton: not found",
		System:   testSysCtx(),
	}
}

func TestDiagnoseHappyPathMapsRootCauseExplanationPlan(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: pytonDiagnosisJSON}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	diag, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	require.NoError(t, err)

	assert.Contains(t, diag.RootCause, "pyton")
	assert.Contains(t, diag.Explanation, "python3")

	require.Len(t, diag.Plan.Steps, 2)
	assert.Equal(t, "sudo apt-get install -y python3", diag.Plan.Steps[0].Command)
	assert.Equal(t, safety.RiskElevated, diag.Plan.Steps[0].Risk)
	assert.Equal(t, safety.Confirm, diag.Plan.Steps[0].Decision.Action, "elevated steps must Confirm, per the same safety.Engine plan generation uses")
	assert.Equal(t, safety.Allow, diag.Plan.Steps[1].Decision.Action)

	// The failing command's own context (command, exit code, stderr, and
	// the "Detected package managers" grounding line) must actually reach
	// the model via the system prompt — proving serializeErrorContext
	// wired ErrorContext into the request, not just Diagnose's own return
	// value.
	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].System, "pyton --version")
	assert.Contains(t, fake.calls[0].System, "127")
	assert.Contains(t, fake.calls[0].System, "not found")
	assert.Contains(t, fake.calls[0].System, "Detected package managers: apt")
	assert.Equal(t, []string{"root_cause", "explanation", "plan"}, fake.calls[0].RequiredFields)
}

func TestDiagnoseMissingRootCauseIsAnError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"explanation": "something went wrong",
		"plan": {"summary": "fix it", "steps": [{"command": "ls", "rationale": "r", "risk": "read", "reversible": true}]}
	}`}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	_, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	assert.Error(t, err)
}

func TestDiagnoseMissingPlanIsAnError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"root_cause": "some cause",
		"explanation": "some explanation",
		"plan": {}
	}`}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	_, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	assert.Error(t, err)
}

// TestDiagnoseFencedJSONIsHandled is the "real parse path" smoke test:
// Diagnose must handle a response wrapped in a markdown code fence
// exactly like every other JSON-schema request in this package, since it
// flows through the exact same llm.ValidateInto/ExtractJSON machinery
// (internal/llm/parse.go) — this is not reimplemented per-prompt.
func TestDiagnoseFencedJSONIsHandled(t *testing.T) {
	fenced := "```json\n" + pytonDiagnosisJSON + "\n```"
	fake := &fakeCompleter{responses: []fakeResponse{{text: fenced}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	diag, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	require.NoError(t, err)
	assert.Contains(t, diag.RootCause, "pyton")
	require.Len(t, diag.Plan.Steps, 2)
}

// TestDiagnoseEmptyStepsIsValidNotAnError proves Diagnose does NOT run
// GeneratePlan's own empty-steps corrective re-prompt: a diagnosis that
// legitimately found nothing to fix is a valid response shape here, per
// prompts/diagnose_system.txt's own "return an empty steps array" escape
// hatch.
func TestDiagnoseEmptyStepsIsValidNotAnError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"root_cause": "the command actually succeeded",
		"explanation": "Looking at the output, this command did not actually fail.",
		"plan": {"summary": "nothing to fix", "steps": []}
	}`}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	diag, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	require.NoError(t, err)
	assert.Empty(t, diag.Plan.Steps)
	require.Len(t, fake.calls, 1, "must not issue any corrective re-prompt")
}

// TestDiagnoseUnknownRiskLabelFailsClosedToDestructive proves Diagnose
// reuses toPlan's fail-closed unknown-risk handling verbatim (same as
// GeneratePlan's own TestGeneratePlanUnknownRiskLabelFailsClosedToDestructive).
func TestDiagnoseUnknownRiskLabelFailsClosedToDestructive(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: `{
		"root_cause": "cause",
		"explanation": "explanation",
		"plan": {"summary": "fix", "steps": [{"command": "ls", "rationale": "r", "risk": "catastrophic", "reversible": true}]}
	}`}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	diag, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	require.NoError(t, err)
	require.Len(t, diag.Plan.Steps, 1)
	assert.Equal(t, safety.RiskDestructive, diag.Plan.Steps[0].Risk)
	assert.Contains(t, diag.Plan.Summary, "unrecognized risk label")
}

// TestDiagnoseUsesTurkishLanguageBlockWhenConfigured proves
// buildDiagnoseSystemPrompt appends diagnoseLangTR (not just plan
// generation's planLangTR) when general.language="tr".
func TestDiagnoseUsesTurkishLanguageBlockWhenConfigured(t *testing.T) {
	cfg := config.Default()
	cfg.General.Language = "tr"
	fake := &fakeCompleter{responses: []fakeResponse{{text: pytonDiagnosisJSON}}}
	diagnoser := NewDiagnoser(fake, cfg)

	_, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	require.NoError(t, err)

	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].System, "Dil talimatı")
}

// TestDiagnoseUnknownExitCodeRendersAsUnknown proves ErrorContext's -1
// "unknown exit code" sentinel (internal/cli's paste-mode fallback never
// observes a real one) renders as "unknown" in the prompt, not as -1.
func TestDiagnoseUnknownExitCodeRendersAsUnknown(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{text: pytonDiagnosisJSON}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	errCtx := pytonErrorContext()
	errCtx.ExitCode = -1

	_, err := diagnoser.Diagnose(context.Background(), errCtx)
	require.NoError(t, err)

	require.Len(t, fake.calls, 1)
	assert.Contains(t, fake.calls[0].System, "Exit code: unknown")
}

func TestDiagnosePropagatesCompleteError(t *testing.T) {
	fake := &fakeCompleter{responses: []fakeResponse{{err: fmt.Errorf("network unreachable")}}}
	diagnoser := NewDiagnoser(fake, config.Default())

	_, err := diagnoser.Diagnose(context.Background(), pytonErrorContext())
	assert.Error(t, err)
}
