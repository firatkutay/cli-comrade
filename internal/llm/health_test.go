package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/config"
)

func TestHealthEndpointAnthropicMatchesConnectorURL(t *testing.T) {
	url, ok := HealthEndpoint("anthropic", config.Default())
	assert.True(t, ok)
	assert.Equal(t, anthropicMessagesURL, url)
}

func TestHealthEndpointGoogleMatchesConnectorURL(t *testing.T) {
	url, ok := HealthEndpoint("google", config.Default())
	assert.True(t, ok)
	assert.Equal(t, googleAPIBase, url)
}

func TestHealthEndpointOpenAICompatDerivesFromConfiguredBaseURL(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.OpenAICompat.BaseURL = "https://example.com/v1/"

	url, ok := HealthEndpoint("openai_compat", cfg)
	assert.True(t, ok)
	assert.Equal(t, "https://example.com/v1/models", url)
}

func TestHealthEndpointOllamaDerivesFromConfiguredBaseURL(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Ollama.BaseURL = "http://localhost:11434/"

	url, ok := HealthEndpoint("ollama", cfg)
	assert.True(t, ok)
	assert.Equal(t, "http://localhost:11434/api/tags", url)
}

func TestHealthEndpointUnknownProviderReportsNotOK(t *testing.T) {
	url, ok := HealthEndpoint("bogus", config.Default())
	assert.False(t, ok)
	assert.Empty(t, url)
}
