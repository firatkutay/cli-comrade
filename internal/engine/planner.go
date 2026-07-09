package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/firatkutay/cli-comrade/internal/config"
	contextpkg "github.com/firatkutay/cli-comrade/internal/context"
	"github.com/firatkutay/cli-comrade/internal/i18n"
	"github.com/firatkutay/cli-comrade/internal/llm"
	"github.com/firatkutay/cli-comrade/internal/safety"
)

// Completer is the minimal llm capability GeneratePlan needs: a
// package-local, consumer-side interface (idiomatic Go — CLAUDE.md "Kod
// Kuralları") so tests can substitute a mock without depending on
// *llm.Client's full surface (Stream, Name, the fallback chain). Any
// *llm.Client satisfies this, since llm.Client.Complete has this exact
// signature.
type Completer interface {
	Complete(ctx context.Context, req llm.CompletionRequest) (llm.CompletionResponse, error)
}

// Planner turns a free-text request plus a collected system context into
// a risk-labeled Plan. It holds no global state: the Completer and config
// are both injected, and safetyEngine is built once at construction from
// that same config, so every GeneratePlan call reuses the same compiled
// denylist rather than recompiling safety.denylist_extra's regexes (and
// re-printing any invalid-pattern warning) on every request.
type Planner struct {
	llm          Completer
	cfg          config.Config
	getenv       func(string) string
	systemLocale func() string
	safetyEngine *safety.Engine
}

// NewPlanner builds a Planner around client (typically an *llm.Client
// from llm.New(cfg), but any Completer works — see Completer's doc
// comment) and cfg.
func NewPlanner(client Completer, cfg config.Config) *Planner {
	return &Planner{
		llm:          client,
		cfg:          cfg,
		getenv:       os.Getenv,
		systemLocale: i18n.SystemLocale,
		safetyEngine: safety.NewEngine(cfg),
	}
}

// rawStep / rawPlan mirror the exact JSON shape prompts/plan_system.txt
// instructs the model to respond with. They exist only as the
// json.Unmarshal target inside requestRawPlan; toPlan converts a rawPlan
// into the public Plan/Step types, parsing Risk via safety.ParseRiskClass
// and failing closed to safety.RiskDestructive on an empty or
// unrecognized label.
type rawStep struct {
	Command    string `json:"command"`
	Rationale  string `json:"rationale"`
	Risk       string `json:"risk"`
	Reversible bool   `json:"reversible"`
}

type rawPlan struct {
	Summary string    `json:"summary"`
	Steps   []rawStep `json:"steps"`
}

const (
	// emptyStepsCorrection is the single automatic re-prompt sent when
	// the model's first response has an empty "steps" array
	// (UYGULAMA_PLANI.md FAZ 5 item 3: "boş plan ... yeniden istem").
	emptyStepsCorrection = `Your previous response had an empty "steps" array. Re-generate the plan for the same request above with at least one concrete step.`

	// consolidateCorrectionFormat is the single automatic re-prompt sent
	// when the model's response has more steps than safety.max_auto_steps
	// allows (UYGULAMA_PLANI.md FAZ 5 item 3: "max_auto_steps aşımı ...
	// yeniden istem"). %d is safety.max_auto_steps.
	consolidateCorrectionFormat = `Your previous plan had more than %d steps. Consolidate it into at most %d steps for the same request above, merging closely related low-risk (read/write) operations where safe, without dropping or merging away any distinct destructive/elevated operation.`
)

// GeneratePlan builds the system prompt from cfg/sysCtx, requests a plan
// from the model, and — per UYGULAMA_PLANI.md FAZ 5 item 3 — recovers
// from two specific malformed-response shapes with exactly one automatic
// corrective re-prompt each:
//
//   - an empty "steps" array: re-prompt once asking for at least one
//     step; still empty after that is a hard error.
//   - more steps than safety.max_auto_steps: re-prompt once asking the
//     model to consolidate; if the retried response (or, if that retry
//     itself failed, the original response) still exceeds the limit, the
//     plan is hard-truncated to max_auto_steps with a warning marker
//     prepended to Summary (see truncationMarker) rather than erroring —
//     a plan that does *something* is more useful here than none at all,
//     and the truncation is impossible to miss in the rendered output.
//
// Every step is then run through the Planner's safety.Engine and
// annotated with its Decision (EffectiveRisk/Action/Reason/MatchedRule) —
// GeneratePlan itself never executes anything; this phase produces a
// verified plan for `comrade do --dry-run` (FAZ 5) and, later, FAZ 6's
// executor to act on.
func (p *Planner) GeneratePlan(ctx context.Context, request string, sysCtx contextpkg.Context) (Plan, error) {
	lang := i18n.ResolveLanguage(p.cfg.General.Language, p.getenv, p.systemLocale).String()
	systemPrompt := buildSystemPrompt(lang, sysCtx)

	raw, err := p.requestRawPlan(ctx, systemPrompt, []llm.Message{
		{Role: "user", Content: request},
	})
	if err != nil {
		return Plan{}, err
	}

	if len(raw.Steps) == 0 {
		raw, err = p.requestRawPlan(ctx, systemPrompt, []llm.Message{
			{Role: "user", Content: request},
			{Role: "user", Content: emptyStepsCorrection},
		})
		if err != nil {
			return Plan{}, err
		}
		if len(raw.Steps) == 0 {
			return Plan{}, fmt.Errorf("engine: model returned an empty plan even after a corrective re-prompt")
		}
	}

	truncated := false
	if maxSteps := p.cfg.Safety.MaxAutoSteps; maxSteps > 0 && len(raw.Steps) > maxSteps {
		retried, retryErr := p.requestRawPlan(ctx, systemPrompt, []llm.Message{
			{Role: "user", Content: request},
			{Role: "user", Content: fmt.Sprintf(consolidateCorrectionFormat, maxSteps, maxSteps)},
		})
		if retryErr == nil && len(retried.Steps) > 0 {
			raw = retried
		}
		if len(raw.Steps) > maxSteps {
			raw.Steps = raw.Steps[:maxSteps]
			truncated = true
		}
	}

	plan, unknownRisk, err := toPlan(raw)
	if err != nil {
		return Plan{}, err
	}

	var notes []string
	if truncated {
		notes = append(notes, truncationMarker(lang, p.cfg.Safety.MaxAutoSteps))
	}
	if unknownRisk {
		notes = append(notes, unknownRiskNote(lang))
	}
	if len(notes) > 0 {
		plan.Summary = strings.TrimSpace(plan.Summary + " " + strings.Join(notes, " "))
	}

	for i := range plan.Steps {
		plan.Steps[i].Decision = p.safetyEngine.Evaluate(plan.Steps[i].Command, plan.Steps[i].Risk)
	}

	return plan, nil
}

// requestRawPlan sends one completion request (System + messages) with
// RequiredFields=["summary"] — llm.Client itself extracts and validates
// that "summary" is a present, non-empty top-level JSON key via
// llm.ValidateInto before ever returning successfully (see
// internal/llm/client.go's tryComplete) — and decodes the resulting
// resp.JSON into a rawPlan.
//
// "steps" is deliberately NOT in RequiredFields: llm.ValidateInto treats
// an empty JSON array as "missing" (see internal/llm/parse.go's
// isEmptyJSONValue), which would turn a legitimate-but-unhelpful
// `"steps": []` response into a hard llm.ErrParseFailure before
// GeneratePlan ever got a chance to run its own, documented empty-steps
// re-prompt (UYGULAMA_PLANI.md FAZ 5 item 3). Requiring only "summary"
// here lets that response through so GeneratePlan's len(raw.Steps) == 0
// check is what handles it.
func (p *Planner) requestRawPlan(ctx context.Context, systemPrompt string, messages []llm.Message) (rawPlan, error) {
	resp, err := p.llm.Complete(ctx, llm.CompletionRequest{
		System:         systemPrompt,
		Messages:       messages,
		MaxTokens:      p.cfg.LLM.MaxTokens,
		RequiredFields: []string{"summary"},
	})
	if err != nil {
		return rawPlan{}, fmt.Errorf("engine: generate plan: %w", err)
	}

	var raw rawPlan
	if err := json.Unmarshal(resp.JSON, &raw); err != nil {
		return rawPlan{}, fmt.Errorf("engine: decode plan response: %w", err)
	}
	return raw, nil
}

// toPlan converts raw into the public Plan type. It returns an error only
// when a step's "command" is empty — a response shape not covered by
// GeneratePlan's two documented re-prompt cases (empty *steps array*, or
// too many steps), and rare enough in practice (every provider connector
// in internal/llm returns non-empty text, and system-prompt-instructed
// models reliably fill "command") that a hard error here, surfaced
// straight to the caller, is preferable to a third bespoke re-prompt path.
// unknownRisk reports whether any step's "risk" field failed to parse via
// safety.ParseRiskClass (missing, empty, or a label the model invented);
// such a step's Risk is set to safety.RiskDestructive — CLAUDE.md's
// fail-closed mandate applied to a risk label this package cannot trust,
// not just a command's safety.Engine verdict.
func toPlan(raw rawPlan) (plan Plan, unknownRisk bool, err error) {
	plan = Plan{
		Summary: raw.Summary,
		Steps:   make([]Step, 0, len(raw.Steps)),
	}

	for i, rs := range raw.Steps {
		if strings.TrimSpace(rs.Command) == "" {
			return Plan{}, false, fmt.Errorf("engine: step %d in model response has an empty command", i+1)
		}

		risk, parseErr := safety.ParseRiskClass(rs.Risk)
		if parseErr != nil {
			risk = safety.RiskDestructive
			unknownRisk = true
		}

		plan.Steps = append(plan.Steps, Step{
			Command:    rs.Command,
			Rationale:  rs.Rationale,
			Risk:       risk,
			Reversible: rs.Reversible,
		})
	}

	return plan, unknownRisk, nil
}

// truncationMarker is the bracketed warning prepended to Plan.Summary
// when GeneratePlan hard-truncated a plan to safety.max_auto_steps.
func truncationMarker(lang string, maxSteps int) string {
	if lang == "tr" {
		return fmt.Sprintf("[uyarı: plan %d adımı aştığı için kırpıldı]", maxSteps)
	}
	return fmt.Sprintf("[warning: plan exceeded %d steps and was truncated]", maxSteps)
}

// unknownRiskNote is the bracketed warning appended to Plan.Summary when
// at least one step's "risk" label did not parse as a known
// safety.RiskClass and was defaulted to "destructive".
func unknownRiskNote(lang string) string {
	if lang == "tr" {
		return "[uyarı: bir veya daha fazla adımın risk etiketi tanınmadı; güvenlik gereği 'destructive' kabul edildi]"
	}
	return "[warning: one or more steps had an unrecognized risk label; treated as 'destructive' to fail closed]"
}
