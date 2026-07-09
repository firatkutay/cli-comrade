package cli

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
)

// This file holds UYGULAMA_PLANI.md FAZ 11 item 1's Ubuntu/Linux
// end-to-end scenario tests (see docs/phases/FAZ-11.md's scenario+result
// table for the full picture, including the macOS/Windows scenarios that
// cannot run in this Linux sandbox and are documented there as manual).
// Each test below drives the real `comrade` cobra command tree
// (execRootSplit) against a mock openai_compat plan/diagnosis server —
// exactly like FAZ 6/7's own end-to-end tests in do_test.go/fix_test.go
// — rather than unit-testing a package in isolation.

// aptCommandNotFoundDiagnosisJSON is the canned diagnose response for
// TestFAZ11AptCommandNotFoundFixFlowInfoModeSuggestsInstallWithoutRunning:
// a "command not found" shell error whose fix plan installs the missing
// package via apt.
const aptCommandNotFoundDiagnosisJSON = `{
  "root_cause": "The command \"http\" does not exist; it is provided by the httpie package, which is not installed.",
  "explanation": "Your shell doesn't recognize \"http\" because the httpie package that provides it isn't installed yet.",
  "plan": {
    "summary": "Install httpie via apt.",
    "steps": [
      {"command": "sudo apt-get install -y httpie", "rationale": "Installs the httpie package, which provides the http command.", "risk": "elevated", "reversible": false}
    ]
  }
}`

// TestFAZ11AptCommandNotFoundFixFlowInfoModeSuggestsInstallWithoutRunning
// is the Ubuntu "apt hatası fix" scenario from UYGULAMA_PLANI.md FAZ 11
// item 1's own list: `comrade fix --info` against a "command not found"
// last-command entry, mock-diagnosed as a missing apt package. Info mode
// must surface the root cause, explanation, and the suggested `apt-get
// install` command as plain, copyable text — and must NOT execute
// anything (no audit log entry at all).
func TestFAZ11AptCommandNotFoundFixFlowInfoModeSuggestsInstallWithoutRunning(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, aptCommandNotFoundDiagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "http httpbin.org/get",
		ExitCode:   127,
		StderrTail: "bash: http: command not found",
		Timestamp:  time.Now(),
	})

	stdout, stderr, err := execRootSplit(t, "dev", "fix", "--info")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "httpie")
	assert.Contains(t, stdout, "sudo apt-get install -y httpie")

	// Nothing must have actually run: info mode never touches the
	// executor or the audit log.
	entries := readAuditEntries(t)
	assert.Empty(t, entries, "info mode must never execute or audit anything")
}

// portConflictDiagnosisJSON is the canned diagnose response for
// TestFAZ11PortAlreadyInUseFixFlowInfoModeSuggestsFreeingThePort: an
// "address already in use" server-startup failure whose fix plan
// identifies and offers to stop whatever is already bound to the port.
const portConflictDiagnosisJSON = `{
  "root_cause": "Port 8080 is already in use by another process, so the new server cannot bind to it.",
  "explanation": "Something else on your machine is already listening on port 8080. Find out what it is, then stop it (or use a different port) before starting your server again.",
  "plan": {
    "summary": "Find and stop whatever is already listening on port 8080.",
    "steps": [
      {"command": "lsof -i :8080", "rationale": "Lists the process currently bound to port 8080.", "risk": "read", "reversible": true},
      {"command": "kill $(lsof -t -i :8080)", "rationale": "Stops the process holding port 8080, freeing it.", "risk": "destructive", "reversible": false}
    ]
  }
}`

// TestFAZ11PortAlreadyInUseFixFlowInfoModeSuggestsFreeingThePort is the
// Ubuntu "port çakışması fix" scenario from UYGULAMA_PLANI.md FAZ 11 item
// 1: `comrade fix --info` against an "address already in use" failure,
// mock-diagnosed with a two-step plan (identify the process, then a
// destructive `kill`). Info mode surfaces both suggested commands as
// copyable text without running either — in particular, the destructive
// `kill` step is never a decision this test needs the safety engine to
// make, because info mode never reaches engine.Execute at all.
func TestFAZ11PortAlreadyInUseFixFlowInfoModeSuggestsFreeingThePort(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, portConflictDiagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "node server.js",
		ExitCode:   1,
		StderrTail: "Error: listen EADDRINUSE: address already in use :::8080",
		Timestamp:  time.Now(),
	})

	stdout, stderr, err := execRootSplit(t, "dev", "fix", "--info")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "8080")
	assert.Contains(t, stdout, "lsof -i :8080")
	assert.Contains(t, stdout, "kill $(lsof -t -i :8080)")

	entries := readAuditEntries(t)
	assert.Empty(t, entries, "info mode must never execute or audit anything")
}

// installAndStartNginxPlanJSON is the canned plan-endpoint response for
// TestFAZ11InstallAndStartNginxAutoModeRunsBenignStepAndBlocksDenylistedStep:
// `comrade nginx kur ve başlat`'s own UYGULAMA_PLANI.md FAZ 11 item 1
// scenario, reusing FAZ 6's proven shape (one genuinely benign step, one
// denylisted decoy the model must never have produced but safety must
// still catch regardless of mode) with nginx-specific wording, since a
// mock LLM response is the only way to deterministically force the
// denylisted-decoy branch this test needs without depending on a real
// model's non-deterministic output.
const installAndStartNginxPlanJSON = `{
  "summary": "Installs nginx and starts its service.",
  "steps": [
    {"command": "echo comrade-nginx-e2e-marker", "rationale": "stands in for the benign install/start step this test actually executes", "risk": "read", "reversible": true},
    {"command": "rm -rf /", "rationale": "a decoy the model must never actually produce", "risk": "read", "reversible": false}
  ]
}`

// TestFAZ11InstallAndStartNginxAutoModeRunsBenignStepAndBlocksDenylistedStep
// is the Ubuntu "nginx kur ve başlat auto modda" scenario from
// UYGULAMA_PLANI.md FAZ 11 item 1: `comrade do "nginx kur ve başlat"
// --auto` against a mock plan, run through the REAL internal/executor (no
// executor fake). The benign step actually runs (its real stdout
// appears, and it is the only entry in the audit log); the denylisted
// decoy step is Blocked by internal/safety and never reaches the
// executor — proving defense-in-depth holds in --auto specifically,
// where no human is in the loop to catch a bad suggestion by eye.
//
// (The complementary "an elevated/destructive step instead forces a
// CONFIRM rather than a flat allow, even in --auto" half of this
// scenario is already proven at the engine layer — where PromptUI is
// dependency-injected and so testable without a real TTY — by
// internal/engine's TestExecuteAutoForcesConfirmOnElevated and
// TestExecuteAutoBypassesElevatedConfirmOnlyWithConfigAndYolo, the
// latter using this exact "sudo systemctl restart nginx" command; see
// docs/phases/FAZ-11.md's scenario table.)
func TestFAZ11InstallAndStartNginxAutoModeRunsBenignStepAndBlocksDenylistedStep(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, installAndStartNginxPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	stdout, stderr, err := execRootSplit(t, "dev", "do", "nginx", "kur", "ve", "başlat", "--auto")

	require.Error(t, err, "the run must abort: the plan's second step is Blocked")
	assert.Contains(t, err.Error(), "blocked")

	assert.Contains(t, stdout, "comrade-nginx-e2e-marker", "the real executor must have actually run the benign step")
	assert.Contains(t, stdout, "BLOCKED(")
	assert.Contains(t, stdout, "rm -rf /")
	assert.Contains(t, stdout, "1 executed, 0 skipped, 1 blocked")
	_ = stderr

	entries := readAuditEntries(t)
	require.Len(t, entries, 1, "only the benign step may ever reach the executor/audit log")
	assert.Equal(t, "echo comrade-nginx-e2e-marker", entries[0].Command)
	assert.Equal(t, 0, entries[0].ExitCode)
}

// TestFAZ11LLMSuggestsDenylistCommandBlockedAtFixLayerInAutoMode is this
// file's `fix`-layer twin of
// TestFAZ11InstallAndStartNginxAutoModeRunsBenignStepAndBlocksDenylistedStep
// above, closing UYGULAMA_PLANI.md FAZ 11 item 2's "LLM'in denylist komut
// önermesi (block edildiğini doğrula)" requirement for BOTH entry points
// named in the task, not just `do`: a mock diagnosis whose fix plan mixes
// one genuinely benign step with a denylisted `rm -rf /` decoy, run via
// `comrade fix --auto` (the one mode/decoy combination FAZ 7's own tests
// didn't already cover — TestFixAskModeRoutesBlockedStepToRunnerAndSkips...
// covers --ask with a decoy-only plan; TestFixAutoModeExecutesPlan...
// covers --auto with no decoy at all). The benign step actually runs
// against the real executor; the decoy is Blocked by internal/safety and
// never reaches it, regardless of the model's own (wrong) "read" label —
// proving defense-in-depth holds for `fix` in the one mode with no human
// confirmation step, exactly as it does for `do`.
func TestFAZ11LLMSuggestsDenylistCommandBlockedAtFixLayerInAutoMode(t *testing.T) {
	withIsolatedConfigDir(t)
	diagnosisJSON := `{
		"root_cause": "a contrived failure",
		"explanation": "explained in plain language",
		"plan": {"summary": "one benign step, one decoy the model never should have produced", "steps": [
			{"command": "echo comrade-fix-auto-e2e-marker", "rationale": "benign marker step", "risk": "read", "reversible": true},
			{"command": "rm -rf /", "rationale": "a decoy the model must never actually produce", "risk": "read", "reversible": false}
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

	require.Error(t, err, "the run must abort: the plan's second step is Blocked")
	assert.Contains(t, err.Error(), "blocked")

	assert.Contains(t, stdout, "comrade-fix-auto-e2e-marker", "the real executor must have actually run the benign step")
	assert.Contains(t, stdout, "BLOCKED(")
	assert.Contains(t, stdout, "rm -rf /")
	assert.NotContains(t, stdout, "verification", "a run that aborted on a Blocked step must never offer post-solution verification")
	_ = stderr

	entries := readAuditEntries(t)
	require.Len(t, entries, 1, "only the benign step may ever reach the executor/audit log")
	assert.Equal(t, "echo comrade-fix-auto-e2e-marker", entries[0].Command)
	assert.Equal(t, 0, entries[0].ExitCode)
}
