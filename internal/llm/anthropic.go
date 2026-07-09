package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// anthropicAPIVersion is the fixed Messages API version header value
// (verified 2026-07 — see docs/phases/FAZ-02.md).
const anthropicAPIVersion = "2023-06-01"

const anthropicMessagesURL = "https://api.anthropic.com/v1/messages"

// anthropicConnector talks to the Anthropic Messages API over raw
// net/http. It is unexported: external packages obtain one only indirectly,
// through Client (see New in client.go).
type anthropicConnector struct {
	apiKey     string
	model      string
	baseURL    string
	httpClient *http.Client
}

func newAnthropicConnector(apiKey, model string, httpClient *http.Client) *anthropicConnector {
	return &anthropicConnector{apiKey: apiKey, model: model, baseURL: anthropicMessagesURL, httpClient: httpClient}
}

func (c *anthropicConnector) Name() string { return "anthropic" }

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
	Stream    bool               `json:"stream,omitempty"`
}

type anthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type anthropicContentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type anthropicResponse struct {
	Content    []anthropicContentBlock `json:"content"`
	Model      string                  `json:"model"`
	StopReason string                  `json:"stop_reason"`
	Usage      anthropicUsage          `json:"usage"`
}

type anthropicErrorBody struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *anthropicConnector) buildRequest(req CompletionRequest, stream bool) anthropicRequest {
	msgs := make([]anthropicMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = anthropicMessage(m)
	}
	return anthropicRequest{
		Model:     c.model,
		MaxTokens: req.MaxTokens,
		System:    req.System,
		Messages:  msgs,
		Stream:    stream,
	}
}

func (c *anthropicConnector) doRequest(ctx context.Context, body anthropicRequest) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("anthropic: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL, bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("anthropic: build request: %w", err)
	}
	httpReq.Header.Set("x-api-key", c.apiKey)
	httpReq.Header.Set("anthropic-version", anthropicAPIVersion)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, wrapReachabilityError("anthropic", c.baseURL, fmt.Errorf("anthropic: request: %w", err))
	}
	return resp, nil
}

func (c *anthropicConnector) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	resp, err := c.doRequest(ctx, c.buildRequest(req, false))
	if err != nil {
		return CompletionResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return CompletionResponse{}, anthropicStatusError(resp.StatusCode, body)
	}

	var parsed anthropicResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return CompletionResponse{}, fmt.Errorf("anthropic: decode response: %w", err)
	}

	return CompletionResponse{
		Text:       anthropicText(parsed.Content),
		Model:      parsed.Model,
		StopReason: parsed.StopReason,
		Usage: Usage{
			InputTokens:  parsed.Usage.InputTokens,
			OutputTokens: parsed.Usage.OutputTokens,
		},
	}, nil
}

func anthropicText(blocks []anthropicContentBlock) string {
	for _, b := range blocks {
		if b.Type == "text" {
			return b.Text
		}
	}
	return ""
}

// anthropicStatusError builds a *StatusError for a non-200 Messages API
// response, classifying 401/403 as ErrAuthRejected and 429/5xx/529
// (overloaded_error) as ErrOverloaded per UYGULAMA_PLANI.md FAZ 2's
// verified API facts.
func anthropicStatusError(status int, body []byte) error {
	var eb anthropicErrorBody
	msg := string(body)
	if err := json.Unmarshal(body, &eb); err == nil && eb.Error.Message != "" {
		msg = eb.Error.Message
	}

	var sentinel error
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		sentinel = ErrAuthRejected
	case status == http.StatusTooManyRequests || status == 529 || status >= 500:
		sentinel = ErrOverloaded
	}

	return &StatusError{Provider: "anthropic", StatusCode: status, Message: msg, Sentinel: sentinel}
}

// anthropicStreamEvent is the union of the SSE event fields this
// connector reads: content_block_delta's text delta, and a mid-stream
// error event's message. Every other event type (message_start,
// content_block_start/stop, message_delta, message_stop, ping) is decoded
// into the same struct and ignored via the Type switch in Stream.
type anthropicStreamEvent struct {
	Type  string `json:"type"`
	Delta struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"delta"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

func (c *anthropicConnector) Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error) {
	resp, err := c.doRequest(ctx, c.buildRequest(req, true)) //nolint:bodyclose // closed on both paths below: immediately on a non-200 status, or by the streaming goroutine's own defer once it finishes reading — bodyclose's static path check doesn't see the latter as a guaranteed close.
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, anthropicStatusError(resp.StatusCode, body)
	}

	ch := make(chan Chunk)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		var streamErr error
		scanErr := scanSSE(resp.Body, func(evt sseEvent) error {
			var typed anthropicStreamEvent
			if err := json.Unmarshal([]byte(evt.Data), &typed); err != nil {
				return fmt.Errorf("anthropic: decode stream event: %w", err)
			}
			switch typed.Type {
			case "content_block_delta":
				if typed.Delta.Type == "text_delta" && typed.Delta.Text != "" {
					ch <- Chunk{Text: typed.Delta.Text}
				}
			case "error":
				return fmt.Errorf("anthropic: stream error: %s", typed.Error.Message)
			}
			return nil
		})
		if scanErr != nil {
			streamErr = fmt.Errorf("anthropic: %w", scanErr)
		}
		ch <- Chunk{Done: true, Err: streamErr}
	}()
	return ch, nil
}
