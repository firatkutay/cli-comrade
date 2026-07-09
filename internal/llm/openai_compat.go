package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// openAICompatConnector talks to any OpenAI Chat Completions-compatible
// endpoint over raw net/http: OpenAI, Mistral, Groq, GLM/Zhipu, Qwen,
// Kimi/Moonshot, OpenRouter, LM Studio all share this one connector,
// distinguished only by baseURL and apiKey. It is unexported: external
// packages obtain one only indirectly, through Client (see New in
// client.go).
type openAICompatConnector struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func newOpenAICompatConnector(apiKey, model, baseURL string, httpClient *http.Client) *openAICompatConnector {
	return &openAICompatConnector{
		apiKey:     apiKey,
		model:      model,
		baseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: httpClient,
	}
}

func (c *openAICompatConnector) Name() string { return "openai_compat" }

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIRequest struct {
	Model     string          `json:"model"`
	Messages  []openAIMessage `json:"messages"`
	MaxTokens int             `json:"max_tokens,omitempty"`
	Stream    bool            `json:"stream,omitempty"`
}

type openAIChoice struct {
	Message      openAIMessage `json:"message"`
	FinishReason string        `json:"finish_reason"`
}

type openAIUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type openAIResponse struct {
	Model   string         `json:"model"`
	Choices []openAIChoice `json:"choices"`
	Usage   openAIUsage    `json:"usage"`
}

type openAIErrorBody struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

// messages prepends req.System as a "system"-role message, matching the
// OpenAI-compatible convention (there is no separate top-level system
// field in this wire format).
func (c *openAICompatConnector) messages(req CompletionRequest) []openAIMessage {
	msgs := make([]openAIMessage, 0, len(req.Messages)+1)
	if req.System != "" {
		msgs = append(msgs, openAIMessage{Role: "system", Content: req.System})
	}
	for _, m := range req.Messages {
		msgs = append(msgs, openAIMessage(m))
	}
	return msgs
}

func (c *openAICompatConnector) doRequest(ctx context.Context, body openAIRequest) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("openai_compat: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/chat/completions", bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("openai_compat: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapReachabilityError("openai_compat", c.baseURL, fmt.Errorf("openai_compat: request: %w", err))
	}
	return resp, nil
}

func (c *openAICompatConnector) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	resp, err := c.doRequest(ctx, openAIRequest{Model: c.model, Messages: c.messages(req), MaxTokens: req.MaxTokens})
	if err != nil {
		return CompletionResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("openai_compat: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return CompletionResponse{}, openAIStatusError(resp.StatusCode, raw)
	}

	var parsed openAIResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CompletionResponse{}, fmt.Errorf("openai_compat: decode response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return CompletionResponse{}, fmt.Errorf("openai_compat: response had no choices")
	}

	return CompletionResponse{
		Text:       parsed.Choices[0].Message.Content,
		Model:      parsed.Model,
		StopReason: parsed.Choices[0].FinishReason,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
	}, nil
}

func openAIStatusError(status int, body []byte) error {
	var eb openAIErrorBody
	msg := string(body)
	if err := json.Unmarshal(body, &eb); err == nil && eb.Error.Message != "" {
		msg = eb.Error.Message
	}

	var sentinel error
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		sentinel = ErrAuthRejected
	case status == http.StatusTooManyRequests || status >= 500:
		sentinel = ErrOverloaded
	}

	return &StatusError{Provider: "openai_compat", StatusCode: status, Message: msg, Sentinel: sentinel}
}

// openAIModelsResponse mirrors GET /models's response shape (only the
// field ListModels needs) — this is the OpenAI-compatible "list models"
// convention every connector target on this connector (OpenAI, Mistral,
// Groq, GLM/Zhipu, Qwen, Kimi/Moonshot, OpenRouter, LM Studio) is expected
// to share, matching /chat/completions itself.
type openAIModelsResponse struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
}

// ListModels queries GET {baseURL}/models for the model ids this
// endpoint serves, for `comrade config models`'s picker
// (UYGULAMA_PLANI.md FAZ 8 item 4). Parsing is deliberately lenient: only
// each entry's "id" field is read, so an endpoint whose /models response
// carries extra provider-specific fields this package doesn't know about
// still yields a usable id list instead of a decode error.
func (c *openAICompatConnector) ListModels(ctx context.Context) ([]string, error) {
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("openai_compat: build request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapReachabilityError("openai_compat", c.baseURL, fmt.Errorf("openai_compat: request: %w", err))
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("openai_compat: read response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, openAIStatusError(resp.StatusCode, raw)
	}

	var parsed openAIModelsResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("openai_compat: decode models response: %w", err)
	}

	names := make([]string, 0, len(parsed.Data))
	for _, m := range parsed.Data {
		if m.ID != "" {
			names = append(names, m.ID)
		}
	}
	return names, nil
}

// ListOpenAICompatModels queries baseURL's GET /models for the model ids
// it serves, authenticating with apiKey. A nil httpClient defaults to a
// plain &http.Client{}, matching every other connector construction path
// in this package.
func ListOpenAICompatModels(ctx context.Context, baseURL, apiKey string, httpClient *http.Client) ([]string, error) {
	if httpClient == nil {
		httpClient = &http.Client{}
	}
	conn := newOpenAICompatConnector(apiKey, "", baseURL, httpClient)
	return conn.ListModels(ctx)
}

func (c *openAICompatConnector) Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error) {
	body := openAIRequest{Model: c.model, Messages: c.messages(req), MaxTokens: req.MaxTokens, Stream: true}
	resp, err := c.doRequest(ctx, body) //nolint:bodyclose // closed on both paths below: immediately on a non-200 status, or by the streaming goroutine's own defer once it finishes reading — bodyclose's static path check doesn't see the latter as a guaranteed close.
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, openAIStatusError(resp.StatusCode, raw)
	}

	ch := make(chan Chunk)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		var streamErr error
		scanErr := scanSSE(resp.Body, func(evt sseEvent) error {
			if evt.Data == "[DONE]" {
				return errSSEDone
			}
			var typed struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(evt.Data), &typed); err != nil {
				return fmt.Errorf("openai_compat: decode stream event: %w", err)
			}
			if len(typed.Choices) > 0 && typed.Choices[0].Delta.Content != "" {
				ch <- Chunk{Text: typed.Choices[0].Delta.Content}
			}
			return nil
		})
		if scanErr != nil {
			streamErr = fmt.Errorf("openai_compat: %w", scanErr)
		}
		ch <- Chunk{Done: true, Err: streamErr}
	}()
	return ch, nil
}
