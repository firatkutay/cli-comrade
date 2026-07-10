package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/spf13/cobra"

	"github.com/firatkutay/cli-comrade/internal/config"
	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/executor"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// chatSystemPromptFormat is the system prompt sent with every plain-text
// chat turn (never for "/do", which bypasses the LLM conversation
// entirely and goes straight to the plan+execute pipeline — see
// runChatDo). %s is the active language's name, purely for the model's
// own benefit; the language instruction itself is the one substantive
// sentence below.
const chatSystemPromptFormat = `You are cli-comrade's interactive chat assistant: a friendly, plain-language terminal companion. Answer the user's questions and requests conversationally. If the user wants an actual command executed, tell them to use "/do <request>" rather than attempting to run anything yourself — you have no execution capability in this conversation. Respond in %s. Respond with plain text only — no JSON, no markdown code fences.`

// chatLLM is the minimal LLM capability a plain-text chat turn needs — a
// package-local, consumer-side interface (mirrors engine.Completer
// exactly; redeclared here so this file never has to import
// internal/engine just for this one method set) so chatTurn is testable
// with a fake, no real provider involved.
type chatLLM interface {
	Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
}

// chatTurn sends one plain-text chat message through client, given the
// conversation history so far (NOT including text itself — chatTurn adds
// it to the request's Messages, but the caller is responsible for
// appending it to the session's own persisted history, exactly once,
// after a successful call — see chatRuntime.handle). It is a pure
// function of its arguments plus one Complete call: no session/bubbletea
// coupling at all, so it is fully unit-testable with a fake chatLLM.
//
// maxTokens is forwarded to llm.CompletionRequest.MaxTokens exactly like
// every other Complete call site in this package (runChatDo's planner,
// engine/explain.go, engine/diagnose.go — all read cfg.LLM.MaxTokens).
// Omitting it here was chat's own bug: the anthropicConnector request
// struct has no `omitempty` on max_tokens, so a zero value is sent to the
// wire as a literal 0, and the Anthropic Messages API rejects that with a
// 400 (max_tokens is a required field, 1-200000 — see
// docs/phases/FAZ-02.md and the Anthropic API reference) — every plain
// chat turn against Anthropic failed before this fix, unconditionally.
func chatTurn(ctx context.Context, client chatLLM, lang i18n.Lang, history []llm.Message, text string, maxTokens int) (string, error) {
	langName := "English"
	if lang == i18n.LangTR {
		langName = "Turkish"
	}

	messages := make([]llm.Message, 0, len(history)+1)
	messages = append(messages, history...)
	messages = append(messages, llm.Message{Role: "user", Content: text})

	resp, err := client.Complete(ctx, llm.CompletionRequest{
		System:    fmt.Sprintf(chatSystemPromptFormat, langName),
		Messages:  messages,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}

// runChatDo is "/do <request>"'s entire safety-gated pipeline: collect
// system context, generate a risk-labeled plan (engine.Planner — exactly
// like `comrade do`), and run it through engine.Execute under mode —
// UYGULAMA_PLANI.md FAZ 9's explicit design choice for chat's "do it"
// trigger: a plain `/do <request>` command that invokes the SAME
// plan→safety→mode-runner pipeline `comrade do` uses, rather than trying
// to detect intent in the model's own conversational replies (see
// docs/phases/FAZ-09.md's "chat /do routing" note for the full rationale
// — heuristic NL intent-detection on assistant prose is fragile and easy
// to spoof/miss; an explicit command is not). stdin/stdout/stderr are
// passed explicitly (not a *cobra.Command) so this function has no
// bubbletea/cobra coupling at all and is independently unit-testable
// (chat_test.go) exactly like runDo/runFix already are via do_test.go/
// fix_test.go — including proving a Blocked command in the generated
// plan is refused, never executed, regardless of mode.
func runChatDo(ctx context.Context, cfg config.Config, client engine.Completer, mode engine.Mode, request string, stdin io.Reader, stdout, stderr io.Writer, colorEnabled bool) (engine.RunSummary, error) {
	collector := contextpkg.NewCollector()
	sysCtx := collector.Collect(ctx, contextpkg.Options{
		SendHistory:  cfg.Context.SendHistory,
		HistoryDepth: cfg.Context.HistoryDepth,
		SendEnvNames: cfg.Context.SendEnvNames,
	})

	tr := newTranslator(cfg)
	planner := engine.NewPlanner(client, cfg)
	stopSpinner := startWaitSpinner(resolveColorEnabled(cfg, os.Environ(), stderr), stderr, tr)
	plan, err := planner.GeneratePlan(ctx, request, sysCtx)
	stopSpinner()
	if err != nil {
		return engine.RunSummary{}, fmt.Errorf("chat /do: %w", err)
	}

	deps := engine.RunDeps{
		Executor:           executor.New(stdout, stderr),
		Safety:             safety.NewEngine(cfg),
		LLM:                client,
		Prompt:             &tuiPromptUI{in: stdin, out: stdout, colorEnabled: colorEnabled, llm: client, tr: tr},
		Stdout:             stdout,
		Stderr:             stderr,
		ColorEnabled:       colorEnabled,
		ConfirmDestructive: cfg.Safety.ConfirmDestructive,
		ConfirmElevated:    cfg.Safety.ConfirmElevated,
		StepTimeout:        time.Duration(cfg.Executor.StepTimeoutSeconds) * time.Second,
		Request:            "chat /do: " + request,
		Translator:         tr,
	}

	summary, err := engine.Execute(ctx, plan, mode, deps)
	if err != nil {
		return engine.RunSummary{}, fmt.Errorf("chat /do: %w", err)
	}
	return summary, nil
}

// newChatCmd builds "comrade chat" (UYGULAMA_PLANI.md FAZ 9 item 4): a
// bubbletea interactive session preserving context in memory across
// turns. See docs/phases/FAZ-09.md for the full design (privacy: no
// autosave, only explicit "/save <file>"; "/do" routing decision).
func newChatCmd(newLoader loaderFactory) *cobra.Command {
	return &cobra.Command{
		Use:   "chat",
		Short: "Start an interactive, context-preserving chat session",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runChat(cmd, newLoader)
		},
	}
}

// runChat loads config/builds the LLM client (no --yolo flag here either,
// same as explain), resolves the session's initial mode via the exact
// same flag/env/config precedence `comrade do` uses (COMRADE_MODE, then
// config general.mode — chat itself defines no --auto/--ask/--info flags:
// "/mode" is the in-session way to change it), and runs the bubbletea
// program.
func runChat(cmd *cobra.Command, newLoader loaderFactory) error {
	cfg, tr, err := loadConfigWithNotice(cmd, newLoader)
	if err != nil {
		return fmt.Errorf("comrade chat: %w", err)
	}
	client, err := buildLLMClient(cmd, cfg)
	if err != nil {
		return fmt.Errorf("comrade chat: %w", err)
	}

	initialMode, err := engine.ResolveMode("", os.Getenv("COMRADE_MODE"), cfg.General.Mode)
	if err != nil {
		return fmt.Errorf("comrade chat: %w", err)
	}

	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
	defer stop()

	m := newChatModel(cfg, tr, client, newChatSession(initialMode))
	return runChatProgram(ctx, m, cmd.InOrStdin(), cmd.OutOrStdout())
}
