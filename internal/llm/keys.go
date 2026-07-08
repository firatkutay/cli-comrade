package llm

import "os"

// providerEnvVars lists, per provider, the environment variables
// resolveAPIKey checks, in priority order: the provider-specific
// COMRADE_<PROVIDER>_API_KEY first, then the provider's well-known vendor
// env var(s). Ollama needs no credential and has no entry here.
//
// This is a plain env-var lookup for FAZ 2, per UYGULAMA_PLANI.md item 5.
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
func resolveAPIKey(provider string) (string, error) {
	vars := providerEnvVars[provider]
	for _, name := range vars {
		if v := os.Getenv(name); v != "" {
			return v, nil
		}
	}
	return "", &KeyMissingError{Provider: provider, EnvVars: vars}
}
