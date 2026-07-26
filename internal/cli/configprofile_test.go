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

// TestConfigProfileGenericEnvOverridesActiveProfile is
// TestConfigProfileEnvOverridesActiveProfile's counterpart via the generic
// COMRADE_GENERAL_PROFILE form instead of the canonical COMRADE_PROFILE —
// GitHub issue #19's real bug, at the actual command surface.
func TestConfigProfileGenericEnvOverridesActiveProfile(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.provider", "openai_compat")
	require.NoError(t, err)
	t.Setenv("COMRADE_GENERAL_PROFILE", "work")

	stdout, _, err := execRootSplit(t, "dev", "config", "get", "llm.provider")
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", strings.TrimSpace(stdout))
}

// TestConfigProfileShowWithFlagTargetsAndWarnsForTheForcedActiveProfile is
// the independent review's Finding 6 pinned at the actual command surface:
// `profile show` (no name arg) must target the profile ACTUALLY in force
// for this invocation — including a --profile flag override, not just the
// persisted general.profile — and fire (or withhold) the mandatory P-5
// safety-override warning for that SAME profile. Before the fix,
// cfg.General.Profile ignored l.profileOverride entirely, so this
// incorrectly targeted/warned about the persisted "personal" profile
// instead of the flag-forced "work" one.
func TestConfigProfileShowWithFlagTargetsAndWarnsForTheForcedActiveProfile(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "personal")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "safety.confirm_destructive", "false")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "use", "personal")
	require.NoError(t, err)

	// --profile work must target "work" (the profile in force for this
	// invocation), not the persisted "personal", and must warn on work's
	// own safety.confirm_destructive override.
	stdout, stderr, err := execRootSplit(t, "dev", "--profile", "work", "config", "profile", "show")
	require.NoError(t, err)
	assert.Contains(t, stdout, `profile "work" (active)`)
	assert.Contains(t, stderr, "safety.confirm_destructive")

	// Without the flag, the persisted "personal" is still targeted, and
	// must NOT warn — it overrides no safety.* key at all.
	stdout, stderr, err = execRootSplit(t, "dev", "config", "profile", "show")
	require.NoError(t, err)
	assert.Contains(t, stdout, `profile "personal" (active)`)
	assert.Empty(t, stderr)
}

// TestConfigProfileListMarksFlagActivatedProfile is
// TestConfigProfileListMarksActiveProfileOnly's counterpart with a
// --profile override in play: `profile list`'s "*" marker must follow the
// profile actually in force, not the persisted general.profile.
func TestConfigProfileListMarksFlagActivatedProfile(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "personal")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "use", "personal")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "--profile", "work", "config", "profile", "list")
	require.NoError(t, err)
	assertProfileListRow(t, stdout, "work", "*", 0)
	assertProfileListRow(t, stdout, "personal", "", 0)
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

// TestConfigProfileSetHonorsProfileFlagRegardlessOfPosition is issue
// #27's core regression guard for `config profile set`: exactly the same
// bug as newConfigSetCmd's own (config_test.go's identically-named
// test) — DisableFlagParsing means cobra never parses root's persistent
// --profile here either, so it used to leak into args and break the
// len(args)==3 arity check. --profile here selects the ACTIVE profile
// only (language/ensureLoaded); <name> (the profile actually being
// edited) is a separate, unaffected positional argument.
func TestConfigProfileSetHonorsProfileFlagRegardlessOfPosition(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"before the subcommand", []string{"--profile", "other", "config", "profile", "set", "work", "llm.provider", "openai_compat"}},
		{"after the leaf", []string{"config", "profile", "set", "work", "llm.provider", "openai_compat", "--profile", "other"}},
		{"equals form before the subcommand", []string{"--profile=other", "config", "profile", "set", "work", "llm.provider", "openai_compat"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			withIsolatedConfigDir(t)
			_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
			require.NoError(t, err)
			_, _, err = execRootSplit(t, "dev", "config", "profile", "add", "other")
			require.NoError(t, err)

			stdout, _, err := execRootSplit(t, "dev", tc.args...)
			require.NoError(t, err, "must not be rejected as a generic arity usage error")
			assert.Equal(t, "work.llm.provider = openai_compat\n", stdout)
		})
	}
}

// TestConfigProfileSetProfileFlagActuallyTakesEffect proves --profile is
// genuinely threaded through to the Loader `config profile set` uses
// (not merely tolerated): a ProfileNotFoundError for the EDITED profile
// (<name>, not the --profile-selected active one) is only reached AFTER
// cfg is loaded via the active profile, so its translated rendering
// reflects the --profile-selected profile's own general.language.
func TestConfigProfileSetProfileFlagActuallyTakesEffect(t *testing.T) {
	withIsolatedConfigDir(t)
	t.Setenv("COMRADE_LANG", "")
	t.Setenv("LANG", "")
	t.Setenv("LC_ALL", "")
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "langtr")
	require.NoError(t, err)
	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "langtr", "general.language", "tr")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "--profile", "langtr", "config", "profile", "set", "doesnotexist", "llm.provider", "openai_compat")
	require.Error(t, err)
	assert.ErrorContains(t, err, "tanımlı değil", "the not-found error must render in the --profile-selected profile's own general.language, proving --profile was actually consumed")
}

// TestConfigProfileSetProfileFlagDoesNotDisturbDashPrefixedValue mirrors
// config_test.go's identically-purposed test: a value that legitimately
// starts with "-" must still reach config.ValidateProfileKey untouched
// even with a real --profile flag present in the same invocation.
func TestConfigProfileSetProfileFlagDoesNotDisturbDashPrefixedValue(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "--profile", "work", "config", "profile", "set", "work", "llm.timeout_seconds", "-5")
	require.Error(t, err)
	assert.ErrorContains(t, err, "greater than 0")
}

// TestConfigProfileSetProfileFlagDoubleDashEscapeHatchKeepsLiteralValue
// mirrors config_test.go's identically-purposed test for the 3-arg
// `config profile set` shape.
func TestConfigProfileSetProfileFlagDoubleDashEscapeHatchKeepsLiteralValue(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)

	stdout, _, err := execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.model", "--", "--profile")
	require.NoError(t, err)
	assert.Equal(t, "work.llm.model = --profile\n", stdout)
}

// TestConfigProfileSetProfileFlagMissingValueErrors mirrors
// config_test.go's identically-purposed test.
func TestConfigProfileSetProfileFlagMissingValueErrors(t *testing.T) {
	withIsolatedConfigDir(t)
	_, _, err := execRootSplit(t, "dev", "config", "profile", "add", "work")
	require.NoError(t, err)

	_, _, err = execRootSplit(t, "dev", "config", "profile", "set", "work", "llm.model", "--profile")
	require.Error(t, err)
	assert.Equal(t, "--profile requires a value, e.g. --profile work", err.Error())
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
