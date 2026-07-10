package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

// fakeChatLLM is chatLLM's test double: it echoes back a fixed reply (or
// a fixed error) and records every CompletionRequest it saw, exactly like
// internal/engine's fakeCompleter. mu guards calls: since chatModel.Update
// now dispatches a turn via an async tea.Cmd (chatmodel.go's
// runChatTurnCmd) rather than calling it inline, Complete runs on
// bubbletea's own command goroutine while a headless-program test
// (chatmodel_test.go) may concurrently poll callCount() from the test
// goroutine — an unguarded slice append/read there is a genuine data race
// (caught by `go test -race`), not merely a hypothetical one.
type fakeChatLLM struct {
	reply string
	err   error

	mu    sync.Mutex
	calls []llm.CompletionRequest
}

func (f *fakeChatLLM) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	f.mu.Lock()
	f.calls = append(f.calls, req)
	f.mu.Unlock()
	if f.err != nil {
		return llm.CompletionResponse{}, f.err
	}
	return llm.CompletionResponse{Text: f.reply}, nil
}

// callCount returns len(f.calls) under f.mu — the race-safe way to poll
// call count from a goroutine other than whichever one is calling
// Complete (see f.mu's doc comment above). Every single-goroutine test in
// this file reads f.calls directly, which remains safe; only
// chatmodel_test.go's cross-goroutine polling needs this.
func (f *fakeChatLLM) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// testChatMaxTokens is newTestController's fixed chatController.maxTokens
// value — matching config.Default()'s llm.max_tokens (2048) purely so it
// reads as a realistic config value, not a magic number — used to prove
// (TestDispatchChatLinePlainTextRequestUsesConfiguredMaxTokens) that a
// plain-text turn's CompletionRequest actually carries it through.
const testChatMaxTokens = 2048

func newTestController(t *testing.T, llmClient chatLLM, doRun chatDoRunner) (*chatController, *chatSession) {
	t.Helper()
	tr := i18n.NewTranslator(i18n.LangEN)
	dc := &chatController{tr: tr, llm: llmClient, doRun: doRun, save: saveTranscript, maxTokens: testChatMaxTokens}
	session := newChatSession(engine.ModeAsk)
	return dc, session
}

func TestDispatchChatLinePlainTextAppendsBothTurnsOnSuccess(t *testing.T) {
	fake := &fakeChatLLM{reply: "it lists files"}
	dc, session := newTestController(t, fake, nil)

	output, exit := dc.dispatchChatLine(context.Background(), session, "what does ls do")

	assert.False(t, exit)
	assert.Equal(t, "it lists files", output)
	require.Len(t, session.history, 2)
	assert.Equal(t, "user", session.history[0].Role)
	assert.Equal(t, "what does ls do", session.history[0].Content)
	assert.Equal(t, "assistant", session.history[1].Role)
	assert.Equal(t, "it lists files", session.history[1].Content)
}

// TestDispatchChatLinePlainTextRequestUsesConfiguredMaxTokens is the
// regression pin for the "comrade chat gives no response at all against
// Anthropic" bug: chatTurn (chat.go) used to build every plain-text
// turn's llm.CompletionRequest without ever setting MaxTokens, so it went
// out at its Go zero value (0). Every other Complete call site in this
// codebase (engine.Planner/Explainer/Diagnoser) reads cfg.LLM.MaxTokens;
// chat's plain-text path alone never did — see chatTurn's doc comment.
// The Anthropic Messages API rejects max_tokens=0 with a 400 (it is a
// required field, range 1-200000: github.com/charmbracelet/
// anthropic-sdk-go's MessageNewParams reference, confirmed 2026-07),
// which anthropicConnector's JSON struct sends as a literal `"max_tokens":
// 0` because that field has no `omitempty` — so on the real provider,
// EVERY plain-text chat turn against Anthropic failed, unconditionally,
// before this fix. This test fails on the unfixed code (fake.calls[0].
// MaxTokens == 0) and passes once chatController.maxTokens is threaded
// through to chatTurn's request.
func TestDispatchChatLinePlainTextRequestUsesConfiguredMaxTokens(t *testing.T) {
	fake := &fakeChatLLM{reply: "ok"}
	dc, session := newTestController(t, fake, nil)

	_, exit := dc.dispatchChatLine(context.Background(), session, "hello")

	assert.False(t, exit)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, testChatMaxTokens, fake.calls[0].MaxTokens)
}

func TestDispatchChatLinePlainTextLeavesHistoryUntouchedOnLLMError(t *testing.T) {
	fake := &fakeChatLLM{err: errors.New("network down")}
	dc, session := newTestController(t, fake, nil)

	output, exit := dc.dispatchChatLine(context.Background(), session, "hello")

	assert.False(t, exit)
	assert.Contains(t, output, "network down")
	assert.Empty(t, session.history, "a failed turn must not leave a phantom half-turn in history")
}

// TestDispatchChatLinePlainTextNoKeyErrorIsRenderedFriendly is QA
// MAJOR-1's chat-side proof: a plain-text turn whose underlying LLM call
// fails with the real internal/llm no-key wrap-chain (exactly the shape
// runtime_test.go's realNoKeyChainError builds — a *llm.KeyMissingError
// wrapped by Client.Complete/finalChainError's own "provider: %w" /
// "llm: all providers failed: %w" layers) renders the SAME friendly
// classifyLLMError message every other LLM-reaching command now uses,
// wrapped in MsgChatLLMError's "chat request failed: %v" — never the
// raw wrap-chain, and history stays untouched exactly like any other
// failed turn.
func TestDispatchChatLinePlainTextNoKeyErrorIsRenderedFriendly(t *testing.T) {
	fake := &fakeChatLLM{err: realNoKeyChainError()}
	dc, session := newTestController(t, fake, nil)

	output, exit := dc.dispatchChatLine(context.Background(), session, "hello")

	assert.False(t, exit)
	assert.Equal(t, `chat request failed: no API key configured for anthropic yet — run "comrade auth login anthropic" to set one up (or export its env var directly; see "comrade auth login --help")`, output)
	assert.NotContains(t, output, "all providers failed")
	assert.Empty(t, session.history, "a failed turn must not leave a phantom half-turn in history")
}

func TestDispatchChatLineModeSwitchesSessionMode(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	output, exit := dc.dispatchChatLine(context.Background(), session, "/mode auto")
	assert.False(t, exit)
	assert.Contains(t, output, "auto")
	assert.Equal(t, engine.ModeAuto, session.mode)
}

func TestDispatchChatLineModeWithMissingArgPrintsUsageAndDoesNotChangeMode(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	output, exit := dc.dispatchChatLine(context.Background(), session, "/mode")
	assert.False(t, exit)
	assert.Contains(t, output, "usage")
	assert.Equal(t, engine.ModeAsk, session.mode)
}

func TestDispatchChatLineModeWithInvalidArgPrintsUsageAndDoesNotChangeMode(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	output, exit := dc.dispatchChatLine(context.Background(), session, "/mode bogus")
	assert.False(t, exit)
	assert.Contains(t, output, "usage")
	assert.Equal(t, engine.ModeAsk, session.mode)
}

func TestDispatchChatLineClearResetsHistory(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{reply: "hi"}, nil)
	dc.dispatchChatLine(context.Background(), session, "hello")
	require.NotEmpty(t, session.history)

	output, exit := dc.dispatchChatLine(context.Background(), session, "/clear")
	assert.False(t, exit)
	assert.NotEmpty(t, output)
	assert.Empty(t, session.history)
}

func TestDispatchChatLineExitReturnsTrue(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	for _, line := range []string{"/exit", "/quit"} {
		_, exit := dc.dispatchChatLine(context.Background(), session, line)
		assert.True(t, exit, "%q must end the session", line)
	}
}

func TestDispatchChatLineHelpListsSlashCommands(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	output, exit := dc.dispatchChatLine(context.Background(), session, "/help")
	assert.False(t, exit)
	assert.Contains(t, output, "/mode")
	assert.Contains(t, output, "/save")
	assert.Contains(t, output, "/do")
}

func TestDispatchChatLineUnknownCommandReportsIt(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	output, exit := dc.dispatchChatLine(context.Background(), session, "/frobnicate")
	assert.False(t, exit)
	assert.Contains(t, output, "/frobnicate")
}

// --- /save: the ONE way anything is ever written to disk -----------------

func TestDispatchChatLineSaveWritesTranscriptAndNothingElseWritesToDisk(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir) // an isolated HOME, per testing-standards' cold-start-style isolation
	target := filepath.Join(dir, "chat.txt")

	dc, session := newTestController(t, &fakeChatLLM{reply: "hi there"}, nil)
	dc.dispatchChatLine(context.Background(), session, "hello")

	entriesBefore, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Empty(t, entriesBefore, "no file must exist in the isolated HOME before /save is ever used")

	output, exit := dc.dispatchChatLine(context.Background(), session, "/save "+target)
	assert.False(t, exit)
	assert.Contains(t, output, target)

	data, err := os.ReadFile(target) //nolint:gosec // test-controlled temp path
	require.NoError(t, err)
	assert.Equal(t, renderTranscript(session.history), string(data))

	entriesAfter, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Len(t, entriesAfter, 1, "/save must be the only thing that ever wrote a file")
}

func TestDispatchChatLineSaveWithMissingArgPrintsUsageAndWritesNothing(t *testing.T) {
	dir := t.TempDir()
	dc, session := newTestController(t, &fakeChatLLM{}, nil)

	output, exit := dc.dispatchChatLine(context.Background(), session, "/save")
	assert.False(t, exit)
	assert.Contains(t, output, "usage")

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	assert.Empty(t, entries)
}

func TestDispatchChatLineSaveReportsWriteFailure(t *testing.T) {
	dc, session := newTestController(t, &fakeChatLLM{}, nil)
	// A path inside a directory that does not exist: os.WriteFile fails.
	badPath := filepath.Join(t.TempDir(), "no-such-dir", "chat.txt")

	output, exit := dc.dispatchChatLine(context.Background(), session, "/save "+badPath)
	assert.False(t, exit)
	assert.Contains(t, output, badPath)
}

// --- /do: routes to the safety-gated runner -------------------------------

func TestDispatchChatLineDoWithMissingArgPrintsUsageAndNeverCallsRunner(t *testing.T) {
	called := false
	doRun := func(context.Context, engine.Mode, string) (engine.RunSummary, error) {
		called = true
		return engine.RunSummary{}, nil
	}
	dc, session := newTestController(t, &fakeChatLLM{}, doRun)

	output, exit := dc.dispatchChatLine(context.Background(), session, "/do")
	assert.False(t, exit)
	assert.Contains(t, output, "usage")
	assert.False(t, called)
}

func TestDispatchChatLineDoPassesSessionModeAndRequestToRunner(t *testing.T) {
	var gotMode engine.Mode
	var gotRequest string
	doRun := func(_ context.Context, mode engine.Mode, request string) (engine.RunSummary, error) {
		gotMode = mode
		gotRequest = request
		return engine.RunSummary{Results: []engine.StepResult{{Outcome: engine.OutcomeExecuted}}}, nil
	}
	dc, session := newTestController(t, &fakeChatLLM{}, doRun)
	session.mode = engine.ModeAuto

	output, exit := dc.dispatchChatLine(context.Background(), session, "/do install docker")
	assert.False(t, exit)
	assert.Equal(t, engine.ModeAuto, gotMode)
	assert.Equal(t, "install docker", gotRequest)
	assert.Contains(t, output, "1 executed, 0 skipped, 0 blocked")
}

// TestDispatchChatLineDoBlockedCommandIsReportedAsBlockedNeverExecuted is
// the safety-gated-runner proof UYGULAMA_PLANI.md FAZ 9 calls for: a
// doRunner reporting exactly what runChatDo returns for a plan whose one
// step was Blocked (aborted, zero executed) must render that faithfully —
// "/do" itself never bypasses or second-guesses the runner's verdict.
func TestDispatchChatLineDoBlockedCommandIsReportedAsBlockedNeverExecuted(t *testing.T) {
	doRun := func(context.Context, engine.Mode, string) (engine.RunSummary, error) {
		return engine.RunSummary{
			Results:     []engine.StepResult{{Outcome: engine.OutcomeBlocked}},
			Aborted:     true,
			AbortReason: "step 1 is blocked: matches denylist rule: rm -rf / (or ~ / $HOME root delete)",
		}, nil
	}
	dc, session := newTestController(t, &fakeChatLLM{}, doRun)

	output, exit := dc.dispatchChatLine(context.Background(), session, "/do rm -rf /")
	assert.False(t, exit)
	assert.Contains(t, output, "0 executed, 0 skipped, 1 blocked")
	assert.Contains(t, output, "blocked")
}

func TestDispatchChatLineDoRunnerErrorIsReported(t *testing.T) {
	doRun := func(context.Context, engine.Mode, string) (engine.RunSummary, error) {
		return engine.RunSummary{}, errors.New("provider unreachable")
	}
	dc, session := newTestController(t, &fakeChatLLM{}, doRun)

	output, exit := dc.dispatchChatLine(context.Background(), session, "/do docker kur")
	assert.False(t, exit)
	assert.Contains(t, output, "provider unreachable")
}

// TestDispatchChatLineDoNoKeyErrorIsRenderedFriendly is
// TestDispatchChatLinePlainTextNoKeyErrorIsRenderedFriendly's "/do"
// counterpart: handleDo funnels its runner's error through the same
// renderChatLLMError as handleText, so a no-key failure reached via
// "/do" gets the identical friendly message, appended to history as the
// assistant's turn (handleDo always appends a reply, success or not).
func TestDispatchChatLineDoNoKeyErrorIsRenderedFriendly(t *testing.T) {
	doRun := func(context.Context, engine.Mode, string) (engine.RunSummary, error) {
		return engine.RunSummary{}, realNoKeyChainError()
	}
	dc, session := newTestController(t, &fakeChatLLM{}, doRun)

	output, exit := dc.dispatchChatLine(context.Background(), session, "/do docker kur")

	assert.False(t, exit)
	assert.Equal(t, `chat request failed: no API key configured for anthropic yet — run "comrade auth login anthropic" to set one up (or export its env var directly; see "comrade auth login --help")`, output)
	assert.NotContains(t, output, "all providers failed")
}

// --- runChatDo: the real safety-gated pipeline, driven directly ----------

// TestRunChatDoBlocksdenylistedStepAndNeverExecutesIt is runChatDo's own
// integration-level proof (mirrors do_test.go's identical
// TestDoAutoModeRunsBenignStepAndBlocksDenylistedStepAgainstRealExecutor):
// a plan whose second step is a denylisted `rm -rf /` must be Blocked by
// the real safety.Engine and never reach the real executor, regardless of
// what the mock LLM labeled its risk.
func TestRunChatDoBlocksDenylistedStepAndNeverExecutesIt(t *testing.T) {
	fake := &fakeChatCompleter{
		text: `{"summary": "test", "steps": [
			{"command": "echo chat-do-marker", "rationale": "benign", "risk": "read", "reversible": true},
			{"command": "rm -rf /", "rationale": "decoy", "risk": "read", "reversible": false}
		]}`,
	}
	cfg := config.Default()

	var stdout, stderr chatBuffer
	summary, err := runChatDo(context.Background(), cfg, fake, engine.ModeAuto, "print a marker then a decoy", nil, &stdout, &stderr, false)

	// runChatDo mirrors runDo's own contract (do.go): engine.Execute only
	// returns a Go error for a hard failure (e.g. an unknown mode); a
	// Blocked step is reported via RunSummary.Aborted/AbortReason, which
	// the chat dispatch layer (chatdispatch.go's formatRunSummaryLine)
	// renders — exactly the same split do.go's runDo itself relies on.
	require.NoError(t, err)
	assert.True(t, summary.Aborted, "the run must abort: the plan's second step is Blocked")
	assert.Contains(t, summary.AbortReason, "blocked")
	require.Len(t, summary.Results, 2)
	assert.Equal(t, engine.OutcomeExecuted, summary.Results[0].Outcome)
	assert.Equal(t, engine.OutcomeBlocked, summary.Results[1].Outcome)
	assert.Contains(t, stdout.String(), "chat-do-marker", "the real executor must have actually run the benign step")
}

// fakeChatCompleter is engine.Completer's test double for runChatDo's own
// test: a single fixed plan-shaped JSON response for every call.
type fakeChatCompleter struct{ text string }

func (f *fakeChatCompleter) Complete(_ context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error) {
	doc, err := llm.ValidateInto(f.text, req.RequiredFields, nil)
	if err != nil {
		return llm.CompletionResponse{}, err
	}
	return llm.CompletionResponse{Text: f.text, JSON: doc}, nil
}

// chatBuffer is a minimal, allocation-light io.Writer test double (avoids
// pulling in bytes.Buffer just for String()).
type chatBuffer struct{ data []byte }

func (b *chatBuffer) Write(p []byte) (int, error) {
	b.data = append(b.data, p...)
	return len(p), nil
}
func (b *chatBuffer) String() string { return string(b.data) }
