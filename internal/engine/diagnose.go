package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/config"
	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// ErrorContext bundles everything Diagnose needs to know about the
// failing command it is asked to diagnose: the command text itself, its
// exit code, its captured stderr/stdout, and the system context
// (OS/shell/package managers/...) already collected for this invocation.
// internal/cli's fallback chain (last_command.json → --rerun/`--
// <command>` → interactive paste mode) is what actually builds one of
// these; this package never reads last_command.json or an executor
// itself.
type ErrorContext struct {
	Command string
	// ExitCode is the failing command's exit code, or -1 when it is
	// genuinely unknown — internal/cli's interactive paste-mode fallback
	// never observed a real exit code (the user only pasted text), and
	// reports -1 rather than guessing 1, so the diagnose prompt can say
	// "unknown" instead of asserting a specific, possibly wrong, code.
	ExitCode int
	Stderr   string
	Stdout   string
	System   contextpkg.Context
}

// Diagnosis is Diagnoser.Diagnose's result: the model's determination of
// what actually went wrong (RootCause), a plain-language, user-facing
// Explanation (always in the user's resolved language — see
// buildDiagnoseSystemPrompt — and, per prompts/diagnose_system.txt,
// written so a terminal beginner can follow it), and a risk-labeled Plan
// built and safety-annotated exactly like Planner.GeneratePlan's own
// Plan (same rawPlan/toPlan/safety.Engine machinery — see Diagnose).
type Diagnosis struct {
	RootCause   string
	Explanation string
	Plan        Plan
}

// rawDiagnosis mirrors the exact JSON shape prompts/diagnose_system.txt
// instructs the model to respond with. Its "plan" field reuses rawPlan
// (planner.go) verbatim — the diagnose prompt's plan shape is exactly
// plan generation's {summary, steps:[{command,rationale,risk,
// reversible}]} shape, so toPlan (also planner.go) builds and
// risk-parses it identically, with no separate conversion logic
// duplicated here.
type rawDiagnosis struct {
	RootCause   string  `json:"root_cause"`
	Explanation string  `json:"explanation"`
	Plan        rawPlan `json:"plan"`
}

// Diagnoser turns a failing command's captured context into a Diagnosis:
// a root cause, a user-language explanation, and a risk-labeled fix plan.
// Like Planner, it holds no global state — the Completer and config are
// injected, and safetyEngine is built once at construction from that same
// config (see NewPlanner's identical rationale).
type Diagnoser struct {
	llm          Completer
	cfg          config.Config
	getenv       func(string) string
	safetyEngine *safety.Engine
}

// NewDiagnoser builds a Diagnoser around client (typically an
// *llm.Client from llm.New(cfg), but any Completer works) and cfg.
func NewDiagnoser(client Completer, cfg config.Config) *Diagnoser {
	return &Diagnoser{
		llm:          client,
		cfg:          cfg,
		getenv:       os.Getenv,
		safetyEngine: safety.NewEngine(cfg),
	}
}

// Diagnose sends one diagnose request built from errCtx and decodes/
// validates the model's {root_cause, explanation, plan} response.
// RequiredFields enforces all three are present and non-empty — an empty
// "plan" object (`{}`) counts as missing, the same isEmptyJSONValue rule
// llm.ValidateInto applies everywhere else in this package (see
// internal/llm/parse.go). An empty "plan.steps" array is NOT separately
// rejected here the way GeneratePlan's own empty-steps re-prompt rejects
// one: a diagnosis that legitimately found nothing to fix (e.g. "the
// command actually succeeded", per prompts/diagnose_system.txt's own
// escape hatch) is a valid response shape, not a malformed one, so it
// simply produces a Plan with an empty Steps slice — Diagnose does not
// run GeneratePlan's corrective re-prompt loop at all.
//
// Every step in the resulting Plan is run through the Diagnoser's
// safety.Engine and annotated with its Decision, exactly like
// GeneratePlan — Diagnose itself never executes anything; the plan is
// handed to FAZ 6's engine.Execute by internal/cli's `comrade fix`,
// exactly as `comrade do` hands GeneratePlan's Plan to it.
func (d *Diagnoser) Diagnose(ctx context.Context, errCtx ErrorContext) (Diagnosis, error) {
	lang := resolveLanguage(d.cfg.General.Language, d.getenv)
	systemPrompt := buildDiagnoseSystemPrompt(lang, errCtx)

	resp, err := d.llm.Complete(ctx, llm.CompletionRequest{
		System: systemPrompt,
		Messages: []llm.Message{
			{Role: "user", Content: diagnoseUserMessage},
		},
		MaxTokens:      d.cfg.LLM.MaxTokens,
		RequiredFields: []string{"root_cause", "explanation", "plan"},
	})
	if err != nil {
		return Diagnosis{}, fmt.Errorf("engine: diagnose: %w", err)
	}

	var raw rawDiagnosis
	if err := json.Unmarshal(resp.JSON, &raw); err != nil {
		return Diagnosis{}, fmt.Errorf("engine: decode diagnosis response: %w", err)
	}

	plan, unknownRisk, err := toPlan(raw.Plan)
	if err != nil {
		return Diagnosis{}, fmt.Errorf("engine: diagnose: %w", err)
	}
	if unknownRisk {
		plan.Summary = strings.TrimSpace(plan.Summary + " " + unknownRiskNote(lang))
	}

	for i := range plan.Steps {
		plan.Steps[i].Decision = d.safetyEngine.Evaluate(plan.Steps[i].Command, plan.Steps[i].Risk)
	}

	return Diagnosis{
		RootCause:   raw.RootCause,
		Explanation: raw.Explanation,
		Plan:        plan,
	}, nil
}
