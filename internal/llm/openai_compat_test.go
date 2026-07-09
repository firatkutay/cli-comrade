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

func newOpenAICompatTestConnector(t *testing.T, handler http.HandlerFunc) *openAICompatConnector {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return newOpenAICompatConnector("test-key", "gpt-5.4-mini", srv.URL, srv.Client())
}

func TestOpenAICompatCompleteRequestShape(t *testing.T) {
	var gotPath, gotAuth, gotContentType string
	var gotBody openAIRequest

	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotContentType = r.Header.Get("content-type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"gpt-5.4-mini","choices":[{"message":{"role":"assistant","content":"hi"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2}}`))
	})

	resp, err := c.Complete(context.Background(), CompletionRequest{
		System:    "be terse",
		Messages:  []Message{{Role: "user", Content: "hello"}},
		MaxTokens: 32,
	})
	require.NoError(t, err)

	assert.Equal(t, "/chat/completions", gotPath)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "gpt-5.4-mini", gotBody.Model)
	assert.False(t, gotBody.Stream)
	require.Len(t, gotBody.Messages, 2)
	assert.Equal(t, "system", gotBody.Messages[0].Role)
	assert.Equal(t, "be terse", gotBody.Messages[0].Content)
	assert.Equal(t, "user", gotBody.Messages[1].Role)
	assert.Equal(t, "hello", gotBody.Messages[1].Content)

	assert.Equal(t, "hi", resp.Text)
	assert.Equal(t, "gpt-5.4-mini", resp.Model)
	assert.Equal(t, "stop", resp.StopReason)
	assert.Equal(t, 5, resp.Usage.InputTokens)
	assert.Equal(t, 2, resp.Usage.OutputTokens)
}

func TestOpenAICompatComplete401IsAuthRejected(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"invalid api key","type":"invalid_request_error"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthRejected))
	assert.ErrorContains(t, err, "invalid api key")
}

func TestOpenAICompatComplete429IsOverloaded(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestOpenAICompatComplete503IsOverloaded(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":{"message":"overloaded","type":"server_error"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestOpenAICompatStreamConcatenatesDeltasUntilDoneSentinel(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"choices":[{"delta":{"content":"Hel"}}]}` + "\n\n",
			`data: {"choices":[{"delta":{"content":"lo"}}]}` + "\n\n",
			`data: [DONE]` + "\n\n",
		}
		for _, f := range frames {
			_, _ = w.Write([]byte(f))
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

// TestOpenAICompatStreamGoroutineExitsWhenContextCancelledWithoutDraining
// is the FAZ 6 hardening regression test: a consumer that reads one chunk
// and then abandons the channel (a Ctrl-C disconnect) must not leave the
// producer goroutine blocked forever on an unbuffered ch<- send nobody
// will ever read. The fake server streams three deltas so the producer is
// guaranteed to still be mid-stream — attempting its second send — by the
// time the test cancels the context without draining ch.
func TestOpenAICompatStreamGoroutineExitsWhenContextCancelledWithoutDraining(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
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

func TestOpenAICompatListModelsParsesIDsLeniently(t *testing.T) {
	var gotPath, gotAuth string
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("content-type", "application/json")
		// Extra fields ("object", "created", "owned_by") a real
		// OpenAI-compatible /models response carries must not break
		// parsing — only "id" is read.
		_, _ = w.Write([]byte(`{"object":"list","data":[{"id":"gpt-5.4-mini","object":"model","created":1,"owned_by":"openai"},{"id":"gpt-5.4"}]}`))
	})

	names, err := c.ListModels(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "/models", gotPath)
	assert.Equal(t, "Bearer test-key", gotAuth)
	assert.Equal(t, []string{"gpt-5.4-mini", "gpt-5.4"}, names)
}

func TestOpenAICompatListModelsSkipsEmptyIDs(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":""},{"id":"gpt-5.4"}]}`))
	})

	names, err := c.ListModels(context.Background())

	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-5.4"}, names)
}

func TestOpenAICompatListModels401IsAuthRejected(t *testing.T) {
	c := newOpenAICompatTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"message":"bad key"}}`))
	})

	_, err := c.ListModels(context.Background())

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthRejected))
}

func TestListOpenAICompatModelsWrapperDefaultsHTTPClient(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "Bearer wrapper-key", r.Header.Get("Authorization"))
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"data":[{"id":"gpt-5.4-mini"}]}`))
	}))
	defer srv.Close()

	names, err := ListOpenAICompatModels(context.Background(), srv.URL, "wrapper-key", nil)

	require.NoError(t, err)
	assert.Equal(t, []string{"gpt-5.4-mini"}, names)
}
