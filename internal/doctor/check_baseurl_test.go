package doctor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
)

func TestBaseURLCheckSkipsNonOpenAICompatProvider(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "anthropic"

	result := BaseURLCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorBaseURLSkip, result.Summary)
}

func TestBaseURLCheckOKWhenAlreadyCustomized(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = "https://api.groq.com/openai/v1"
	deps.Store = newFakeStore(map[string]string{"openai_compat": "gsk_abc123"})

	result := BaseURLCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorBaseURLOK, result.Summary)
}

func TestBaseURLCheckOKWhenKeyLooksLikeOpenAI(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = config.Default().LLM.OpenAICompat.BaseURL
	deps.Store = newFakeStore(map[string]string{"openai_compat": "sk-proj-abc123"})

	result := BaseURLCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorBaseURLOK, result.Summary)
}

func TestBaseURLCheckWarnsOnSuspectedVendorMismatch(t *testing.T) {
	cases := []struct {
		key    string
		vendor string
	}{
		{"sk-ant-abc123", "Anthropic"},
		{"gsk_abc123", "Groq"},
		{"sk-or-abc123", "OpenRouter"},
		{"AIzaSyAbc123", "Google"},
	}

	for _, tc := range cases {
		deps := baseDeps()
		deps.Cfg.LLM.Provider = "openai_compat"
		deps.Cfg.LLM.OpenAICompat.BaseURL = config.Default().LLM.OpenAICompat.BaseURL
		deps.Store = newFakeStore(map[string]string{"openai_compat": tc.key})

		result := BaseURLCheck(context.Background(), deps)

		assert.Equal(t, SeverityWarn, result.Severity, "key %q", tc.key)
		assert.Equal(t, i18n.MsgDoctorBaseURLSuspectedVendor, result.Summary)
		assert.Equal(t, []any{tc.vendor}, result.SummaryArgs)
		assert.Equal(t, "comrade config set llm.openai_compat.base_url <url>", result.Fix)
		assert.NotContains(t, result.Fix, tc.key, "the key value must never appear in Fix")
		assert.NotContains(t, result.Detail, tc.key, "the key value must never appear in Detail")
	}
}

func TestBaseURLCheckOKWhenNoKeyResolvedAtAll(t *testing.T) {
	// llm.ResolveEnvKey (BaseURLCheck's fallback once Store has nothing)
	// reads the REAL process environment, not deps.Getenv — clear both
	// known openai_compat env vars so this test is deterministic
	// regardless of what happens to be set on the machine running it.
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = config.Default().LLM.OpenAICompat.BaseURL

	result := BaseURLCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
}
