package doctor

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// TestReachCheckAnyStatusIsReachable proves ReachCheck's "ANY HTTP
// status counts as reachable" rule (401/404 included) against a real
// httptest.Server pointed at via openai_compat's configurable base_url.
func TestReachCheckAnyStatusIsReachable(t *testing.T) {
	for _, status := range []int{200, 401, 404, 500} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(status)
		}))

		deps := baseDeps()
		deps.Cfg.LLM.Provider = "openai_compat"
		deps.Cfg.LLM.OpenAICompat.BaseURL = srv.URL
		deps.HTTP = srv.Client()

		result := ReachCheck(context.Background(), deps)

		assert.Equal(t, SeverityOK, result.Severity, "status %d must be reachable", status)
		assert.Equal(t, i18n.MsgDoctorReachOK, result.Summary)
		srv.Close()
	}
}

func TestReachCheckTransportFailureIsFail(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = "http://127.0.0.1:1" // nothing listens here
	deps.HTTP = &http.Client{Timeout: 2 * time.Second}

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityFail, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachFail, result.Summary)
	assert.Equal(t, []any{"openai_compat"}, result.SummaryArgs)
	assert.NotEmpty(t, result.Detail)
}

func TestReachCheckSkipsUnknownProvider(t *testing.T) {
	deps := baseDeps()
	deps.Cfg.LLM.Provider = "bogus"

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeveritySkip, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachSkip, result.Summary)
}

func TestReachCheckOllamaWarnsOnZeroModels(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "ollama"
	deps.Cfg.LLM.Ollama.BaseURL = srv.URL
	deps.HTTP = srv.Client()

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachOllamaNoModels, result.Summary)
	assert.Equal(t, "ollama pull llama3.1", result.Fix)
}

func TestReachCheckOllamaOKWhenModelsPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.1"}]}`))
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "ollama"
	deps.Cfg.LLM.Ollama.BaseURL = srv.URL
	deps.HTTP = srv.Client()

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachOK, result.Summary)
}

// TestReachCheckLiveOKAppendsLatency proves --live, on a reachable
// provider, calls deps.LivePing and reports success with the latency it
// returned.
func TestReachCheckLiveOKAppendsLatency(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = srv.URL
	deps.HTTP = srv.Client()
	deps.Live = true
	deps.Store = newFakeStore(map[string]string{"openai_compat": "sk-test"})
	var gotProvider, gotKey string
	deps.LivePing = func(_ context.Context, _ config.Config, provider, key string) (llm.CompletionResponse, time.Duration, error) {
		gotProvider, gotKey = provider, key
		return llm.CompletionResponse{Model: "gpt-5.4-mini"}, 42 * time.Millisecond, nil
	}

	result := ReachCheck(context.Background(), deps)

	require.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachLiveOK, result.Summary)
	assert.Equal(t, "openai_compat", gotProvider)
	assert.Equal(t, "sk-test", gotKey)
}

// TestReachCheckLiveRejectedIsFail proves a live 401/403
// (llm.ErrAuthRejected) is classified as Fail, naming the rejection —
// not a generic Warn.
func TestReachCheckLiveRejectedIsFail(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = srv.URL
	deps.HTTP = srv.Client()
	deps.Live = true
	deps.Store = newFakeStore(map[string]string{"openai_compat": "sk-bad"})
	deps.LivePing = func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
		return llm.CompletionResponse{}, 0, llm.ErrAuthRejected
	}

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityFail, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachLiveRejected, result.Summary)
}

// TestReachCheckLiveOtherFailureIsWarn proves any OTHER live-ping
// failure (network/timeout/parse) is a Warn — the key might still be
// fine.
func TestReachCheckLiveOtherFailureIsWarn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = srv.URL
	deps.HTTP = srv.Client()
	deps.Live = true
	deps.Store = newFakeStore(map[string]string{"openai_compat": "sk-test"})
	deps.LivePing = func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
		return llm.CompletionResponse{}, 0, assertNotFoundErr
	}

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityWarn, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachLiveFailed, result.Summary)
}

// TestReachCheckLiveNeverRunsForOllama proves --live never fires a live
// ping for ollama (it needs no credential to reject in the first
// place) — a nil LivePing must never be dereferenced/called for ollama.
func TestReachCheckLiveNeverRunsForOllama(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.1"}]}`))
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "ollama"
	deps.Cfg.LLM.Ollama.BaseURL = srv.URL
	deps.HTTP = srv.Client()
	deps.Live = true
	livePingCalled := false
	deps.LivePing = func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
		livePingCalled = true
		return llm.CompletionResponse{}, 0, nil
	}

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.False(t, livePingCalled)
}

// TestReachCheckLiveNeverSendsCredentialWhenNotLive proves LivePing is
// never invoked unless deps.Live is explicitly true — the default-mode
// "never sends a credential anywhere" guarantee.
func TestReachCheckLiveNeverSendsCredentialWhenNotLive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	deps := baseDeps()
	deps.Cfg.LLM.Provider = "openai_compat"
	deps.Cfg.LLM.OpenAICompat.BaseURL = srv.URL
	deps.HTTP = srv.Client()
	deps.Live = false
	livePingCalled := false
	deps.LivePing = func(context.Context, config.Config, string, string) (llm.CompletionResponse, time.Duration, error) {
		livePingCalled = true
		return llm.CompletionResponse{}, 0, nil
	}

	result := ReachCheck(context.Background(), deps)

	assert.Equal(t, SeverityOK, result.Severity)
	assert.Equal(t, i18n.MsgDoctorReachOK, result.Summary)
	assert.False(t, livePingCalled)
}
