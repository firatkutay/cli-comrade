package cli

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// fakeUsageChatLLM is chatLLM's test double for the usage-display tests
// in this file: on every successful Complete call it invokes onUsage
// (when set) with a fixed llm.UsageEvent, standing in for what a real
// *llm.Client's WithUsageObserver callback does internally during a real
// Complete call — dispatchChatLine's own turnTally reset/read window
// (chatdispatch.go) doesn't care HOW a tally got updated during a
// dispatch, only that it happened inside that window, so this is a
// faithful enough stand-in without needing a real *llm.Client/httptest
// server here.
type fakeUsageChatLLM struct {
	reply   string
	err     error
	event   llm.UsageEvent
	onUsage func(llm.UsageEvent)
}

func (f *fakeUsageChatLLM) Complete(context.Context, llm.CompletionRequest) (llm.CompletionResponse, error) {
	if f.err != nil {
		return llm.CompletionResponse{}, f.err
	}
	if f.onUsage != nil {
		f.onUsage(f.event)
	}
	return llm.CompletionResponse{Text: f.reply}, nil
}

// newUsageAwareController builds a chatController wired exactly like
// runChat wires the real one (chat.go): one llm.WithUsageObserver-shaped
// callback feeding BOTH dc.sessionTally and dc.turnTally — callers read
// either tally straight off the returned *chatController (they're
// ordinary exported-within-package fields), so this only needs to return
// the one value.
func newUsageAwareController(t *testing.T, llmClient chatLLM, showUsage bool) *chatController {
	t.Helper()
	return &chatController{
		tr:           i18n.NewTranslator(i18n.LangEN),
		llm:          llmClient,
		save:         saveTranscript,
		maxTokens:    100,
		showUsage:    showUsage,
		sessionTally: newUsageTally(),
		turnTally:    newUsageTally(),
	}
}

func TestDispatchChatLineAppendsPerTurnUsageLineWhenShowUsageTrue(t *testing.T) {
	fake := &fakeUsageChatLLM{reply: "hi there", event: llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}}}
	dc := newUsageAwareController(t, fake, true)
	fake.onUsage = func(ev llm.UsageEvent) { dc.sessionTally.record(ev); dc.turnTally.record(ev) }

	session := newChatSession(engine.ModeAsk)
	output, exit := dc.dispatchChatLine(context.Background(), session, "hello")

	assert.False(t, exit)
	assert.Equal(t, "hi there\ntokens: 10 in / 5 out across 1 requests (anthropic/claude-haiku-4-5) · est. $0.0000", output)
}

func TestDispatchChatLineOmitsUsageLineWhenShowUsageFalse(t *testing.T) {
	fake := &fakeUsageChatLLM{reply: "hi there", event: llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}}}
	dc := newUsageAwareController(t, fake, false)
	fake.onUsage = func(ev llm.UsageEvent) { dc.sessionTally.record(ev); dc.turnTally.record(ev) }

	output, _ := dc.dispatchChatLine(context.Background(), newChatSession(engine.ModeAsk), "hello")

	assert.Equal(t, "hi there", output, "showUsage=false must never append a usage line")
}

func TestDispatchChatLineNonLLMCommandGetsNoUsageLine(t *testing.T) {
	dc := newUsageAwareController(t, &fakeUsageChatLLM{}, true)

	output, _ := dc.dispatchChatLine(context.Background(), newChatSession(engine.ModeAsk), "/mode auto")

	assert.Equal(t, "mode set to auto", output, "a slash command that never reaches the LLM must not gain a usage suffix")
}

func TestDispatchChatLineTurnTallyIsResetBetweenTurnsShowingOnlyThatTurn(t *testing.T) {
	fake := &fakeUsageChatLLM{reply: "ok", event: llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}}}
	dc := newUsageAwareController(t, fake, true)
	fake.onUsage = func(ev llm.UsageEvent) { dc.sessionTally.record(ev); dc.turnTally.record(ev) }

	session := newChatSession(engine.ModeAsk)
	first, _ := dc.dispatchChatLine(context.Background(), session, "one")
	second, _ := dc.dispatchChatLine(context.Background(), session, "two")

	assert.Equal(t, first, second, "each turn's own usage figures are identical here since every call reports the same fixed event — this pins that the SECOND turn's line is not cumulative (20 in/10 out) despite two turns having run")
	assert.Contains(t, first, "10 in / 5 out across 1 requests")
	assert.Contains(t, second, "10 in / 5 out across 1 requests")
}

func TestDispatchChatLineExitAppendsSessionTotalWhenShowUsageTrue(t *testing.T) {
	dc := newUsageAwareController(t, &fakeUsageChatLLM{}, true)
	sessionTally := dc.sessionTally
	sessionTally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 40, OutputTokens: 20}})
	sessionTally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 10, OutputTokens: 5}})

	output, exit := dc.dispatchChatLine(context.Background(), newChatSession(engine.ModeAsk), "/exit")

	assert.True(t, exit)
	assert.Equal(t, "goodbye.\nsession total — tokens: 50 in / 25 out across 2 requests (anthropic/claude-haiku-4-5) · est. $0.0002", output)
}

func TestDispatchChatLineExitOmitsSessionTotalWhenNothingWasRecorded(t *testing.T) {
	dc := newUsageAwareController(t, &fakeUsageChatLLM{}, true)

	output, exit := dc.dispatchChatLine(context.Background(), newChatSession(engine.ModeAsk), "/exit")

	assert.True(t, exit)
	assert.Equal(t, "goodbye.", output, "an empty session (no LLM calls made at all) must not print a session-total line")
}

func TestDispatchChatLineExitOmitsSessionTotalWhenShowUsageFalse(t *testing.T) {
	dc := newUsageAwareController(t, &fakeUsageChatLLM{}, false)
	sessionTally := dc.sessionTally
	sessionTally.record(llm.UsageEvent{Provider: "anthropic", Model: "claude-haiku-4-5", Usage: llm.Usage{InputTokens: 40, OutputTokens: 20}})

	output, exit := dc.dispatchChatLine(context.Background(), newChatSession(engine.ModeAsk), "/exit")

	assert.True(t, exit)
	assert.Equal(t, "goodbye.", output)
}
