package llm

// Default models used when llm.model is empty in config. These are
// time-sensitive choices, verified against each provider's currently
// recommended small/cheap default as of 2026-07 — revisit if a provider
// retires or renames the alias. Ollama has no static default here; it is
// resolved dynamically from the local /api/tags list at attempt time (see
// ollamaConnector.resolveModel), since "the first available model" is
// inherently an installation-specific runtime fact, not a fixed string.
const (
	defaultAnthropicModel    = "claude-haiku-4-5"
	defaultOpenAICompatModel = "gpt-5.4-mini"
	defaultGoogleModel       = "gemini-3.5-flash"
)

// DefaultOpenAICompatModel exports defaultOpenAICompatModel for callers
// outside this package (internal/cli's auth login flow names the
// effective model in a 404-model-not-found notice) that need to name the
// same model buildProvider falls back to when cfg.LLM.Model is empty,
// without hand-duplicating the literal — see Derive-or-Guard.
func DefaultOpenAICompatModel() string {
	return defaultOpenAICompatModel
}

// Docs links printed alongside the static model lists below by
// `comrade config models` (docs/history/UYGULAMA_PLANI.md FAZ 8 item 4), since neither
// provider exposes a public, unauthenticated "list models" endpoint this
// CLI can query the way ollamaConnector.ListModels/ListOpenAICompatModels
// do — the lists below are a snapshot, not a live query.
const (
	AnthropicModelsDocsURL = "https://docs.claude.com/en/docs/about-claude/models"
	GoogleModelsDocsURL    = "https://ai.google.dev/gemini-api/docs/models"
)

// KnownAnthropicModels returns a static, hand-maintained snapshot (as of
// 2026-07, matching defaultAnthropicModel above) of Anthropic's current
// model lineup, for `comrade config models`'s picker. Revisit alongside
// defaultAnthropicModel when Anthropic retires or renames one of these —
// see AnthropicModelsDocsURL for the authoritative current list.
func KnownAnthropicModels() []string {
	return []string{"claude-haiku-4-5", "claude-sonnet-5", "claude-opus-4-8"}
}

// KnownGoogleModels returns a static, hand-maintained snapshot (as of
// 2026-07, matching defaultGoogleModel above) of Google's current Gemini
// lineup, for `comrade config models`'s picker. Revisit alongside
// defaultGoogleModel when Google retires or renames one of these — see
// GoogleModelsDocsURL for the authoritative current list.
func KnownGoogleModels() []string {
	return []string{"gemini-3.5-flash", "gemini-3.1-flash-lite", "gemini-3.1-pro"}
}
