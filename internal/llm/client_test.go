package llm

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
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

// TestClientStreamGoroutineExitsWhenContextCancelledWithoutDraining is the
// FAZ 6 hardening regression test at the Client level: it proves the full
// chain — the connector's own producer goroutine (guarded by sendChunk)
// AND releaseOnClose's forwarding goroutine (guarded the same way) — both
// exit once the caller cancels ctx without ever draining the channel
// Client.Stream returned. The fake server streams three deltas so the
// producer is guaranteed to still be mid-stream by the time the test
// cancels.
func TestClientStreamGoroutineExitsWhenContextCancelledWithoutDraining(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"choices":[{"delta":{"content":"one"}}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":"two"}}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":"three"}}]}` + "\n\n",
		}
		for _, f := range frames {
			_, _ = w.Write([]byte(f))
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
	defer srv.Close()

	httpClient := srv.Client()
	disableKeepAlives(t, httpClient)

	client := &Client{
		timeout: 5 * time.Second,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", srv.URL, httpClient)},
		},
	}

	baseline := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := client.Stream(ctx, CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.NoError(t, err)

	first := <-ch
	require.Equal(t, "one", first.Text, "sanity: must have received the first chunk before cancelling")

	cancel() // abandon ch without draining it

	assertGoroutinesReturnToBaseline(t, baseline)
}

func TestNewUnknownProviderErrors(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "bogus"

	_, err := New(cfg)
	require.Error(t, err)
	assert.ErrorContains(t, err, `unknown provider "bogus"`)
}

// TestNewRejectsMetadataBaseURLForActiveProvider is SAST finding #3's
// point-of-use enforcement (buildProvider, client.go): unlike
// config.validateLoadedConfig at config-load time (which only warns, so
// Load() itself never bricks), the moment a Client is actually built for
// the active provider — the moment its API key would be handed to a
// connector holding this base_url — a cloud-metadata/link-local host must
// hard-fail Client construction. The returned error must remain
// errors.As-reachable to *config.InvalidValueError, with the SAME Reason
// checkBaseURL itself would produce, so internal/cli's
// translateBaseURLRejectedError (runtime.go) can render it.
func TestNewRejectsMetadataBaseURLForActiveProvider(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "openai_compat"
	cfg.LLM.OpenAICompat.BaseURL = "http://169.254.169.254/latest/meta-data/"

	_, err := New(cfg)

	require.Error(t, err)
	var invalid *config.InvalidValueError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, config.ReasonMetadataOrLinkLocal, invalid.Reason)
	assert.Equal(t, "llm.openai_compat.base_url", invalid.Key)
	assert.Equal(t, "http://169.254.169.254/latest/meta-data/", invalid.Raw)
}

// TestNewRejectsNonHTTPBaseURLForActiveProvider is
// TestNewRejectsMetadataBaseURLForActiveProvider's counterpart for the
// OTHER reject class (ReasonNotURL) and the ollama connector, proving both
// base_url-holding connectors (openai_compat and ollama) and both reject
// reasons are enforced, not just one of each.
func TestNewRejectsNonHTTPBaseURLForActiveProvider(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Ollama.BaseURL = "ftp://x"

	_, err := New(cfg)

	require.Error(t, err)
	var invalid *config.InvalidValueError
	require.ErrorAs(t, err, &invalid)
	assert.Equal(t, config.ReasonNotURL, invalid.Reason)
	assert.Equal(t, "llm.ollama.base_url", invalid.Key)
}

// TestNewAllowsWarnClassBaseURLForActiveProvider proves the reject-class
// check does NOT also reject a warn-class value (http:// to a non-loopback
// host, e.g. a legitimate self-hosted LAN Ollama) — client construction
// must succeed exactly like it always has for this case, matching
// checkBaseURL's own documented warn-but-allow rule.
func TestNewAllowsWarnClassBaseURLForActiveProvider(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Ollama.BaseURL = "http://192.168.1.5:11434"

	client, err := New(cfg)

	require.NoError(t, err)
	require.Len(t, client.attempts, 1)
}

// TestNewAllowsPublicHTTPSBaseURLForActiveProvider is the plain-good-path
// sanity check: the shipped default-shaped https:// endpoint must keep
// building a Client with no error at all.
func TestNewAllowsPublicHTTPSBaseURLForActiveProvider(t *testing.T) {
	cfg := config.Default()
	cfg.LLM.Provider = "openai_compat"
	cfg.LLM.OpenAICompat.BaseURL = "https://api.openai.com/v1"

	client, err := New(cfg)

	require.NoError(t, err)
	require.Len(t, client.attempts, 1)
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

// TestIdleTimeoutDurationDisabledForNonPositive pins idleTimeoutDuration's
// contract deliberately diverging from httpTimeout's: 0 and negative both
// mean "disabled" (return 0), never a fallback default duration — because
// 0 is idle_timeout_seconds's own documented default, and defaulting it
// to some non-zero duration would silently turn the feature on for every
// existing config file that predates it.
func TestIdleTimeoutDurationDisabledForNonPositive(t *testing.T) {
	assert.Equal(t, time.Duration(0), idleTimeoutDuration(0))
	assert.Equal(t, time.Duration(0), idleTimeoutDuration(-5))
	assert.Equal(t, 30*time.Second, idleTimeoutDuration(30))
}

// TestClientStreamIdleTimeoutAbortsWhenGapBetweenChunksExceeded proves
// idle_timeout_seconds actually enforces a per-chunk gap, independent of
// the whole-stream timeout_seconds deadline: the fake server sends one
// chunk, then sleeps far longer than the configured idle timeout before
// sending a second one. Client.Stream must abort — with a final Chunk
// wrapping ErrIdleTimeout — well before that second, late chunk ever
// arrives.
func TestClientStreamIdleTimeoutAbortsWhenGapBetweenChunksExceeded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"first"}}]}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(300 * time.Millisecond) // far longer than the 50ms idle timeout below
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"late"}}]}` + "\n\n" + `data: [DONE]` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := &Client{
		timeout:     5 * time.Second,
		idleTimeout: 50 * time.Millisecond,
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", srv.URL, srv.Client())},
		},
	}

	start := time.Now()
	ch, err := client.Stream(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.NoError(t, err)

	var text string
	var final Chunk
	for chunk := range ch {
		if chunk.Done {
			final = chunk
			continue
		}
		text += chunk.Text
	}
	elapsed := time.Since(start)

	assert.Equal(t, "first", text, "the late second chunk must never arrive — the idle timeout must abort before it does")
	require.Error(t, final.Err)
	assert.True(t, errors.Is(final.Err, ErrIdleTimeout))
	assert.Less(t, elapsed, 250*time.Millisecond, "must abort on the ~50ms idle timeout, not wait out the server's 300ms sleep")
}

// TestClientStreamIdleTimeoutDisabledByDefaultAllowsSlowGaps proves
// idle_timeout_seconds's 0 default is truly a no-op: a Client built
// without idleTimeout set (its zero value) must tolerate an inter-chunk
// gap that would trip a configured idle timeout, and complete normally —
// this is the "identical to today" backward-compatibility requirement.
func TestClientStreamIdleTimeoutDisabledByDefaultAllowsSlowGaps(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"first"}}]}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte(`data: {"choices":[{"delta":{"content":"second"}}]}` + "\n\n" + `data: [DONE]` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer srv.Close()

	client := &Client{
		timeout: 5 * time.Second,
		// idleTimeout intentionally left at its zero value: disabled.
		attempts: []clientAttempt{
			{providerName: "openai_compat", provider: newOpenAICompatConnector("k", "m", srv.URL, srv.Client())},
		},
	}

	ch, err := client.Stream(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.NoError(t, err)

	var text string
	var final Chunk
	for chunk := range ch {
		if chunk.Done {
			final = chunk
			continue
		}
		text += chunk.Text
	}

	assert.Equal(t, "firstsecond", text)
	assert.NoError(t, final.Err)
}
