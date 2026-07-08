package llm

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
)

// TestClientCompleteRedactsPayloadBeforeReachingConnector proves the
// non-bypassable middleware from CLAUDE.md security rule #3: a Client
// built the only way external code can build one — New(cfg) — must
// never let a secret in System/Messages reach the wire. The server
// below is the "connector's actual HTTP call"; if this test failed the
// raw secret would show up in capturedBody.
func TestClientCompleteRedactsPayloadBeforeReachingConnector(t *testing.T) {
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = body

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llama3.1","message":{"role":"assistant","content":"ok"},"done":true}`))
	}))
	defer server.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Model = "llama3.1"
	cfg.LLM.TimeoutSeconds = 5
	cfg.LLM.Ollama.BaseURL = server.URL
	cfg.Privacy.RedactEmails = false
	cfg.Privacy.RedactIPs = false

	client, err := New(cfg)
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), CompletionRequest{
		System: "You are a helpful assistant. api_key=sk-ABCDEFGHIJ1234567890KL",
		Messages: []Message{
			{Role: "user", Content: "my password=hunter2forever and my email is alice@example.com"},
		},
		MaxTokens: 8,
	})
	require.NoError(t, err)
	require.NotEmpty(t, capturedBody)

	got := string(capturedBody)

	// The masked forms must be present...
	assert.Contains(t, got, "[REDACTED:credential]")
	// ...and the raw secrets must be completely absent from what left
	// this process.
	assert.NotContains(t, got, "sk-ABCDEFGHIJ1234567890KL")
	assert.NotContains(t, got, "hunter2forever")

	// redact_emails=false: the email must be left intact — proving the
	// middleware respects the config flag rather than always masking
	// everything.
	assert.Contains(t, got, "alice@example.com")
}

// TestClientCompleteRedactsEmailWhenConfigured proves the redact_emails
// config flag actually reaches the hardwired Redactor Client builds:
// when true, the same payload's email must be masked too.
func TestClientCompleteRedactsEmailWhenConfigured(t *testing.T) {
	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = body

		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"model":"llama3.1","message":{"role":"assistant","content":"ok"},"done":true}`))
	}))
	defer server.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "ollama"
	cfg.LLM.Model = "llama3.1"
	cfg.LLM.TimeoutSeconds = 5
	cfg.LLM.Ollama.BaseURL = server.URL
	cfg.Privacy.RedactEmails = true

	client, err := New(cfg)
	require.NoError(t, err)

	_, err = client.Complete(context.Background(), CompletionRequest{
		Messages:  []Message{{Role: "user", Content: "reach me at alice@example.com"}},
		MaxTokens: 8,
	})
	require.NoError(t, err)

	got := string(capturedBody)
	assert.NotContains(t, got, "alice@example.com")
	assert.Contains(t, got, "[REDACTED:email]")
}

// TestClientStreamRedactsPayloadBeforeReachingConnector mirrors the
// Complete proof above but drives Client.Stream, against the
// openai_compat connector's real Server-Sent Events wire format — this
// is the "SSE server" leg of the non-bypassable middleware proof.
// Stream's redaction happens once, up front in Client.Stream (before
// the connector's initial POST), so draining the returned channel to
// completion and then inspecting the single captured request body is
// sufficient to prove no raw secret from either System or a message's
// Content ever reached the wire.
func TestClientStreamRedactsPayloadBeforeReachingConnector(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")
	t.Setenv("COMRADE_OPENAI_COMPAT_API_KEY", "")

	var capturedBody []byte

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		capturedBody = body

		w.Header().Set("content-type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	cfg := config.Config{}
	cfg.LLM.Provider = "openai_compat"
	cfg.LLM.Model = "test-model"
	cfg.LLM.TimeoutSeconds = 5
	cfg.LLM.OpenAICompat.BaseURL = server.URL

	client, err := New(cfg)
	require.NoError(t, err)

	ch, err := client.Stream(context.Background(), CompletionRequest{
		System: "You are a helpful assistant. api_key=sk-ABCDEFGHIJ1234567890KL",
		Messages: []Message{
			{Role: "user", Content: "my password=hunter2forever please help"},
		},
		MaxTokens: 8,
	})
	require.NoError(t, err)

	// Drain the stream fully before inspecting the captured request —
	// the wire call already happened by the time Stream returned the
	// channel, but draining confirms the stream itself completes
	// cleanly on this redacted payload.
	var lastErr error
	for chunk := range ch {
		if chunk.Done {
			lastErr = chunk.Err
		}
	}
	require.NoError(t, lastErr)
	require.NotEmpty(t, capturedBody)

	got := string(capturedBody)

	assert.Contains(t, got, "[REDACTED:credential]", "both System's api_key= and the message's password= must be masked")
	assert.NotContains(t, got, "sk-ABCDEFGHIJ1234567890KL")
	assert.NotContains(t, got, "hunter2forever")
}
