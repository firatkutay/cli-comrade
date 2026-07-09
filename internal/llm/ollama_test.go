package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newOllamaTestConnector(t *testing.T, model string, handler http.HandlerFunc) *ollamaConnector {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newOllamaConnector(model, srv.URL, srv.Client())
}

func TestOllamaCompleteRequestShapeNoAuthHeader(t *testing.T) {
	var gotPath, gotAuth string
	var gotBody ollamaChatRequest

	c := newOllamaTestConnector(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llama3.1","message":{"role":"assistant","content":"hi"},"done":true,"prompt_eval_count":6,"eval_count":2}`))
	})

	resp, err := c.Complete(context.Background(), CompletionRequest{
		System:    "be terse",
		Messages:  []Message{{Role: "user", Content: "hello"}},
		MaxTokens: 32,
	})
	require.NoError(t, err)

	assert.Equal(t, "/api/chat", gotPath)
	assert.Empty(t, gotAuth, "ollama needs no Authorization header")
	assert.Equal(t, "llama3.1", gotBody.Model)
	assert.False(t, gotBody.Stream)
	require.Len(t, gotBody.Messages, 2)
	assert.Equal(t, "system", gotBody.Messages[0].Role)
	assert.Equal(t, "user", gotBody.Messages[1].Role)

	assert.Equal(t, "hi", resp.Text)
	assert.Equal(t, "llama3.1", resp.Model)
	assert.Equal(t, "stop", resp.StopReason)
	assert.Equal(t, 6, resp.Usage.InputTokens)
	assert.Equal(t, 2, resp.Usage.OutputTokens)
}

func TestOllamaComplete500IsOverloaded(t *testing.T) {
	c := newOllamaTestConnector(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`model runner crashed`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestOllamaStreamConcatenatesJSONLinesAndClosesCleanly(t *testing.T) {
	c := newOllamaTestConnector(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		flusher, _ := w.(http.Flusher)
		lines := []string{
			`{"model":"llama3.1","message":{"role":"assistant","content":"Hel"},"done":false}` + "\n",
			`{"model":"llama3.1","message":{"role":"assistant","content":"lo"},"done":false}` + "\n",
			`{"model":"llama3.1","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":3,"eval_count":2}` + "\n",
		}
		for _, l := range lines {
			_, _ = w.Write([]byte(l))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})

	ch, err := c.Stream(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
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

	assert.Equal(t, "Hello", text)
	assert.NoError(t, final.Err)
	assert.True(t, final.Done)

	_, open := <-ch
	assert.False(t, open, "channel must be closed after the final chunk")
}

// TestOllamaStreamGoroutineExitsWhenContextCancelledWithoutDraining is the
// FAZ 6 hardening regression test: a consumer that reads one chunk and
// then abandons the channel (a Ctrl-C disconnect) must not leave the
// producer goroutine blocked forever on an unbuffered ch<- send nobody
// will ever read. The fake server streams three JSON lines so the
// producer is guaranteed to still be mid-stream — attempting its second
// send — by the time the test cancels the context without draining ch.
func TestOllamaStreamGoroutineExitsWhenContextCancelledWithoutDraining(t *testing.T) {
	c := newOllamaTestConnector(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		flusher, _ := w.(http.Flusher)
		lines := []string{
			`{"model":"llama3.1","message":{"role":"assistant","content":"one"},"done":false}` + "\n",
			`{"model":"llama3.1","message":{"role":"assistant","content":"two"},"done":false}` + "\n",
			`{"model":"llama3.1","message":{"role":"assistant","content":"three"},"done":false}` + "\n",
		}
		for _, l := range lines {
			_, _ = w.Write([]byte(l))
			if flusher != nil {
				flusher.Flush()
			}
		}
	})
	disableKeepAlives(t, c.httpClient)

	baseline := runtime.NumGoroutine()

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.Stream(ctx, CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.NoError(t, err)

	first := <-ch
	require.Equal(t, "one", first.Text, "sanity: must have received the first chunk before cancelling")

	cancel() // abandon ch without draining it

	assertGoroutinesReturnToBaseline(t, baseline)
}

func TestOllamaListModelsParsesTagsFixture(t *testing.T) {
	c := newOllamaTestConnector(t, "", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.1","model":"llama3.1:latest","modified_at":"2026-01-01T00:00:00Z","size":123},{"name":"mistral","model":"mistral:latest"}]}`))
	})

	names, err := c.ListModels(context.Background())
	require.NoError(t, err)
	assert.Equal(t, []string{"llama3.1", "mistral"}, names)
}

func TestOllamaListModelsEmptyProducesGuidanceError(t *testing.T) {
	c := newOllamaTestConnector(t, "", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.ErrorContains(t, err, "no local models found")
	assert.ErrorContains(t, err, "ollama pull")
}

func TestOllamaCompleteUnreachableProducesFriendlyReachabilityError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close() // nothing listens at url anymore: every request now fails at the transport level

	c := newOllamaConnector("llama3.1", url, http.DefaultClient)

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})

	require.Error(t, err)
	assert.ErrorContains(t, err, "does not appear to be running")
	assert.ErrorContains(t, err, "ollama serve")
	assert.ErrorContains(t, err, url)
}

func TestListOllamaModelsUnreachableProducesFriendlyReachabilityError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()

	_, err := ListOllamaModels(context.Background(), url, nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "does not appear to be running")
	assert.ErrorContains(t, err, "ollama serve")
}

func TestListOllamaModelsWrapperDefaultsHTTPClientAndReturnsNames(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/api/tags", r.URL.Path)
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"llama3.1"},{"name":"mistral"}]}`))
	}))
	defer srv.Close()

	names, err := ListOllamaModels(context.Background(), srv.URL, nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"llama3.1", "mistral"}, names)
}

func TestOllamaCompleteResolvesEmptyModelFromTags(t *testing.T) {
	var gotModel string
	c := newOllamaTestConnector(t, "", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.Header().Set("content-type", "application/json")
			_, _ = w.Write([]byte(`{"models":[{"name":"llama3.1"}]}`))
			return
		}
		var body ollamaChatRequest
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		gotModel = body.Model
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llama3.1","message":{"role":"assistant","content":"hi"},"done":true}`))
	})

	resp, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.NoError(t, err)
	assert.Equal(t, "llama3.1", gotModel)
	assert.Equal(t, "hi", resp.Text)
}
