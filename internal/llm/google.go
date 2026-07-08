package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

const googleAPIBase = "https://generativelanguage.googleapis.com/v1beta/models"

// googleConnector talks to the Gemini generateContent API over raw
// net/http. It is unexported: external packages obtain one only
// indirectly, through Client (see New in client.go).
type googleConnector struct {
	apiKey     string
	model      string
	baseURL    string // defaults to googleAPIBase; overridden in tests
	httpClient *http.Client
}

func newGoogleConnector(apiKey, model string, httpClient *http.Client) *googleConnector {
	return &googleConnector{apiKey: apiKey, model: model, baseURL: googleAPIBase, httpClient: httpClient}
}

func (c *googleConnector) Name() string { return "google" }

type googlePart struct {
	Text string `json:"text"`
}

type googleContent struct {
	Role  string       `json:"role,omitempty"`
	Parts []googlePart `json:"parts"`
}

type googleGenerationConfig struct {
	MaxOutputTokens int `json:"maxOutputTokens,omitempty"`
}

type googleRequest struct {
	Contents          []googleContent         `json:"contents"`
	SystemInstruction *googleContent          `json:"systemInstruction,omitempty"`
	GenerationConfig  *googleGenerationConfig `json:"generationConfig,omitempty"`
}

type googleUsageMetadata struct {
	PromptTokenCount     int `json:"promptTokenCount"`
	CandidatesTokenCount int `json:"candidatesTokenCount"`
}

type googleCandidate struct {
	Content      googleContent `json:"content"`
	FinishReason string        `json:"finishReason"`
}

type googleResponse struct {
	Candidates    []googleCandidate   `json:"candidates"`
	UsageMetadata googleUsageMetadata `json:"usageMetadata"`
}

type googleErrorBody struct {
	Error struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
		Status  string `json:"status"`
	} `json:"error"`
}

// googleRole maps this package's Message.Role ("user"/"assistant") to
// Gemini's "user"/"model" role vocabulary. Anything else (including an
// empty string) defaults to "user".
func googleRole(role string) string {
	if role == "assistant" {
		return "model"
	}
	return "user"
}

func (c *googleConnector) buildRequest(req CompletionRequest) googleRequest {
	contents := make([]googleContent, len(req.Messages))
	for i, m := range req.Messages {
		contents[i] = googleContent{Role: googleRole(m.Role), Parts: []googlePart{{Text: m.Content}}}
	}

	body := googleRequest{Contents: contents}
	if req.System != "" {
		body.SystemInstruction = &googleContent{Parts: []googlePart{{Text: req.System}}}
	}
	if req.MaxTokens > 0 {
		body.GenerationConfig = &googleGenerationConfig{MaxOutputTokens: req.MaxTokens}
	}
	return body
}

// url builds the model-path-encoded endpoint for a completion. The model
// is path-encoded (never a body field, per Gemini's wire format) via
// url.PathEscape so a model name containing characters like "/" can never
// alter the request path.
func (c *googleConnector) url(streaming bool) string {
	action := "generateContent"
	suffix := ""
	if streaming {
		action = "streamGenerateContent"
		suffix = "?alt=sse"
	}
	return fmt.Sprintf("%s/%s:%s%s", c.baseURL, url.PathEscape(c.model), action, suffix)
}

func (c *googleConnector) doRequest(ctx context.Context, streaming bool, body googleRequest) (*http.Response, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("google: marshal request: %w", err)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url(streaming), bytes.NewReader(payload))
	if err != nil {
		return nil, fmt.Errorf("google: build request: %w", err)
	}
	httpReq.Header.Set("x-goog-api-key", c.apiKey)
	httpReq.Header.Set("content-type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("google: request: %w", err)
	}
	return resp, nil
}

func (c *googleConnector) Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error) {
	resp, err := c.doRequest(ctx, false, c.buildRequest(req))
	if err != nil {
		return CompletionResponse{}, err
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return CompletionResponse{}, fmt.Errorf("google: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return CompletionResponse{}, googleStatusError(resp.StatusCode, raw)
	}

	var parsed googleResponse
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return CompletionResponse{}, fmt.Errorf("google: decode response: %w", err)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return CompletionResponse{}, fmt.Errorf("google: response had no candidates")
	}

	return CompletionResponse{
		Text:       parsed.Candidates[0].Content.Parts[0].Text,
		Model:      c.model,
		StopReason: parsed.Candidates[0].FinishReason,
		Usage: Usage{
			InputTokens:  parsed.UsageMetadata.PromptTokenCount,
			OutputTokens: parsed.UsageMetadata.CandidatesTokenCount,
		},
	}, nil
}

func googleStatusError(status int, body []byte) error {
	var eb googleErrorBody
	msg := string(body)
	if err := json.Unmarshal(body, &eb); err == nil && eb.Error.Message != "" {
		msg = eb.Error.Message
	}

	var sentinel error
	switch {
	case status == http.StatusUnauthorized || status == http.StatusForbidden:
		sentinel = ErrAuthRejected
	case status == http.StatusTooManyRequests || status >= 500 || eb.Error.Status == "RESOURCE_EXHAUSTED":
		sentinel = ErrOverloaded
	}

	return &StatusError{Provider: "google", StatusCode: status, Message: msg, Sentinel: sentinel}
}

func (c *googleConnector) Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error) {
	resp, err := c.doRequest(ctx, true, c.buildRequest(req))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, googleStatusError(resp.StatusCode, raw)
	}

	ch := make(chan Chunk)
	go func() {
		defer close(ch)
		defer func() { _ = resp.Body.Close() }()

		var streamErr error
		scanErr := scanSSE(resp.Body, func(evt sseEvent) error {
			var typed googleResponse
			if err := json.Unmarshal([]byte(evt.Data), &typed); err != nil {
				return fmt.Errorf("google: decode stream event: %w", err)
			}
			if len(typed.Candidates) > 0 && len(typed.Candidates[0].Content.Parts) > 0 {
				if text := typed.Candidates[0].Content.Parts[0].Text; text != "" {
					ch <- Chunk{Text: text}
				}
			}
			return nil
		})
		if scanErr != nil {
			streamErr = fmt.Errorf("google: %w", scanErr)
		}
		ch <- Chunk{Done: true, Err: streamErr}
	}()
	return ch, nil
}
