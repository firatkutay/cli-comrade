package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

// ollamaConnector talks to a local Ollama server over raw net/http. No
// authentication is required. It is unexported: external packages obtain
// one only indirectly, through Client (see New in client.go).
type ollamaConnector struct {
	// model may be empty: Ollama has no fixed default model, so an empty
	// value means "resolve the first model from /api/tags at attempt
	// time" (see resolveModel), not "use some hardcoded default".
	model      string
	baseURL    string
	httpClient *http.Client
}

func newOllamaConnector(model, baseURL string, httpClient *http.Client) *ollamaConnector {
	return &ollamaConnector{model: model, baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient}
}

func (c *ollamaConnector) Name() string { return "ollama" }

type ollamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ollamaChatRequest struct {
	Model    string          `json:"model"`
	Messages []ollamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type ollamaChatResponse struct {
	Model           string        `json:"model"`
	Message         ollamaMessage `json:"message"`
	Done            bool          `json:"done"`
	PromptEvalCount int           `json:"prompt_eval_count"`
	EvalCount       int           `json:"eval_count"`
}

func (c *ollamaConnector) messages(req CompletionRequest) []ollamaMessage {
	msgs := make([]ollamaMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, ollamaMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, ollamaMessage(m))
	}
	return msgs
}

// resolveModel returns c.model if it is non-empty, otherwise the first
// entry from /api/tags. It returns a helpful "pull a model first" error
// when no local models are installed.
func (c *ollamaConnector) resolveModel(ctx context.Context) (string, error) {
	if c.model != "" {
		return c.model, nil
	}
	names, err := c.ListModels(ctx)
	if err != nil {
		return "", err
	}
	if len(names) == 0 {
		return "", fmt.Errorf("ollama: no local models found; pull one first, e.g. `ollama pull llama3.1`")
	}
	return names[0], nil
}

func (c *ollamaConnector) doRequest(ctx context.Context, body ollamaChatRequest) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapOllamaReachabilityError(c.baseURL, fmt.Errorf("ollama: request: %w", err))
	}
	return resp, nil
}

// wrapOllamaReachabilityError recognizes a transport-level failure (the
// connection was refused, timed out, or never resolved) inside err — the
// shape *http.Client.Do returns as a *url.Error for exactly this class of
// failure, as opposed to a non-2xx HTTP response (handled separately by
// ollamaStatusError) — and replaces it with a message actionable for a
// terminal beginner: UYGULAMA_PLANI.md FAZ 8 item 5's "Ollama çalışmıyor
// görünüyor; `ollama serve` ..." requirement. Applied at doRequest and
// ListModels, so every ollamaConnector entry point (Complete, Stream, and
// `comrade config models`'s live /api/tags query) gets the same
// friendly message instead of a bare "connection refused".
func wrapOllamaReachabilityError(baseURL string, err error) error {
	var urlErr *url.Error
	if errors.As(err, &urlErr) {
		return fmt.Errorf("ollama does not appear to be running at %s; start it with `ollama serve`, or set llm.ollama.base_url to the correct address (%w)", baseURL, err)
	}
	return err
}

func (c *ollamaConnector) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	model, err := c.resolveModel(ctx)
	if err != nil {
		return CompletionResponse{}, err
	}

	resp, err := c.doRequest(ctx, ollamaChatRequest{Model: model, Messages: c.messages(req), Stream: false})
	if err != nil {
		return CompletionResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("ollama: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return CompletionResponse{}, ollamaStatusError(resp.StatusCode, raw)
	}

	var parsed ollamaChatResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CompletionResponse{}, fmt.Errorf("ollama: decode response: %w", err)
	}

	stopReason := ""
	if parsed.Done {
		stopReason = "stop"
	}
	return CompletionResponse{
		Text:       parsed.Message.Content,
		Model:      parsed.Model,
		StopReason: stopReason,
		Usage: Usage{
			InputTokens:  parsed.PromptEvalCount,
			OutputTokens: parsed.EvalCount,
		},
	}, nil
}

// ollamaStatusError classifies a non-200 Ollama response. Ollama has no
// notion of an auth-rejected response (there is no credential to reject);
// 429/5xx are treated as retryable overload, matching every other
// connector's classification.
func ollamaStatusError(status int, body []byte) error {
	msg := strings.TrimSpace(string(body))

	var sentinel error
	if status == http.StatusTooManyRequests || status >= 500 {
		sentinel = ErrOverloaded
	}

	return &StatusError{Provider: "ollama", StatusCode: status, Message: msg, Sentinel: sentinel}
}

func (c *ollamaConnector) Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error) {
	model, err := c.resolveModel(ctx)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(ctx, ollamaChatRequest{Model: model, Messages: c.messages(req), Stream: true}) //nolint:bodyclose // closed on both paths below: immediately on a non-200 status, or by the streaming goroutine's own defer once it finishes reading — bodyclose's static path check doesn't see the latter as a guaranteed close.
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, ollamaStatusError(resp.StatusCode, raw)
	}

	ch := make(chan Chunk)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		var streamErr error
		scanner := bufio.NewScanner(resp.Body)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line == "" {
				continue
			}
			var typed ollamaChatResponse
			if err := json.Unmarshal([]byte(line), &typed); err != nil {
				streamErr = fmt.Errorf("ollama: decode stream line: %w", err)
				break
			}
			if typed.Message.Content != "" {
				if !sendChunk(ctx, ch, Chunk{Text: typed.Message.Content}) {
					streamErr = ctx.Err()
					break
				}
			}
			if typed.Done {
				break
			}
		}
		if streamErr == nil {
			if err := scanner.Err(); err != nil {
				streamErr = fmt.Errorf("ollama: read stream: %w", err)
			}
		}
		sendChunk(ctx, ch, Chunk{Done: true, Err: streamErr})
	}()
	return ch, nil
}

// ollamaTagsResponse mirrors GET /api/tags's response shape (only the
// fields ListModels needs).
type ollamaTagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// ListModels queries GET /api/tags for locally-pulled Ollama models, used
// to resolve the default model (see resolveModel) and, in a later phase,
// by comrade config's model picker.
func (c *ollamaConnector) ListModels(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/tags", nil)
	if err != nil {
		return nil, fmt.Errorf("ollama: build request: %w", err)
	}
	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapOllamaReachabilityError(c.baseURL, fmt.Errorf("ollama: request: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("ollama: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, ollamaStatusError(resp.StatusCode, raw)
	}

	var parsed ollamaTagsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("ollama: decode tags response: %w", err)
	}

	names := make([]string, 0, len(parsed.Models))
	for _, m := range parsed.Models {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return names, nil
}

// ListOllamaModels queries a local (or remote, per baseURL) Ollama
// server's GET /api/tags for its locally-pulled model names, for
// `comrade config models`'s picker (UYGULAMA_PLANI.md FAZ 8 item 4). A
// nil httpClient defaults to a plain &http.Client{}, matching every other
// connector construction path in this package. Unlike Client, calling
// this does not require an llm.Provider or an API key — Ollama has
// neither.
func ListOllamaModels(ctx context.Context, baseURL string, httpClient *http.Client) ([]string, error) {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	conn := newOllamaConnector("", baseURL, httpClient)
	return conn.ListModels(ctx)
}
