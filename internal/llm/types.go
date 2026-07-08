package llm

import (
	"context"
	"encoding/json"
)

// Message is one turn of conversation history sent to a provider. Role is
// "user" or "assistant"; connectors translate it to whatever their wire
// format expects (e.g. Google's "model" instead of "assistant").
type Message struct {
	Role    string
	Content string
}

// CompletionRequest is the provider-agnostic request every connector
// receives. System carries the system prompt — including, under this
// project's JSON strategy (system-prompt instruction + parse.go
// extraction, not native structured-output params; see
// docs/phases/FAZ-02.md), any "respond with a single JSON object"
// instruction the caller wants honored.
//
// RequiredFields names the top-level JSON keys ValidateInto must find
// present and non-empty in the model's response text. Leave it nil/empty
// to skip JSON extraction/validation entirely and just use Text as plain
// output.
type CompletionRequest struct {
	System         string
	Messages       []Message
	MaxTokens      int
	RequiredFields []string
}

// Usage reports token accounting for one completion, in each provider's
// own vocabulary normalized to input/output.
type Usage struct {
	InputTokens  int
	OutputTokens int
}

// CompletionResponse is the provider-agnostic result of a completion.
// JSON is populated only when the request set RequiredFields and
// extraction/validation succeeded; it holds the single top-level JSON
// object ExtractJSON found in Text.
type CompletionResponse struct {
	Text       string
	Model      string
	Usage      Usage
	StopReason string
	JSON       json.RawMessage
}

// Chunk is one piece of a streamed completion. Every connector in this
// package follows the same contract: zero or more chunks with Done=false
// and a non-empty Text delta, followed by exactly one final chunk with
// Done=true, after which the channel is closed. Err is set on that final
// chunk when the stream ended abnormally (a mid-stream provider error
// event, a transport failure, a decode failure) rather than by reaching
// the model's natural stop — a nil Err on the final chunk means the
// stream completed successfully. There is no separate error channel:
// this single "final chunk carries the verdict" shape is the one all
// four connectors and their callers rely on.
type Chunk struct {
	Text string
	Done bool
	Err  error
}

// Provider is the interface every connector implements, verbatim from
// CLAUDE.md's "LLM Provider Mimarisi". Connector constructors
// (newAnthropicConnector, newOpenAICompatConnector, newGoogleConnector,
// newOllamaConnector) are unexported — the only way to obtain a Provider
// from outside this package is Client, built via New(cfg).
type Provider interface {
	Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
	Stream(ctx context.Context, req CompletionRequest) (<-chan Chunk, error)
	Name() string
}
