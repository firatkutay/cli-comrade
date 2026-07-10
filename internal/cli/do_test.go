package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// dockerKurPlanJSON is the canned model response used by
// TestDoDryRunRendersPlanTableAgainstMockProvider: three steps for a
// "docker kur" request, including one elevated `sudo apt-get install`
// step and one `rm -rf /` decoy step that internal/safety must Block
// regardless of what the model itself said about it. The decoy is
// deliberately labeled "read" (not "destructive") — the strongest form of
// this proof (MINOR 7 hardening): the safety engine's denylist Block
// doesn't even consult the declared risk, so it must fire even when the
// model's own label is actively, maximally wrong in the *opposite*
// direction (claiming something totally benign).
const dockerKurPlanJSON = `{
  "summary": "Docker kurulur ve başlatılır.",
  "steps": [
    {"command": "sudo apt-get install -y docker.io", "rationale": "Docker paketini kurar.", "risk": "elevated", "reversible": false},
    {"command": "sudo systemctl enable --now docker", "rationale": "Docker servisini etkinleştirir ve başlatır.", "risk": "elevated", "reversible": true},
    {"command": "rm -rf /", "rationale": "Modelin asla üretmemesi gereken bir deneme.", "risk": "read", "reversible": false}
  ]
}`

// openAICompatMessage/openAIChoice/openAICompatResponse mirror just
// enough of internal/llm's openAI-compatible wire shape to build a canned
// /chat/completions response — internal/llm's own shape is unexported, so
// this test builds the JSON body directly rather than importing it.
type openAICompatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
type openAICompatChoice struct {
	Message      openAICompatMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
}
type openAICompatResponse struct {
	Model   string               `json:"model"`
	Choices []openAICompatChoice `json:"choices"`
}

// newMockPlanServer starts an httptest server standing in for an
// openai_compat-compatible /chat/completions endpoint, always answering
// with planJSON as the assistant message content.
func newMockPlanServer(t *testing.T, planJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		resp := openAICompatResponse{
			Model: "mock-model",
			Choices: []openAICompatChoice{
				{Message: openAICompatMessage{Role: "assistant", Content: planJSON}, FinishReason: "stop"},
			},
		}
		w.Header().Set("content-type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
}

// TestDoDryRunRendersPlanTableAgainstMockProvider is FAZ 5's end-to-end
// proof: `comrade do "docker kur" --dry-run`, pointed at a mock
// openai_compat server via config env overrides, renders the model's
// 3-step plan as a table — and independently proves internal/safety's
// second check by Blocking the decoy `rm -rf /` step even though the
// model itself labeled it "read" (not caught by any mode/override, since
// this phase performs no execution at all). The RISK column renders the
// safety engine's EffectiveRisk/Action, never the model's raw label
// (MEDIUM 6): the two elevated, non-blocked steps render
// "CONFIRM(elevated)", and the decoy renders "BLOCKED(<reason>)" instead
// of "read".
func TestDoDryRunRendersPlanTableAgainstMockProvider(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, dockerKurPlanJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	stdout, stderr, err := execRootSplit(t, "dev", "do", "docker", "kur", "--dry-run")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "Docker kurulur ve başlatılır.")

	assert.Contains(t, stdout, "STEP")
	assert.Contains(t, stdout, "COMMAND")
	assert.Contains(t, stdout, "RISK")
	assert.Contains(t, stdout, "REVERSIBLE")
	assert.Contains(t, stdout, "RATIONALE")

	assert.Contains(t, stdout, "sudo apt-get install -y docker.io")
	assert.Contains(t, stdout, "sudo systemctl enable --now docker")

	// Both elevated steps must render the safety engine's EffectiveRisk
	// wrapped as CONFIRM(...), not the model's bare "elevated" label.
	assert.Contains(t, stdout, "CONFIRM(elevated)")

	// The decoy rm -rf / step must be Blocked, rendered as
	// BLOCKED(<reason>) rather than its self-declared "read" label —
	// the denylist Block never even consults the declared risk.
	assert.Contains(t, stdout, "rm -rf /")
	assert.Contains(t, stdout, "BLOCKED(")
	assert.Contains(t, stdout, "denylist rule")
}

// TestDoShowsSpinnerOnStderrWhileWaitingForPlan is Part 2(d)'s wiring
// proof for `comrade do`: with CLICOLOR_FORCE=1 (so the spinner renders
// even though this test's stderr is a strings.Builder, not a real TTY —
// exactly the env-var override QA/tests are meant to use per
// resolveColorEnabled's own contract), planner.GeneratePlan's blocking
// call against the mock server leaves the spinner's ANSI clear-line
// sequence in stderr, and NOTHING spinner-related ever reaches stdout,
// which stays exactly the plain rendered plan table it was before this
// feature existed.
func TestDoShowsSpinnerOnStderrWhileWaitingForPlan(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, dockerKurPlanJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")
	t.Setenv("CLICOLOR_FORCE", "1")

	stdout, stderr, err := execRootSplit(t, "dev", "do", "docker", "kur", "--dry-run")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, "\r\x1b[K", "spinner's clear-line sequence must appear in stderr")
	assert.Contains(t, stderr, "thinking…", "spinner's EN label must appear in stderr")
	assert.NotContains(t, stdout, "\x1b[K", "spinner output must never reach stdout")
	assert.NotContains(t, stdout, "thinking…", "spinner label must never reach stdout")
	assert.Contains(t, stdout, "Docker kurulur ve başlatılır.", "the real plan table must still render on stdout, unaffected")
}

// benignAndDecoyPlanJSON is the canned model response for
// TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor:
// one truly benign `echo` step, and one `rm -rf /` decoy mislabeled
// "read" — exactly docs/history/UYGULAMA_PLANI.md FAZ 6's own end-to-end acceptance
// scenario.
const benignAndDecoyPlanJSON = `{
  "summary": "Prints a marker, then a decoy the model never should have produced.",
  "steps": [
    {"command": "echo comrade-auto-e2e-marker", "rationale": "benign marker step", "risk": "read", "reversible": true},
    {"command": "rm -rf /", "rationale": "a decoy the model must never actually produce", "risk": "read", "reversible": false}
  ]
}`

// TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor
// is FAZ 6's end-to-end proof, run against the REAL internal/executor
// (no executor fake): `comrade do ... --auto`, backed by a mock
// openai_compat plan server, actually runs the benign echo step (its
// real stdout appears) while the mislabeled `rm -rf /` step is Blocked by
// internal/safety and never reaches the executor at all — proven both by
// its absence from stdout's executed-step evidence and by the audit log
// only ever recording the one step that actually ran.
func TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, benignAndDecoyPlanJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	stdout, stderr, err := execRootSplit(t, "dev", "do", "print", "a", "marker", "--auto")

	require.Error(t, err, "the run must abort: the plan's second step is Blocked")
	assert.Contains(t, err.Error(), "blocked")

	assert.Contains(t, stdout, "comrade-auto-e2e-marker", "the real executor must have actually run the benign echo step")
	assert.Contains(t, stdout, "BLOCKED(")
	assert.Contains(t, stdout, "rm -rf /")
	assert.Contains(t, stdout, "1 executed, 0 skipped, 1 blocked")
	_ = stderr

	// Independently confirm via the audit log: exactly one entry (the
	// benign step), never the blocked one.
	entries := readAuditEntries(t)
	require.Len(t, entries, 1)
	assert.Equal(t, "echo comrade-auto-e2e-marker", entries[0].Command)
	assert.Equal(t, 0, entries[0].ExitCode)
}

// TestDoIsVisibleInHelp proves `do` is FAZ 6's real, user-facing entry
// point (no longer the FAZ 5 hidden diagnostic command).
func TestDoIsVisibleInHelp(t *testing.T) {
	withIsolatedConfigDir(t)
	out := execRoot(t, "dev")
	assert.Contains(t, out, "do          Generate a plan", "`do` must be a visible subcommand as of FAZ 6")
}

// TestDoNoArgsShowsTranslatedUsageError proves `comrade do`'s Args
// (translatedMinArgs, do.go) renders a friendly, i18n'd usage error —
// including an actionable example request — for a bare `comrade do`
// invocation, instead of cobra's raw English "requires at least 1
// arg(s), only received 0". No mock LLM server is set up: a translated
// arg-count usage error must fire before any plan generation is even
// attempted.
func TestDoNoArgsShowsTranslatedUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "do")

	require.Error(t, err)
	assert.Equal(t, `usage: comrade do <request...> (e.g. comrade do "install docker")`, err.Error())
}

// TestDoNoArgsShowsTranslatedUsageErrorInTurkish is the same proof under
// COMRADE_LANG=tr.
func TestDoNoArgsShowsTranslatedUsageErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "do")

	require.Error(t, err)
	assert.Equal(t, `kullanım: comrade do <istek...> (örnek: comrade do "docker kur")`, err.Error())
}

// TestRenderPlanShowsEffectiveRiskNotDeclaredRisk is renderPlan's own
// exact-value unit test for MEDIUM 6: the RISK column must always render
// internal/safety's independent Decision, never the LLM-declared
// step.Risk it was built from — an Allow row shows the plain effective
// risk name, a Confirm row shows "CONFIRM(<effective risk>)", and a
// Block row shows "BLOCKED(<reason>)", regardless of what step.Risk says.
func TestRenderPlanShowsEffectiveRiskNotDeclaredRisk(t *testing.T) {
	plan := engine.Plan{
		Summary: "Test plan.",
		Steps: []engine.Step{
			{
				Command: "ls -la", Rationale: "lists files", Risk: safety.RiskDestructive, Reversible: true,
				Decision: safety.Decision{Action: safety.Allow, EffectiveRisk: safety.RiskRead},
			},
			{
				Command: "sudo systemctl restart nginx", Rationale: "restarts nginx", Risk: safety.RiskWrite, Reversible: true,
				Decision: safety.Decision{Action: safety.Confirm, EffectiveRisk: safety.RiskElevated, Reason: "escalated by sudo"},
			},
			{
				Command: "rm -rf /", Rationale: "decoy", Risk: safety.RiskRead, Reversible: false,
				Decision: safety.Decision{Action: safety.Block, EffectiveRisk: safety.RiskDestructive, Reason: "matches denylist rule: rm -rf /"},
			},
		},
	}

	var buf bytes.Buffer
	require.NoError(t, renderPlan(&buf, plan, i18n.NewTranslator(i18n.LangEN)))
	out := buf.String()

	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	require.Len(t, lines, 6, "summary, blank line, header, 3 step rows")
	assert.Equal(t, "Test plan.", lines[0])
	assert.Equal(t, "", lines[1])

	// tabwriter separates columns with a run of 2+ spaces (its own
	// padding), while a multi-word command/rationale value only ever
	// has single spaces between its own words — so splitting on a 2+
	// space run reliably recovers columns without being confused by a
	// command like "sudo systemctl restart nginx".
	columnSplit := regexp.MustCompile(`\s{2,}`)
	columns := func(line string) []string { return columnSplit.Split(line, -1) }

	// Step 1: Allow with EffectiveRisk read — even though step.Risk was
	// declared "destructive" — renders the plain risk name "read".
	row1 := columns(lines[3])
	require.Len(t, row1, 5)
	assert.Equal(t, "read", row1[2], "an Allow row must show the EffectiveRisk name, not the declared risk")

	// Step 2: Confirm with EffectiveRisk elevated renders
	// "CONFIRM(elevated)", not the declared "write".
	row2 := columns(lines[4])
	require.Len(t, row2, 5)
	assert.Equal(t, "CONFIRM(elevated)", row2[2])

	// Step 3: Block renders "BLOCKED(<reason>)", not the declared "read".
	row3 := columns(lines[5])
	require.Len(t, row3, 5)
	assert.Equal(t, "BLOCKED(matches denylist rule: rm -rf /)", row3[2])
}
