package engine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

func TestOfferVerificationSkipsEmptyCommand(t *testing.T) {
	exec := &fakeExecutor{}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	for _, mode := range []Mode{ModeInfo, ModeAsk, ModeAuto} {
		err := OfferVerification(context.Background(), deps, mode, "   ")
		require.NoError(t, err)
	}
	assert.Equal(t, 0, exec.callCount())
	assert.Empty(t, stdout.String())
}

// TestOfferVerificationSkippedWhenOriginalCommandIsDestructive proves FAZ
// 7 item 4's "destructive değilse" gate: a command the independent
// safety.Engine classifies as destructive (here, via the "rm -r/-f"
// escalation rule — not a declared risk, since there is none for an
// original failing command) is never offered for verification, in any
// mode, and the executor is never even asked to consider it.
func TestOfferVerificationSkippedWhenOriginalCommandIsDestructive(t *testing.T) {
	for _, mode := range []Mode{ModeInfo, ModeAsk, ModeAuto} {
		exec := &fakeExecutor{}
		deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

		err := OfferVerification(context.Background(), deps, mode, "rm -rf /tmp/somedir")
		require.NoError(t, err)

		assert.Equal(t, 0, exec.callCount(), "mode %v must never run a destructive verification command", mode)
		assert.NotContains(t, stdout.String(), "verification", "mode %v must never even suggest a destructive verification command", mode)
	}
}

// TestOfferVerificationInfoModePrintsSuggestionWithoutExecuting is FAZ 7
// item 4's info-mode behavior: print the suggested command, run nothing.
func TestOfferVerificationInfoModePrintsSuggestionWithoutExecuting(t *testing.T) {
	exec := &fakeExecutor{}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	err := OfferVerification(context.Background(), deps, ModeInfo, "echo ok")
	require.NoError(t, err)

	assert.Equal(t, 0, exec.callCount())
	assert.Contains(t, stdout.String(), "Suggested verification: echo ok")
}

// TestOfferVerificationInfoModePrintsSuggestionInTurkish is QA D6's
// regression guard: the "Suggested verification: ..." line was a raw
// hardcoded English literal that never routed through
// RunDeps.Translator like every other engine-printed string, so it
// stayed English even in an otherwise fully-Turkish `comrade fix` run.
// This project's established TR-smoke-test convention: exact match, not
// just a substring check that could pass on a fallback-to-English catalog
// lookup.
func TestOfferVerificationInfoModePrintsSuggestionInTurkish(t *testing.T) {
	exec := &fakeExecutor{}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.Translator = i18n.NewTranslator(i18n.LangTR)

	err := OfferVerification(context.Background(), deps, ModeInfo, "echo ok")
	require.NoError(t, err)

	assert.Equal(t, 0, exec.callCount())
	assert.Equal(t, "\nÖnerilen doğrulama: echo ok\n", stdout.String())
	assert.NotContains(t, stdout.String(), "Suggested verification")
}

// TestOfferVerificationAutoModeReportsSuccessInTurkish is D6's sweep
// extending to runVerification's own two sibling strings (found in the
// same file, same previously-unrouted pattern, while fixing the
// QA-reported one) — the "verification: ... succeeded" line ask/auto
// mode prints after actually re-running the command.
func TestOfferVerificationAutoModeReportsSuccessInTurkish(t *testing.T) {
	exec := &fakeExecutor{respond: func(int, string) (executor.Result, error) {
		return executor.Result{ExitCode: 0}, nil
	}}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.Translator = i18n.NewTranslator(i18n.LangTR)

	err := OfferVerification(context.Background(), deps, ModeAuto, "echo ok")
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "doğrulama: echo ok başarılı oldu")
	assert.NotContains(t, stdout.String(), "verification:")
}

// TestOfferVerificationAutoModeReportsFailureInTurkish is
// TestOfferVerificationAutoModeReportsSuccessInTurkish's failure-path
// counterpart, for MsgVerificationStillFails.
func TestOfferVerificationAutoModeReportsFailureInTurkish(t *testing.T) {
	exec := &fakeExecutor{respond: func(int, string) (executor.Result, error) {
		return executor.Result{ExitCode: 3}, nil
	}}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})
	deps.Translator = i18n.NewTranslator(i18n.LangTR)

	err := OfferVerification(context.Background(), deps, ModeAuto, "echo fail")
	require.NoError(t, err)

	assert.Contains(t, stdout.String(), "doğrulama: echo fail hâlâ başarısız (çıkış 3)")
	assert.NotContains(t, stdout.String(), "verification:")
}

// TestOfferVerificationAutoModeRunsNonElevatedCommandDirectly is FAZ 7
// item 4's auto-mode behavior for a plain (non-elevated) verification
// command: run it immediately, with no confirm prompt, report success,
// and record it in the audit log (CLAUDE.md security rule #4 applies to
// this re-run exactly like every other executed command).
func TestOfferVerificationAutoModeRunsNonElevatedCommandDirectly(t *testing.T) {
	exec := &fakeExecutor{respond: func(int, string) (executor.Result, error) {
		return executor.Result{ExitCode: 0}, nil
	}}
	aud := &fakeAudit{}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, aud)

	err := OfferVerification(context.Background(), deps, ModeAuto, "echo verify-marker")
	require.NoError(t, err)

	require.Equal(t, 1, exec.callCount())
	assert.Equal(t, "echo verify-marker", exec.commandAt(0))
	assert.Contains(t, stdout.String(), "verification: echo verify-marker succeeded")
	assert.Equal(t, 1, aud.entryCount(), "the verification re-run must be audited like any other executed command")
}

// TestOfferVerificationAutoModeReportsFailure proves a still-failing
// verification re-run is reported as such, not silently swallowed.
func TestOfferVerificationAutoModeReportsFailure(t *testing.T) {
	exec := &fakeExecutor{respond: func(int, string) (executor.Result, error) {
		return executor.Result{ExitCode: 1}, nil
	}}
	deps, stdout := baseDeps(t, exec, &fakePrompt{}, &fakeCorrectionCompleter{}, &fakeAudit{})

	err := OfferVerification(context.Background(), deps, ModeAuto, "false")
	require.NoError(t, err)

	require.Equal(t, 1, exec.callCount())
	assert.Contains(t, stdout.String(), "verification: false still fails (exit 1)")
}

// TestOfferVerificationAutoModeElevatedDropsToConfirm proves auto mode
// never bypasses CLAUDE.md's non-negotiable elevated-confirmation
// requirement just because the command being run is a *verification*
// re-run rather than a real plan step: an elevated original command still
// drops to the same interactive confirm loop a real elevated plan step
// would use.
func TestOfferVerificationAutoModeElevatedDropsToConfirm(t *testing.T) {
	exec := &fakeExecutor{respond: func(int, string) (executor.Result, error) {
		return executor.Result{ExitCode: 0}, nil
	}}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceYes}}}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	err := OfferVerification(context.Background(), deps, ModeAuto, "sudo systemctl restart nginx")
	require.NoError(t, err)

	assert.Equal(t, 1, prompt.shownCount(), "an elevated verification command must be confirmed, exactly like a real plan step")
	require.Equal(t, 1, exec.callCount())
	assert.Contains(t, stdout.String(), "succeeded")
}

// TestOfferVerificationAutoModeElevatedDeclinedNeverRuns proves declining
// the elevated verification confirm actually skips execution.
func TestOfferVerificationAutoModeElevatedDeclinedNeverRuns(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceNo}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	err := OfferVerification(context.Background(), deps, ModeAuto, "sudo systemctl restart nginx")
	require.NoError(t, err)

	assert.Equal(t, 0, exec.callCount())
}

// TestOfferVerificationAskModePromptsAndRunsOnYes is FAZ 7 item 4's
// ask-mode behavior: prompt, then run on [e]vet.
func TestOfferVerificationAskModePromptsAndRunsOnYes(t *testing.T) {
	exec := &fakeExecutor{respond: func(int, string) (executor.Result, error) {
		return executor.Result{ExitCode: 0}, nil
	}}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceYes}}}
	deps, stdout := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	err := OfferVerification(context.Background(), deps, ModeAsk, "echo verify-marker")
	require.NoError(t, err)

	assert.Equal(t, 1, prompt.shownCount())
	require.Equal(t, 1, exec.callCount())
	assert.Contains(t, stdout.String(), "verification: echo verify-marker succeeded")
}

// TestOfferVerificationAskModeSkipsOnNo proves [h]ayır never runs the
// verification command.
func TestOfferVerificationAskModeSkipsOnNo(t *testing.T) {
	exec := &fakeExecutor{}
	prompt := &fakePrompt{responses: []promptResponse{{choice: ChoiceNo}}}
	deps, _ := baseDeps(t, exec, prompt, &fakeCorrectionCompleter{}, &fakeAudit{})

	err := OfferVerification(context.Background(), deps, ModeAsk, "echo verify-marker")
	require.NoError(t, err)

	assert.Equal(t, 0, exec.callCount())
	assert.Equal(t, 1, prompt.shownCount())
}
