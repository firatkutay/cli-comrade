package engine

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/audit"
	"github.com/firatkutay/cli-comrade/internal/config"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

//go:embed prompts/undo_system.txt
var undoSystemPromptEN string

// undoLangTR reuses plan generation's own Turkish language-instruction
// block verbatim: DeriveUndo's response is exactly plan generation's
// {summary, steps:[{command, rationale, risk, reversible}]} JSON shape
// (see rawPlan/toPlan, planner.go), so the instruction for which fields'
// TEXT CONTENT must be Turkish — vs. which field names/values stay
// English — is identical wording to plan_lang_tr.txt; embedding it a
// second time here (rather than duplicating the same Turkish sentence in
// a second file) keeps the two prompts from ever drifting apart on this
// shared instruction.
//
//go:embed prompts/plan_lang_tr.txt
var undoLangTR string

// undoUserMessage is the fixed user-turn content sent with every undo
// request — exactly like diagnoseUserMessage/plan generation's own
// request message, everything the model actually needs (instructions,
// the run's recorded steps) lives in the system prompt
// buildUndoSystemPrompt assembles.
const undoUserMessage = "Derive the undo plan for the run described in the system context above and produce the JSON response."

// UndoTarget bundles everything DeriveUndo needs to know about the run
// being undone: its RunID (for the model's own reference only — never
// executed), the free-text Request that originally produced it, the
// working directory it ran in, and the run's own recorded Steps — every
// audit.Entry internal/cli's target-selection logic decided is actually
// eligible for reversal (already filtered to exclude a nonzero-exit step
// — see that package's own doc comment on why DeriveUndo trusts its
// caller for that, rather than re-filtering here).
type UndoTarget struct {
	RunID   string
	Request string
	Cwd     string
	Steps   []audit.Entry
}

// Undoer turns an UndoTarget into a risk-labeled Plan reversing it, or a
// Plan with an empty Steps slice and an explanatory Summary when nothing
// can be honestly reversed — mirroring Diagnoser exactly (see that
// type's own doc comment): the same Completer/config-injected
// construction, the same safety.Engine built once at construction, and
// the same reuse of rawPlan/toPlan (planner.go) for decoding, since
// DeriveUndo's requested JSON shape is byte-for-byte plan generation's
// own {summary, steps:[...]} shape.
type Undoer struct {
	llm          Completer
	cfg          config.Config
	getenv       func(string) string
	systemLocale func() string
	safetyEngine *safety.Engine
}

// NewUndoer builds an Undoer around client (typically the SAME
// *llm.Client instance a do/fix/undo invocation already built via
// buildLLMClient — see internal/cli's runUndo — so DeriveUndo's request
// flows through that Client's own redaction pipeline exactly like every
// other engine.Completer call in this codebase; internal/llm.Client
// redacts every outgoing System/Messages payload unconditionally,
// regardless of which caller built the request) and cfg.
func NewUndoer(client Completer, cfg config.Config) *Undoer {
	return &Undoer{
		llm:          client,
		cfg:          cfg,
		getenv:       os.Getenv,
		systemLocale: i18n.SystemLocale,
		safetyEngine: safety.NewEngine(cfg),
	}
}

// DeriveUndo sends one undo request built from target and decodes/
// validates the model's {summary, steps} response, reusing toPlan
// (planner.go) verbatim — the same fail-closed risk parsing (an
// unrecognized/missing "risk" label defaults to RiskDestructive) applies
// here exactly as it does to a freshly generated plan. Only "summary" is
// required (RequiredFields): an empty "steps" array is a legitimate,
// honest "nothing could be reversed" response (see prompts/
// undo_system.txt's own explicit escape hatch), not a malformed one — so,
// like Diagnose (and unlike GeneratePlan), this never runs a corrective
// empty-steps re-prompt.
//
// Every resulting step is run through the Undoer's safety.Engine and
// annotated with its Decision, exactly like GeneratePlan/Diagnose —
// DeriveUndo itself never executes anything; the returned Plan is handed
// to engine.Execute by internal/cli's `comrade undo`, always under
// ModeAsk (see that command's own doc comment on why there is no
// --yolo path for it at all).
func (u *Undoer) DeriveUndo(ctx context.Context, target UndoTarget) (Plan, error) {
	lang := i18n.ResolveLanguage(u.cfg.General.Language, u.getenv, u.systemLocale).String()
	systemPrompt := buildUndoSystemPrompt(lang, target)

	resp, err := u.llm.Complete(ctx, llm.CompletionRequest{
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: undoUserMessage},
		},
		MaxTokens:      u.cfg.LLM.MaxTokens,
		RequiredFields: []string{"summary"},
	})
	if err != nil {
		return Plan{}, fmt.Errorf("engine: derive undo: %w", err)
	}

	var raw rawPlan
	if err := json.Unmarshal(resp.JSON, &raw); err != nil {
		return Plan{}, fmt.Errorf("engine: decode undo response: %w", err)
	}

	plan, unknownRisk, err := toPlan(raw)
	if err != nil {
		return Plan{}, fmt.Errorf("engine: derive undo: %w", err)
	}
	if unknownRisk {
		plan.Summary = strings.TrimSpace(plan.Summary + " " + unknownRiskNote(lang))
	}

	for i := range plan.Steps {
		plan.Steps[i].Decision = u.safetyEngine.Evaluate(plan.Steps[i].Command, plan.Steps[i].Risk)
	}

	return plan, nil
}

// buildUndoSystemPrompt assembles the full system prompt sent with every
// undo request: the English core instruction (JSON schema, reverse-order
// rule, the "never guess" escape hatch), the Turkish language
// instruction block appended only when lang == "tr" (reused verbatim
// from plan generation — see undoLangTR's own doc comment), and finally
// target's own recorded steps, via serializeUndoTarget.
func buildUndoSystemPrompt(lang string, target UndoTarget) string {
	var b strings.Builder
	b.WriteString(undoSystemPromptEN)
	if lang == "tr" {
		b.WriteString("\n\n")
		b.WriteString(undoLangTR)
	}
	b.WriteString("\n\n")
	b.WriteString(serializeUndoTarget(target))
	return b.String()
}

// serializeUndoTarget renders target as the grounding block appended to
// the undo system prompt: the original free-text request, the CURRENT
// working directory this undo invocation is actually running in
// (target.Cwd), and every recorded step in its ORIGINAL (oldest-first)
// order — the prompt's own "consider steps in reverse order" rule tells
// the model to read this list backward itself, rather than this function
// pre-reversing it, so the model always sees the run's own true
// chronological order once, unambiguously.
//
// Each step also carries its OWN recorded working directory (e.Cwd) —
// distinct from, and possibly different from, target.Cwd above. This
// matters most for exactly the steps internal/cli's own buildUndoPlan
// downgrades to this LLM tier BECAUSE e.Cwd != the current directory (a
// relative-path heuristic match it refuses to trust blindly — see
// undo_plan.go's own doc comment): without the step's recorded cwd here,
// the model would only ever see the CURRENT directory and a relative
// path like "mkdir demo", and would have no way to notice — let alone
// reason about — the very mismatch that caused the downgrade in the
// first place, silently reproducing the same wrong-directory guess the
// downgrade exists to avoid.
func serializeUndoTarget(target UndoTarget) string {
	var b strings.Builder
	b.WriteString("Run to undo:\n")
	fmt.Fprintf(&b, "- Original request: %s\n", orUnknown(target.Request))
	fmt.Fprintf(&b, "- Current working directory (where any undo command will actually run): %s\n", orUnknown(target.Cwd))
	b.WriteString("- Executed steps, in original (oldest-first) order:\n")
	for i, e := range target.Steps {
		fmt.Fprintf(&b, "  %d. command: %s | risk: %s | exit_code: %d | recorded working directory at the time: %s\n",
			i+1, orUnknown(e.Command), orUnknown(e.Risk), e.ExitCode, orUnknown(e.Cwd))
	}
	return b.String()
}
