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

// withIsolatedConfigDir points the config path resolution AND (as of FAZ
// 6) the audit log path resolution at a fresh temp directory for the
// duration of the test, so config/do/history tests never touch the real
// user's config.toml or audit.jsonl. XDG_STATE_HOME/LOCALAPPDATA are set
// explicitly (not left to internal/audit.PathFor's HOME fallback) so this
// helper isolates both resolvers symmetrically regardless of which one a
// given test actually exercises.
func withIsolatedConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if runtime.GOOS == "windows" {
		t.Setenv("APPDATA", dir)
		t.Setenv("LOCALAPPDATA", dir)
	} else {
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("XDG_STATE_HOME", dir)
		t.Setenv("HOME", dir)
	}
	return dir
}

// execRootSplit runs the root command with args and returns stdout and
// stderr separately, so tests can assert on stdout purity (e.g. no
// first-run notice, no cobra error/usage boilerplate) independently of
// whatever lands on stderr.
func execRootSplit(t *testing.T, version string, args ...string) (stdout, stderr string, err error) { //nolint:unparam // version mirrors NewRootCmd's own real parameter (the build-time version); every current call site happens to pass "dev", but that's incidental to what's under test, not a reason to strip the parameter a real caller varies
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

// TestConfigSetUnknownKeyRejectedInTurkish is QA D4a's core regression
// guard: `comrade config set`'s validation errors used to surface
// internal/config.Validate's own hardcoded English text VERBATIM,
// bypassing i18n entirely, unlike every other user-facing message in
// this tree (QA found this specifically via "unknown config key ...
// valid keys are ..." staying English under general.language=tr).
// config.Validate/Loader now return structured
// UnknownKeyError/InvalidValueError values that internal/cli/config.go's
// translateConfigError re-renders through i18n via errors.As — this is
// the exact-match TR smoke test for that path, using envOnlyTranslator
// (COMRADE_LANG, since "set" validates before ever loading config).
func TestConfigSetUnknownKeyRejectedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.bogus", "x")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "bilinmeyen config anahtarı"), "got: %s", err.Error())
	assert.NotContains(t, err.Error(), "unknown config key")
}

// TestConfigSetInvalidEnumRejectedInTurkish is
// TestConfigSetUnknownKeyRejectedInTurkish's counterpart for
// InvalidValueError{Reason: ReasonInvalidEnum} — exact match, pinning
// the full translated sentence, not just a substring.
func TestConfigSetInvalidEnumRejectedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "set", "general.mode", "hizli")

	require.Error(t, err)
	assert.Equal(t, `"hizli" değeri general.mode için geçersiz; şunlardan biri olmalı: auto, ask, info`, err.Error())
}

// TestConfigGetUnknownKeyRejectedInTurkish proves `config get`'s SAME
// error class — reached through a different call path (Loader.Get,
// after config IS loaded, so via the real resolved general.language, not
// envOnlyTranslator) — is translated too, not just "set"'s.
func TestConfigGetUnknownKeyRejectedInTurkish(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cli-comrade"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "cli-comrade", "config.toml"),
		[]byte("[general]\nlanguage = \"tr\"\n"), 0o600))

	_, _, err := execRootSplit(t, "dev", "config", "get", "general.bogus")

	require.Error(t, err)
	assert.True(t, strings.HasPrefix(err.Error(), "bilinmeyen config anahtarı"), "got: %s", err.Error())
}

// TestConfigSetNonHTTPBaseURLRejectedInTurkish is QA D4a's pattern
// (TestConfigSetInvalidEnumRejectedInTurkish) applied to the two base_url
// InvalidValueReasons (ReasonNotURL/ReasonMetadataOrLinkLocal) that used
// to fall through translateConfigError's switch untranslated — the fix
// this task closes. `comrade config set llm.ollama.base_url ftp://x` must
// still be a HARD reject (Fix A's "Validate stays a hard reject,
// unchanged" requirement), and now render fully in Turkish.
func TestConfigSetNonHTTPBaseURLRejectedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.ollama.base_url", "ftp://x")

	require.Error(t, err)
	assert.Equal(t, `"ftp://x" değeri llm.ollama.base_url için geçersiz: geçerli bir http:// veya https:// URL'si olmalı ve bir host içermeli`, err.Error())
}

// TestConfigSetNonHTTPBaseURLRejectedInEnglish is the EN-default
// counterpart, pinning the exact same text `config.Validate` has always
// produced for this case (byte-identical, per this catalog's own
// convention) — proving the new translateConfigError case didn't change
// English behavior at all.
func TestConfigSetNonHTTPBaseURLRejectedInEnglish(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.ollama.base_url", "ftp://x")

	require.Error(t, err)
	assert.Equal(t, `invalid value "ftp://x" for llm.ollama.base_url: must be a valid http:// or https:// URL with a host`, err.Error())
}

// TestConfigSetMetadataBaseURLRejectedInTurkish is
// TestConfigSetNonHTTPBaseURLRejectedInTurkish's counterpart for the other
// reject reason, ReasonMetadataOrLinkLocal.
func TestConfigSetMetadataBaseURLRejectedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", "https://169.254.169.254/v1")

	require.Error(t, err)
	assert.Equal(t, `"https://169.254.169.254/v1" değeri llm.openai_compat.base_url için geçersiz: bulut metadata / link-local adresine izin verilmiyor`, err.Error())
}

// TestConfigCommandsWorkOnFileWithMetadataBaseURLForInactiveProvider is the
// CLI-level mirror of internal/config's
// TestLoaderDoesNotBrickOnMetadataBaseURLForInactiveProvider — the
// reviewer's exact proof, run at the real `comrade config` command
// surface: a file whose llm.openai_compat.base_url is a hand-edited
// cloud-metadata address, with openai_compat NOT the active provider,
// used to make EVERY config subcommand fail at Load() — including
// `config set`/`config edit`, the very commands that would otherwise fix
// it — with no in-tool way back in. All of path/get/set/list must now
// work normally.
func TestConfigCommandsWorkOnFileWithMetadataBaseURLForInactiveProvider(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cli-comrade"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "cli-comrade", "config.toml"),
		[]byte("[llm]\nprovider = \"anthropic\"\n\n[llm.openai_compat]\nbase_url = \"https://169.254.169.254/v1\"\n"),
		0o600))

	_, _, err := execRootSplit(t, "dev", "config", "path")
	require.NoError(t, err, "config path must not even load config, so it was never broken by this bug, but pin it anyway")

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "llm.openai_compat.base_url")
	require.NoError(t, err, "config get must survive a bad INACTIVE-provider base_url on disk")
	assert.Equal(t, "https://169.254.169.254/v1\n", stdout)

	stdout, _, err = execRootSplit(t, "dev", "config", "list")
	require.NoError(t, err, "config list must survive it too")
	assert.Contains(t, stdout, "llm.openai_compat.base_url")

	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", "https://api.openai.com/v1")
	require.NoError(t, err, "config set — the actual repair command — must remain reachable")

	stdout, _, err = execRootSplit(t, "dev", "config", "get", "llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1\n", stdout, "the repaired value must have persisted")
}

// TestConfigCommandsWorkOnFileWithMetadataBaseURLForActiveProvider is
// TestConfigCommandsWorkOnFileWithMetadataBaseURLForInactiveProvider's
// counterpart when the bad base_url DOES belong to the active provider:
// config subcommands must still all work (Load() only warns, never
// fails) — only building an actual LLM client (do/fix/chat/explain; see
// do_test.go's TestDoRefusesToBuildClientForMetadataBaseURLActiveProvider)
// refuses.
func TestConfigCommandsWorkOnFileWithMetadataBaseURLForActiveProvider(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cli-comrade"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "cli-comrade", "config.toml"),
		[]byte("[llm]\nprovider = \"openai_compat\"\n\n[llm.openai_compat]\nbase_url = \"https://169.254.169.254/v1\"\n"),
		0o600))

	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.openai_compat.base_url", "https://api.openai.com/v1")
	require.NoError(t, err, "config set must remain reachable even for the ACTIVE provider's bad value")

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "llm.openai_compat.base_url")
	require.NoError(t, err)
	assert.Equal(t, "https://api.openai.com/v1\n", stdout)
}

func TestConfigSetDoesNotPersistInvalidValue(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, _ = execRootSplit(t, "dev", "config", "set", "general.mode", "hizli") // expected to fail; asserted elsewhere

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "general.mode")
	require.NoError(t, err)
	assert.Equal(t, "ask\n", stdout, "an invalid set must leave the default untouched")
}

// TestConfigSetHelpFlagExitsZeroWithUsage is QA D2's core regression
// guard: "comrade config set --help" (and "-h") used to fail with
// cobra's own raw "accepts 2 arg(s), received 1" error and never show
// help at all — newConfigSetCmd's DisableFlagParsing meant cobra's own
// automatic -h/--help interception never ran, and cobra.ExactArgs(2)
// rejected the single "--help" argument before RunE was ever reached.
func TestConfigSetHelpFlagExitsZeroWithUsage(t *testing.T) {
	for _, helpArg := range []string{"--help", "-h"} {
		t.Run(helpArg, func(t *testing.T) {
			withIsolatedConfigDir(t)

			stdout, _, err := execRootSplit(t, "dev", "config", "set", helpArg)

			require.NoError(t, err)
			assert.Contains(t, stdout, "Usage:")
			assert.Contains(t, stdout, "comrade config set")
			assert.Contains(t, stdout, "Validate and persist a config key's value")
		})
	}
}

// TestConfigSetWrongArgCountShowsTranslatedUsageError proves the
// (now-mandatory, since Args is cobra.ArbitraryArgs) manual arg-count
// check RunE performs itself renders a translated usage error rather
// than either cobra's own raw English message or an out-of-range panic
// from indexing args[0]/args[1] directly.
func TestConfigSetWrongArgCountShowsTranslatedUsageError(t *testing.T) {
	for _, extraArgs := range [][]string{{"onlykey"}, {"a", "b", "c"}, {}} {
		withIsolatedConfigDir(t)
		args := append([]string{"config", "set"}, extraArgs...)
		_, _, err := execRootSplit(t, "dev", args...)
		require.Error(t, err)
		assert.Equal(t, "usage: comrade config set <key> <value>", err.Error())
	}
}

// TestConfigSetWrongArgCountShowsTranslatedUsageErrorInTurkish is the
// same case under COMRADE_LANG=tr.
func TestConfigSetWrongArgCountShowsTranslatedUsageErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "set", "onlykey")
	require.Error(t, err)
	assert.Equal(t, "kullanım: comrade config set <anahtar> <değer>", err.Error())
}

// TestConfigSetWrongArgCountShowsTranslatedUsageErrorInTurkishFromConfigLanguageAlone
// is the exact scenario a real host exposed: general.language="tr" in the
// on-disk config file, with NO COMRADE_LANG/LANG/LC_ALL set at all.
// envOnlyTranslator (which skips config entirely) rendered this in
// English; bestEffortTranslator (which loads config first) must render
// it in Turkish.
func TestConfigSetWrongArgCountShowsTranslatedUsageErrorInTurkishFromConfigLanguageAlone(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "")
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "cli-comrade"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "cli-comrade", "config.toml"),
		[]byte("[general]\nlanguage = \"tr\"\n"), 0o600))

	_, _, err := execRootSplit(t, "dev", "config", "set", "onlykey")

	require.Error(t, err)
	assert.Equal(t, "kullanım: comrade config set <anahtar> <değer>", err.Error())
}

// TestConfigSetWrongArgCountShowsTranslatedUsageErrorInEnglishWhenNeitherConfigNorEnvSetLanguage
// is the EN-default counterpart: a totally fresh install (no config file
// yet, no language env vars) must still render English, and must still
// create the default config file — consistency with every other
// command's first invocation, now that this usage-error path loads
// config too.
func TestConfigSetWrongArgCountShowsTranslatedUsageErrorInEnglishWhenNeitherConfigNorEnvSetLanguage(t *testing.T) {
	dir := withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "")
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")

	_, stderr, err := execRootSplit(t, "dev", "config", "set", "onlykey")

	require.Error(t, err)
	assert.Equal(t, "usage: comrade config set <key> <value>", err.Error())
	assert.Contains(t, stderr, "Created default config at")
	_, statErr := os.Stat(filepath.Join(dir, "cli-comrade", "config.toml"))
	assert.NoError(t, statErr, "the usage-error path must create the default config file, same as every other command's first invocation")
}

// TestConfigGetWrongArgCountShowsTranslatedUsageError proves `comrade
// config get`'s Args (translatedExactArgs, config.go) renders a
// friendly, i18n'd usage error instead of cobra's raw English "accepts 1
// arg(s), received 0/2", for both too few (0) and too many (2+)
// arguments.
func TestConfigGetWrongArgCountShowsTranslatedUsageError(t *testing.T) {
	for _, extraArgs := range [][]string{{}, {"a", "b"}} {
		withIsolatedConfigDir(t)
		args := append([]string{"config", "get"}, extraArgs...)
		_, _, err := execRootSplit(t, "dev", args...)
		require.Error(t, err)
		assert.Equal(t, "usage: comrade config get <key>", err.Error())
	}
}

// TestConfigListStrayArgShowsTranslatedUsageError proves `comrade config
// list`'s Args (translatedNoArgs) renders a friendly, i18n'd usage error
// naming its own full command path, instead of cobra's raw English
// "accepts 0 arg(s), received 1", when given a stray positional argument.
func TestConfigListStrayArgShowsTranslatedUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "list", "unexpected")

	require.Error(t, err)
	assert.Equal(t, "comrade config list does not take any arguments", err.Error())
}

// TestConfigUnknownSubcommandShowsTranslatedError proves `comrade config
// <bogus>` renders a friendly, i18n'd "unknown subcommand" error
// (translatedUnknownSubcommand, argvalidation.go) naming every real,
// non-Hidden subcommand — "test-llm" (Hidden) must NOT appear — instead
// of silently printing help and exiting 0.
func TestConfigUnknownSubcommandShowsTranslatedError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "bogus")

	require.Error(t, err)
	assert.Equal(t, `unknown subcommand "bogus" for comrade config (expected one of: edit, get, list, models, path, profile, set)`, err.Error())
	assert.NotContains(t, err.Error(), "test-llm", "the Hidden test-llm subcommand must never appear in the suggested list")
}

// TestConfigUnknownSubcommandShowsTranslatedErrorInTurkish is the same
// proof under COMRADE_LANG=tr.
func TestConfigUnknownSubcommandShowsTranslatedErrorInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "bogus")

	require.Error(t, err)
	assert.Equal(t, `"bogus": comrade config için bilinmeyen alt komut (beklenen: edit, get, list, models, path, profile, set)`, err.Error())
}

// TestConfigBareInvocationStillPrintsHelpAndExitsZero is `comrade
// config`'s counterpart to TestAuthBareInvocationStillPrintsHelpAndExitsZero.
func TestConfigBareInvocationStillPrintsHelpAndExitsZero(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, _, err := execRootSplit(t, "dev", "config")

	require.NoError(t, err)
	assert.Contains(t, stdout, "Usage:")
	assert.Contains(t, stdout, "comrade config")
	assert.Contains(t, stdout, "get")
}

// TestConfigGetSubcommandResolutionBypassesParentUnknownSubcommandCheck
// is `comrade config`'s counterpart to auth_test.go's
// TestAuthLoginSubcommandResolutionBypassesParentUnknownSubcommandCheck —
// proving cobra's Find() resolves a real child name ("get") all the way
// down to the leaf command, so the parent's translatedUnknownSubcommand
// never runs for it.
func TestConfigGetSubcommandResolutionBypassesParentUnknownSubcommandCheck(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "get")

	require.Error(t, err)
	assert.Equal(t, "usage: comrade config get <key>", err.Error())
	assert.NotContains(t, err.Error(), "unknown subcommand")
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
	withIsolatedConfigDir(t)
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

// TestConfigTestLLMRefusesToBuildClientForMetadataBaseURLActiveProvider is
// the reviewer-flagged residual's fix for `config test-llm`: it used to
// wrap llm.New's error as "test-llm: %w", surfacing a reject-class
// base_url's raw, untranslated *config.InvalidValueError.Error() text
// verbatim. Like TestDoRefusesToBuildClientForMetadataBaseURLActiveProvider
// (do_test.go), the active provider's base_url reaches config via a
// COMRADE_ env var — `config set` itself still hard-rejects this value
// directly, so it cannot be the source here — and the command must now
// refuse with the SAME translated, actionable MsgLLMBaseURLRejected
// message do/fix/explain/chat already render for this error class, not
// the raw wrap-chain.
func TestConfigTestLLMRefusesToBuildClientForMetadataBaseURLActiveProvider(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", "http://169.254.169.254/latest/meta-data/")
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	_, _, err := execRootSplit(t, "dev", "config", "test-llm")

	require.Error(t, err)
	assert.Equal(t,
		`refusing to send your API key to llm.openai_compat.base_url (currently "http://169.254.169.254/latest/meta-data/") — it is not a safe endpoint; fix it with: comrade config set llm.openai_compat.base_url <valid-url>`,
		err.Error())
	assert.NotContains(t, err.Error(), "InvalidValueError")
	assert.NotContains(t, err.Error(), "test-llm:")
}

// TestConfigTestLLMRefusesToBuildClientForMetadataBaseURLActiveProviderInTurkish
// is the same proof under COMRADE_LANG=tr.
func TestConfigTestLLMRefusesToBuildClientForMetadataBaseURLActiveProviderInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")
	t.Setenv("COMRADE_PROVIDER", "openai_compat")
	t.Setenv("COMRADE_LLM_OPENAI_COMPAT_BASE_URL", "http://169.254.169.254/latest/meta-data/")
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "test-key")

	_, _, err := execRootSplit(t, "dev", "config", "test-llm")

	require.Error(t, err)
	assert.Equal(t,
		`API anahtarınız llm.openai_compat.base_url (şu an "http://169.254.169.254/latest/meta-data/") adresine gönderilmeyecek — güvenli bir uç nokta değil; düzeltmek için: comrade config set llm.openai_compat.base_url <geçerli-url>`,
		err.Error())
}

func TestRootHelpFlagStillPrintsUsage(t *testing.T) {
	// Guards against SilenceUsage/SilenceErrors (added to fix the
	// duplicate error-output bug) accidentally suppressing the
	// explicitly-requested --help output too: SilenceUsage only affects
	// the automatic usage dump on a RunE error, not a deliberate --help.
	withIsolatedConfigDir(t)
	out := execRoot(t, "dev", "--help")

	assert.Contains(t, out, "Usage:")
	assert.Contains(t, out, "config")
}
