package doctor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/i18n"
)

func TestKeyCheckSkipsOllama(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "ollama"

	result := KeyCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorKeySkipOllama, result.Summary)
}

func TestKeyCheckSkipsWhenConfigFailedToLoad(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "anthropic"
	deps.ConfigErr = assertNotFoundErr

	result := KeyCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
}

func TestKeyCheckOKWhenStoreHasKey(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "anthropic"
	deps.Store = newFakeStore(map[string]string{"anthropic": "sk-ant-abc123"})

	result := KeyCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorKeyFound, result.Summary)
	assert.Equal(t, []any{"anthropic", "keychain"}, result.SummaryArgs)
	assert.NotContains(t, result.Fix, "sk-ant-abc123", "the key value itself must never appear in Fix")
	assert.NotContains(t, result.Detail, "sk-ant-abc123", "the key value itself must never appear in Detail")
}

func TestKeyCheckOKWhenOnlyEnvVarSet(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "anthropic"
	deps.Getenv = func(name string) string {
		if name == "ANTHROPIC_API_KEY" {
			return "vendor-key"
		}
		return ""
	}

	result := KeyCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorKeyFound, result.Summary)
	assert.Equal(t, []any{"anthropic", "ANTHROPIC_API_KEY"}, result.SummaryArgs)
}

func TestKeyCheckStoreTakesPrecedenceOverEnv(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "anthropic"
	deps.Store = newFakeStore(map[string]string{"anthropic": "stored-key"})
	deps.Getenv = func(name string) string {
		if name == "ANTHROPIC_API_KEY" {
			return "vendor-key"
		}
		return ""
	}

	result := KeyCheck(context.Background(), deps)

	assert.Equal(t, []any{"anthropic", "keychain"}, result.SummaryArgs)
}

func TestKeyCheckFailsWithLoginFixWhenNothingResolves(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "anthropic"

	result := KeyCheck(context.Background(), deps)

	assert.Equal(t, SeverityFail, result.Severity)
	assert.Equal(t, i18n.MsgDoctorKeyMissing, result.Summary)
	assert.Equal(t, []any{"anthropic"}, result.SummaryArgs)
	assert.Equal(t, "comrade auth login anthropic", result.Fix)
}
