package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
)

// oneStepReadPlanJSON is a minimal, benign, single-step plan — used by
// this file's do/fix --usage tests, which only care about the usage line
// printed after the run, not about plan content or execution.
const oneStepReadPlanJSON = `{"summary":"list files","steps":[{"command":"echo hi","rationale":"benign","risk":"read","reversible":true}]}`

// openAICompatUsageField/openAICompatResponseWithUsage extend
// do_test.go's openAICompatMessage/openAICompatChoice with a "usage"
// field newMockPlanServer's own openAICompatResponse deliberately omits
// (every existing mock-server test in this package asserts nothing about
// tokens) — this file's tests need real, non-zero prompt/completion
// token counts on the wire.
type openAICompatUsageField struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}
type openAICompatResponseWithUsage struct {
	Model   string                 `json:"model"`
	Choices []openAICompatChoice   `json:"choices"`
	Usage   openAICompatUsageField `json:"usage"`
}

// mockUsagePromptTokens/mockUsageCompletionTokens are the fixed
// prompt/completion token counts newMockPlanServerWithUsage always
// returns — every test in this file expects exactly these, via
// wantUsageLineFragment below.
const (
	mockUsagePromptTokens     = 42
	mockUsageCompletionTokens = 8
)

// newMockPlanServerWithUsage is newMockPlanServer (do_test.go) plus a
// "usage" field carrying a fixed, non-zero prompt/completion token count
// — the model name is always "mock-model", deliberately unpriced in
// llm's pricingTable, so these tests also exercise the "cost unknown,
// tokens still shown" path end-to-end, not just llm.EstimateUSD in
// isolation.
func newMockPlanServerWithUsage(t *testing.T, planJSON string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		resp := openAICompatResponseWithUsage{
			Model: "mock-model",
			Choices: []openAICompatChoice{
				{Message: openAICompatMessage{Role: "assistant", Content: planJSON}, FinishReason: "stop"},
			},
			Usage: openAICompatUsageField{PromptTokens: mockUsagePromptTokens, CompletionTokens: mockUsageCompletionTokens},
		}
		w.Header().Set("content-type", "application/json")
		require.NoError(t, json.NewEncoder(w).Encode(resp))
	}))
}

// wantUsageLineFragment is the exact tail every test in this file
// expects — provider/model here are always openai_compat/mock-model, and
// "mock-model" is never in llm's pricingTable, so the cost segment is
// always omitted (no "· est." / "$" anywhere in these lines).
const wantUsageLineFragment = "tokens: 42 in / 8 out across 1 requests (openai_compat/mock-model)"

func TestDoUsageFlagPrintsSummaryLineOnStderrNotStdout(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, oneStepReadPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	stdout, stderr, err := execRootSplit(t, "dev", "do", "list", "files", "--dry-run", "--usage")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, wantUsageLineFragment)
	assert.NotContains(t, stdout, "tokens:", "the usage line must never reach stdout")
	assert.NotContains(t, stderr, "· est.", "mock-model has no pricingTable entry — cost must be omitted, never guessed")
}

func TestDoWithoutUsageFlagAndConfigOffPrintsNoSummaryLine(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, oneStepReadPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	_, stderr, err := execRootSplit(t, "dev", "do", "list", "files", "--dry-run")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.NotContains(t, stderr, "tokens:", "usage display is opt-in — off by default with neither --usage nor general.show_usage set")
}

func TestDoGeneralShowUsageConfigPrintsSummaryLineWithoutTheFlag(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, oneStepReadPlanJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.show_usage", "true")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "do", "list", "files", "--dry-run")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, wantUsageLineFragment)
}

func TestFixUsageFlagPrintsSummaryLineOnStderr(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, pytonFixDiagnosisJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	seedLastCommand(t, contextpkg.LastCommand{
		Command:    "pyton --version",
		ExitCode:   127,
		StderrTail: "sh: 1: pyton: not found",
		Timestamp:  time.Now(),
		Shell:      "bash",
	})

	stdout, stderr, err := execRootSplit(t, "dev", "fix", "--info", "--usage")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, wantUsageLineFragment)
	assert.NotContains(t, stdout, "tokens:")
}

func TestExplainUsageFlagPrintsSummaryLineOnStderr(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, lsExplanationJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	stdout, stderr, err := execRootSplit(t, "dev", "explain", "--usage", "ls", "-la")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, wantUsageLineFragment)
	assert.NotContains(t, stdout, "tokens:")
}

func TestExplainUsageFlagPrintsSummaryLineInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, lsExplanationJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.language", "tr")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "explain", "--usage", "ls", "-la")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stderr, "token: 42 giriş / 8 çıkış, 1 istekte (openai_compat/mock-model)")
}

func TestExplainWithoutUsageFlagPrintsNoSummaryLine(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, lsExplanationJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	_, stderr, err := execRootSplit(t, "dev", "explain", "ls", "-la")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.NotContains(t, stderr, "tokens:")
}

// TestExplainDoubleDashEscapeStillExplainsLiteralUsageFlag proves the
// "--" escape hatch documented in explain.go's newExplainCmd still wins
// over the hand-parsed --usage stripping: "comrade explain -- --usage"
// must explain the literal string "--usage", never treat it as the
// usage-display flag.
func TestExplainDoubleDashEscapeStillExplainsLiteralUsageFlag(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServerWithUsage(t, lsExplanationJSON)
	defer server.Close()
	setLLMEnv(t, server.URL)

	var capturedContent string
	server.Config.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make(map[string]any)
		_ = json.NewDecoder(r.Body).Decode(&body)
		if msgs, ok := body["messages"].([]any); ok && len(msgs) > 0 {
			last := msgs[len(msgs)-1].(map[string]any)
			capturedContent = last["content"].(string)
		}
		resp := openAICompatResponseWithUsage{
			Model:   "mock-model",
			Choices: []openAICompatChoice{{Message: openAICompatMessage{Role: "assistant", Content: lsExplanationJSON}, FinishReason: "stop"}},
			Usage:   openAICompatUsageField{PromptTokens: 1, CompletionTokens: 1},
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	_, stderr, err := execRootSplit(t, "dev", "explain", "--", "--usage")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, capturedContent, "--usage", "the literal \"--usage\" text must reach the LLM as the command being explained")
	assert.NotContains(t, stderr, "tokens:", "no usage line: --usage after \"--\" is the literal command, not the display flag")
}
