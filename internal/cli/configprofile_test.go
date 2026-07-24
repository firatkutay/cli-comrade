package cli

import (
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertProfileListRow asserts stdout (comrade config profile list's
// tabwriter-aligned output) has a row for name whose ACTIVE/KEYS columns
// match marker/count — tabwriter pads columns with a variable run of
// spaces (not a literal tab) once rendered, so this matches on a
// whitespace-tolerant per-line pattern instead of a fixed literal string.
func assertProfileListRow(t *testing.T, stdout, name, marker string, count int) {
	t.Helper()
	pattern := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(name) + `\s+` + regexp.QuoteMeta(marker) + `\s+` + regexp.QuoteMeta(strconv.Itoa(count)) + `$`)
	assert.True(t, pattern.MatchString(stdout), "expected a row matching %q in:\n%s", pattern.String(), stdout)
}

func TestConfigProfileAddListShowUseRemoveHappyPath(t *testing.T) {
	withIsolatedConfigDir(t)

	_, stderr, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err, "stderr: %s", stderr)

	stdout, stderr, err := execRootSplit(t, "dev", "config", "profile", "list")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "PROFILE")
	assert.Contains(t, stdout, "work")

	_, stderr, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider", "openai_compat")
	require.NoError(t, err, "stderr: %s", stderr)

	stdout, stderr, err = execRootSplit(t, "dev", "config", "profile", "show", "work")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, "profile \"work\"")
	assert.Contains(t, stdout, "llm.provider = openai_compat")

	stdout, stderr, err = execRootSplit(t, "dev", "config", "profile", "use", "work")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, `activated profile "work"`)

	stdout, stderr, err = execRootSplit(t, "dev", "config", "get", "llm.provider")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Equal(t, "openai_compat", strings.TrimSpace(stdout))

	stdout, stderr, err = execRootSplit(t, "dev", "config", "profile", "list")
	require.NoError(t, err, "stderr: %s", stderr)
	assertProfileListRow(t, stdout, "work", "*", 1)

	stdout, stderr, err = execRootSplit(t, "dev", "config", "profile", "remove", "work")
	require.NoError(t, err, "stderr: %s", stderr)
	assert.Contains(t, stdout, `removed profile "work"`)

	value, _, err := execRootSplit(t, "dev", "config", "get", "general.profile")
	require.NoError(t, err)
	assert.Equal(t, "", strings.TrimSpace(value), "general.profile must be cleared once the active profile is removed")
}

func TestConfigProfileAddRejectsDuplicateName(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.Error(t, err)
	assert.ErrorContains(t, err, "already exists")
}

func TestConfigProfileAddRejectsInvalidName(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "Not-Valid")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid profile name")
}

func TestConfigProfileUseRejectsUndefinedProfile(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "use", "ghost")
	require.Error(t, err)
	assert.ErrorContains(t, err, `"ghost"`)
	assert.ErrorContains(t, err, "is not defined")
}

func TestConfigProfileShowRejectsUndefinedProfile(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "show", "ghost")
	require.Error(t, err)
	assert.ErrorContains(t, err, "is not defined")
}

func TestConfigProfileRemoveRejectsUndefinedProfile(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "remove", "ghost")
	require.Error(t, err)
	assert.ErrorContains(t, err, "is not defined")
}

func TestConfigProfileSetRejectsUndefinedProfile(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "set", "ghost", "llm.provider", "openai_compat")
	require.Error(t, err)
	assert.ErrorContains(t, err, "is not defined")
}

func TestConfigProfileSetRejectsInvalidValue(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider", "chatgpt")
	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid value")
}

func TestConfigProfileSetRejectsGeneralProfileKey(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "general.profile", "personal")
	require.Error(t, err)
	assert.ErrorContains(t, err, "cannot be set inside a profile")
}

func TestConfigProfileAddFromCurrentSeedsLLMSection(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "set", "llm.model", "gpt-4o")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "snapshot", "--from-current")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "profile", "show", "snapshot")
	require.NoError(t, err)
	assert.Contains(t, stdout, "llm.provider = openai_compat")
	assert.Contains(t, stdout, "llm.model = gpt-4o")
}

func TestConfigProfileFlagOverridesActiveProfileForOneInvocation(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "personal")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "personal", "llm.provider", "google")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider", "openai_compat")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "use", "personal")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "--profile", "work", "config", "get", "llm.provider")
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", strings.TrimSpace(stdout), "--profile must override the persisted general.profile for this one invocation")

	// The persisted choice itself is untouched by the one-off flag.
	stdout, _, err = execRootSplit(t, "dev", "config", "get", "llm.provider")
	require.NoError(t, err)
	assert.Equal(t, "google", strings.TrimSpace(stdout))
}

func TestConfigProfileEnvOverridesActiveProfile(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider", "openai_compat")
	require.NoError(t, err)
	t.Setenv("COMRADE_PROFILE", "work")

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "llm.provider")
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", strings.TrimSpace(stdout))
}

// TestConfigProfileUseWarnsOnSafetyOverride is P-5's pinned regression
// proof: `profile use` must print a highlighted warning whenever the
// activated profile overrides any safety.* key.
func TestConfigProfileUseWarnsOnSafetyOverride(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "yolo-work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "yolo-work", "safety.confirm_destructive", "false")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "config", "profile", "use", "yolo-work")
	require.NoError(t, err)
	assert.Contains(t, stderr, "yolo-work")
	assert.Contains(t, stderr, "safety.confirm_destructive")
}

func TestConfigProfileShowWarnsOnSafetyOverride(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "yolo-work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "yolo-work", "safety.confirm_elevated", "false")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "config", "profile", "show", "yolo-work")
	require.NoError(t, err)
	assert.Contains(t, stderr, "safety.confirm_elevated")
}

func TestConfigProfileUseDoesNotWarnWithoutSafetyOverride(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider", "openai_compat")
	require.NoError(t, err)

	_, stderr, err := execRootSplit(t, "dev", "config", "profile", "use", "work")
	require.NoError(t, err)
	assert.Empty(t, stderr)
}

func TestConfigProfileShowDefaultsToActiveProfile(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "use", "work")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "profile", "show")
	require.NoError(t, err)
	assert.Contains(t, stdout, `profile "work" (active)`)
}

func TestConfigProfileListMarksActiveProfileOnly(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "personal")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "use", "personal")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "profile", "list")
	require.NoError(t, err)
	assertProfileListRow(t, stdout, "personal", "*", 0)
	assertProfileListRow(t, stdout, "work", "", 0)
}

func TestConfigProfileUseWrongArgCountShowsTranslatedUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "use")
	require.Error(t, err)
	assert.ErrorContains(t, err, "usage:")
	assert.ErrorContains(t, err, "comrade config profile use")
}

func TestConfigProfileSetWrongArgCountShowsUsageError(t *testing.T) {
	withIsolatedConfigDir(t)

	_, _, err := execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider")
	require.Error(t, err)
	assert.ErrorContains(t, err, "usage:")
}

// TestConfigProfileUndefinedActiveProfileNeverFailsACommand pins that a
// bogus general.profile value never fails a real end-to-end CLI
// invocation — the warning text itself (config.emitProfileWarning writes
// straight to the real os.Stderr, like validateLoadedConfig's own
// base_url warning, not through cobra's captured writer — see
// TestConfigCommandsWorkOnFileWithMetadataBaseURLForActiveProvider's own
// precedent in config_test.go) is pinned at the internal/config unit
// level instead (TestLoaderWarnsOnUndefinedActiveProfileButNeverFails).
func TestConfigProfileUndefinedActiveProfileNeverFailsACommand(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "set", "general.profile", "ghost")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "general.mode")
	require.NoError(t, err, "an undefined active profile must never fail a command")
	assert.Equal(t, "ask\n", stdout)
}

func TestConfigProfileInvalidNameRejectedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "Bad Name")
	require.Error(t, err)
	assert.ErrorContains(t, err, "geçersiz profil adı")
}

func TestConfigProfileNotFoundRejectedInTurkish(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "tr")

	_, _, err := execRootSplit(t, "dev", "config", "profile", "use", "ghost")
	require.Error(t, err)
	assert.ErrorContains(t, err, "tanımlı değil")
}

// TestConfigProfileHelpFlagExitsZeroWithUsage mirrors
// TestConfigSetHelpFlagExitsZeroWithUsage's own regression proof for
// `config profile set`'s DisableFlagParsing.
func TestConfigProfileSetHelpFlagExitsZeroWithUsage(t *testing.T) {
	withIsolatedConfigDir(t)

	stdout, _, err := execRootSplit(t, "dev", "config", "profile", "set", "--help")
	require.NoError(t, err)
	assert.Contains(t, stdout, "Usage:")
}
