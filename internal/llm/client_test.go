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

	"github.com/firatkutay/cli-comrade/internal/config"
)

func TestClientCompleteFallsBackOnOverloadedAndBothServersHit(t *testing.T) {
	var primaryHit, secondaryHit bool

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		primaryHit = true
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom","type":"server_error"}}`))
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondaryHit = true
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"secondary-model","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer secondary.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "primary-model", primary.URL, primary.Client())},
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "secondary-model", secondary.URL, secondary.Client())},
		},
	}

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.NoError(t, err)
	assert.True(t, primaryHit, "primary must have been tried")
	assert.True(t, secondaryHit, "secondary must have been tried after primary failed")
	assert.Equal(t, "secondary-model", resp.Model)
	assert.Equal(t, "ok", resp.Text)
}

func TestClientCompleteAuthErrorDoesNotTriggerFallback(t *testing.T) {
	var secondaryHit bool

	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key","type":"invalid_request_error"}}`))
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		secondaryHit = true
		w.WriteHeader(http.StatusOK)
	}))
	defer secondary.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", primary.URL, primary.Client())},
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", secondary.URL, secondary.Client())},
		},
	}

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthRejected))
	assert.False(t, secondaryHit, "an auth-rejected error must stop the chain, never reaching the next attempt")
}

func TestClientCompleteParseFailureTriggersFallback(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"primary","choices":[{"message":{"role":"assistant","content":"not json at all"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"secondary","choices":[{"message":{"role":"assistant","content":"{\"command\":\"ls\"}"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer secondary.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", primary.URL, primary.Client())},
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", secondary.URL, secondary.Client())},
		},
	}

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages:       []Message{{Role: "user", Content: "hi"}},
		MaxTokens:      8,
		RequiredFields: []string{"command"},
	})
	require.NoError(t, err)
	assert.Equal(t, "secondary", resp.Model, "a parse failure on the primary must fall back to the secondary")
	assert.JSONEq(t, `{"command":"ls"}`, string(resp.JSON))
}

func TestClientCompleteAllProvidersFailReturnsWrappedLastError(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"first failure"}}`))
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"second failure"}}`))
	}))
	defer secondary.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", primary.URL, primary.Client())},
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", secondary.URL, secondary.Client())},
		},
	}

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
	assert.ErrorContains(t, err, "second failure", "the wrapped error must be the last attempt's, not the first")
}

func TestClientStreamFallsBackOnInitialHandshakeFailure(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"ok"}}]}` + "\n\n" + `data: [DONE]` + "\n\n"))
	}))
	defer secondary.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", primary.URL, primary.Client())},
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", secondary.URL, secondary.Client())},
		},
	}

	ch, err := client.Stream(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.NoError(t, err)

	var text string
	for chunk := range ch {
		if !chunk.Done {
			text += chunk.Text
		}
	}
	assert.Equal(t, "ok", text)
}

func TestNewUnknownProviderErrors(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "bogus"

	_, err := New(cfg)
	require.Error(t, err)
	assert.ErrorContains(t, err, `unknown provider "bogus"`)
}

func TestNewMissingAPIKeyDefersErrorToAttemptTime(t *testing.T) {
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	cfg := config.Default()
	cfg.LLM.Provider = "anthropic"

	client, err := New(cfg)
	require.NoError(t, err, "a missing API key must not fail Client construction")

	_, err = client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAPIKeyMissing))
	assert.ErrorContains(t, err, "ANTHROPIC_API_KEY")
}

func TestNewBuildsPrimaryAndFallbackAttemptsInOrder(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Model = "llama3.1"
	cfg.LLM.Fallback = []string{"ollama/mistral"}

	client, err := New(cfg)
	require.NoError(t, err)
	require.Len(t, client.attempts, 2)
	assert.Equal(t, "ollama", client.attempts[0].providerName)
	assert.Equal(t, "ollama", client.attempts[1].providerName)
	assert.Equal(t, "ollama", client.Name())
}

func TestNewWithKeyResolverOverridesEnvLookup(t *testing.T) {
	// No env var set for anthropic at all — proves the key New's
	// connector ends up holding came from the resolver, not from
	// resolveAPIKey's env fallback.
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "")
	t.Setenv("ANTHROPIC_API_KEY", "")

	cfg := config.Default()
	cfg.LLM.Provider = "anthropic"
	cfg.LLM.Model = "m"

	var seenProvider string
	client, err := New(cfg, WithKeyResolver(func(provider string) (string, error) {
		seenProvider = provider
		return "resolver-supplied-key", nil
	}))
	require.NoError(t, err)
	require.Len(t, client.attempts, 1)
	assert.Equal(t, "anthropic", seenProvider)

	conn, ok := client.attempts[0].provider.(*anthropicConnector)
	require.True(t, ok, "expected the anthropic attempt to be a real *anthropicConnector, not a missingKeyProvider")
	assert.Equal(t, "resolver-supplied-key", conn.apiKey)
}

func TestNewWithKeyResolverNilOptionKeepsDefaultEnvResolver(t *testing.T) {
	t.Setenv("COMRADE_ANTHROPIC_API_KEY", "env-key")

	cfg := config.Default()
	cfg.LLM.Provider = "anthropic"

	client, err := New(cfg, WithKeyResolver(nil))
	require.NoError(t, err)
	require.Len(t, client.attempts, 1)
	// A nil resolver must not turn into a "missing key" provider — proof
	// the env-based default resolver was kept, not replaced with nil.
	_, isMissingKey := client.attempts[0].provider.(*missingKeyProvider)
	assert.False(t, isMissingKey, "WithKeyResolver(nil) must not disable the default env resolver")
}

func TestSplitProviderModel(t *testing.T) {
	provider, model := splitProviderModel("ollama/llama3.1")
	assert.Equal(t, "ollama", provider)
	assert.Equal(t, "llama3.1", model)

	provider, model = splitProviderModel("anthropic")
	assert.Equal(t, "anthropic", provider)
	assert.Equal(t, "", model)
}

func TestHTTPTimeoutDefaultsForNonPositive(t *testing.T) {
	assert.Equal(t, 60*time.Second, httpTimeout(0))
	assert.Equal(t, 60*time.Second, httpTimeout(-5))
	assert.Equal(t, 30*time.Second, httpTimeout(30))
}
