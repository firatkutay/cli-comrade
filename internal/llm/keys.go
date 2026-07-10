package llm

import "os"

// providerEnvVars lists, per provider, the environment variables
// resolveAPIKey checks, in priority order: the provider-specific
// COMRADE_<PROVIDER>_API_KEY first, then the provider's well-known vendor
// env var(s). Ollama needs no credential and has no entry here.
//
// This is a plain env-var lookup for FAZ 2, per docs/history/UYGULAMA_PLANI.md item 5.
// FAZ 8 adds an OS keychain (zalando/go-keyring) as a higher-priority
// source ahead of these; resolveAPIKey's (string, error) signature and
// this table are structured so that lookup can be prepended there without
// changing any caller in this package.
var providerEnvVars = map[string][]string{
	"anthropic":     {"COMRADE_ANTHROPIC_API_KEY", "ANTHROPIC_API_KEY"},
	"openai_compat": {"COMRADE_OPENAI_COMPAT_API_KEY", "OPENAI_API_KEY"},
	"google":        {"COMRADE_GOOGLE_API_KEY", "GEMINI_API_KEY", "GOOGLE_API_KEY"},
}

// resolveAPIKey returns the first non-empty value among provider's known
// environment variables (see providerEnvVars), or a *KeyMissingError
// naming every variable it checked. provider must have a providerEnvVars
// entry — it must never be called for "ollama", which needs no key.
//
// This is also Client's default KeyResolver (see WithKeyResolver in
// client.go): New(cfg) with no options behaves exactly as it did before
// FAZ 8, so this package's own tests need no internal/secrets dependency.
func resolveAPIKey(provider string) (string, error) {
	vars := providerEnvVars[provider]
	for _, name := range vars {
		if v := os.Getenv(name); v != "" {
			return v, nil
		}
	}
	return "", &KeyMissingError{Provider: provider, EnvVars: vars}
}

// ResolveEnvKey resolves provider's API key from known environment
// variables only (COMRADE_<PROVIDER>_API_KEY, then the provider's vendor
// env var(s)) — skipping any keychain/file lookup. It is the exported
// form of resolveAPIKey, for internal/cli's secrets-backed KeyResolver
// (see docs/history/phases/FAZ-08.md) to delegate to after checking the
// keychain/file store first, without this package importing
// internal/secrets (that would invert the intended cli -> {llm, secrets}
// dependency arrow).
func ResolveEnvKey(provider string) (string, error) {
	return resolveAPIKey(provider)
}

// ProviderEnvVars returns, in priority order, the environment variable
// names ResolveEnvKey checks for provider. It exists purely for display
// (e.g. `comrade auth status` listing which env var it found a key
// under) — never for resolving a key itself — and returns a copy so a
// caller can't mutate this package's internal table.
func ProviderEnvVars(provider string) []string {
	vars := providerEnvVars[provider]
	out := make([]string, len(vars))
	copy(out, vars)
	return out
}
