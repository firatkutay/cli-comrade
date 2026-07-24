package cli

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// testConfigWithOpenAICompatBaseURL returns config.Default() with
// llm.openai_compat.base_url pointed at baseURL — the fixture every
// pingProviderWithKey test in this file needs.
func testConfigWithOpenAICompatBaseURL(baseURL string) config.Config {
	cfg := config.Default()
	cfg.LLM.OpenAICompat.BaseURL = baseURL
	return cfg
}

func TestPingProviderWithKeySendsBearerHeaderAndReportsSuccess(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5.4-mini","choices":[{"message":{"role":"assistant","content":"pong"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1}}`))
	}))
	defer srv.Close()

	cfg := testConfigWithOpenAICompatBaseURL(srv.URL)
	resp, latency, err := pingProviderWithKey(context.Background(), cfg, "openai_compat", "sk-test-key")

	require.NoError(t, err)
	assert.Equal(t, "gpt-5.4-mini", resp.Model)
	assert.Equal(t, "Bearer sk-test-key", gotAuth)
	assert.GreaterOrEqual(t, latency.Nanoseconds(), int64(0))
}

func TestPingProviderWithKeyPropagatesAuthRejection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid key"}}`))
	}))
	defer srv.Close()

	cfg := testConfigWithOpenAICompatBaseURL(srv.URL)
	_, _, err := pingProviderWithKey(context.Background(), cfg, "openai_compat", "sk-bad-key")

	require.Error(t, err)
	assert.ErrorContains(t, err, "invalid key")
}
