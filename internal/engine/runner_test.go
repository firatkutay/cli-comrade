package engine

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// --- test fakes -------------------------------------------------------

// fakeExecutor records every command it was asked to run and answers from
// a scripted per-call function; it never actually execs anything.
type fakeExecutor struct {
	mu      sync.Mutex
	calls   []string
	respond func(callIndex int, command string) (executor.Result, error)
	// blockUntilCancel, when true, makes Run block until ctx is done and
	// then return a Canceled result — the Ctrl-C scenario.
	blockUntilCancel bool
}

func (f *fakeExecutor) Run(ctx context.Context, command string, _ executor.Options) (executor.Result, error) {
	f.mu.Lock()
	f.calls = append(f.calls, command)
	idx := len(f.calls) - 1
	f.mu.Unlock()

	if f.blockUntilCancel {
		<-ctx.Done()
		return executor.Result{ExitCode: -1, Canceled: true}, nil
	}
	if f.respond != nil {
		return f.respond(idx, command)
	}
	return executor.Result{ExitCode: 0}, nil
}

func (f *fakeExecutor) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeExecutor) commandAt(i int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[i]
}

// promptResponse is one scripted answer fakePrompt.Confirm gives, in order.
type promptResponse struct {
	choice Choice
	edited string
	err    error
}

// fakePrompt is a scripted engine.PromptUI: each Confirm call consumes the
// next promptResponse in order (failing the test via a returned error if
// the script runs out), and records every Step it was shown.
type fakePrompt struct {
	mu           sync.Mutex
	responses    []promptResponse
	idx          int
	shown        []Step
	explainText  string
	explainErr   error
	explainCalls int
}

func (f *fakePrompt) Confirm(_ context.Context, step Step) (Choice, string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.shown = append(f.shown, step)
	if f.idx >= len(f.responses) {
		return ChoiceNo, "", fmt.Errorf("fakePrompt: no more scripted responses (call %d)", f.idx+1)
	}
	r := f.responses[f.idx]
	f.idx++
	return r.choice, r.edited, r.err
}

func (f *fakePrompt) Explain(_ context.Context, _ Step) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.explainCalls++
	return f.explainText, f.explainErr
}

func (f *fakePrompt) shownCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.shown)
}

// fakeCorrectionCompleter is a scripted engine.Completer for self-correction
// requests: each Complete call consumes the next respond function.
type fakeCorrectionCompleter struct {
	mu      sync.Mutex
	calls   int
	respond func(callIndex int) (llm.CompletionResponse, error)
}

func (f *fakeCorrectionCompleter) Complete(_ context.Context, _ llm.CompletionRequest) (llm.CompletionResponse, error) {
	f.mu.Lock()
	idx := f.calls
	f.calls++
	f.mu.Unlock()
	return f.respond(idx)
}

func (f *fakeCorrectionCompleter) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls
}

// correctionResponseJSON builds a llm.CompletionResponse whose JSON field
// is already populated (bypassing llm.ValidateInto, which is
// internal/llm.Client's own concern, not Completer's) — Complete's callers
// in this package only ever look at resp.JSON.
func correctionResponseJSON(t *testing.T, command, risk string) llm.CompletionResponse {
	t.Helper()
	body, err := json.Marshal(correctionResponse{Command: command, Rationale: "corrected", Risk: risk, Reversible: true})
	require.NoError(t, err)
	return llm.CompletionResponse{Text: string(body), JSON: body}
}

// fakeAudit records every audit.Entry appended to it.
type fakeAudit struct {
	mu      sync.Mutex
	entries []audit.Entry
}

func (f *fakeAudit) Append(e audit.Entry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}

func (f *fakeAudit) entryCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.entries)
}

// --- helpers -----------------------------------------------------------

func testSafetyEngine(t *testing.T) *safety.Engine {
	t.Helper()
	return safety.NewEngine(config.Default())
}

func allowStep(command string, risk safety.RiskClass) Step {
	return Step{
		Command: command,
		Risk:    risk,
		Decision: safety.Decision{
			Action:        safety.Allow,
			EffectiveRisk: risk,
			Evaluated:     true,
		},
	}
}

func confirmStep(command string, risk safety.RiskClass) Step {
	return Step{
		Command: command,
		Risk:    risk,
		Decision: safety.Decision{
			Action:        safety.Confirm,
			EffectiveRisk: risk,
			Reason:        "requires confirmation",
			Evaluated:     true,
		},
	}
}

func blockStep() Step {
	return Step{
		Command: "rm -rf /",
		Risk:    safety.RiskRead,
		Decision: safety.Decision{
			Action:        safety.Block,
			EffectiveRisk: safety.RiskDestructive,
			Reason:        "matches denylist rule: rm -rf / (or ~ / $HOME root delete)",
			Evaluated:     true,
		},
	}
}

func baseDeps(t *testing.T, exec *fakeExecutor, prompt *fakePrompt, completer *fakeCorrectionCompleter, aud *fakeAudit) (RunDeps, *bytes.Buffer) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	deps := RunDeps{
		Executor:           exec,
		Safety:             testSafetyEngine(t),
		LLM:                completer,
		Prompt:             prompt,
		Audit:              aud,
		Stdout:             &stdout,
		Stderr:             &stderr,
		ColorEnabled:       false,
		ConfirmDestructive: true,
		ConfirmElevated:    true,
		Request:            "test request",
		Now:                func() time.Time { return time.Date(2026, 7, 8, 0, 0, 0, 0, time.UTC) },
	}
	return deps, &stdout
}

// --- info mode -----------------------------------------------------------

func TestExecuteInfoModeExecutesNothing(t *testing.T) {
	exec := &fakeExecutor{}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{
		Summary: "Test plan.",
		Steps: []Step{
			allowStep("ls -la", safety.RiskRead),
			blockStep(),
		},
	}

	summary, err := Execute(context.Background(), plan, ModeInfo, deps)
	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "info mode must never execute anything")
	assert.Empty(t, summary.Results)
	assert.Contains(t, stdout.String(), "ls -la")
	assert.Contains(t, stdout.String(), "BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete))")
}

// --- ask mode --------------------------------------------------------------

func TestExecuteAskYesRunsStep(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceYes}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("echo hi", safety.RiskRead)}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, "echo hi", exec.commandAt(0))
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeExecuted, summary.Results[0].Outcome)
	assert.False(t, summary.Aborted)
}

func TestExecuteAskNoSkipsStepWithoutRunning(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceNo}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("rm somefile", safety.RiskWrite)}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount())
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeSkipped, summary.Results[0].Outcome)
	assert.False(t, summary.Aborted)
}

func TestExecuteAskExplainPrintsThenReprompts(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceExplain},
			{choice: ChoiceYes},
		},
		explainText: "this command lists files in the current directory",
	}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("ls", safety.RiskRead)}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, prompt.explainCalls)
	assert.Contains(t, stdout.String(), "this command lists files in the current directory")
	assert.Equal(t, 2, prompt.shownCount(), "the prompt must be re-shown after explain")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeExecuted, summary.Results[0].Outcome)
}

// TestExecuteAskEditThenBlockRefusesAndReprompts is the ask-mode edit
// security invariant: an edited command is RE-EVALUATED by safety before
// ever running — editing a benign command into `rm -rf /` must be
// Blocked, never executed, and the user re-prompted rather than the loop
// silently proceeding. Declining ([h]ayır) the re-prompt records the
// step as Blocked (not merely Skipped) — the command was genuinely
// unsafe, not just declined; see the caller-level re-check this pins
// (post-review hardening: TestExecuteAskEditThenYesOnBlockedEditNeverRuns
// covers the actual bypass this test alone did not catch).
func TestExecuteAskEditThenBlockRefusesAndReprompts(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceEdit, edited: "rm -rf /"},
			{choice: ChoiceNo},
		},
	}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("echo hi", safety.RiskRead)}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "the blocked edited command must never run")
	assert.Contains(t, stdout.String(), "BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete)): rm -rf /")
	assert.Equal(t, 2, prompt.shownCount(), "must re-prompt after refusing the blocked edit")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome, "a since-Blocked edit is recorded Blocked, not merely Skipped, regardless of the final choice")
}

// TestExecuteAskEditThenYesOnBlockedEditNeverRuns is the regression test
// for the CRITICAL denylist bypass an independent security review found:
// resolveAskChoice's [d]üzenle case re-evaluates safety and prints
// BLOCKED, then re-loops — but the ORIGINAL bug let a subsequent [e]vet
// on that same, still-Blocked step fall through the `default:` case and
// run anyway, because only the plan's *original* Decision was ever
// re-checked, never the one produced by the edit. This must now be
// caught at the caller: the executor must never run.
func TestExecuteAskEditThenYesOnBlockedEditNeverRuns(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceEdit, edited: "rm -rf /"},
			{choice: ChoiceYes},
		},
	}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("echo hi", safety.RiskRead)}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "pressing [e]vet on a since-Blocked edit must never run it")
	assert.Contains(t, stdout.String(), "BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete)): rm -rf /")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
}

// TestExecuteAskEditThenAllOnBlockedEditNeverRuns is the same regression
// for the [t]ümü path specifically — the review's fix description calls
// this out separately since [t]ümü also sets autoApproveRemaining, which
// must NOT happen for a step that turned out Blocked (asserted here via a
// second, later step that must still be individually prompted, proving
// no "approve all remaining" state leaked out of the blocked edit).
func TestExecuteAskEditThenAllOnBlockedEditNeverRuns(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceEdit, edited: "rm -rf /"},
			{choice: ChoiceAll},
			{choice: ChoiceYes},
		},
	}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{
		allowStep("echo hi", safety.RiskRead),
		allowStep("echo two", safety.RiskWrite),
	}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	require.Equal(t, 1, exec.callCount(), "only the second step's own explicit [e]vet must have run anything")
	assert.Equal(t, "echo two", exec.commandAt(0))
	assert.Contains(t, stdout.String(), "BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete)): rm -rf /")
	assert.Equal(t, 3, prompt.shownCount(), "the second step must still be individually prompted — [t]ümü on a Blocked edit must not auto-approve remaining steps")
	require.Len(t, summary.Results, 2)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
	assert.Equal(t, OutcomeExecuted, summary.Results[1].Outcome)
}

// TestExecuteAutoEditThenYesOnBlockedEditNeverRuns is the auto-mode
// equivalent: a destructive step drops into resolveAutoGate's confirm
// loop (the exact same resolveAskChoice), gets edited into `rm -rf /`,
// and a subsequent [e]vet must still never reach the executor. Auto
// mode's own abort-on-block design decision applies identically here.
func TestExecuteAutoEditThenYesOnBlockedEditNeverRuns(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceEdit, edited: "rm -rf /"},
			{choice: ChoiceYes},
		},
	}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{confirmStep("rm -rf ./build", safety.RiskDestructive)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "pressing [e]vet on a since-Blocked edit must never run it, even in auto mode")
	assert.Contains(t, stdout.String(), "BLOCKED(matches denylist rule: rm -rf / (or ~ / $HOME root delete)): rm -rf /")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
	assert.True(t, summary.Aborted)
	assert.Contains(t, summary.AbortReason, "blocked")
}

// TestExecuteAutoEditThenAllOnBlockedEditNeverRuns is the auto-mode
// [t]ümü variant of the same regression.
func TestExecuteAutoEditThenAllOnBlockedEditNeverRuns(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceEdit, edited: "rm -rf /"},
			{choice: ChoiceAll},
		},
	}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{confirmStep("sudo rm -rf ./build", safety.RiskElevated)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "[t]ümü on a since-Blocked edit must never run it")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
	assert.True(t, summary.Aborted)
}

func TestExecuteAskEditThenYesRunsEditedCommand(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceEdit, edited: "apt-get install nginx"},
			{choice: ChoiceYes},
		},
	}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("apt-get install docker.io", safety.RiskWrite)}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, "apt-get install nginx", exec.commandAt(0))
	require.Len(t, summary.Results, 1)
	assert.Equal(t, "apt-get install nginx", summary.Results[0].Command)
}

// TestExecuteAskAllSkipsPromptForLowRiskButNotForDestructive pins ask
// mode's [t]ümü semantics exactly: it approves the current step and every
// remaining read/write/network step without asking again, but a later
// destructive/elevated step still prompts individually.
func TestExecuteAskAllSkipsPromptForLowRiskButNotForDestructive(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{
		responses: []promptResponse{
			{choice: ChoiceAll}, // step 1 (write) — approves this + sets auto-approve
			{choice: ChoiceYes}, // step 3 (destructive) — still prompted individually
		},
	}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{
		allowStep("mkdir foo", safety.RiskWrite),
		allowStep("curl example.com", safety.RiskNetwork),
		confirmStep("rm -rf ./build", safety.RiskDestructive),
	}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	require.Equal(t, 3, exec.callCount(), "all three steps must run")
	assert.Equal(t, 2, prompt.shownCount(), "step 2 (network, low-risk) must not be prompted after [t]ümü; the destructive step 3 must still be prompted")
	assert.False(t, summary.Aborted)
}

// TestExecuteBlockNeverExecutesInAskMode is one half of the "Block NEVER
// executes in ANY mode" security invariant.
func TestExecuteBlockNeverExecutesInAskMode(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{} // no responses scripted: Confirm must never even be called
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{blockStep()}}
	summary, err := Execute(context.Background(), plan, ModeAsk, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount())
	assert.Equal(t, 0, prompt.shownCount(), "a Blocked step must never even reach the confirm prompt")
	assert.Contains(t, stdout.String(), "BLOCKED(")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
}

// --- auto mode -------------------------------------------------------------

func TestExecuteAutoRunsLowRiskStepsSequentiallyWithoutPrompting(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{} // must never be consulted
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{
		allowStep("echo one", safety.RiskRead),
		allowStep("echo two", safety.RiskWrite),
	}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	require.Equal(t, 2, exec.callCount())
	assert.Equal(t, "echo one", exec.commandAt(0))
	assert.Equal(t, "echo two", exec.commandAt(1))
	assert.Equal(t, 0, prompt.shownCount())
	assert.Contains(t, stdout.String(), "echo one")
	assert.False(t, summary.Aborted)
}

// TestExecuteAutoForcesConfirmOnDestructive pins the non-negotiable safety
// exception: even in auto mode, a destructive step drops to an
// interactive confirm.
func TestExecuteAutoForcesConfirmOnDestructive(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceYes}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{confirmStep("rm -rf ./build", safety.RiskDestructive)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, prompt.shownCount())
	assert.Equal(t, 1, exec.callCount())
	assert.False(t, summary.Aborted)
}

func TestExecuteAutoForcesConfirmOnElevated(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceNo}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{confirmStep("sudo apt-get install docker.io", safety.RiskElevated)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, prompt.shownCount())
	assert.Equal(t, 0, exec.callCount(), "declining the forced confirm must skip, not run")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeSkipped, summary.Results[0].Outcome)
}

// TestExecuteAutoBypassesDestructiveConfirmOnlyWithConfigAndYolo is the
// second security-invariant test: the destructive confirm is skipped
// ONLY when BOTH confirm_destructive=false AND --yolo are set, and doing
// so always prints the mandatory red warning line.
func TestExecuteAutoBypassesDestructiveConfirmOnlyWithConfigAndYolo(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{} // must never be consulted once bypass fires
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.ConfirmDestructive = false
	deps.Yolo = true

	plan := Plan{Steps: []Step{confirmStep("rm -rf ./build", safety.RiskDestructive)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, prompt.shownCount(), "the bypass must skip the confirm prompt entirely")
	assert.Equal(t, 1, exec.callCount())
	assert.Contains(t, stdout.String(), "--yolo bypass")
	assert.Contains(t, stdout.String(), "destructive")
	assert.False(t, summary.Aborted)
}

func TestExecuteAutoBypassRequiresBothConfigFlagAndYolo(t *testing.T) {
	// confirm_destructive=false alone, WITHOUT --yolo, must still force
	// the confirm prompt.
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceYes}}}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.ConfirmDestructive = false
	deps.Yolo = false

	plan := Plan{Steps: []Step{confirmStep("rm -rf ./build", safety.RiskDestructive)}}
	_, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, prompt.shownCount(), "without --yolo the confirm must still fire")
	assert.NotContains(t, stdout.String(), "--yolo bypass")
}

func TestExecuteAutoBypassesElevatedConfirmOnlyWithConfigAndYolo(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.ConfirmElevated = false
	deps.Yolo = true

	plan := Plan{Steps: []Step{confirmStep("sudo systemctl restart nginx", safety.RiskElevated)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, prompt.shownCount())
	assert.Equal(t, 1, exec.callCount())
	assert.Contains(t, stdout.String(), "--yolo bypass")
	assert.Contains(t, stdout.String(), "elevated")
	assert.False(t, summary.Aborted)
}

// TestExecuteBlockNeverExecutesInAutoModeEvenWithYolo is the strongest
// form of the "Block NEVER executes" invariant: --yolo plus both config
// flags disabled still must never run (or even prompt for) a Blocked step.
func TestExecuteBlockNeverExecutesInAutoModeEvenWithYolo(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.ConfirmDestructive = false
	deps.ConfirmElevated = false
	deps.Yolo = true

	plan := Plan{Steps: []Step{blockStep()}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "Block must never execute, even in auto+--yolo")
	assert.Equal(t, 0, prompt.shownCount())
	assert.NotContains(t, stdout.String(), "--yolo bypass", "a Block must never even reach the yolo-bypass path")
	assert.True(t, summary.Aborted, "auto mode aborts the remaining plan on a Block")
	assert.Contains(t, summary.AbortReason, "blocked")
}

// TestExecuteAutoAbortsRemainingStepsOnBlock pins auto mode's
// abort-on-block design decision: a Blocked step stops the whole
// remaining plan, and every later step is reported Skipped in the
// summary rather than silently omitted.
func TestExecuteAutoAbortsRemainingStepsOnBlock(t *testing.T) {
	exec := &fakeExecutor{}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{
		blockStep(),
		allowStep("echo never-reached", safety.RiskRead),
	}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount())
	require.Len(t, summary.Results, 2)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
	assert.Equal(t, OutcomeSkipped, summary.Results[1].Outcome)
	assert.True(t, summary.Aborted)
}

// TestExecuteAutoBlockRendersTurkishWhenRunDepsTranslatorIsTurkish is
// FAZ 9's TR smoke test for the engine layer: the exact same Block/abort
// scenario as TestExecuteAutoAbortsRemainingStepsOnBlock above, but with
// deps.Translator set to a Turkish Translator, proving the BLOCKED
// line/AbortReason actually route through it — and that every OTHER test
// in this file (which never sets Translator) keeps getting byte-for-byte
// English output, since RunDeps.tr() defaults to English for a zero-value
// Translator.
func TestExecuteAutoBlockRendersTurkishWhenRunDepsTranslatorIsTurkish(t *testing.T) {
	exec := &fakeExecutor{}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.Translator = i18n.NewTranslator(i18n.LangTR)

	plan := Plan{Steps: []Step{blockStep()}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Contains(t, stdout.String(), "ENGELLENDİ(")
	assert.NotContains(t, stdout.String(), "BLOCKED(")
	assert.True(t, summary.Aborted)
	assert.Contains(t, summary.AbortReason, "engellendi")
}

// TestExecuteStepWithSelfCorrectionRefusesToRunBlockedStepDirectly is the
// belt-and-suspenders defense-in-depth guard's own direct unit test: even
// called directly with a Blocked step (bypassing every caller-level
// check), executeStepWithSelfCorrection itself must refuse to reach the
// executor.
func TestExecuteStepWithSelfCorrectionRefusesToRunBlockedStepDirectly(t *testing.T) {
	exec := &fakeExecutor{}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	correctionsUsed := 0
	_, _, _, err := executeStepWithSelfCorrection(context.Background(), deps, ModeAuto, blockStep(), &correctionsUsed)

	require.Error(t, err)
	assert.Equal(t, 0, exec.callCount())
}

// --- self-correction ---------------------------------------------------

// TestExecuteAutoSelfCorrectsOnFailureThenSucceeds proves a failing step
// gets one corrected retry (well under the 3-attempt cap) that then
// succeeds, and the successful revised command is what gets recorded.
func TestExecuteAutoSelfCorrectsOnFailureThenSucceeds(t *testing.T) {
	exec := &fakeExecutor{
		respond: func(callIndex int, _ string) (executor.Result, error) {
			if callIndex == 0 {
				return executor.Result{ExitCode: 1, Stderr: "E: Unable to locate package nginx-typo"}, nil
			}
			return executor.Result{ExitCode: 0}, nil
		},
	}
	completer := &fakeCorrectionCompleter{
		respond: func(_ int) (llm.CompletionResponse, error) {
			return correctionResponseJSON(t, "apt-get install -y nginx", "write"), nil
		},
	}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, completer, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("apt-get install -y nginx-typo", safety.RiskWrite)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, completer.callCount())
	require.Equal(t, 2, exec.callCount())
	assert.Equal(t, "apt-get install -y nginx-typo", exec.commandAt(0))
	assert.Equal(t, "apt-get install -y nginx", exec.commandAt(1))
	require.Len(t, summary.Results, 1)
	assert.Equal(t, "apt-get install -y nginx", summary.Results[0].Command)
	assert.True(t, summary.Results[0].SelfCorrected)
	assert.Equal(t, 0, summary.Results[0].ExitCode)
	assert.False(t, summary.Aborted)
}

// TestExecuteAutoSelfCorrectionStopsAfterThreeAttempts proves the 3-total
// self-correction cap: a step that keeps failing gets exactly 3 correction
// round-trips and then the run stops with a summary+suggestion, never a
// 4th correction attempt.
func TestExecuteAutoSelfCorrectionStopsAfterThreeAttempts(t *testing.T) {
	exec := &fakeExecutor{
		respond: func(_ int, _ string) (executor.Result, error) {
			return executor.Result{ExitCode: 1, Stderr: "still broken"}, nil
		},
	}
	completer := &fakeCorrectionCompleter{
		respond: func(callIndex int) (llm.CompletionResponse, error) {
			return correctionResponseJSON(t, fmt.Sprintf("still-broken-command-%d", callIndex), "write"), nil
		},
	}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, completer, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("broken-command", safety.RiskWrite)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, selfCorrectionMaxAttempts, completer.callCount(), "must attempt exactly the capped number of corrections, never more")
	assert.Equal(t, selfCorrectionMaxAttempts+1, exec.callCount(), "the original attempt plus each correction attempt")
	assert.True(t, summary.Aborted)
	assert.Contains(t, summary.AbortReason, "self-correction")
	assert.Contains(t, summary.AbortReason, "review the command and retry manually")
}

// TestExecuteSelfCorrectionRevisionItselfBlockedStopsWithoutRunning proves
// a self-correction revision is ALSO re-evaluated by safety before ever
// running — a correction that "fixes" a failure by suggesting `rm -rf /`
// must never execute.
func TestExecuteSelfCorrectionRevisionItselfBlockedStopsWithoutRunning(t *testing.T) {
	exec := &fakeExecutor{
		respond: func(_ int, _ string) (executor.Result, error) {
			return executor.Result{ExitCode: 1, Stderr: "boom"}, nil
		},
	}
	completer := &fakeCorrectionCompleter{
		respond: func(_ int) (llm.CompletionResponse, error) {
			return correctionResponseJSON(t, "rm -rf /", "read"), nil
		},
	}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, completer, &fakeAudit{})

	plan := Plan{Steps: []Step{allowStep("broken-command", safety.RiskWrite)}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, completer.callCount(), "must stop after the first revision is found unsafe, not keep retrying")
	assert.Equal(t, 1, exec.callCount(), "the blocked revision must never reach the executor")
	assert.True(t, summary.Aborted)
}

// --- audit -----------------------------------------------------------------

func TestExecuteAppendsOneAuditEntryPerExecutedStep(t *testing.T) {
	exec := &fakeExecutor{}
	aud := &fakeAudit{}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, aud)

	plan := Plan{Steps: []Step{
		allowStep("echo one", safety.RiskRead),
		blockStep(),
	}}
	_, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	require.Equal(t, 1, aud.entryCount(), "only the executed step is audited; the blocked one never ran")
	assert.Equal(t, "echo one", aud.entries[0].Command)
	assert.Equal(t, "auto", aud.entries[0].Mode)
	assert.Equal(t, "test request", aud.entries[0].Request)
}

// --- Ctrl-C / cancellation --------------------------------------------------

// TestExecuteCancelDuringExecutionSkipsRemainingStepsAndSummarizes is the
// Ctrl-C scenario: a cancellable ctx plus a fake executor that blocks
// until canceled, run against a 2-step plan. The first step reports
// Canceled, the second is never even attempted and shows up as Skipped.
func TestExecuteCancelDuringExecutionSkipsRemainingStepsAndSummarizes(t *testing.T) {
	exec := &fakeExecutor{blockUntilCancel: true}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	plan := Plan{Steps: []Step{
		allowStep("sleep 5", safety.RiskRead),
		allowStep("echo never-reached", safety.RiskRead),
	}}

	done := make(chan struct{})
	var summary RunSummary
	var err error
	go func() {
		summary, err = Execute(ctx, plan, ModeAuto, deps)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Execute did not return promptly after cancellation")
	}

	require.NoError(t, err)
	require.Len(t, summary.Results, 2)
	assert.Equal(t, OutcomeExecuted, summary.Results[0].Outcome)
	assert.Equal(t, OutcomeSkipped, summary.Results[1].Outcome)
	assert.True(t, summary.Aborted)
	assert.Equal(t, "canceled", summary.AbortReason)
}

// --- LOW#6: zero-value Decision fails closed, not open --------------------

// TestNormalizeStepDecisionsLeavesEvaluatedStepsUnchanged pins that
// normalizeStepDecisions never touches a Step whose Decision already came
// out of safety.Engine.Evaluate (Decision.Evaluated == true): the
// helper-built confirmStep below carries a Decision that is deliberately
// decoupled from what re-evaluating "some read-only command" would
// actually produce, so any re-derivation at all would flip it -- proving
// it survives unchanged proves normalizeStepDecisions truly skips
// already-evaluated steps rather than merely happening to agree with them.
func TestNormalizeStepDecisionsLeavesEvaluatedStepsUnchanged(t *testing.T) {
	safetyEngine := testSafetyEngine(t)
	original := confirmStep("some read-only command", safety.RiskRead)
	plan := Plan{Steps: []Step{original}}

	got := normalizeStepDecisions(plan, safetyEngine)

	require.Len(t, got.Steps, 1)
	assert.Equal(t, original.Decision, got.Steps[0].Decision, "an already-Evaluated Decision must never be re-derived")
}

// TestNormalizeStepDecisionsReDerivesUnevaluatedDestructiveStep pins the
// core LOW#6 mechanism directly: a Step reaching normalizeStepDecisions
// with a zero-value Decision (Evaluated: false, Action: Allow,
// EffectiveRisk: RiskRead -- indistinguishable from a legitimate
// read-Allow by field inspection alone) for a destructive command must be
// re-derived to Block, not left as the zero value's Allow.
func TestNormalizeStepDecisionsReDerivesUnevaluatedDestructiveStep(t *testing.T) {
	safetyEngine := testSafetyEngine(t)
	plan := Plan{Steps: []Step{{
		Command:  "rm -rf /",
		Risk:     safety.RiskRead,
		Decision: safety.Decision{}, // zero value: never produced by Evaluate
	}}}

	got := normalizeStepDecisions(plan, safetyEngine)

	require.Len(t, got.Steps, 1)
	assert.Equal(t, safety.Block, got.Steps[0].Decision.Action)
	assert.True(t, got.Steps[0].Decision.Evaluated)
}

// TestExecuteReEvaluatesUnpopulatedDecisionInsteadOfAllowingItUnprompted is
// the end-to-end proof: a Step carrying a zero-value Decision for a
// destructive command reaches Execute directly (as if a future caller
// forgot to run it through safety.Engine.Evaluate first) and must come
// out Blocked -- never an unprompted execution, which is what the
// zero-value Decision's Action == Allow would otherwise cause.
func TestExecuteReEvaluatesUnpopulatedDecisionInsteadOfAllowingItUnprompted(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{} // no responses scripted: must never even be consulted
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{{
		Command:  "rm -rf /",
		Risk:     safety.RiskRead, // as if mislabeled, or never labeled at all
		Decision: safety.Decision{},
	}}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 0, exec.callCount(), "an unevaluated destructive command must never run unprompted")
	assert.Equal(t, 0, prompt.shownCount())
	assert.Contains(t, stdout.String(), "BLOCKED(")
	require.Len(t, summary.Results, 1)
	assert.Equal(t, OutcomeBlocked, summary.Results[0].Outcome)
	assert.True(t, summary.Aborted)
}

// TestExecuteReEvaluatesUnpopulatedDecisionElevatedGatesBehindConfirm is
// the Confirm-tier sibling of the Block-tier proof above: an unevaluated
// step whose command actually re-derives to RiskElevated (sudo) must still
// drop to an interactive confirm before it ever reaches the executor,
// exactly like a normally-Evaluated Confirm step would.
func TestExecuteReEvaluatesUnpopulatedDecisionElevatedGatesBehindConfirm(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceYes}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	plan := Plan{Steps: []Step{{
		Command:  "sudo systemctl restart nginx",
		Risk:     safety.RiskRead, // as if mislabeled
		Decision: safety.Decision{},
	}}}
	summary, err := Execute(context.Background(), plan, ModeAuto, deps)

	require.NoError(t, err)
	assert.Equal(t, 1, prompt.shownCount(), "an unevaluated elevated command must be re-derived and gated behind a confirm, not run unprompted")
	require.Equal(t, 1, exec.callCount())
	assert.False(t, summary.Aborted)
}

// TestExecuteAppendsRunIDWorkingDirAndReversibleOntoEveryAuditEntry is
// comrade-undo's schema-plumbing proof: RunDeps.RunID/WorkingDir/UndoOf
// (all new, empty-by-default fields) and each Step's own Reversible must
// all reach appendAudit's resulting audit.Entry unchanged, for every step
// actually executed — not just the ones a hand-written fakeAudit
// assertion happened to already cover.
func TestExecuteAppendsRunIDWorkingDirAndReversibleOntoEveryAuditEntry(t *testing.T) {
	exec := &fakeExecutor{}
	aud := &fakeAudit{}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, aud)
	deps.RunID = "run-abc123"
	deps.WorkingDir = "/home/user/project"

	step := allowStep("echo hi", safety.RiskRead)
	step.Reversible = true
	plan := Plan{Steps: []Step{step}}

	_, err := Execute(context.Background(), plan, ModeAuto, deps)
	require.NoError(t, err)

	require.Equal(t, 1, aud.entryCount())
	entry := aud.entries[0]
	assert.Equal(t, "run-abc123", entry.RunID)
	assert.Equal(t, "/home/user/project", entry.Cwd)
	assert.Empty(t, entry.UndoOf, "an ordinary (non-undo) run must never stamp UndoOf")
	require.NotNil(t, entry.Reversible)
	assert.True(t, *entry.Reversible)
}

// TestExecuteStampsUndoOfWhenRunDepsSetsIt proves the OTHER half of the
// same wiring: when RunDeps.UndoOf is set (comrade undo's own case —
// internal/cli sets it to the target run's RunID before calling
// Execute), every entry this run appends carries that same UndoOf value,
// so a later `comrade undo` invocation can recognize the target run as
// already undone (see audit.Entry.UndoOf's own doc comment).
func TestExecuteStampsUndoOfWhenRunDepsSetsIt(t *testing.T) {
	exec := &fakeExecutor{}
	aud := &fakeAudit{}
	deps, _ := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, aud)
	deps.RunID = "undo-run-xyz"
	deps.UndoOf = "original-run-id"

	step := allowStep("rmdir /tmp/example", safety.RiskWrite)
	step.Reversible = false
	plan := Plan{Steps: []Step{step}}

	_, err := Execute(context.Background(), plan, ModeAuto, deps)
	require.NoError(t, err)

	require.Equal(t, 1, aud.entryCount())
	entry := aud.entries[0]
	assert.Equal(t, "original-run-id", entry.UndoOf)
	require.NotNil(t, entry.Reversible)
	assert.False(t, *entry.Reversible)
}
