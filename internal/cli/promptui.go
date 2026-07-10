package cli

import (
	"context"
	"fmt"
	"io"

	"github.com/firatkutay/cli-comrade/internal/engine"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/tui"
)

// tuiPromptUI is the one place a concrete UI toolkit (bubbletea, via
// internal/tui) and a concrete LLM client are wired into
// internal/engine.Runner's decoupled PromptUI interface — see
// internal/engine/runner.go's PromptUI doc comment and
// docs/history/phases/FAZ-06.md's layering note.
type tuiPromptUI struct {
	in           io.Reader
	out          io.Writer
	colorEnabled bool
	llm          engine.Completer
	// tr resolves the confirm prompt's rendered language, per the same
	// general.language/COMRADE_LANG/LANG/LC_ALL chain every command's
	// output uses (see internal/cli's newTranslator) — every construction
	// site in this package sets it from that same call, never a separate
	// resolution.
	tr i18n.Translator
}

// Confirm implements engine.PromptUI by rendering step through
// internal/tui.Confirm and translating its tui.PromptChoice result back
// to this package's engine.Choice.
func (p *tuiPromptUI) Confirm(ctx context.Context, step engine.Step) (engine.Choice, string, error) {
	ps := tui.PromptStep{
		Command:   step.Command,
		Rationale: step.Rationale,
		Risk:      step.Decision.EffectiveRisk,
	}
	choice, edited, err := tui.Confirm(ctx, ps, p.colorEnabled, p.in, p.out, p.tr)
	if err != nil {
		return engine.ChoiceNo, "", err
	}
	return convertChoice(choice), edited, nil
}

// convertChoice maps a tui.PromptChoice to this package's engine.Choice.
// Any value tui.Confirm cannot itself have produced defaults to
// engine.ChoiceNo — the fail-closed choice.
func convertChoice(c tui.PromptChoice) engine.Choice {
	switch c {
	case tui.Yes:
		return engine.ChoiceYes
	case tui.Edit:
		return engine.ChoiceEdit
	case tui.Explain:
		return engine.ChoiceExplain
	case tui.All:
		return engine.ChoiceAll
	default:
		return engine.ChoiceNo
	}
}

// explainSystemPrompt is the system prompt sent for ask mode's [a]çıkla
// option: a single, plain-language, non-technical explanation of exactly
// what one command does, including any risk. FAZ 9's i18n catalog will
// route this through the resolved user language; today it is always
// English, matching every other hardcoded string in this phase (per
// CLAUDE.md's "funnel them so migration is mechanical" note).
const explainSystemPrompt = `You explain a single shell command to a terminal beginner, in plain, non-technical language. Given the command, its one-line rationale, and its risk class, write a short, clear explanation of exactly what running it will do. If its risk class is "elevated" or "destructive", explicitly call out what could go wrong. Respond with plain text only — no JSON, no markdown code fences.`

// Explain implements engine.PromptUI's [a]çıkla option by asking the LLM
// for a detailed, plain-language explanation of step.
func (p *tuiPromptUI) Explain(ctx context.Context, step engine.Step) (string, error) {
	user := fmt.Sprintf("Command: %s\nRationale: %s\nRisk: %s\n", step.Command, step.Rationale, step.Decision.EffectiveRisk.String())
	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		System:    explainSystemPrompt,
		Messages:  []llm.Message{{Role: "user", Content: user}},
		MaxTokens: 512,
	})
	if err != nil {
		return "", err
	}
	return resp.Text, nil
}
