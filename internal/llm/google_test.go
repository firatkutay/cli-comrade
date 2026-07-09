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

// newGoogleTestConnector builds a googleConnector pointed at a fake
// server: baseURL is set to "<server>/models" so the connector's own
// url() method still builds "<baseURL>/<model>:<action>" exactly as it
// does against the real googleAPIBase, keeping the path-encoding and
// action-suffix logic under test unchanged.
func newGoogleTestConnector(t *testing.T, handler http.HandlerFunc) *googleConnector {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &googleConnector{
		apiKey:     "test-key",
		model:      "gemini-3.5-flash",
		baseURL:    srv.URL + "/models",
		httpClient: srv.Client(),
	}
}

func TestGoogleCompleteRequestShapeAndModelPathEncoding(t *testing.T) {
	var gotPath, gotAPIKey, gotContentType string
	var gotBody googleRequest

	c := newGoogleTestConnector(t, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAPIKey = r.Header.Get("x-goog-api-key")
		gotContentType = r.Header.Get("content-type")
		require.NoError(t, json.NewDecoder(r.Body).Decode(&gotBody))

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"candidates":[{"content":{"parts":[{"text":"hi"}],"role":"model"},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":1}}`))
	})

	resp, err := c.Complete(context.Background(), CompletionRequest{
		System:    "be terse",
		Messages:  []Message{{Role: "user", Content: "hello"}, {Role: "assistant", Content: "sure"}},
		MaxTokens: 32,
	})
	require.NoError(t, err)

	// Model is path-encoded, never a body field: the fake server sees it
	// as a literal path segment, followed by the ":generateContent" action.
	assert.Equal(t, "/models/gemini-3.5-flash:generateContent", gotPath)
	assert.Equal(t, "test-key", gotAPIKey)
	assert.Equal(t, "application/json", gotContentType)

	require.NotNil(t, gotBody.SystemInstruction)
	assert.Equal(t, "be terse", gotBody.SystemInstruction.Parts[0].Text)
	require.Len(t, gotBody.Contents, 2)
	assert.Equal(t, "user", gotBody.Contents[0].Role)
	assert.Equal(t, "hello", gotBody.Contents[0].Parts[0].Text)
	assert.Equal(t, "model", gotBody.Contents[1].Role, "assistant role must map to Gemini's \"model\" role")
	require.NotNil(t, gotBody.GenerationConfig)
	assert.Equal(t, 32, gotBody.GenerationConfig.MaxOutputTokens)

	assert.Equal(t, "hi", resp.Text)
	assert.Equal(t, "STOP", resp.StopReason)
	assert.Equal(t, 4, resp.Usage.InputTokens)
	assert.Equal(t, 1, resp.Usage.OutputTokens)
}

func TestGoogleComplete401IsAuthRejected(t *testing.T) {
	c := newGoogleTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":{"code":401,"message":"API key not valid","status":"UNAUTHENTICATED"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrAuthRejected))
	assert.ErrorContains(t, err, "API key not valid")
}

func TestGoogleComplete429ResourceExhaustedIsOverloaded(t *testing.T) {
	c := newGoogleTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error":{"code":429,"message":"quota exceeded","status":"RESOURCE_EXHAUSTED"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestGoogleComplete500IsOverloaded(t *testing.T) {
	c := newGoogleTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"code":500,"message":"internal","status":"INTERNAL"}}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestGoogleStreamConcatenatesDeltasAndClosesCleanly(t *testing.T) {
	c := newGoogleTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"Hel"}],"role":"model"}}]}` + "\n\n",
			`data: {"candidates":[{"content":{"parts":[{"text":"lo"}],"role":"model"}}]}` + "\n\n",
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

// TestGoogleStreamGoroutineExitsWhenContextCancelledWithoutDraining is the
// FAZ 6 hardening regression test: a consumer that reads one chunk and
// then abandons the channel (a Ctrl-C disconnect) must not leave the
// producer goroutine blocked forever on an unbuffered ch<- send nobody
// will ever read. The fake server streams three deltas so the producer is
// guaranteed to still be mid-stream — attempting its second send — by the
// time the test cancels the context without draining ch.
func TestGoogleStreamGoroutineExitsWhenContextCancelledWithoutDraining(t *testing.T) {
	c := newGoogleTestConnector(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("content-type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		frames := []string{
			`data: {"candidates":[{"content":{"parts":[{"text":"one"}],"role":"model"}}]}` + "\n\n",
			`data: {"candidates":[{"content":{"parts":[{"text":"two"}],"role":"model"}}]}` + "\n\n",
			`data: {"candidates":[{"content":{"parts":[{"text":"three"}],"role":"model"}}]}` + "\n\n",
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
