package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const rmRfNodeModulesExplanationJSON = `{
  "summary": "Deletes the node_modules directory and everything inside it, without asking for confirmation.",
  "parts": [
    {"token": "rm", "meaning": "Removes files or directories."},
    {"token": "-r", "meaning": "Removes directories and everything inside them, recursively."},
    {"token": "-f", "meaning": "Never asks for confirmation and ignores missing files."},
    {"token": "node_modules", "meaning": "The directory this command deletes."}
  ],
  "risk_note": "This permanently deletes node_modules; it cannot be undone, though it is usually safe to regenerate."
}`

const lsExplanationJSON = `{
  "summary": "Lists the files in the current directory in long format.",
  "parts": [
    {"token": "ls", "meaning": "Lists directory contents."},
    {"token": "-la", "meaning": "Shows all files, including hidden ones, in a detailed listing."}
  ],
  "risk_note": ""
}`

func TestExplainDestructiveCommandShowsSafetyWarningAndLLMBreakdown(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, rmRfNodeModulesExplanationJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	stdout, stderr, err := execRootSplit(t, "dev", "explain", "rm", "-rf", "node_modules")
	require.NoError(t, err, "stderr: %s", stderr)

	// Layer 1 (local safety.Engine, authoritative): destructive.
	assert.Contains(t, stdout, "destructive")

	// Layer 2 (LLM breakdown), rendered after the safety verdict.
	assert.Contains(t, stdout, "Deletes the node_modules directory")
	assert.Contains(t, stdout, "Removes files or directories.")
	assert.Contains(t, stdout, "node_modules")
	assert.Contains(t, stdout, "permanently deletes")
}

func TestExplainBenignCommandShowsNoSafetyWarning(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, lsExplanationJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	stdout, stderr, err := execRootSplit(t, "dev", "explain", "ls", "-la")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.NotContains(t, stdout, "destructive")
	assert.Contains(t, stdout, "Lists the files in the current directory")
}

// TestExplainDenylistedCommandIsReportedBlocked proves the safety layer's
// verdict is authoritative even for a command the (mocked) LLM would
// otherwise just explain neutrally: `rm -rf /` matches the built-in
// denylist outright (Block), and that must be what's shown regardless of
// what the LLM's own risk_note says.
func TestExplainDenylistedCommandIsReportedBlocked(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, rmRfNodeModulesExplanationJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	stdout, stderr, err := execRootSplit(t, "dev", "explain", "rm", "-rf", "/")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Contains(t, stdout, "denylist")
}

// TestExplainNeverImportsExecutor is a structural guard, independent of
// any one test scenario: internal/cli/explain.go's source must never
// import internal/executor at all — the strongest possible proof that
// `comrade explain` cannot execute a command, since there would be no
// executor type available to call even by mistake.
func TestExplainNeverImportsExecutor(t *testing.T) {
	src, err := os.ReadFile("explain.go")
	require.NoError(t, err)
	assert.NotContains(t, string(src), "internal/executor",
		"comrade explain must never import internal/executor - it must never execute the command it explains")
}

// TestExplainTurkishLanguageProducesTurkishSafetyWarning is FAZ 9's
// acceptance criterion in miniature: COMRADE_LANG=tr must route the
// safety-layer warning through the Turkish catalog (the LLM side is
// exercised independently by internal/engine's own
// TestExplainerRequestsTurkishInstructionBlockWhenConfigured — this test
// only needs to prove internal/cli's own local-safety-warning layer
// picks up the language, since that's the one part of `comrade explain`'s
// output this package renders itself).
func TestExplainTurkishLanguageProducesTurkishSafetyWarning(t *testing.T) {
	withIsolatedConfigDir(t)
	server := newMockPlanServer(t, rmRfNodeModulesExplanationJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")
	t.Setenv("COMRADE_LANG", "tr")

	stdout, stderr, err := execRootSplit(t, "dev", "explain", "rm", "-rf", "node_modules")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.True(t, strings.Contains(stdout, "güvenlik kontrolüne göre"),
		"expected the Turkish safety-warning text, got: %s", stdout)
}

// countingServer starts an httptest server that counts every request it
// receives and answers with planJSON exactly like newMockPlanServer, so
// QA D1's regression test can assert the LLM was (or, for --help/no-args,
// was NOT) actually called — the bug's whole harm was a real network
// call being made silently.
func countingServer(t *testing.T, planJSON string) (srv *httptest.Server, requestCount *int32) {
	t.Helper()
	var count int32
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&count, 1)
		resp := openAICompatResponse{
			Model: "mock-model",
			Choices: []openAICompatChoice{
				{Message: openAICompatMessage{Role: "assistant", Content: planJSON}, FinishReason: "stop"},
			},
		}
		w.Header().Set("content-type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	return srv, &count
}

// TestExplainHelpFlagNeverCallsTheProvider is QA D1's core regression
// guard: "comrade explain --help" (and "-h") must show help and make
// ZERO requests to the LLM provider — the actual reported bug was
// DisableFlagParsing letting "--help" reach runExplain as literal
// command text to explain, silently spending the user's tokens.
func TestExplainHelpFlagNeverCallsTheProvider(t *testing.T) {
	for _, helpArg := range []string{"--help", "-h"} {
		t.Run(helpArg, func(t *testing.T) {
			withIsolatedConfigDir(t)
			server, requests := countingServer(t, lsExplanationJSON)
			defer server.Close()

			t.Setenv("COMRADE_PROVIDER", "openai_compat")
			t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
			t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

			stdout, _, err := execRootSplit(t, "dev", "explain", helpArg)
			require.NoError(t, err)

			assert.Equal(t, int32(0), atomic.LoadInt32(requests), "explain --help must never call the LLM provider")
			assert.Contains(t, stdout, "Usage:")
			assert.Contains(t, stdout, "comrade explain")
		})
	}
}

// TestExplainNoArgsShowsUsageErrorWithoutCallingProvider is
// TestExplainHelpFlagNeverCallsTheProvider's no-args counterpart: a bare
// "comrade explain" must fail with a clear usage error, not attempt to
// explain an empty string via the LLM.
func TestExplainNoArgsShowsUsageErrorWithoutCallingProvider(t *testing.T) {
	withIsolatedConfigDir(t)
	server, requests := countingServer(t, lsExplanationJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	_, _, err := execRootSplit(t, "dev", "explain")
	require.Error(t, err)
	assert.Equal(t, int32(0), atomic.LoadInt32(requests))
	assert.Equal(t, `usage: comrade explain <command...> (to explain a command that starts with a flag, e.g. --help, use "comrade explain -- <command>")`, err.Error())
}

// TestExplainNoArgsShowsUsageErrorInTurkish is the same case under
// COMRADE_LANG=tr, this project's established TR-smoke-test convention.
func TestExplainNoArgsShowsUsageErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "explain")
	require.Error(t, err)
	assert.Equal(t, `kullanım: comrade explain <komut...> (--help gibi bayrakla başlayan bir komutu açıklamak için "comrade explain -- <komut>" kullanın)`, err.Error())
}

// TestExplainDoubleDashEscapeHatchExplainsLiteralHelpFlag proves the
// documented escape hatch (MsgExplainUsageError's own text): "comrade
// explain -- --help" always explains "--help" literally — including
// actually reaching the LLM provider — rather than showing comrade's own
// help, even though "--help" alone (without "--") would.
func TestExplainDoubleDashEscapeHatchExplainsLiteralHelpFlag(t *testing.T) {
	withIsolatedConfigDir(t)
	server, requests := countingServer(t, lsExplanationJSON)
	defer server.Close()

	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", server.URL)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	stdout, stderr, err := execRootSplit(t, "dev", "explain", "--", "--help")
	require.NoError(t, err, "stderr: %s", stderr)

	assert.Equal(t, int32(1), atomic.LoadInt32(requests), "the escape hatch must actually reach the LLM provider")
	assert.Contains(t, stdout, "Lists the files in the current directory")
}
