package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProviderNamesMatchesLLMProviderEnum(t *testing.T) {
	assert.Equal(t, []string{"anthropic", "openai_compat", "google", "ollama"}, ProviderNames())
}

func TestValidateUnknownKeyListsValidKeys(t *testing.T) {
	_, err := Validate("general.nonexistent", "x")

	assert.ErrorContains(t, err, `unknown config key "general.nonexistent"`)
	assert.ErrorContains(t, err, "general.mode")
	assert.ErrorContains(t, err, "llm.provider")
}

func TestValidateEnumRejectsInvalidValue(t *testing.T) {
	cases := []struct {
		name string
		key  string
		raw  string
	}{
		{"mode", "general.mode", "hizli"},
		{"language", "general.language", "fr"},
		{"provider", "llm.provider", "chatgpt"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Validate(tc.key, tc.raw)
			assert.ErrorContains(t, err, "invalid value")
			assert.ErrorContains(t, err, tc.key)
		})
	}
}

func TestValidateEnumAcceptsValidValues(t *testing.T) {
	cases := []struct {
		key  string
		raw  string
		want string
	}{
		{"general.mode", "auto", "auto"},
		{"general.mode", "ask", "ask"},
		{"general.mode", "info", "info"},
		{"general.language", "tr", "tr"},
		{"llm.provider", "ollama", "ollama"},
	}
	for _, tc := range cases {
		got, err := Validate(tc.key, tc.raw)
		assert.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestValidateStringWithoutEnumAcceptsAnyValue(t *testing.T) {
	got, err := Validate("llm.model", "claude-opus-4")
	assert.NoError(t, err)
	assert.Equal(t, "claude-opus-4", got)
}

func TestValidateBoolParsesValidValues(t *testing.T) {
	cases := []struct {
		raw  string
		want bool
	}{
		{"true", true},
		{"false", false},
		{"1", true},
		{"0", false},
	}
	for _, tc := range cases {
		got, err := Validate("general.color", tc.raw)
		assert.NoError(t, err)
		assert.Equal(t, tc.want, got)
	}
}

func TestValidateBoolRejectsInvalidValue(t *testing.T) {
	_, err := Validate("general.color", "yesplease")
	assert.ErrorContains(t, err, "must be a boolean")
}

func TestValidatePositiveIntAcceptsValidValue(t *testing.T) {
	got, err := Validate("llm.timeout_seconds", "120")
	assert.NoError(t, err)
	assert.Equal(t, 120, got)
}

func TestValidatePositiveIntRejectsNonNumeric(t *testing.T) {
	_, err := Validate("llm.max_tokens", "a lot")
	assert.ErrorContains(t, err, "must be an integer")
}

func TestValidatePositiveIntRejectsZeroAndNegative(t *testing.T) {
	for _, raw := range []string{"0", "-1", "-100"} {
		_, err := Validate("safety.max_auto_steps", raw)
		assert.ErrorContains(t, err, "must be greater than 0")
	}
}

func TestValidateStringSliceParsesCommaSeparatedList(t *testing.T) {
	got, err := Validate("llm.fallback", "ollama/llama3.1, openai_compat/gpt-4o-mini")
	assert.NoError(t, err)
	assert.Equal(t, []string{"ollama/llama3.1", "openai_compat/gpt-4o-mini"}, got)
}

func TestValidateStringSliceEmptyStringYieldsEmptySlice(t *testing.T) {
	got, err := Validate("safety.denylist_extra", "")
	assert.NoError(t, err)
	assert.Equal(t, []string{}, got)
}

func TestIsValidKey(t *testing.T) {
	assert.True(t, IsValidKey("general.mode"))
	assert.False(t, IsValidKey("general.bogus"))
}

func TestKeysIsSorted(t *testing.T) {
	keys := Keys()
	assert.Contains(t, keys, "general.mode")
	assert.Contains(t, keys, "audit.retention_days")
	for i := 1; i < len(keys); i++ {
		assert.LessOrEqual(t, keys[i-1], keys[i], "Keys() must be sorted")
	}
}
