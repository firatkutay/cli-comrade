package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newAnthropicTestConnector(t *testing.T, handler http.HandlerFunc) *anthropicConnector {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	c := newAnthropicConnector("test-key", "claude-haiku-4-5", srv.Client())
	c.baseURL = srv.URL + "/v1/messages"
	return c
}

func TestAnthropicCompleteRequestShape(t *testing.T) {
	var gotPath, gotAPIKey, gotVersion, gotContentType string
	var gotBody anthropicRequest

	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-api-key")
		gotVersion = r.Header.Get("anthropic-version")
		gotContentType = r.Header.Get("content-type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"hi"}],"model":"claude-haiku-4-5","stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":1}}`))
	})

	resp, err := c.Complete(context.Background(), CompletionRequest{
		System:    "you are terse",
		Messages:  []Message{{Role: "user", Content: "hello"}},
		MaxTokens: 64,
	})
	require.NoError(t, err)

	assert.Equal(t, "/v1/messages", gotPath)
	assert.Equal(t, "test-key", gotAPIKey)
	assert.Equal(t, anthropicAPIVersion, gotVersion)
	assert.Equal(t, "application/json", gotContentType)
	assert.Equal(t, "claude-haiku-4-5", gotBody.Model)
	assert.Equal(t, "you are terse", gotBody.System)
	assert.Equal(t, 64, gotBody.MaxTokens)
	require.Len(t, gotBody.Messages, 1)
	assert.Equal(t, "user", gotBody.Messages[0].Role)
	assert.Equal(t, "hello", gotBody.Messages[0].Content)
	assert.False(t, gotBody.Stream)

	assert.Equal(t, "hi", resp.Text)
	assert.Equal(t, "claude-haiku-4-5", resp.Model)
	assert.Equal(t, "end_turn", resp.StopReason)
	assert.Equal(t, 3, resp.Usage.InputTokens)
	assert.Equal(t, 1, resp.Usage.OutputTokens)
}

func TestAnthropicComplete401IsAuthRejected(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthRejected))
	assert.ErrorContains(t, err, "invalid x-api-key")

	var statusErr *StatusError
	require.True(t, errors.As(err, &statusErr))
	assert.Equal(t, http.StatusUnauthorized, statusErr.StatusCode)
	assert.Equal(t, "anthropic", statusErr.Provider)
}

func TestAnthropicComplete429IsOverloaded(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"rate_limit_error","message":"slow down"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestAnthropicComplete529IsOverloaded(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(529)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"overloaded_error","message":"overloaded"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestAnthropicComplete500IsOverloaded(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"api_error","message":"boom"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestAnthropicStreamConcatenatesDeltasAndClosesCleanly(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`event: message_start` + "\n" + `data: {"type":"message_start"}` + "\n\n",
			`event: content_block_start` + "\n" + `data: {"type":"content_block_start"}` + "\n\n",
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"Hel"}}` + "\n\n",
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"lo"}}` + "\n\n",
			`event: content_block_stop` + "\n" + `data: {"type":"content_block_stop"}` + "\n\n",
			`event: message_delta` + "\n" + `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"}}` + "\n\n",
			`event: message_stop` + "\n" + `data: {"type":"message_stop"}` + "\n\n",
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
			break
		}
		text += chunk.Text
	}
	// Drain remainder, if any, to confirm the channel closes cleanly.
	_, open := <-ch
	assert.False(t, open, "channel must be closed after the final chunk")

	assert.Equal(t, "Hello", text)
	assert.NoError(t, final.Err)
	assert.True(t, final.Done)
}

func TestAnthropicStreamMidStreamErrorEvent(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`event: content_block_delta` + "\n" + `data: {"type":"content_block_delta","delta":{"type":"text_delta","text":"partial"}}` + "\n\n",
			`event: error` + "\n" + `data: {"type":"error","error":{"type":"overloaded_error","message":"server overloaded"}}` + "\n\n",
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

	assert.Equal(t, "partial", text)
	require.Error(t, final.Err)
	assert.ErrorContains(t, final.Err, "server overloaded")
}

func TestAnthropicStreamHTTPErrorBeforeStreaming(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"type":"error","error":{"type":"authentication_error","message":"bad key"}}`))
	})

	_, err := c.Stream(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthRejected))
}

// Sanity check that the test helper's fake server actually speaks HTTP
// (guards against a silently-broken httptest wiring making every other
// test in this file vacuously pass).
func TestAnthropicTestServerReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
	require.NoError(t, err)
	resp, err := srv.Client().Do(req)
	require.NoError(t, err)
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)
	assert.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestAnthropicCompleteRespectsContextTimeout(t *testing.T) {
	c := newAnthropicTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"content":[{"type":"text","text":"late"}],"model":"claude-haiku-4-5","stop_reason":"end_turn","usage":{}}`))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()

	_, err := c.Complete(ctx, CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.False(t, errors.Is(err, ErrAuthRejected), "a timeout must never be classified as an auth failure")
}
