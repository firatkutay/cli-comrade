package cli

import (
	"bytes"
	stdctx "context"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// pytonFixDiagnosisJSON is the canned diagnose-endpoint response used by
// this file's end-to-end tests: docs/history/UYGULAMA_PLANI.md FAZ 7's own named
// acceptance scenario, a typo'd "pyton --version" command-not-found
// error, whose fix plan installs python3 via the (test-injected) detected
// package manager.
const pytonFixDiagnosisJSON = `{
  "root_cause": "The command \"pyton\" does not exist; it looks like a typo for python3, which is also not installed.",
  "explanation": "Your computer doesn't recognize pyton. It's likely a typo of python3, and that isn't installed on this machine yet.",
  "plan": {
    "summary": "Install python3, then check its version.",
    "steps": [
      {"command": "sudo apt-get install -y python3", "rationale": "Installs Python 3.", "risk": "elevated", "reversible": false}
    ]
  }
}`

// seedLastCommand writes cmd to the isolated (per withIsolatedConfigDir)
// last_command.json location, exactly like FAZ 4's shell hook would.
func seedLastCommand(t *testing.T, cmd contextpkg.LastCommand) {
	t.Helper()
	path, err := contextpkg.LastCommandPath(runtime.GOOS, os.Getenv)
	require.NoError(t, err)
	require.NoError(t, contextpkg.WriteLastCommand(path, cmd))
}

// execRootSplitWithStdin is execRootSplit (config_test.go) plus a
// scripted stdin, for exercising `comrade fix`'s interactive paste-mode
// fallback.
func execRootSplitWithStdin(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd("dev")
	outBuf := &strings.Builder{}
	errBuf := &strings.Builder{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetIn(strings.NewReader(stdin))
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

// setLLMEnv points cli-comrade's LLM config at the given mock server,
// exactly like TestDoDryRunRendersPlanTableAgainstMockProvider does.
func setLLMEnv(t *testing.T, serverURL string) {
	t.Helper()
	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", serverURL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")
}

// TestFixInfoModeSurfacesRootCauseExplanationAndPlan is FAZ 7's own named
// acceptance scenario end-to-end: a fresh, failing last_command.json
// entry ("pyton --version") is diagnosed against a mock LLM, and info
// mode prints the root cause, the plain-language explanation, and the
// fix plan's steps — without executing anything — plus the offered
// post-solution verification command (FAZ 7 item 4), since the original
// command is not destructive.
func TestFixInfoModeSurfacesRootCauseExplanationAndPlan(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, pytonFixDiagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "pyton --version",
		ExitCode:   127,
		StderrTail: "sh: 1: pyton: not found",
		Timestamp:  time.Now(),
		Shell:      "bash",
	})

	stdout, stderr, err := execRootSplit(t, "dev", "fix", "--info")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "pyton")
	assert.Contains(t, stdout, "python3")
	assert.Contains(t, stdout, "sudo apt-get install -y python3")
	assert.Contains(t, stdout, "Suggested verification: pyton --version")
}

// TestFixAutoModeExecutesPlanAndRunsPostSolutionVerification proves auto
// mode actually routes the diagnosis's Plan into engine.Execute (a
// benign, non-elevated step really runs against the real executor, its
// stdout appears) and then, because the run completed cleanly and the
// original command is not destructive, actually re-runs it too (FAZ 7
// item 4's "auto modda çalıştır" behavior) — asserted via the real
// executor's own printed status line, not a mock.
func TestFixAutoModeExecutesPlanAndRunsPostSolutionVerification(t *testing.T) {
	withIsolatedConfigDir(t)
	diagnosisJSON := `{
		"root_cause": "a benign, contrived failure for this test",
		"explanation": "explained in plain language",
		"plan": {"summary": "prints a marker", "steps": [
			{"command": "echo fix-plan-marker", "rationale": "benign marker step", "risk": "read", "reversible": true}
		]}
	}`
	server := newMockPlanServer(t, diagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "echo original-failing-marker; exit 1",
		ExitCode:   1,
		StderrTail: "contrived failure",
		Timestamp:  time.Now(),
	})

	stdout, stderr, err := execRootSplit(t, "dev", "fix", "--auto")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "fix-plan-marker", "the real executor must have run the plan's benign step")
	assert.Contains(t, stdout, "1 executed, 0 skipped, 0 blocked")

	// Post-solution verification actually re-ran the original command
	// (a non-elevated, non-destructive shell one-liner) against the real
	// executor, and reported its outcome.
	assert.Contains(t, stdout, "verification: echo original-failing-marker; exit 1")
	assert.Contains(t, stdout, "original-failing-marker", "the verification re-run's own stdout must appear too")
}

// TestFixAskModeRoutesBlockedStepToRunnerAndSkipsVerificationForDestructiveOriginal
// proves ask mode also routes the diagnosis's Plan into the exact same
// engine.Execute/safety.Engine machinery `do` uses (a denylisted plan
// step is Blocked without ever prompting, since executeAsk's Block branch
// never calls PromptUI at all — see internal/engine/runner.go), and that
// post-solution verification is never even offered when the ORIGINAL
// failing command is independently classified destructive (FAZ 7 item
// 4's "destructive değilse" gate) — regardless of what mode fix ran in.
func TestFixAskModeRoutesBlockedStepToRunnerAndSkipsVerificationForDestructiveOriginal(t *testing.T) {
	withIsolatedConfigDir(t)
	diagnosisJSON := `{
		"root_cause": "a contrived failure",
		"explanation": "explained in plain language",
		"plan": {"summary": "a plan the model never should have produced", "steps": [
			{"command": "rm -rf /", "rationale": "a decoy the model must never actually produce", "risk": "read", "reversible": false}
		]}
	}`
	server := newMockPlanServer(t, diagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "rm -rf /tmp/dangerzone",
		ExitCode:   1,
		StderrTail: "contrived failure",
		Timestamp:  time.Now(),
	})

	stdout, stderr, err := execRootSplit(t, "dev", "fix", "--ask")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "BLOCKED(", "ask mode must route the diagnosis's plan through the same safety-Blocking Execute path do uses")
	assert.Contains(t, stdout, "rm -rf /")
	assert.NotContains(t, stdout, "verification", "a destructive original command must never be offered for post-solution verification, in any mode")
}

// TestFixStaleLastCommandFallsThroughToPasteMode proves a last_command.json
// entry older than the 10-minute freshness window is never silently used
// — the chain falls through to interactive paste mode instead, with a
// one-line notice on stderr explaining why.
func TestFixStaleLastCommandFallsThroughToPasteMode(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, pytonFixDiagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "pyton --version",
		ExitCode:   127,
		StderrTail: "sh: 1: pyton: not found",
		Timestamp:  time.Now().Add(-15 * time.Minute),
	})

	stdin := "pasted-command --flag\nsome pasted error line\n\n"
	stdout, stderr, err := execRootSplitWithStdin(t, stdin, "fix", "--info")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, "more than 10 minutes old")
	assert.Contains(t, stdout, "No recent failed command available")
	assert.Contains(t, stdout, "pyton", "the mock diagnosis must still have been produced from the paste-mode fallback")
}

// TestFixSuccessfulLastCommandFallsThroughToPasteMode proves a fresh
// last_command.json entry whose exit_code == 0 (the command actually
// succeeded) is never treated as something to fix.
func TestFixSuccessfulLastCommandFallsThroughToPasteMode(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, pytonFixDiagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:   "echo all good",
		ExitCode:  0,
		Timestamp: time.Now(),
	})

	stdin := "pasted-command --flag\nsome pasted error line\n\n"
	stdout, stderr, err := execRootSplitWithStdin(t, stdin, "fix", "--info")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, "exited successfully")
	assert.Contains(t, stdout, "No recent failed command available")
}

// recordingExecutor is a minimal engine.CommandExecutor test double that
// records every command it is asked to run and always answers success —
// used only to prove, precisely and without touching the real host, that
// a command is never even handed to the executor.
type recordingExecutor struct {
	calls []string
}

func (r *recordingExecutor) Run(_ stdctx.Context, command string, _ executor.Options) (executor.Result, error) {
	r.calls = append(r.calls, command)
	return executor.Result{ExitCode: 0}, nil
}

// TestAcquireErrorContextRefusesDestructiveRerun is FAZ 7 item 2's core
// safety proof at the acquireErrorContext/captureByRunning level: a
// last_command.json entry the independent safety.Engine classifies as
// destructive (here, via the "rm -r/-f" escalation rule) must never
// reach the executor when --rerun is given — the chain instead falls
// through to interactive paste mode, exactly like a stale/exit-0 entry
// does.
func TestAcquireErrorContextRefusesDestructiveRerun(t *testing.T) {
	rec := &recordingExecutor{}
	safetyEngine := safety.NewEngine(config.Default())

	var stdout, stderr bytes.Buffer
	cmd := &cobra.Command{}
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetIn(strings.NewReader("echo pasted-command\npasted error output\n\n"))

	sysCtx := contextpkg.Context{
		LastCommand: &contextpkg.LastCommand{
			Command:   "rm -rf /tmp/dangerzone",
			ExitCode:  1,
			Timestamp: time.Now(),
		},
	}

	errCtx, err := acquireErrorContext(cmd, sysCtx, safetyEngine, rec, true, "", i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)

	assert.Empty(t, rec.calls, "the destructive last command must NEVER reach the executor via --rerun")
	assert.Contains(t, stderr.String(), "refusing to re-run")

	// The refusal fell through to paste mode, which read the scripted
	// stdin instead of the refused command.
	assert.Equal(t, "echo pasted-command", errCtx.Command)
	assert.Equal(t, "pasted error output", errCtx.Stderr)
	assert.Equal(t, -1, errCtx.ExitCode)
}

// TestAcquireErrorContextRerunWithoutLastCommandIsAnError proves --rerun
// with no recorded last command at all is a clear error, not a silent
// fallback.
func TestAcquireErrorContextRerunWithoutLastCommandIsAnError(t *testing.T) {
	rec := &recordingExecutor{}
	safetyEngine := safety.NewEngine(config.Default())
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	_, err := acquireErrorContext(cmd, contextpkg.Context{}, safetyEngine, rec, true, "", i18n.NewTranslator(i18n.LangEN))
	require.Error(t, err)
	assert.Empty(t, rec.calls)
}

// TestAcquireErrorContextExplicitCommandRunsItAndCapturesOutput proves
// `comrade fix -- <command>`'s path: a benign, non-destructive command is
// actually run via the injected executor and its result becomes the
// ErrorContext.
func TestAcquireErrorContextExplicitCommandRunsItAndCapturesOutput(t *testing.T) {
	rec := &recordingExecutor{}
	safetyEngine := safety.NewEngine(config.Default())
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})

	errCtx, err := acquireErrorContext(cmd, contextpkg.Context{}, safetyEngine, rec, false, "some benign command", i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)

	require.Len(t, rec.calls, 1)
	assert.Equal(t, "some benign command", rec.calls[0])
	assert.Equal(t, "some benign command", errCtx.Command)
}

// TestPasteModeParsesCommandAndErrorUntilBlankLine is pasteMode's own
// direct unit test: the first line is the command, subsequent lines
// (until a blank line or EOF) are joined as Stderr, and ExitCode is the
// documented -1 "unknown" sentinel.
func TestPasteModeParsesCommandAndErrorUntilBlankLine(t *testing.T) {
	cmd := &cobra.Command{}
	var stdout bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetIn(strings.NewReader("mycmd --flag\nline one\nline two\n\nthis must not be read"))

	errCtx, err := pasteMode(cmd, contextpkg.Context{OS: "linux"}, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)

	assert.Equal(t, "mycmd --flag", errCtx.Command)
	assert.Equal(t, "line one\nline two", errCtx.Stderr)
	assert.Equal(t, -1, errCtx.ExitCode)
	assert.Equal(t, "linux", errCtx.System.OS)
}

// TestPasteModeHandlesEOFWithoutBlankLine proves EOF (no trailing blank
// line at all) also terminates error-output collection instead of
// hanging or erroring.
func TestPasteModeHandlesEOFWithoutBlankLine(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetIn(strings.NewReader("mycmd\nonly line, no trailing blank"))

	errCtx, err := pasteMode(cmd, contextpkg.Context{}, i18n.NewTranslator(i18n.LangEN))
	require.NoError(t, err)

	assert.Equal(t, "mycmd", errCtx.Command)
	assert.Equal(t, "only line, no trailing blank", errCtx.Stderr)
}
