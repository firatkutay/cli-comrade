package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// unreachableURL starts and immediately closes an httptest server,
// returning its URL: every request against it now fails at the
// transport level (connection refused) instead of getting a real HTTP
// response — the same trick ollama_test.go's own reachability tests use,
// applied here to the three cloud connectors.
func unreachableURL(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	return url
}

// TestFAZ11AnthropicUnreachableProducesFriendlyOfflineError proves
// docs/history/UYGULAMA_PLANI.md FAZ 11 item 2's "ağ yokken ... anlaşılır offline
// mesajı" for the anthropic connector: a transport-level failure (not a
// non-2xx response) is replaced with a message naming the provider and
// classified via errors.Is(err, ErrOffline), instead of surfacing Go's
// raw *url.Error text.
func TestFAZ11AnthropicUnreachableProducesFriendlyOfflineError(t *testing.T) {
	url := unreachableURL(t)
	c := newAnthropicConnector("k", "model", http.DefaultClient)
	c.baseURL = url

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOffline), "err must classify as ErrOffline: %v", err)
	assert.ErrorContains(t, err, "anthropic")
	assert.ErrorContains(t, err, "could not reach")
}

// TestFAZ11OpenAICompatUnreachableProducesFriendlyOfflineError is the
// same proof for openai_compat (the connector every one of OpenAI/
// Mistral/Groq/GLM/Qwen/Kimi/OpenRouter/LM Studio shares).
func TestFAZ11OpenAICompatUnreachableProducesFriendlyOfflineError(t *testing.T) {
	url := unreachableURL(t)
	c := newOpenAICompatConnector("k", "model", url, http.DefaultClient)

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOffline), "err must classify as ErrOffline: %v", err)
	assert.ErrorContains(t, err, "openai_compat")
	assert.ErrorContains(t, err, "could not reach")
}

// TestFAZ11GoogleUnreachableProducesFriendlyOfflineError is the same
// proof for the google (Gemini) connector.
func TestFAZ11GoogleUnreachableProducesFriendlyOfflineError(t *testing.T) {
	url := unreachableURL(t)
	c := newGoogleConnector("k", "model", http.DefaultClient)
	c.baseURL = url

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOffline), "err must classify as ErrOffline: %v", err)
	assert.ErrorContains(t, err, "google")
	assert.ErrorContains(t, err, "could not reach")
}

// TestFAZ11ClientSuggestsOllamaFallbackWhenWholeChainIsOffline proves
// Client.Complete's "if Ollama is available, suggest falling back to
// it" behavior (docs/history/UYGULAMA_PLANI.md FAZ 11 item 2): when every configured
// attempt fails with ErrOffline and none of them is already ollama, the
// final aggregated error names Ollama as a local, network-free
// alternative.
func TestFAZ11ClientSuggestsOllamaFallbackWhenWholeChainIsOffline(t *testing.T) {
	url := unreachableURL(t)
	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "anthropic", provider: func() Provider { c := newAnthropicConnector("k", "m", http.DefaultClient); c.baseURL = url; return c }()},
		},
	}

	_, err := client.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.ErrorContains(t, err, "Ollama")
	assert.ErrorContains(t, err, "llm.fallback")
}

// TestFAZ11ClientDoesNotSuggestOllamaWhenAlreadyConfigured proves the
// suggestion is suppressed (not useless noise) when Ollama is already one
// of the configured attempts — even though that attempt, too, failed
// (e.g. `ollama serve` isn't running either): recommending the user add
// something they already have configured would be actively unhelpful.
func TestFAZ11ClientDoesNotSuggestOllamaWhenAlreadyConfigured(t *testing.T) {
	url := unreachableURL(t)
	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "anthropic", provider: func() Provider { c := newAnthropicConnector("k", "m", http.DefaultClient); c.baseURL = url; return c }()},
			{providerName: "ollama", provider: newOllamaConnector("m", url, http.DefaultClient)},
		},
	}

	_, err := client.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "add \"ollama\"")
}

// TestFAZ11ClientDoesNotSuggestOllamaForNonOfflineFailure proves the
// suggestion is specific to network unreachability: a non-2xx HTTP
// response (a real server rejecting the request, e.g. malformed body)
// never triggers it, since adding Ollama would not fix that class of
// failure.
func TestFAZ11ClientDoesNotSuggestOllamaForNonOfflineFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request","type":"invalid_request_error"}}`))
	}))
	defer srv.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", srv.URL, srv.Client())},
		},
	}

	_, err := client.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "Ollama")
}
