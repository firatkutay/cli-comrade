package cli

import (
	"os"
	"strings"
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
