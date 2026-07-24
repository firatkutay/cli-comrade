package doctor

import (
	"context"

	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/secrets"
)

// KeyCheck reports whether the active provider has a resolvable API key:
// Skip for ollama (needs none), OK naming where the key came from
// (keychain/file/an env var name — the same vocabulary
// `comrade auth status` already uses, left untranslated), or Fail with a
// fix of `comrade auth login <provider>` when nothing resolves at all.
// This mirrors llm.KeyResolver's own precedence (Store first, then known
// environment variables) without ever printing the key's own value.
func KeyCheck(ctx context.Context, deps Deps) Result {
	provider := deps.Cfg.LLM.Provider
	if deps.ConfigErr != nil || provider == "" {
		return Result{Severity: SeveritySkip}
	}
	if provider == "ollama" {
		return Result{Severity: SeveritySkip, Summary: i18n.MsgDoctorKeySkipOllama}
	}

	if deps.Store != nil {
		if _, source, err := deps.Store.Get(ctx, provider); err == nil && source != secrets.SourceNone {
			return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorKeyFound, SummaryArgs: []any{provider, string(source)}}
		}
	}
	if envVar, ok := firstSetEnvVar(deps.Getenv, provider); ok {
		return Result{Severity: SeverityOK, Summary: i18n.MsgDoctorKeyFound, SummaryArgs: []any{provider, envVar}}
	}

	return Result{
		Severity:    SeverityFail,
		Summary:     i18n.MsgDoctorKeyMissing,
		SummaryArgs: []any{provider},
		Fix:         "comrade auth login " + provider,
	}
}

// firstSetEnvVar returns the first of provider's known environment
// variables (llm.ProviderEnvVars, in llm.ResolveEnvKey's own priority
// order) that is actually set, per deps' own injectable getenv — never
// the value itself. Mirrors internal/cli/auth.go's firstSetEnvVar exactly
// (kept as its own small copy for the same import-cycle reason
// check_shellhook.go's readFileOrEmpty documents).
func firstSetEnvVar(getenv func(string) string, provider string) (string, bool) {
	if getenv == nil {
		return "", false
	}
	for _, name := range llm.ProviderEnvVars(provider) {
		if getenv(name) != "" {
			return name, true
		}
	}
	return "", false
}
