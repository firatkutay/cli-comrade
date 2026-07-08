package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// withIsolatedConfigDir points the config path resolution at a fresh
// temp directory for the duration of the test, so config subcommand
// tests never touch the real user's config.toml.
func withIsolatedConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("HOME", dir)
	}
	return dir
}

// execRootSplit runs the root command with args and returns stdout and
// stderr separately, so tests can assert on stdout purity (e.g. no
// first-run notice, no cobra error/usage boilerplate) independently of
// whatever lands on stderr.
func execRootSplit(t *testing.T, version string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	root := NewRootCmd(version)
	outBuf := &strings.Builder{}
	errBuf := &strings.Builder{}
	root.SetOut(outBuf)
	root.SetErr(errBuf)
	root.SetArgs(args)
	err = root.Execute()
	return outBuf.String(), errBuf.String(), err
}

func TestConfigPathPrintsResolvedPath(t *testing.T) {
	withIsolatedConfigDir(t)

	out := execRoot(t, "dev", "config", "path")

	assert.True(t, strings.HasSuffix(strings.TrimSpace(out), filepath.Join("cli-comrade", "config.toml")),
		"expected path to end with cli-comrade/config.toml, got: %q", out)
}

func TestConfigPathDoesNotCreateFile(t *testing.T) {
	withIsolatedConfigDir(t)

	out := execRoot(t, "dev", "config", "path")
	path := strings.TrimSpace(out)

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err), "config path should not itself create the file")
}

func TestConfigListFirstRunPrintsNoticeOnStderrAndTableOnStdout(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, stderr, err := execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err)

	assert.Contains(t, stderr, "Created default config at")
	assert.NotContains(t, stdout, "Created default config at",
		"the first-run notice must not pollute stdout (e.g. $(comrade config list) capture)")

	assert.Contains(t, stdout, "KEY")
	assert.Contains(t, stdout, "VALUE")
	assert.Contains(t, stdout, "SOURCE")
	assert.Contains(t, stdout, "general.mode")
	assert.Contains(t, stdout, "ask")
	// A freshly-created file (from defaultConfigTOML, which is never
	// sparse) explicitly contains every key, so every row's source is
	// "file" here, not "default" — see
	// TestLoaderSourceReportsDefaultThenFileThenEnv in
	// internal/config/loader_test.go for the case ("default") that
	// actually is reachable, against a hand-edited partial file.
	assert.Contains(t, stdout, "file")
}

func TestConfigListSecondRunDoesNotRepeatNotice(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err)
	_, stderr, err := execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err)

	assert.NotContains(t, stderr, "Created default config at")
}

func TestConfigGetPrintsDefaultValueOnStdoutOnly(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, stderr, err := execRootSplit(t, "dev", "config", "get", "general.mode")
	require.NoError(t, err)

	assert.Contains(t, stderr, "Created default config at", "first run must still create the file")
	assert.Equal(t, "ask\n", stdout, "stdout must contain exactly the value, nothing else")
}

func TestConfigGetUnknownKeyErrors(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "get", "general.bogus")

	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown config key")
	assert.ErrorContains(t, err, "general.mode")
}

func TestConfigSetPersistsAndGetReflectsItOnStdoutOnly(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.mode", "auto")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "general.mode")
	require.NoError(t, err)
	assert.Equal(t, "auto\n", stdout)
}

func TestConfigSetGetFallbackRoundTripRendersCommaFormat(t *testing.T) {
	withIsolatedConfigDir(t)
	const want = "ollama/llama3.1,openai_compat/gpt-4o-mini"

	setStdout, _, err := execRootSplit(t, "dev", "config", "set", "llm.fallback", want)
	require.NoError(t, err)
	assert.Equal(t, "llm.fallback = "+want+"\n", setStdout, "set's own echo must already use the comma format")

	getStdout, _, err := execRootSplit(t, "dev", "config", "get", "llm.fallback")
	require.NoError(t, err)
	assert.Equal(t, want+"\n", getStdout,
		"get must render the value read back from the merged/persisted config the same way set echoed it, "+
			"not Go's default []interface{} slice format")

	listStdout, _, err := execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err)
	assert.Contains(t, listStdout, "llm.fallback", "sanity: the row must be present")
	for _, line := range strings.Split(listStdout, "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "llm.fallback") {
			assert.Contains(t, line, want, "list row must render the same comma format")
			assert.NotContains(t, line, "[", "list row must not render Go's slice bracket syntax")
			return
		}
	}
	t.Fatal("llm.fallback row not found in config list output")
}

func TestConfigSetGetDenylistExtraRoundTripRendersCommaFormat(t *testing.T) {
	withIsolatedConfigDir(t)
	const want = "rm -rf /custom,shutdown -h now"

	_, _, err := execRootSplit(t, "dev", "config", "set", "safety.denylist_extra", want)
	require.NoError(t, err)

	getStdout, _, err := execRootSplit(t, "dev", "config", "get", "safety.denylist_extra")
	require.NoError(t, err)
	assert.Equal(t, want+"\n", getStdout)
}

func TestConfigGetEmptyListRendersEmptyNotBrackets(t *testing.T) {
	withIsolatedConfigDir(t)

	// llm.fallback defaults to an empty list; nothing has set it yet.
	stdout, _, err := execRootSplit(t, "dev", "config", "get", "llm.fallback")
	require.NoError(t, err)
	assert.Equal(t, "\n", stdout, "an empty list must render as an empty string, not \"[]\"")

	stdout, _, err = execRootSplit(t, "dev", "config", "get", "safety.denylist_extra")
	require.NoError(t, err)
	assert.Equal(t, "\n", stdout, "an empty list must render as an empty string, not \"[]\"")
}

func TestConfigListEmptyListRowRendersEmptyCellNotBrackets(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, _, err := execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err)

	for _, line := range strings.Split(stdout, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "llm.fallback") || strings.HasPrefix(trimmed, "safety.denylist_extra") {
			assert.NotContains(t, line, "[]", "empty-list row must not render Go's \"[]\" slice syntax: %q", line)
		}
	}
}

func TestConfigSetInvalidEnumRejectedWithHelpfulMessage(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.mode", "hizli")

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid value")
	assert.ErrorContains(t, err, "general.mode")
	assert.ErrorContains(t, err, "auto, ask, info")
}

func TestConfigSetInvalidValueProducesNoCobraSideOutput(t *testing.T) {
	// Regression guard for the double/triple error-output bug: cobra must
	// not print its own "Error: ..." line or a "Usage:" block on top of
	// the error cmd/comrade/main.go already prints once. With
	// SilenceErrors/SilenceUsage set on the root command, a failing
	// RunE must leave both of cobra's own output streams untouched.
	withIsolatedConfigDir(t)

	stdout, stderr, err := execRootSplit(t, "dev", "config", "set", "general.mode", "hizli")

	require.Error(t, err)
	assert.Equal(t, `invalid value "hizli" for general.mode; must be one of: auto, ask, info`, err.Error())
	assert.Empty(t, stdout, "stdout must stay clean on error")
	assert.Empty(t, stderr, "cobra itself must print nothing; main.go alone prints the error, once")
	assert.NotContains(t, stderr, "Usage:")
}

func TestConfigSetInvalidIntRejected(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.timeout_seconds", "-5")

	require.Error(t, err)
	assert.ErrorContains(t, err, "greater than 0")
}

func TestConfigSetUnknownKeyRejected(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.bogus", "x")

	require.Error(t, err)
	assert.ErrorContains(t, err, "unknown config key")
}

func TestConfigSetDoesNotPersistInvalidValue(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, _ = execRootSplit(t, "dev", "config", "set", "general.mode", "hizli") // expected to fail; asserted elsewhere

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "general.mode")
	require.NoError(t, err)
	assert.Equal(t, "ask\n", stdout, "an invalid set must leave the default untouched")
}

func TestConfigEditOpensEditorOnConfigFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake shell-script editor is Unix-only")
	}

	dir := withIsolatedConfigDir(t)
	touchedPath := filepath.Join(dir, "editor-touched")

	// The real editor invocation's only argument is the config file
	// path, which this test doesn't know ahead of resolving it itself —
	// so the fake "editor" is a wrapper script that touches a fixed
	// marker file, ignoring its argument, rather than trying to predict
	// and assert on the config path directly.
	script := filepath.Join(dir, "fake-editor.sh")
	require.NoError(t, os.WriteFile(script, []byte("#!/bin/sh\ntouch \""+touchedPath+"\"\n"), 0o755))
	t.Setenv("EDITOR", script)

	_ = execRoot(t, "dev", "config", "edit")

	_, err := os.Stat(touchedPath)
	assert.NoError(t, err, "expected the configured $EDITOR to have run")
}

func TestConfigTestLLMPrintsProviderModelAndLatency(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5.4-mini","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", srv.URL)
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "test-llm")
	require.NoError(t, err)

	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Contains(t, stdout, "provider=openai_compat")
	assert.Contains(t, stdout, "model=gpt-5.4-mini")
	assert.Contains(t, stdout, "latency=")
}

func TestConfigTestLLMIsHiddenFromHelp(t *testing.T) {
	out := execRoot(t, "dev", "config", "--help")
	assert.NotContains(t, out, "test-llm")
}

func TestConfigTestLLMReportsHelpfulErrorOnMissingKey(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	_, _, err := execRootSplit(t, "dev", "config", "test-llm")

	require.Error(t, err)
	assert.ErrorContains(t, err, "ANTHROPIC_API_KEY")
}

func TestRootHelpFlagStillPrintsUsage(t *testing.T) {
	// Guards against SilenceUsage/SilenceErrors (added to fix the
	// duplicate error-output bug) accidentally suppressing the
	// explicitly-requested --help output too: SilenceUsage only affects
	// the automatic usage dump on a RunE error, not a deliberate --help.
	out := execRoot(t, "dev", "--help")

	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "config")
}
