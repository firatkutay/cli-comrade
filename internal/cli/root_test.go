package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// execRoot runs the root command with the given args and returns combined
// stdout/stderr output.
func execRoot(t *testing.T, version string, args ...string) string {
	t.Helper()
	root := NewRootCmd(version)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() returned unexpected error: %v", err)
	}
	return buf.String()
}

func TestRootCmdBareInvocationPrintsVersionAndHelp(t *testing.T) {
	out := execRoot(t, "1.2.3")

	assert.True(t, strings.HasPrefix(out, "comrade version 1.2.3\n\n"),
		"expected output to start with the version banner, got: %q", out)
	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "comrade is a cross-platform AI CLI companion for the terminal")
}

func TestRootCmdVersionFlagPrintsExactVersionString(t *testing.T) {
	out := execRoot(t, "9.9.9", "--version")

	assert.Equal(t, "comrade version 9.9.9\n", out)
}

func TestRootCmdDefaultVersionIsDevWhenUnset(t *testing.T) {
	out := execRoot(t, "dev", "--version")

	assert.Equal(t, "comrade version dev\n", out)
}

func TestSubcommandStubsPrintNotReadyMessage(t *testing.T) {
	// "config" and "init" are deliberately excluded here: FAZ 1 replaced
	// config's stub with a real command tree (internal/cli/config.go)
	// and FAZ 4 replaced init's (internal/cli/init.go, tested in
	// internal/cli/init_test.go). "history" is excluded as of FAZ 6:
	// internal/cli/history.go replaced its stub (tested in
	// internal/cli/history_test.go).
	cases := []struct {
		name string
		args []string
		want string
	}{
		{"fix", []string{"fix"}, "comrade fix: this feature is not ready yet.\n"},
		{"explain", []string{"explain", "ls"}, "comrade explain: this feature is not ready yet.\n"},
		{"chat", []string{"chat"}, "comrade chat: this feature is not ready yet.\n"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			out := execRoot(t, "dev", tc.args...)
			assert.Equal(t, tc.want, out)
		})
	}
}

// --- FAZ 6 root fallback dispatch ------------------------------------------

// TestRootDispatchKnownSubcommandRoutesNormally proves a known subcommand
// name (e.g. "config") is routed to its own command tree, never treated
// as a free-text `do` request.
func TestRootDispatchKnownSubcommandRoutesNormally(t *testing.T) {
	withIsolatedConfigDir(t)

	out := execRoot(t, "dev", "config", "list")
	assert.Contains(t, out, "KEY", "must reach config's own list output, not a do/plan attempt")
	assert.Contains(t, out, "general.mode")
}

// TestRootDispatchUnmatchedArgsFallsBackToDo proves
// UYGULAMA_PLANI.md FAZ 6 item 3's root fallback: `comrade docker kur`
// (an arg vector that matches no known subcommand) is treated as
// `do("docker kur")`, not rejected with cobra's "unknown command" error.
// No mock LLM server is set up here — with an isolated config dir and no
// API key, the pipeline deterministically fails once it actually reaches
// plan generation, which is exactly what proves dispatch worked: the
// error is llm/engine-shaped, never "unknown command" or the old
// "--dry-run" message.
func TestRootDispatchUnmatchedArgsFallsBackToDo(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	_, stderr, err := execRootSplit(t, "dev", "docker", "kur")

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "unknown command")
	assert.Contains(t, err.Error(), "comrade do:", "must have reached runDo, proving free-text dispatch")
	_ = stderr
}

// TestRootDispatchHelpFlagShowsHelp proves --help is intercepted by
// cobra's own help handling before any fallback dispatch runs.
func TestRootDispatchHelpFlagShowsHelp(t *testing.T) {
	out := execRoot(t, "dev", "--help")
	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "comrade is a cross-platform AI CLI companion for the terminal")
}
