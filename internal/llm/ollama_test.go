package llm

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
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
	c := newOllamaTestConnector(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`model runner crashed`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrOverloaded))
}

func TestOllamaStreamConcatenatesJSONLinesAndClosesCleanly(t *testing.T) {
	c := newOllamaTestConnector(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
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
	c := newOllamaTestConnector(t, "", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"models":[]}`))
	})

	_, err := c.Complete(context.Background(), CompletionRequest{Messages: []Message{{Role: "user", Content: "hi"}}, MaxTokens: 8})
	require.Error(t, err)
	assert.ErrorContains(t, err, "no local models found")
	assert.ErrorContains(t, err, "ollama pull")
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
