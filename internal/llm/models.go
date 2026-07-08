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
