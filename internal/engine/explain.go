package engine

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
)

//go:embed prompts/explain_system.txt
var explainSystemPromptEN string

//go:embed prompts/explain_lang_tr.txt
var explainLangTR string

// explainUserMessageFormat is the fixed user-turn content sent with every
// explain request: just the command text itself, exactly like
// diagnoseUserMessage keeps the user turn minimal and puts every actual
// instruction in the system prompt (see buildExplainSystemPrompt).
const explainUserMessageFormat = "Explain this command: %s"

// ExplanationPart is one token-level breakdown entry in an Explanation —
// a single flag, subcommand, or argument and what it does.
type ExplanationPart struct {
	Token   string
	Meaning string
}

// Explanation is Explainer.Explain's result: a plain-language summary, a
// token-by-token breakdown, and the LLM's own risk note. `comrade
// explain` (internal/cli) renders this ALONGSIDE its own, independent
// local safety.Engine verdict — the safety verdict is authoritative for
// whether to warn the user at all (see docs/phases/FAZ-09.md); RiskNote
// here is the LLM's secondary, descriptive color commentary, never the
// only source of a destructive/blocked warning.
type Explanation struct {
	Summary  string
	Parts    []ExplanationPart
	RiskNote string
}

// rawExplanationPart/rawExplanation mirror the exact JSON shape
// prompts/explain_system.txt instructs the model to respond with.
type rawExplanationPart struct {
	Token   string `json:"token"`
	Meaning string `json:"meaning"`
}

type rawExplanation struct {
	Summary  string               `json:"summary"`
	Parts    []rawExplanationPart `json:"parts"`
	RiskNote string               `json:"risk_note"`
}

// Explainer turns a single command string into an Explanation via the
// LLM, in the user's resolved language — never executing the command
// itself, and holding no global state (Completer/config injected, exactly
// like Planner/Diagnoser).
type Explainer struct {
	llm          Completer
	cfg          config.Config
	getenv       func(string) string
	systemLocale func() string
}

// NewExplainer builds an Explainer around client (typically an
// *llm.Client from llm.New(cfg), but any Completer works) and cfg.
func NewExplainer(client Completer, cfg config.Config) *Explainer {
	return &Explainer{llm: client, cfg: cfg, getenv: os.Getenv, systemLocale: i18n.SystemLocale}
}

// Explain sends one explain request for command and decodes/validates the
// model's {summary, parts, risk_note} response. RequiredFields enforces
// "summary" is present and non-empty; "parts" may legitimately be a short
// or even single-entry list for a bare, single-token command, so it is
// not required the way GeneratePlan requires "steps" to be non-empty.
//
// Explain never touches internal/executor or internal/safety — command
// classification is internal/cli's job, using the same safety.Engine
// every other command uses, kept entirely separate from this LLM call so
// a slow/failed/hallucinated LLM response can never affect whether the
// safety warning is shown (see docs/phases/FAZ-09.md's two-layer design).
func (e *Explainer) Explain(ctx context.Context, command string) (Explanation, error) {
	lang := i18n.ResolveLanguage(e.cfg.General.Language, e.getenv, e.systemLocale).String()
	systemPrompt := buildExplainSystemPrompt(lang)

	resp, err := e.llm.Complete(ctx, llm.CompletionRequest{
		System:         systemPrompt,
		Messages:       []llm.Message{{Role: "user", Content: fmt.Sprintf(explainUserMessageFormat, command)}},
		MaxTokens:      e.cfg.LLM.MaxTokens,
		RequiredFields: []string{"summary"},
	})
	if err != nil {
		return Explanation{}, fmt.Errorf("engine: explain: %w", err)
	}

	var raw rawExplanation
	if err := json.Unmarshal(resp.JSON, &raw); err != nil {
		return Explanation{}, fmt.Errorf("engine: decode explanation response: %w", err)
	}

	parts := make([]ExplanationPart, 0, len(raw.Parts))
	for _, p := range raw.Parts {
		if strings.TrimSpace(p.Token) == "" && strings.TrimSpace(p.Meaning) == "" {
			continue
		}
		parts = append(parts, ExplanationPart(p))
	}

	return Explanation{
		Summary:  raw.Summary,
		Parts:    parts,
		RiskNote: raw.RiskNote,
	}, nil
}

// buildExplainSystemPrompt assembles the full system prompt sent with
// every explain request: the English core instruction (JSON schema,
// token-breakdown/plain-language quality bar, execution-never rule), and
// the Turkish language instruction block appended only when lang == "tr"
// — exactly like buildSystemPrompt/buildDiagnoseSystemPrompt. Explain has
// no system/error context to append (unlike plan/diagnose generation):
// the command string itself, sent as the user message, is the entire
// input this prompt needs.
func buildExplainSystemPrompt(lang string) string {
	var b strings.Builder
	b.WriteString(explainSystemPromptEN)
	if lang == "tr" {
		b.WriteString("\n\n")
		b.WriteString(explainLangTR)
	}
	return b.String()
}
