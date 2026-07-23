package config

import (
	"bytes"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestValidateNonNegativeIntAcceptsZeroAndPositive(t *testing.T) {
	for _, raw := range []string{"0", "1", "120"} {
		got, err := Validate("llm.idle_timeout_seconds", raw)
		assert.NoError(t, err)
		assert.Equal(t, mustAtoi(t, raw), got)
	}
}

func TestValidateNonNegativeIntRejectsNegative(t *testing.T) {
	_, err := Validate("llm.idle_timeout_seconds", "-1")
	assert.ErrorContains(t, err, "must be 0 or greater")
}

func TestValidateNonNegativeIntRejectsNonNumeric(t *testing.T) {
	_, err := Validate("llm.idle_timeout_seconds", "soon")
	assert.ErrorContains(t, err, "must be an integer")
}

func mustAtoi(t *testing.T, raw string) int {
	t.Helper()
	n, err := strconv.Atoi(raw)
	require.NoError(t, err)
	return n
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

// captureBaseURLWarnings redirects baseURLWarningWriter to a buffer for the
// duration of the calling test, restoring the original (os.Stderr) after
// via t.Cleanup, and returns the buffer so the test can assert on it.
func captureBaseURLWarnings(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	original := baseURLWarningWriter
	baseURLWarningWriter = &buf
	t.Cleanup(func() { baseURLWarningWriter = original })
	return &buf
}

// TestValidateBaseURL is SAST finding #3's table: neither
// llm.openai_compat.base_url nor llm.ollama.base_url may send the
// provider API key to an arbitrary or cloud-metadata/link-local host, but
// both must keep accepting (with only a cleartext warning, never a
// rejection) a self-hosted http:// endpoint on a non-loopback host.
func TestValidateBaseURL(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		wantError string // substring, or "" if raw must be accepted
		wantWarn  bool
	}{
		{name: "https public host ok, no warning", raw: "https://api.example.com/v1"},
		{name: "http localhost ok, no warning", raw: "http://localhost:11434"},
		{name: "http loopback IP ok, no warning", raw: "http://127.0.0.1:8080"},
		{name: "http private LAN host ok, but warns", raw: "http://192.168.1.50:11434", wantWarn: true},
		{name: "http arbitrary host ok, but warns", raw: "http://api.evil.tld", wantWarn: true},
		{name: "https metadata IP rejected", raw: "https://169.254.169.254/v1", wantError: "cloud metadata / link-local address not allowed"},
		{name: "http metadata IP rejected", raw: "http://169.254.169.254/latest/meta-data/", wantError: "cloud metadata / link-local address not allowed"},
		{name: "non-http(s) scheme rejected", raw: "ftp://x", wantError: "must be a valid http:// or https:// URL with a host"},
		{name: "file scheme rejected", raw: "file:///etc/passwd", wantError: "must be a valid http:// or https:// URL with a host"},
		{name: "no scheme rejected", raw: "not-a-url", wantError: "must be a valid http:// or https:// URL with a host"},
		{name: "no host rejected", raw: "https://", wantError: "must be a valid http:// or https:// URL with a host"},
	}

	for _, key := range []string{"llm.openai_compat.base_url", "llm.ollama.base_url"} {
		key := key
		t.Run(key, func(t *testing.T) {
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					buf := captureBaseURLWarnings(t)

					got, err := Validate(key, tc.raw)

					if tc.wantError != "" {
						require.Error(t, err)
						assert.ErrorContains(t, err, tc.wantError)
						assert.ErrorContains(t, err, key)
						assert.Empty(t, buf.String(), "a rejected value must never also warn")
						return
					}

					require.NoError(t, err)
					assert.Equal(t, tc.raw, got, "a valid base_url must be stored verbatim, unmodified")
					if tc.wantWarn {
						assert.Contains(t, buf.String(), key)
						assert.Contains(t, buf.String(), "unencrypted")
					} else {
						assert.Empty(t, buf.String())
					}
				})
			}
		})
	}
}

func TestValidateBaseURLRejectsUnknownKeyBeforeParsingURL(t *testing.T) {
	_, err := Validate("llm.openai_compat.nonexistent", "https://api.example.com")
	assert.ErrorContains(t, err, "unknown config key")
}

func TestKeysIsSorted(t *testing.T) {
	keys := Keys()
	assert.Contains(t, keys, "general.mode")
	assert.Contains(t, keys, "audit.retention_days")
	for i := 1; i < len(keys); i++ {
		assert.LessOrEqual(t, keys[i-1], keys[i], "Keys() must be sorted")
	}
}
