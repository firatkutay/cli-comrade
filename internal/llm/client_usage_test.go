package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// TestClientCompleteFiresUsageObserverOnceOnSuccess proves
// WithUsageObserver's contract (usage.go): exactly one UsageEvent per
// successful Complete call, carrying the connector's own reported
// token counts and the response's Model.
func TestClientCompleteFiresUsageObserverOnceOnSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m-primary","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":11,"completion_tokens":7}}`))
	}))
	defer server.Close()

	var events []UsageEvent
	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m-primary", server.URL, server.Client()), model: "m-primary"},
		},
		usageObserver: func(ev UsageEvent) { events = append(events, ev) },
	}

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.NoError(t, err)
	require.Len(t, events, 1, "observer must fire exactly once for one successful Complete call")
	assert.Equal(t, "openai_compat", events[0].Provider)
	assert.Equal(t, "m-primary", events[0].Model)
	assert.Equal(t, 11, events[0].Usage.InputTokens)
	assert.Equal(t, 7, events[0].Usage.OutputTokens)
	assert.GreaterOrEqual(t, events[0].Latency, time.Duration(0))
}

// TestClientCompleteNeverFiresUsageObserverOnFailure proves a failed
// attempt carries no usage: the observer must not fire at all when every
// configured attempt fails.
func TestClientCompleteNeverFiresUsageObserverOnFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer server.Close()

	var events []UsageEvent
	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", server.URL, server.Client()), model: "m"},
		},
		usageObserver: func(ev UsageEvent) { events = append(events, ev) },
	}

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.Error(t, err)
	assert.Empty(t, events, "a failed attempt must never fire the usage observer")
}

// TestClientCompleteUsageObserverSumsAcrossFallbackRetries proves the
// observer fires once per SUCCESSFUL attempt across a fallback chain — a
// failed primary contributes nothing, the succeeding secondary
// contributes exactly one event — so a caller accumulating events (e.g.
// internal/cli's usageTally) sums correctly across retries instead of
// double-counting or missing the eventual success.
func TestClientCompleteUsageObserverSumsAcrossFallbackRetries(t *testing.T) {
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"boom"}}`))
	}))
	defer primary.Close()

	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m-secondary","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":3,"completion_tokens":2}}`))
	}))
	defer secondary.Close()

	var events []UsageEvent
	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m-primary", primary.URL, primary.Client()), model: "m-primary"},
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m-secondary", secondary.URL, secondary.Client()), model: "m-secondary"},
		},
		usageObserver: func(ev UsageEvent) { events = append(events, ev) },
	}

	resp, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.NoError(t, err)
	assert.Equal(t, "m-secondary", resp.Model)
	require.Len(t, events, 1, "only the eventual success reports usage, not the failed primary attempt")
	assert.Equal(t, "m-secondary", events[0].Model)
	assert.Equal(t, 3, events[0].Usage.InputTokens)
	assert.Equal(t, 2, events[0].Usage.OutputTokens)
}

// TestClientCompleteUsageObserverFallsBackToAttemptModelWhenResponseModelEmpty
// proves fireUsage's documented fallback: when a (synthetic, for this
// test — no real connector actually does this) successful response comes
// back with an empty Model, the event still attributes the usage to the
// attempt's own configured model rather than reporting "" and silently
// losing the pricing/display attribution.
func TestClientCompleteUsageObserverFallsBackToAttemptModelWhenResponseModelEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer server.Close()

	var events []UsageEvent
	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "configured-model", server.URL, server.Client()), model: "configured-model"},
		},
		usageObserver: func(ev UsageEvent) { events = append(events, ev) },
	}

	_, err := client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "configured-model", events[0].Model)
}

// TestClientCompleteNoObserverConfiguredDoesNotPanic proves fireUsage is
// safe to call unconditionally from Complete's success path even when no
// caller registered WithUsageObserver — the zero-value Client built by
// struct literal in every other test in this package (usageObserver ==
// nil) exercises exactly this path already; this test names it
// explicitly.
func TestClientCompleteNoObserverConfiguredDoesNotPanic(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"m","choices":[{"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}],"usage":{}}`))
	}))
	defer server.Close()

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", server.URL, server.Client()), model: "m"},
		},
	}

	assert.NotPanics(t, func() {
		_, err := client.Complete(context.Background(), CompletionRequest{
			Messages:  []Message{{Role: "user", Content: "hi"}},
			MaxTokens: 8,
		})
		require.NoError(t, err)
	})
}

// TestNewWithUsageObserverFiresThroughRealBuiltClient proves
// WithUsageObserver is correctly wired end to end through New (not just
// through a hand-built *Client struct literal, as every other test in
// this file uses) — the observer must fire for a Complete call against a
// Client only the public constructor produced.
func TestNewWithUsageObserverFiresThroughRealBuiltClient(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"qwen-max","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":9,"completion_tokens":4}}`))
	}))
	defer server.Close()

	cfg := config.Default()
	cfg.LLM.Provider = "openai_compat"
	cfg.LLM.OpenAICompat.BaseURL = server.URL

	var events []UsageEvent
	client, err := New(cfg,
		WithKeyResolver(func(string) (string, error) { return "sk-test", nil }),
		WithUsageObserver(func(ev UsageEvent) { events = append(events, ev) }),
	)
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "hi"}},
		MaxTokens: 8,
	})
	require.NoError(t, err)

	require.Len(t, events, 1)
	assert.Equal(t, "openai_compat", events[0].Provider)
	assert.Equal(t, "qwen-max", events[0].Model)
	assert.Equal(t, 9, events[0].Usage.InputTokens)
	assert.Equal(t, 4, events[0].Usage.OutputTokens)
}

// TestNewWithUsageObserverNilOptionIsSafe mirrors
// TestNewWithKeyResolverNilOptionKeepsDefaultEnvResolver's nil-option
// safety proof for WithUsageObserver: passing a nil observer must not
// panic and must leave the Client with no observer attached (fireUsage's
// own nil check then makes it a no-op).
func TestNewWithUsageObserverNilOptionIsSafe(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "ollama"

	client, err := New(cfg, WithUsageObserver(nil))
	require.NoError(t, err)
	assert.Nil(t, client.usageObserver)
}

// TestNewResolvesAttemptModelToConnectorDefaultWhenConfigModelEmpty
// proves clientAttempt.model (fireUsage's fallback attribution — see its
// own doc comment) is populated with the connector package's own default
// model, not left at "", when config.LLM.Model was empty.
func TestNewResolvesAttemptModelToConnectorDefaultWhenConfigModelEmpty(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "anthropic"
	cfg.LLM.Model = ""

	client, err := New(cfg, WithKeyResolver(func(string) (string, error) { return "sk-test", nil }))
	require.NoError(t, err)
	require.Len(t, client.attempts, 1)
	assert.Equal(t, defaultAnthropicModel, client.attempts[0].model)
}
