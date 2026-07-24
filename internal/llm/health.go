package llm

import (
	"strings"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// HealthEndpoint returns the keyless probe URL `comrade doctor`'s "reach"
// check GETs to determine whether provider's API host is reachable at
// all, WITHOUT ever sending a credential. Every URL is derived from the
// SAME endpoint constants/config fields the real connectors themselves
// use — anthropicMessagesURL, googleAPIBase, cfg.LLM.OpenAICompat.BaseURL,
// cfg.LLM.Ollama.BaseURL — never a hand-copied literal, per this
// project's derive-or-guard rule: a future change to any connector's own
// endpoint constant is picked up here automatically instead of silently
// drifting out of sync with what doctor actually probes.
//
// ok is false for any provider name this package does not recognize
// (e.g. an already-invalid llm.provider value slipping through some
// other validation gap) — the caller treats that as "skip this check"
// rather than guessing at a URL.
func HealthEndpoint(provider string, cfg config.Config) (url string, ok bool) {
	switch provider {
	case "anthropic":
		return anthropicMessagesURL, true
	case "google":
		return googleAPIBase, true
	case "openai_compat":
		return strings.TrimRight(cfg.LLM.OpenAICompat.BaseURL, "/") + "/models", true
	case "ollama":
		return strings.TrimRight(cfg.LLM.Ollama.BaseURL, "/") + "/api/tags", true
	default:
		return "", false
	}
}
