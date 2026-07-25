package config

import (
	"bytes"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateProfileName(t *testing.T) {
	valid := []string{"work", "personal", "a", "a-b_c", "team2", "x123456789012345678901234567890a"}
	for _, name := range valid {
		t.Run("valid_"+name, func(t *testing.T) {
			assert.NoError(t, ValidateProfileName(name))
		})
	}

	invalid := []string{"", "Work", "WORK", "-work", "_work", "work!", "work name", "a.b", "x1234567890123456789012345678901a"}
	for _, name := range invalid {
		t.Run("invalid_"+name, func(t *testing.T) {
			err := ValidateProfileName(name)
			require.Error(t, err)
			var invalidName *InvalidProfileNameError
			assert.ErrorAs(t, err, &invalidName)
			assert.Equal(t, name, invalidName.Name)
		})
	}
}

func TestValidateProfileKeyRejectsGeneralProfile(t *testing.T) {
	_, err := ValidateProfileKey("general.profile", "work")
	require.Error(t, err)
	var notAllowed *ProfileKeyNotAllowedError
	assert.ErrorAs(t, err, &notAllowed)
	assert.Equal(t, "general.profile", notAllowed.Key)
}

func TestValidateProfileKeyDelegatesToValidateForEveryOtherKey(t *testing.T) {
	value, err := ValidateProfileKey("llm.provider", "openai_compat")
	require.NoError(t, err)
	assert.Equal(t, "openai_compat", value)

	_, err = ValidateProfileKey("llm.provider", "chatgpt")
	assert.ErrorContains(t, err, "invalid value")

	_, err = ValidateProfileKey("general.bogus", "x")
	var unknown *UnknownKeyError
	assert.ErrorAs(t, err, &unknown)
}

func TestValidateProfileKeyEnforcesBaseURLRulesInsideProfile(t *testing.T) {
	_, err := ValidateProfileKey("llm.openai_compat.base_url", "https://169.254.169.254/v1")
	require.Error(t, err)
	assert.ErrorContains(t, err, "cloud metadata")
}

func TestResolveActiveProfilePrecedence(t *testing.T) {
	cases := []struct {
		name         string
		flag         string
		canonicalEnv string
		genericEnv   string
		file         string
		want         string
	}{
		{"flag wins over everything", "work", "personal", "generic", "default", "work"},
		{"canonical env wins over generic env", "", "personal", "generic", "default", "personal"},
		{"canonical env wins over file", "", "personal", "", "default", "personal"},
		{"generic env wins over file when canonical env unset", "", "", "generic", "default", "generic"},
		{"file when neither flag nor either env set", "", "", "", "default", "default"},
		{"empty when nothing set", "", "", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveActiveProfile(tc.flag, tc.canonicalEnv, tc.genericEnv, tc.file)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestProfileKeysExcludesPlaceholderAndSorts(t *testing.T) {
	profile := map[string]any{
		"llm": map[string]any{
			"provider": "openai_compat",
			"model":    "gpt-4o",
		},
		profilePlaceholderKey: true,
	}
	assert.Equal(t, []string{"llm.model", "llm.provider"}, ProfileKeys(profile))
}

func TestProfileKeysOnPlaceholderOnlyProfileIsEmpty(t *testing.T) {
	profile := map[string]any{profilePlaceholderKey: true}
	assert.Empty(t, ProfileKeys(profile))
}

func TestProfileSafetyOverridesOnlyReturnsSafetyKeys(t *testing.T) {
	profile := map[string]any{
		"llm": map[string]any{"provider": "openai_compat"},
		"safety": map[string]any{
			"confirm_destructive": false,
			"max_auto_steps":      20,
		},
	}
	assert.Equal(t, []string{"safety.confirm_destructive", "safety.max_auto_steps"}, ProfileSafetyOverrides(profile))
}

func TestProfileSafetyOverridesEmptyWhenNoneOverridden(t *testing.T) {
	profile := map[string]any{"llm": map[string]any{"provider": "openai_compat"}}
	assert.Empty(t, ProfileSafetyOverrides(profile))
}

// TestStripPlaceholderRemovesOnlyThePlaceholderKey is stripPlaceholder's
// own unit proof: the placeholder marker is dropped, every real key
// survives untouched.
func TestStripPlaceholderRemovesOnlyThePlaceholderKey(t *testing.T) {
	profile := map[string]any{
		"llm":                 map[string]any{"provider": "openai_compat"},
		profilePlaceholderKey: true,
	}
	stripped := stripPlaceholder(profile)
	assert.NotContains(t, stripped, profilePlaceholderKey)
	assert.Equal(t, map[string]any{"provider": "openai_compat"}, stripped["llm"])
}

// TestStripPlaceholderReturnsSameMapWhenNoPlaceholderPresent proves the
// no-op fast path: a profile that never had the placeholder key is
// returned as-is (same keys/values), so the common case (a non-empty
// profile) does zero allocation work.
func TestStripPlaceholderReturnsSameMapWhenNoPlaceholderPresent(t *testing.T) {
	profile := map[string]any{"llm": map[string]any{"provider": "openai_compat"}}
	assert.Equal(t, profile, stripPlaceholder(profile))
}

// TestApplyProfileOverlayStripsPlaceholderKeyFromEffectiveConfig is the
// regression proof for the nit in GitHub issue #19: activating a
// placeholder-only (empty) profile must merge ZERO keys into v's
// top-level effective config — specifically, profilePlaceholderKey itself
// must never leak out of the profiles table it was invented to keep
// durable on disk (see profilePlaceholderKey's own doc comment).
func TestApplyProfileOverlayStripsPlaceholderKeyFromEffectiveConfig(t *testing.T) {
	v := viper.New()
	v.SetConfigType("toml")
	require.NoError(t, v.MergeConfigMap(map[string]any{
		"profiles": map[string]any{
			"empty": map[string]any{profilePlaceholderKey: true},
		},
	}))

	applyProfileOverlay(v, "empty")

	assert.Nil(t, v.Get(profilePlaceholderKey), "the internal placeholder key must never leak into the effective top-level config")
}

// captureProfileWarnings redirects profileWarningWriter to a buffer for
// the duration of the calling test, restoring the original (os.Stderr)
// after via t.Cleanup — mirrors captureBaseURLWarnings' own established
// pattern (validate_test.go) for the profile-overlay warning family.
func captureProfileWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := profileWarningWriter
	profileWarningWriter = &buf
	t.Cleanup(func() { profileWarningWriter = original })
	return &buf
}
